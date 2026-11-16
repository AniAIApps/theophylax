package providers

import (
	"encoding/json"
	"testing"
)

func usageMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// OpenAI counts cache reads and cache writes inside prompt_tokens /
// input_tokens; they must be moved out of Input so each is billed once, at its
// own rate.
func TestOpenAIUsageCacheWrites(t *testing.T) {
	u := OpenAIUsage(usageMap(t, `{"prompt_tokens": 10000, "completion_tokens": 50,
		"prompt_tokens_details": {"cached_tokens": 2000, "cache_write_tokens": 7000,
			"cache_write_tokens_details": {"ttl_30m_tokens": 4000, "ttl_12h_tokens": 3000}}}`))
	want := Usage{Input: 1000, Output: 50, CacheRead: 2000, CacheWrite: 7000, CacheWrite1h: 3000}
	if u != want {
		t.Fatalf("usage = %+v, want %+v", u, want)
	}
	if u.Total() != 10050 {
		t.Fatalf("total = %d, want 10050", u.Total())
	}
}

func TestOpenAIUsageWithoutCacheWrites(t *testing.T) {
	u := OpenAIUsage(usageMap(t, `{"prompt_tokens": 100, "completion_tokens": 5, "prompt_tokens_details": {"cached_tokens": 40}}`))
	if want := (Usage{Input: 60, Output: 5, CacheRead: 40}); u != want {
		t.Fatalf("usage = %+v, want %+v", u, want)
	}
}

// A malformed report must not produce negative input or more extended-TTL
// writes than writes.
func TestOpenAICacheDetailsClamp(t *testing.T) {
	u := Usage{Input: 10}
	u.AddOpenAICacheDetails(usageMap(t, `{"cached_tokens": 8, "cache_write_tokens": 5, "cache_write_tokens_details": {"ttl_12h_tokens": 9}}`))
	if u.Input != 0 || u.CacheRead != 8 || u.CacheWrite != 5 || u.CacheWrite1h != 5 {
		t.Fatalf("usage = %+v", u)
	}
}
