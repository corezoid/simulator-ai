package publicapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rpcCall struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// rpcBackend answers every JSON-RPC call with result and records the calls.
func rpcBackend(t *testing.T, result func(method string) string) (*Client, *[]rpcCall) {
	t.Helper()
	var calls []rpcCall
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID string `json:"id"`
			rpcCall
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		calls = append(calls, q.rpcCall)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, result(q.Method))
	}))
	t.Cleanup(api.Close)
	c, err := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func TestAccessRulesDispatchByObjectKind(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return `{"list":[]}` })
	for _, tc := range []struct {
		in     AccessObject
		method string
		params string
	}{
		{AccessObject{ObjType: "actor", ObjID: "a-1"}, "access_rule/actor/list", `{"accId":"ws","actorId":"a-1"}`},
		{AccessObject{ObjType: "formTemplate", ObjID: "12"}, "access_rule/formTemplate/list", `{"accId":"ws","formId":12}`},
		{AccessObject{ObjType: "templateActors", ObjID: "12"}, "access_rule/templateActors/list", `{"accId":"ws","formId":12}`},
		{AccessObject{ObjType: "account", ObjID: "name-1", CurrencyID: 3}, "access_rule/account/list", `{"accId":"ws","currencyId":3,"nameId":"name-1"}`},
	} {
		if _, err := c.AccessRules(context.Background(), tc.in); err != nil {
			t.Fatal(tc.in, err)
		}
		last := (*calls)[len(*calls)-1]
		if params, _ := json.Marshal(last.Params); last.Method != tc.method || string(params) != tc.params {
			t.Errorf("%v called %s %s", tc.in, last.Method, params)
		}
	}
	for _, bad := range []AccessObject{{ObjType: "treeLayer", ObjID: "x"}, {ObjType: "formTemplate", ObjID: "x"}, {ObjType: "account", ObjID: "n"}, {ObjType: "actor", ObjID: "a", CurrencyID: 1}} {
		if _, err := c.AccessRules(context.Background(), bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestSaveAccessRulesMergeAndReplace(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		if strings.HasSuffix(method, "/list") {
			return `{"list":[{"userId":1,"userType":"user","privs":{"view":true}},{"userId":7,"userType":"user","privs":{"view":true}},{"userId":9,"userType":"group","privs":{"view":true,"execute":true}},` +
				`{"userId":3,"userType":"user","privs":{"view":true,"modify":true}},{"userId":0,"userType":"system","privs":{"view":true}},{"userId":5,"userType":"anonymous","privs":{"view":true}}]}`
		}
		return `{"list":[]}`
	})
	view := json.RawMessage(`{"view":true}`)
	in := AccessRulesSaveInput{AccessObject: AccessObject{ObjType: "actor", ObjID: "a-1"}, Rules: []AccessRule{{UserID: 7, Privs: json.RawMessage(`{"execute":true}`)}}, Remove: []int64{8}, IsNotify: true}
	if _, err := c.SaveAccessRules(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	merged, _ := json.Marshal((*calls)[0])
	if len(*calls) != 1 || !strings.Contains(string(merged), `"actions":[{"action":"create","privs":{"execute":true,"view":true},"userId":7},{"action":"delete","userId":8}]`) || !strings.Contains(string(merged), `"isNotify":true`) || strings.Contains(string(merged), "isRecursive") {
		t.Fatalf("merge sent %s", merged)
	}
	*calls = nil
	in.Mode = "replace"
	if _, err := c.SaveAccessRules(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	replaced, _ := json.Marshal((*calls)[1])
	// 3 may modify (the caller may be one of those), 0 is the system owner, 5 a public-link guest: kept.
	if len(*calls) != 2 || (*calls)[0].Method != "access_rule/actor/list" || !strings.Contains(string(replaced), `{"action":"delete","userId":8},{"action":"delete","userId":1},{"action":"delete","userId":9}],"actorId"`) || !strings.Contains(string(replaced), `"filterIncorrect":false`) {
		t.Fatalf("replace sent %v %s", (*calls)[0].Method, replaced)
	}
	for name, bad := range map[string]AccessRulesSaveInput{
		"nothing to merge": {AccessObject: in.AccessObject},
		"a subject twice":  {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7, Privs: view}}, Remove: []int64{7}},
		"no privs":         {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7}}},
		"unknown mode":     {AccessObject: in.AccessObject, Remove: []int64{7}, Mode: "set"},
		"privs not flags":  {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7, Privs: json.RawMessage(`{"view":"yes"}`)}}},
	} {
		if _, err := c.SaveAccessRules(context.Background(), bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestUploadAttachmentSendsTheFileAsMultipart(t *testing.T) {
	var form map[string]string
	var auth, fileType, fileName string
	var attached map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/rpc" {
			var q struct {
				ID     string         `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&q)
			if q.Params["actorId"] == "read-only" {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":-32003,"message":"actor is read-only","data":{"reason":"accessDenied"}}}`, q.ID)
				return
			}
			attached = q.Params
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{}}`, q.ID)
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
	// The route only logs a failed attach, so the tool attaches with public attachment/attach.
	if sent, _ := json.Marshal(attached); string(sent) != `{"accId":"ws","actorId":"r-1","attachmentIds":[42],"rootActorId":"a-1"}` {
		t.Fatal("attach sent", string(sent))
	}
	if auth != "Simulator run" || len(form) != 1 || form["accId"] != "ws" || !strings.HasPrefix(fileType, "text/plain") || fileName != "notes.txt" {
		t.Fatalf("sent auth %q form %v type %q name %q", auth, form, fileType, fileName)
	}
	if _, err = c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: "aGVsbG8=", ContentType: "text/plain\r\n"}); err != nil || fileType != "text/plain" {
		t.Fatalf("a padded contentType is sent as %q: %v", fileType, err)
	}
	if _, err = c.UploadAttachment(context.Background(), AttachmentUploadInput{FileName: "notes.txt", Content: "aGVsbG8=", ActorID: "read-only"}); err == nil || !strings.Contains(err.Error(), "uploaded as attachment 42 but not attached") {
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
	if got, err := read("long"); err != nil || !got.Truncated || len(got.Text) > maxReadText || !strings.HasSuffix(got.Text, "я") || got.Size != int64(2*maxReadText) {
		t.Fatal(len(got.Text), got.Truncated, got.Size, err)
	}
	if got, err := read("picture"); err != nil || !got.Binary || got.Text != "" || got.Size != 8 {
		t.Fatal(got, err)
	}
	for name, want := range map[string]string{"infected": "quarantined", "missing": "no such file", "../etc": "storage key"} {
		if _, err := read(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
