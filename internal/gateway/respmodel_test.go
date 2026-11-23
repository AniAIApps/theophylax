package gateway

import (
	"encoding/json"
	"testing"
)

func TestWithModel(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"anthropic message", `{"id":"msg_1","type":"message","model":"us.anthropic.claude-x","content":[{"type":"text","text":"\"model\":\"no\""}]}`,
			`{"id":"msg_1","type":"message","model":"demo","content":[{"type":"text","text":"\"model\":\"no\""}]}`},
		{"openai chunk", `{"id":"c","object":"chat.completion.chunk","created":1,"model":"gpt-x-2026","choices":[]}`,
			`{"id":"c","object":"chat.completion.chunk","created":1,"model":"demo","choices":[]}`},
		{"spaced", "{ \"model\" :\n \"up\" , \"n\": 1 }", "{ \"model\" :\n \"demo\" , \"n\": 1 }"},
		{"nested model only", `{"message":{"model":"up"},"x":{"model":"up"}}`, `{"message":{"model":"up"},"x":{"model":"up"}}`},
		{"model after nested values", `{"a":{"model":"up","b":[1,{"model":"z"}]},"model":"up"}`, `{"a":{"model":"up","b":[1,{"model":"z"}]},"model":"demo"}`},
		{"no model", `{"type":"error","error":{"type":"x"}}`, `{"type":"error","error":{"type":"x"}}`},
		{"not a string", `{"model":null}`, `{"model":null}`},
		{"already requested", `{"model":"demo"}`, `{"model":"demo"}`},
		{"not json", `[DONE]`, `[DONE]`},
		{"array", `[{"model":"up"}]`, `[{"model":"up"}]`},
		{"escaped quotes before", `{"t":"a\\\"b","model":"up"}`, `{"t":"a\\\"b","model":"demo"}`},
		{"value named model", `{"kind":"model","model":"up"}`, `{"kind":"model","model":"demo"}`},
		{"truncated value", `{"model":"up`, `{"model":"up`},
		{"truncated key", `{"mod`, `{"mod`},
		{"no value", `{"model":}`, `{"model":}`},
		{"empty", ``, ``},
	} {
		got := string(withModel([]byte(tc.in), "demo"))
		if got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
	// The replacement is JSON-encoded.
	if got := string(withModel([]byte(`{"model":"up"}`), `a"b`)); !json.Valid([]byte(got)) || got != `{"model":"a\"b"}` {
		t.Errorf("escaping: %s", got)
	}
}

func TestWithMessageModel(t *testing.T) {
	in := `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"anthropic.claude-x-v1:0","content":[],"usage":{"input_tokens":3,"output_tokens":1}}}`
	want := `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"demo","content":[],"usage":{"input_tokens":3,"output_tokens":1}}}`
	if got := string(withMessageModel([]byte(in), "demo")); got != want {
		t.Errorf("got %s", got)
	}
	for _, in := range []string{`{"type":"ping"}`, `{"type":"message_start","message":"x"}`, `not json`} {
		if got := string(withMessageModel([]byte(in), "demo")); got != in {
			t.Errorf("%s changed to %s", in, got)
		}
	}
}

// BenchmarkWithModelChunk measures the per-chunk cost on OpenAI passthrough streams.
func BenchmarkWithModelChunk(b *testing.B) {
	chunk := []byte(`{"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1795424570,"model":"gpt-6.2-sol-2026-11-17","system_fingerprint":"fp_1","choices":[{"index":0,"delta":{"content":" some streamed text"},"finish_reason":null}]}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = withModel(chunk, "demo")
	}
}

// FuzzWithModel checks that rewriting never panics and keeps valid JSON valid.
func FuzzWithModel(f *testing.F) {
	for _, s := range []string{`{"model":"up"}`, `{"a":[1,{"b":"}"}],"model":"x","c":null}`, `{"message":{"model":"m"}}`, `{"model":`, `[`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		out := withModel(b, "demo")
		ev := withMessageModel(b, "demo")
		if json.Valid(b) && (!json.Valid(out) || !json.Valid(ev)) {
			t.Fatalf("valid %q became invalid: %q / %q", b, out, ev)
		}
	})
}
