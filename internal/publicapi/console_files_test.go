package publicapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadAttachmentSendsTheFileAsMultipart(t *testing.T) {
	var form map[string]string
	var auth, fileType, fileName string
	rpcCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/rpc" {
			rpcCalls++
			http.Error(w, "no JSON-RPC call is expected", http.StatusBadRequest)
			return
		}
		if r.FormValue("actorId") == "read-only" { // sim-api checks the attach before storing
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"actor is read-only"}`)
			return
		}
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/2.0/rest/upload" || r.ParseMultipartForm(1<<20) != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		form = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			form[k] = v[0]
		}
		f, h, _ := r.FormFile("file")
		b, _ := io.ReadAll(f)
		fileType, fileName = h.Header.Get("Content-Type"), h.Filename
		if string(b) != "hello" {
			http.Error(w, "wrong bytes", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"id":42,"title":"notes.txt","type":"text/plain","size":5,"fileName":"k1","accId":"ws"}`)
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	a, err := c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: base64.StdEncoding.EncodeToString([]byte("hello")), ActorID: "r-1", RootActorID: "a-1"})
	if err != nil || a.ID != 42 || a.FileName != "k1" || a.AttachedTo != "r-1" {
		t.Fatal(a, err)
	}
	// The route attaches it, or refuses before storing the file: no JSON-RPC call follows.
	if rpcCalls != 0 {
		t.Fatal(rpcCalls, "JSON-RPC calls after the upload")
	}
	if auth != "Simulator run" || form["accId"] != "ws" || form["actorId"] != "r-1" || form["rootActorId"] != "a-1" || form["ttl"] != "" || fileType != "text/plain" || fileName != "notes.txt" {
		t.Fatalf("sent auth %q form %v type %q name %q", auth, form, fileType, fileName)
	}
	// A temporary file goes to the route with its ttl and is attached nowhere.
	if _, err = c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: "aGVsbG8=", TTL: 60}); err != nil || form["ttl"] != "60" || form["actorId"] != "" {
		t.Fatalf("a temporary upload sent %v: %v", form, err)
	}
	if _, err = c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: "aGVsbG8=", ContentType: "text/plain; charset=utf-8\r\n"}); err != nil || fileType != "text/plain" {
		t.Fatalf("a padded contentType is sent as %q: %v", fileType, err)
	}
	if _, err = c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: "aGVsbG8=", ActorID: "read-only"}); err == nil || !strings.Contains(err.Error(), "http 400") || !strings.Contains(err.Error(), "read-only") {
		t.Fatal("a refused attach must be reported:", err)
	}
	for name, bad := range map[string]AttachmentUploadInput{
		"not base64":          {FileName: "a.txt", Content: "%%%"},
		"too large":           {FileName: "a.bin", Content: base64.StdEncoding.EncodeToString(make([]byte, MaxUploadBytes+1))},
		"a path":              {FileName: "../a.txt", Content: "aGk="},
		"root without actor":  {FileName: "a.txt", Content: "aGk=", RootActorID: "r"},
		"temporary attached":  {FileName: "a.txt", Content: "aGk=", ActorID: "a", TTL: 60},
		"not a media type":    {FileName: "a.txt", Content: "aGk=", ContentType: "nope/"},
		"a quote in the name": {FileName: `a".txt`, Content: "aGk="},
	} {
		if _, err := c.UploadAttachment(context.Background(), bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestUploadAttachmentReportsTheRefusal(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "this route is not available to an AI agent run")
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	_, err := c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "a.txt", Content: "aGk="})
	if err == nil || !strings.Contains(err.Error(), "http 401") || !strings.Contains(err.Error(), "not available to an AI agent run") {
		t.Fatal(err)
	}
}

func TestReadAttachment(t *testing.T) {
	scanWait = time.Millisecond
	defer func() { scanWait = time.Second }()
	scans := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "the download needs no credential", http.StatusBadRequest)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/1.0/download/") {
		case "scanning":
			if scans++; scans < 3 {
				http.Error(w, "file scan in progress", http.StatusTooEarly)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"a":1}`)
		case "long":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, strings.Repeat("я", maxReadText))
		case "picture":
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, "\x89PNG....")
		case "late": // a stand serving files from ufs: not found until the scan ends
			if scans++; scans < 6 {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, "ok")
		case "infected":
			http.Error(w, "file quarantined", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	read := func(name string) (AttachmentContent, error) {
		return c.ReadAttachment(context.Background(), AttachmentReadInput{FileName: name})
	}
	if got, err := read("scanning"); err != nil || got.Text != `{"a":1}` || got.Size != 7 || scans != 3 {
		t.Fatal(got, err, scans)
	}
	scans = 3 // two 404s, then the file: within the two 404 retries
	if got, err := read("late"); err != nil || got.Text != "ok" || scans != 6 {
		t.Fatal("a file found after the scan:", got, err, scans)
	}
	if got, err := read("long"); err != nil || !got.Truncated || len(got.Text) > maxReadText || !strings.HasSuffix(got.Text, "я") || got.Size != int64(2*maxReadText) {
		t.Fatal(len(got.Text), got.Truncated, got.Size, err)
	}
	if got, err := read("picture"); err != nil || !got.Binary || got.Text != "" || got.Size != 8 {
		t.Fatal(got, err)
	}
	for name, want := range map[string]string{"infected": "quarantined", "missing": "no such file, or its antivirus scan has not finished", "../etc": "storage key"} {
		if _, err := read(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
