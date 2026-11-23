package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"theophylax/internal/receipts"
)

func TestSplitReceiptTrailer(t *testing.T) {
	events := "event: message_start\ndata: {}\n\nevent: message_stop\ndata: {}\n\n"
	for _, tc := range []struct {
		name, in, body, jws string
		ok                  bool
	}{
		{"trailer", events + ": theophylax-receipt a.b.c\n\n", events, "a.b.c", true},
		{"crlf and no blank line", events + ": theophylax-receipt a.b.c\r\n", events, "a.b.c", true},
		{"only line", ": theophylax-receipt a.b.c\n", "", "a.b.c", true},
		{"no trailer", events, events, "", false},
		{"json body", `{"type":"message"}`, `{"type":"message"}`, "", false},
		{"trailer not last", ": theophylax-receipt a.b.c\n\n" + events, ": theophylax-receipt a.b.c\n\n" + events, "", false},
		{"empty jws", events + ": theophylax-receipt \n\n", events + ": theophylax-receipt \n\n", "", false},
	} {
		body, jws, ok := splitReceiptTrailer([]byte(tc.in))
		if string(body) != tc.body || jws != tc.jws || ok != tc.ok {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q, %v)", tc.name, body, jws, ok, tc.body, tc.jws, tc.ok)
		}
	}
}

// TestVerifyReceiptCapturedStream runs the CLI on a stream exactly as a client
// received it (receipt comment included), with and without the JWS argument,
// and on a non-streamed body.
func TestVerifyReceiptCapturedStream(t *testing.T) {
	s, err := receipts.NewSigner(nil, bytes.Repeat([]byte{0x2a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	jwks, _ := json.Marshal(map[string]any{"keys": []any{s.JWK()}})
	keys := write("keys.json", jwks)
	req := []byte(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	reqF := write("req.json", req)
	sign := func(resp []byte) string {
		jws, _ := s.Sign(&receipts.Receipt{Version: 1, ID: "req_1", ReqSHA256: receipts.SHA256Hex(req), RespSHA256: receipts.SHA256Hex(resp), Status: 200})
		return jws
	}

	events := []byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	jws := sign(events)
	captured := write("stream.raw", append(append([]byte{}, events...), ": theophylax-receipt "+jws+"\n\n"...))
	if code := verifyReceiptCLI([]string{"--keys", keys, "--request", reqF, "--response", captured}); code != 0 {
		t.Fatalf("captured stream, JWS from trailer: exit %d", code)
	}
	if code := verifyReceiptCLI([]string{"--keys", keys, "--request", reqF, "--response", captured, jws}); code != 0 {
		t.Fatalf("captured stream, JWS given: exit %d", code)
	}
	tampered := write("tampered.raw", append(bytes.Replace(events, []byte("message_stop"), []byte("message_stoq"), 1), ": theophylax-receipt "+jws+"\n\n"...))
	if code := verifyReceiptCLI([]string{"--keys", keys, "--response", tampered}); code != 1 {
		t.Fatalf("tampered stream: exit %d, want 1", code)
	}

	unary := []byte(`{"type":"message","content":[{"type":"text","text":"hello"}]}`)
	unaryF := write("resp.json", unary)
	if code := verifyReceiptCLI([]string{"--keys", keys, "--response", unaryF, sign(unary)}); code != 0 {
		t.Fatalf("non-streamed body: exit %d", code)
	}
	if code := verifyReceiptCLI([]string{"--keys", keys, "--response", unaryF}); code != 2 {
		t.Fatalf("no JWS and no trailer: exit %d, want 2 (usage)", code)
	}
}
