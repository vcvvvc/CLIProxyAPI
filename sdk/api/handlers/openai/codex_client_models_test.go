package openai

import "testing"

// What：验证 Codex 客户端模型的 Search 能力过滤契约。
// Why：模板来源与 provider 集合任一不可信时都必须默认关闭 Alpha Search。
func TestApplyCodexClientSearchToolSupport(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		supports      bool
		templateModel bool
		providers     []string
		want          bool
	}{
		{name: "gpt-5.5 template unchanged", id: "gpt-5.5", supports: true, templateModel: true, providers: []string{"openai"}, want: true},
		{name: "gpt-5.5 synthesized unchanged", id: "gpt-5.5-custom", supports: true, providers: []string{"codex"}, want: true},
		{name: "disabled declaration", id: "gpt-5.6-sol", supports: false, templateModel: true, providers: []string{"codex"}, want: false},
		{name: "codex template", id: "gpt-5.6-sol", supports: true, templateModel: true, providers: []string{"codex"}, want: true},
		{name: "missing providers", id: "gpt-5.6-sol", supports: true, templateModel: true, want: false},
		{name: "mixed providers", id: "gpt-5.6-sol", supports: true, templateModel: true, providers: []string{"codex", "openai"}, want: false},
		{name: "synthesized model", id: "gpt-5.6-custom", supports: true, providers: []string{"codex"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := map[string]any{"supports_search_tool": tt.supports}
			applyCodexClientSearchToolSupport(entry, tt.id, tt.templateModel, tt.providers)
			if got, ok := entry["supports_search_tool"].(bool); !ok || got != tt.want {
				t.Fatalf("supports_search_tool = %#v, want %t", entry["supports_search_tool"], tt.want)
			}
		})
	}
}
