package api

import (
	"encoding/json"
	"testing"
)

// What：验证 Alpha Search body 清理与 routing 提取的边界行为。
// Why：上游只允许移除两个 cache 字段，解析失败时必须保留原始请求并安全降级。
func TestCodexAlphaSearchBodyHelpers(t *testing.T) {
	body := []byte(`{"id":" req-1 ","model":" gpt-5.6 ","prompt_cache_key":"x","prompt_cache_retention":7,"keep":true}`)
	sanitized := sanitizeCodexAlphaSearchBody(body)
	var payload map[string]any
	if err := json.Unmarshal(sanitized, &payload); err != nil {
		t.Fatalf("sanitized body is invalid JSON: %v", err)
	}
	for _, field := range []string{"prompt_cache_key", "prompt_cache_retention"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("sanitized body retained %s", field)
		}
	}
	if payload["keep"] != true {
		t.Fatalf("sanitized body lost unaffected field: %#v", payload)
	}
	if id, model := parseCodexAlphaSearchRouting(body); id != "req-1" || model != "gpt-5.6" {
		t.Fatalf("routing = %q, %q; want req-1, gpt-5.6", id, model)
	}
	if id, model := parseCodexAlphaSearchRouting([]byte(`{"id":123,"model":"gpt-5.6-sol"}`)); id != "" || model != "gpt-5.6-sol" {
		t.Fatalf("partial routing = %q, %q; want empty id and gpt-5.6-sol", id, model)
	}
	if id, model := parseCodexAlphaSearchRouting([]byte(`{"id":"session-2","model":123}`)); id != "session-2" || model != "" {
		t.Fatalf("partial routing = %q, %q; want session-2 and empty model", id, model)
	}
	for _, raw := range [][]byte{[]byte(`{"keep":true}`), []byte(`{`)} {
		if got := sanitizeCodexAlphaSearchBody(raw); string(got) != string(raw) {
			t.Fatalf("sanitize(%q) = %q, want unchanged", raw, got)
		}
		if id, model := parseCodexAlphaSearchRouting(raw); id != "" || model != "" {
			t.Fatalf("routing(%q) = %q, %q; want empty", raw, id, model)
		}
	}
}
