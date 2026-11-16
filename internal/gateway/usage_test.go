package gateway

import (
	"testing"

	"theophylax/internal/providers"
)

func TestParseAnyUsageOpenAICacheWrites(t *testing.T) {
	cases := []struct {
		name, body string
		want       providers.Usage
	}{
		{"responses completed event", `{"type":"response.completed","response":{"usage":{"input_tokens":12000,"output_tokens":30,
			"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":12000,"cache_write_tokens_details":{"ttl_30m_tokens":0,"ttl_12h_tokens":12000}}}}}`,
			providers.Usage{Output: 30, CacheWrite: 12000, CacheWrite1h: 12000}},
		{"responses body", `{"usage":{"input_tokens":500,"output_tokens":7,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":300}}}`,
			providers.Usage{Input: 100, Output: 7, CacheRead: 100, CacheWrite: 300}},
		{"chat completions", `{"usage":{"prompt_tokens":11,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":2}}}`,
			providers.Usage{Input: 9, Output: 5, CacheRead: 2}},
	}
	for _, tc := range cases {
		if got := parseAnyUsage([]byte(tc.body)); got != tc.want {
			t.Errorf("%s: usage = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
