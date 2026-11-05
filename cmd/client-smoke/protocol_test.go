package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type writeBuffer struct{ bytes.Buffer }

func (*writeBuffer) Close() error { return nil }

func TestProtocolRejectsInvalidRepliesAndInteractiveRequests(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		valid      bool
	}{
		{"reply", `{"id":1,"result":{"ok":true}}`, true},
		{"notification", "{\"method\":\"ready\"}\n{\"id\":1,\"result\":{\"ok\":true}}", true},
		{"wrong-id", `{"id":9,"result":{}}`, false},
		{"error", `{"id":1,"error":{"message":"secret"}}`, false},
		{"approval", `{"id":2,"method":"approval/request"}`, false},
		{"invalid", `not json`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &peer{in: &writeBuffer{}, scanner: bufio.NewScanner(strings.NewReader(tc.wire))}
			var result struct {
				OK bool `json:"ok"`
			}
			err := p.call("example", nil, &result)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && !result.OK {
				t.Fatal("result lost")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("raw error leaked")
			}
		})
	}
}
func TestFixtureAcceptsOnlyProjectedRead(t *testing.T) {
	f := startFixture()
	defer f.server.Close()
	for _, method := range []string{"actor/get", "actor/update"} {
		body := `{"id":"1","method":"` + method + `","params":{"accId":"synthetic-workspace","actorId":"synthetic-actor","fields":["id","title","description","formId","status","ref"]}}`
		req, err := http.NewRequest("POST", f.server.URL+"/api/2.0/rpc", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "ApiKey synthetic-client-key")
		res, err := f.server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if (res.StatusCode == 200) != (method == "actor/get") {
			t.Fatal("unexpected fixture result")
		}
	}
	if f.reads.Load() != 1 || f.unexpected.Load() != 1 {
		t.Fatal("fixture did not count unexpected operation")
	}
}
func TestClaudeConnectionMustBelongToInstalledPlugin(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"plugin:simulator-ai:simulator-ai: simulator-ai - ✓ Connected", true},
		{"plugin:simulator-ai:simulator-ai: simulator-ai - ✔ Connected", true},
		{"plugin:simulator-ai:simulator-ai: failed\nother: ✓ Connected", false},
		{"simulator-ai configured", false},
	} {
		if claudeConnected(tc.text) != tc.want {
			t.Fatalf("wrong result for %s", tc.text)
		}
	}
}
func TestBoundedOutput(t *testing.T) {
	var b boundedBuffer
	if _, err := b.Write(make([]byte, 4<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("output bound ignored")
	}
}
func TestChild(t *testing.T) {
	if os.Getenv("SIMULATOR_SMOKE_TEST_CHILD") != "1" {
		return
	}
	if mode := os.Getenv("SIMULATOR_SMOKE_TEST_MODE"); mode != "" {
		path := os.Getenv("SIMULATOR_SMOKE_TEST_HEARTBEAT")
		if mode == "grandchild" {
			for {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					os.Exit(2)
				}
				_, _ = f.Write([]byte("x"))
				_ = f.Close()
				time.Sleep(10 * time.Millisecond)
			}
		}
		child := exec.Command(os.Args[0], "-test.run=^TestChild$")
		child.Env = []string{"SIMULATOR_SMOKE_TEST_CHILD=1", "SIMULATOR_SMOKE_TEST_MODE=grandchild", "SIMULATOR_SMOKE_TEST_HEARTBEAT=" + path}
		child.Stdout, child.Stderr = io.Discard, io.Discard
		if child.Start() != nil {
			os.Exit(2)
		}
		deadline := time.Now().Add(time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = child.Process.Kill()
				os.Exit(2)
			}
			time.Sleep(time.Millisecond)
		}
		_ = os.WriteFile(path+".pid", []byte(strconv.Itoa(child.Process.Pid)), 0600)
		if mode == "failure" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv("SIMULATOR_SMOKE_TEST_SLEEP") == "1" {
		time.Sleep(time.Minute)
	}
	_ = json.NewEncoder(os.Stdout).Encode("synthetic-child")
	os.Exit(0)
}
func TestProcessCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := run(ctx, []string{"SIMULATOR_SMOKE_TEST_CHILD=1", "SIMULATOR_SMOKE_TEST_SLEEP=1"}, t.TempDir(), os.Args[0], "-test.run=^TestChild$")
	if err == nil {
		t.Fatal("deadline did not stop child")
	}
}

func TestProcessCleansGrandchildrenOnEveryExit(t *testing.T) {
	for _, mode := range []string{"success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "heartbeat")
			_, err := run(ctx, []string{"SIMULATOR_SMOKE_TEST_CHILD=1", "SIMULATOR_SMOKE_TEST_MODE=" + mode, "SIMULATOR_SMOKE_TEST_HEARTBEAT=" + path}, t.TempDir(), os.Args[0], "-test.run=^TestChild$")
			data, readErr := os.ReadFile(path + ".pid")
			if readErr != nil {
				t.Fatal(readErr)
			}
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil || pid < 1 {
				t.Fatal("invalid fixture pid")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if (err != nil) != (mode == "failure") {
				t.Fatalf("unexpected parent exit: %v", err)
			}
			time.Sleep(30 * time.Millisecond)
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(100 * time.Millisecond)
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if before.Size() != after.Size() {
				t.Fatal("grandchild survived completed CLI command")
			}
		})
	}
}

func TestFixtureRejectsTokenProbesAndModelExecution(t *testing.T) {
	f := startFixture()
	defer f.server.Close()
	for _, path := range []string{"/disabled-model/v1/messages/count_tokens", "/disabled-model/v1/messages"} {
		res, err := f.server.Client().Post(f.server.URL+path, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatal("model endpoint was accepted")
		}
	}
	if f.tokenCountProbes.Load() != 1 || f.unexpected.Load() != 1 || f.reads.Load() != 0 {
		t.Fatal("metadata probe hid a model call")
	}
}
