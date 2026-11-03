// Command console-smoke drives the console profile of a built simulator-ai binary over stdio
// against a live sim-api: `console-smoke -bin bin/simulator-ai graph form`. The binary reads its
// origin, workspace and credential from this process's environment (make smoke loads them from
// the smoke.env file); nothing secret is printed. Every scenario writes new objects.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type client struct {
	in  *json.Encoder
	out *bufio.Scanner
	seq int
}

func (c *client) rpc(method string, params any) (json.RawMessage, error) {
	c.seq++
	if err := c.in.Encode(map[string]any{"jsonrpc": "2.0", "id": c.seq, "method": method, "params": params}); err != nil {
		return nil, err
	}
	if !c.out.Scan() {
		return nil, errors.New("simulator-ai closed stdout")
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(c.out.Bytes(), &reply); err != nil {
		return nil, err
	}
	if reply.Error != nil {
		return nil, errors.New(reply.Error.Message)
	}
	return reply.Result, nil
}

// tool calls one tool, prints a shortened result and decodes the structured content into out.
func (c *client) tool(name string, args map[string]any, out any) error {
	raw, err := c.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	var r struct {
		IsError    bool                    `json:"isError"`
		Content    []struct{ Text string } `json:"content"`
		Structured json.RawMessage         `json:"structuredContent"`
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.IsError {
		text := ""
		if len(r.Content) > 0 {
			text = r.Content[0].Text
		}
		return fmt.Errorf("%s failed: %s", name, text)
	}
	shown := string(r.Structured)
	if len(shown) > 400 {
		shown = shown[:400] + "…"
	}
	fmt.Printf("%s: %s\n", name, shown)
	return json.Unmarshal(r.Structured, out)
}

func main() {
	bin := flag.String("bin", "bin/simulator-ai", "simulator-ai binary")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: console-smoke -bin <binary> graph|form ...")
		os.Exit(2)
	}
	// Bounded like client-smoke: the whole run, all of its scenarios, gets five minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, *bin)
	cmd.Env = append(os.Environ(), "SIMULATOR_AI_PROFILE=console")
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		fail(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	out := bufio.NewScanner(stdout)
	out.Buffer(make([]byte, 1<<20), 8<<20)
	c := &client{in: json.NewEncoder(stdin), out: out}
	if _, err := c.rpc("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "console-smoke", "version": "1"}}); err != nil {
		fail(err)
	}
	stamp := time.Now().Format("20060102_150405")
	for _, scenario := range flag.Args() {
		var err error
		switch scenario {
		case "graph":
			err = graph(c, stamp)
		case "form":
			err = form(c, stamp)
		default:
			err = fmt.Errorf("unknown scenario %q", scenario)
		}
		if err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "FAIL:", err)
	os.Exit(1)
}
