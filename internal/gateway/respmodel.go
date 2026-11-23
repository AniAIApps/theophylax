package gateway

import (
	"bytes"
	"encoding/json"
)

// Clients pin behaviour on a response's "model", so the gateway returns the
// model name the client asked for on every route, not the upstream ID (a
// Bedrock inference profile, an Azure deployment, a router's tier). Translated
// responses already carry the requested name; these helpers rewrite
// same-format passthrough responses. The upstream ID stays in
// X-Theophylax-Model, logs and the receipt's route.upstream_model.

// withModel returns obj (a JSON object) with its top-level "model" string set
// to model. Everything else is left byte for byte as the upstream sent it.
// obj is returned unchanged if it is not an object or has no string "model".
func withModel(obj []byte, model string) []byte {
	start, end, ok := topLevelValue(obj, "model")
	if !ok || obj[start] != '"' {
		return obj
	}
	enc, err := json.Marshal(model)
	if err != nil || bytes.Equal(obj[start:end], enc) {
		return obj
	}
	out := make([]byte, 0, len(obj)-(end-start)+len(enc))
	out = append(out, obj[:start]...)
	out = append(out, enc...)
	return append(out, obj[end:]...)
}

// withMessageModel does the same for message.model, as in an Anthropic
// streaming message_start event.
func withMessageModel(ev []byte, model string) []byte {
	start, end, ok := topLevelValue(ev, "message")
	if !ok || ev[start] != '{' {
		return ev
	}
	msg := withModel(ev[start:end], model)
	if len(msg) == end-start && bytes.Equal(msg, ev[start:end]) {
		return ev
	}
	out := make([]byte, 0, len(ev)-(end-start)+len(msg))
	out = append(out, ev[:start]...)
	out = append(out, msg...)
	return append(out, ev[end:]...)
}

// topLevelValue finds the byte span of key's value in a JSON object. It scans
// without allocating, because OpenAI passthrough streams call it per chunk.
// It assumes obj is valid JSON (upstream responses are); on anything it does
// not understand it reports ok=false and the caller leaves obj unchanged.
func topLevelValue(obj []byte, key string) (start, end int, ok bool) {
	i := skipSpace(obj, 0)
	if i >= len(obj) || obj[i] != '{' {
		return 0, 0, false
	}
	i++
	for {
		i = skipSpace(obj, i)
		if i >= len(obj) || obj[i] != '"' {
			return 0, 0, false
		}
		kEnd := skipString(obj, i)
		if kEnd < 0 {
			return 0, 0, false
		}
		// Keys are compared raw: "model" and "message" never need escaping.
		match := kEnd-i == len(key)+2 && string(obj[i+1:kEnd-1]) == key
		i = skipSpace(obj, kEnd)
		if i >= len(obj) || obj[i] != ':' {
			return 0, 0, false
		}
		vStart := skipSpace(obj, i+1)
		vEnd := skipValue(obj, vStart)
		if vEnd < 0 {
			return 0, 0, false
		}
		if match {
			return vStart, vEnd, true
		}
		i = skipSpace(obj, vEnd)
		if i >= len(obj) || obj[i] != ',' {
			return 0, 0, false // '}' (key not present) or malformed
		}
		i++
	}
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// skipString returns the index just past the string starting at b[i] == '"'.
func skipString(b []byte, i int) int {
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return -1
}

// skipValue returns the index just past the JSON value starting at b[i].
func skipValue(b []byte, i int) int {
	if i >= len(b) {
		return -1
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{', '[':
		depth := 0
		for j := i; j < len(b); j++ {
			switch b[j] {
			case '"':
				if j = skipString(b, j); j < 0 {
					return -1
				}
				j--
			case '{', '[':
				depth++
			case '}', ']':
				if depth--; depth == 0 {
					return j + 1
				}
			}
		}
		return -1
	default: // number, true, false, null
		j := i
		for j < len(b) && b[j] != ',' && b[j] != '}' && b[j] != ']' && b[j] != ' ' && b[j] != '\t' && b[j] != '\n' && b[j] != '\r' {
			j++
		}
		if j == i {
			return -1
		}
		return j
	}
}
