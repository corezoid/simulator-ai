package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// callTool sends one tools/call and returns the reply with its JSON string escapes undone.
func callTool(send func(string) string, tool, args string) string {
	return strings.ReplaceAll(send(fmt.Sprintf(`"method":"tools/call","params":{"name":%q,"arguments":%s}`, tool, args)), `\"`, `"`)
}

func methodsOf(calls []consoleCall) string {
	var m []string
	for _, c := range calls {
		m = append(m, c.Method)
	}
	return strings.Join(m, ",")
}

func TestRegistryTools(t *testing.T) {
	skills := `"result":{"list":[{"id":"s1","title":"Onboard a client","ref":"onboard","status":"verified"},{"id":"s2","title":"Draft","ref":"draft","status":"pending"}]}`
	skill := `"result":{"id":"s1","title":"Onboard a client","ref":"onboard","status":"verified","formId":7,"description":"[md]1. actor_create[/md]"}`
	agent := `"result":{"id":"t1","title":"Olena","status":"verified","formId":3,"description":"# Agent\nKnows finance"}`
	for _, tc := range []struct {
		name, tool, args, methods string
		overrides                 map[string]string
		wire, out                 []string
	}{
		{"skills are verified only", "skills_find", `{"query":" contract ","limit":2}`, "form/system/get,actor/filter/list", map[string]string{"actor/filter/list": skills},
			[]string{`"fields":["id","title","ref","status"]`, `"formId":7`, `"limit":2`, `"search":"contract"`}, []string{`"list":[{"id":"s1","title":"Onboard a client","ref":"onboard","status":"verified"}],"hasMore":true`}},
		{"skill by ref", "skill_get", `{"ref":"onboard"}`, "form/system/get,actor/by-ref/get", map[string]string{"actor/by-ref/get": skill},
			[]string{`"formId":7`, `"ref":"onboard"`, `"description"`}, []string{`"description":"[md]1. actor_create[/md]"`}},
		{"skill by id", "skill_get", `{"id":"s1"}`, "actor/get", map[string]string{"actor/get": skill}, []string{`"actorId":"s1"`}, []string{`"ref":"onboard"`}},
		{"members without query", "agents_find", `{}`, "user/list", map[string]string{"user/list": `"result":{"list":[{"id":42,"nick":"Olena","type":"user","logins":[{"login":"olena@example.com"}]}]}`},
			[]string{`{"accId":"ws","limit":20}`}, []string{`"members":[{"id":42,"nick":"Olena","type":"user"}],"hasMore":false`}},
		{"a registry template without query", "agents_find", `{"formId":9,"offset":20}`, "actor/filter/list", map[string]string{"actor/filter/list": `"result":{"list":[{"id":"b1","title":"Billing bot"}]}`},
			[]string{`"formId":9`, `"offset":20`, `"with":["avatar"]`}, []string{`"agents":[{"id":"b1","title":"Billing bot"}]`}},
		{"profiles by competency", "agents_find", `{"query":"counters"}`, "form/system/get,actor/search/list", map[string]string{"actor/search/list": `"result":{"list":[{"id":"t1","title":"Olena","systemObjType":"user","systemObjId":"42"}]}`},
			[]string{`"formId":7`, `"query":"counters"`, `"searchType":"semantic"`}, []string{`"systemObjId":"42"`}},
		{"a twin by userId", "agent_get", `{"userId":42}`, "actor/by-system_obj/get", map[string]string{"actor/by-system_obj/get": agent},
			[]string{`"objId":"42","objType":"user"`}, []string{`"description":"# Agent\nKnows finance"`, `"systemObjType":"user","systemObjId":"42"`}},
		{"an agent actor", "agent_get", `{"actorId":"b1"}`, "actor/get", map[string]string{"actor/get": agent}, []string{`"actorId":"b1"`, `"with":["avatar"]`}, []string{`"title":"Olena"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			send, calls := consoleServer(t, tc.overrides)
			reply := callTool(send, tc.tool, tc.args)
			if strings.Contains(reply, `"isError":true`) || methodsOf(*calls) != tc.methods {
				t.Fatalf("%s called %s: %s", tc.tool, methodsOf(*calls), reply)
			}
			want := "System"
			if strings.HasPrefix(tc.tool, "skill") {
				want = "Skills"
			}
			if first := (*calls)[0]; first.Method == "form/system/get" && first.Params["title"] != want {
				t.Fatalf("looked up the %v template", first.Params["title"])
			}
			last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
			for _, w := range tc.wire {
				if !strings.Contains(string(last), w) {
					t.Errorf("request %s lacks %s", last, w)
				}
			}
			for _, w := range tc.out {
				if !strings.Contains(reply, w) {
					t.Errorf("reply %s lacks %s", reply, w)
				}
			}
			if strings.Contains(reply, "olena@example.com") {
				t.Error("a member's login reached the model")
			}
		})
	}
}

func TestRegistryRefusals(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{
		"actor/by-ref/get":  `"error":{"code":1,"message":"actor not found","data":{"reason":"notFound"}}`,
		"actor/search/list": `"error":{"code":1,"message":"vector search is not configured","data":{"reason":"notAvailable"}}`,
	})
	for _, args := range []string{`{"name":"skill_get","arguments":{}}`, `{"name":"skill_get","arguments":{"ref":"a","id":"b"}}`, `{"name":"agent_get","arguments":{"userId":1,"actorId":"b"}}`,
		`{"name":"agents_find","arguments":{"query":"x"}}`, `{"name":"build_link","arguments":{"entity":"smartForm","ref":"leave"}}`, `{"name":"build_link","arguments":{"entity":"actor"}}`} {
		if reply := send(`"method":"tools/call","params":` + args); !strings.Contains(reply, `"isError":true`) {
			t.Error("accepted", args, reply)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid arguments reached the API:", methodsOf(*calls))
	}
	if reply := callTool(send, "skill_get", `{"ref":"gone"}`); !strings.Contains(reply, `no skill with ref "gone"`) || !strings.Contains(reply, "skills_find") {
		t.Error("unknown ref:", reply)
	}
	// Without an explicit searchType a failed semantic search is retried as text; an explicit one is not.
	*calls = nil
	callTool(send, "agents_find", `{"query":"counters"}`)
	if methodsOf(*calls) != "form/system/get,actor/search/list,actor/search/list" || (*calls)[2].Params["searchType"] != "text" {
		t.Fatal("no text fallback:", *calls)
	}
	*calls = nil
	callTool(send, "agents_find", `{"query":"counters","searchType":"semantic","formId":9}`)
	if methodsOf(*calls) != "actor/search/list" {
		t.Fatal("an explicit searchType fell back:", *calls)
	}
}

func TestLocalTools(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{"route/build": `"result":{"href":"/sf/leave/confirm/7?step=2"}`})
	reply := callTool(send, "build_link", `{"entity":"layer","id":"g1","focusId":"l1"}`)
	if !strings.Contains(reply, `/actors_graph/ws/graph/g1/layers/l1"`) || len(*calls) != 0 {
		t.Fatal("layer link:", reply)
	}
	reply = callTool(send, "build_link", `{"entity":"smartForm","ref":"leave_form","route":"order.confirm","params":{"order":7},"query":{"step":"2"}}`)
	sent, _ := json.Marshal((*calls)[0].Params)
	if !strings.Contains(reply, `/sf/leave/confirm/7?step=2"`) || (*calls)[0].Method != "route/build" ||
		string(sent) != `{"name":"order.confirm","params":{"order":7},"query":{"step":"2"},"sourceTarget":{"environment":"production","kind":"direct","scriptRef":"leave_form","workspaceId":"ws"}}` {
		t.Fatal("smart form link:", reply, string(sent))
	}
	reply = callTool(send, "bbcode_tags_get", `{}`)
	if !strings.Contains(reply, `"tag":"actor"`) || !strings.Contains(reply, `"tag":"md"`) || len(*calls) != 1 {
		t.Fatal("bbcode catalog:", reply[:200])
	}
}
