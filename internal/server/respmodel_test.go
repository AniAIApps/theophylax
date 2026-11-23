package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResponseModelIsRequestedName checks that every route reports the model
// name the client asked for in the response's "model" field, unary and
// streaming, whether the gateway passes the upstream response through
// (Anthropic → Anthropic, OpenAI → OpenAI) or translates it (issue #29).
func TestResponseModelIsRequestedName(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []string{"hello ", "there"} {
				fmt.Fprintf(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"model\":\"up-oai-2026\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", part)
			}
			fmt.Fprint(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"model\":\"up-oai-2026\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c","object":"chat.completion","model":"up-oai-2026","choices":[{"index":0,"message":{"role":"assistant","content":"hello there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)
	}))
	defer up.Close()

	ts, s := setup(t)
	root := newClient(t, ts.URL)
	root.must("POST", "/api/auth/login", map[string]string{"email": "root@example.com", "password": "root-password-123"}, 200)
	org := root.list("/api/orgs")[0]["id"].(string)
	team := root.must("POST", "/api/orgs/"+org+"/teams", map[string]any{"name": "T"}, 201)["id"].(string)
	proj := root.must("POST", "/api/teams/"+team+"/projects", map[string]any{"name": "P"}, 201)["id"].(string)
	anthProv := root.must("POST", "/api/providers", map[string]any{"name": "anth", "connection_string": "mock://a"}, 201)["id"].(string)
	oaiProv := root.must("POST", "/api/providers", map[string]any{"name": "oai", "connection_string": "openai://up?api_key=x&base_url=" + up.URL + "/v1"}, 201)["id"].(string)
	am := root.must("POST", "/api/models", map[string]any{"name": "anth-model"}, 201)["id"].(string)
	root.must("POST", "/api/models/"+am+"/deployments", map[string]any{"provider_id": anthProv, "upstream_model": "mock-echo"}, 201)
	om := root.must("POST", "/api/models", map[string]any{"name": "oai-model"}, 201)["id"].(string)
	root.must("POST", "/api/models/"+om+"/deployments", map[string]any{"provider_id": oaiProv, "upstream_model": "up-oai-2026"}, 201)
	key := root.must("POST", "/api/keys", map[string]any{"name": "k", "project_id": proj}, 201)["secret"].(string)
	waitReload(s)

	for _, tc := range []struct{ path, model, upstream string }{
		{"/v1/messages", "anth-model", "mock-echo"},          // passthrough
		{"/v1/chat/completions", "oai-model", "up-oai-2026"}, // passthrough
		{"/v1/messages", "oai-model", "up-oai-2026"},         // translated
		{"/v1/chat/completions", "anth-model", "mock-echo"},  // translated
	} {
		for _, stream := range []bool{false, true} {
			body := msg(tc.model, "hi")
			body["stream"] = stream
			code, m, raw, _ := gw(t, ts.URL, key, tc.path, body)
			name := fmt.Sprintf("%s %s stream=%v", tc.path, tc.model, stream)
			if code != 200 {
				t.Fatalf("%s: %d %s", name, code, raw)
			}
			// The mock echoes its upstream ID in the reply text, so look for it as a model value.
			if strings.Contains(string(raw), `"model":"`+tc.upstream+`"`) {
				t.Errorf("%s: response exposes the upstream ID %q:\n%s", name, tc.upstream, raw)
			}
			if !stream && m["model"] != tc.model {
				t.Errorf("%s: model %v, want %q", name, m["model"], tc.model)
			}
			if stream && !strings.Contains(string(raw), `"model":"`+tc.model+`"`) {
				t.Errorf("%s: stream never names %q:\n%s", name, tc.model, raw)
			}
		}
	}
}
