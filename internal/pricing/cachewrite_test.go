package pricing

import (
	"testing"

	"theophylax/internal/providers"
)

// OpenAI GPT-5.6 and later bill prompt-cache writes: 1.25x input for the
// default 30-minute TTL (cache_creation_input_token_cost) and 2.5x for the
// 12-hour TTL (cache_creation_input_token_cost_above_12hr). Rates are from
// the LiteLLM catalog's gpt-6.1-sol entry.
const openAICacheFixture = `{
  "gpt-6.1-sol": {"litellm_provider": "openai", "input_cost_per_token": 2e-06, "output_cost_per_token": 1.6e-05,
    "cache_read_input_token_cost": 1e-07, "cache_creation_input_token_cost": 2.5e-06, "cache_creation_input_token_cost_above_12hr": 5e-06,
    "input_cost_per_token_above_272k_tokens": 4e-06, "cache_creation_input_token_cost_above_272k_tokens": 5e-06,
    "cache_creation_input_token_cost_above_12hr_above_272k_tokens": 1e-05},
  "both-ttls": {"litellm_provider": "openai", "input_cost_per_token": 1e-06, "output_cost_per_token": 1e-06,
    "cache_creation_input_token_cost_above_1hr": 2e-06, "cache_creation_input_token_cost_above_12hr": 9e-06,
    "cache_creation_input_token_cost_above_1hr_above_100k_tokens": 3e-06, "cache_creation_input_token_cost_above_12hr_above_100k_tokens": 9e-06}
}`

func openAICatalog(t *testing.T) *Catalog {
	t.Helper()
	entries, err := Parse([]byte(openAICacheFixture))
	if err != nil {
		t.Fatal(err)
	}
	c := New(nil, "off", 0)
	c.set(entries, "test", c.fetched)
	return c
}

func TestParseOpenAI12hCacheWriteRate(t *testing.T) {
	p := openAICatalog(t).entries["gpt-6.1-sol"]
	if p == nil || p.Rates.CacheWrite == nil || *p.Rates.CacheWrite != 2.5e-06 || p.Rates.CacheWrite1h == nil || *p.Rates.CacheWrite1h != 5e-06 {
		t.Fatalf("base rates: %+v", p)
	}
	if len(p.Tiers) != 1 || p.Tiers[0].Above != 272000 || p.Tiers[0].Rates.CacheWrite1h == nil || *p.Tiers[0].Rates.CacheWrite1h != 1e-05 {
		t.Fatalf("272k tier: %+v", p.Tiers)
	}
}

func TestOneHourRateWinsOverTwelveHour(t *testing.T) {
	// Parse many times: map iteration order must not decide the result.
	for range 20 {
		p := openAICatalog(t).entries["both-ttls"]
		if *p.Rates.CacheWrite1h != 2e-06 || len(p.Tiers) != 1 || *p.Tiers[0].Rates.CacheWrite1h != 3e-06 {
			t.Fatalf("1h rate should win: %+v %+v", p.Rates, p.Tiers)
		}
	}
}

func TestOpenAICacheWriteCost(t *testing.T) {
	c := openAICatalog(t)
	// 1,000 uncached input, 2,000 cached, 3,000 written (1,000 of them with
	// the 12-hour TTL) and 500 output.
	u := providers.Usage{Input: 1000, CacheRead: 2000, CacheWrite: 3000, CacheWrite1h: 1000, Output: 500}
	got, key := c.Cost(providers.KindOpenAI, u, "gpt-6.1-sol")
	want := 1000*2e-06 + 2000*1e-07 + 2000*2.5e-06 + 1000*5e-06 + 500*1.6e-05
	if key != "gpt-6.1-sol" || !near(got, want) {
		t.Fatalf("cost = %v (%s), want %v", got, key, want)
	}
}
