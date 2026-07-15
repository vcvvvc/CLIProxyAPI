package api

import (
	"net/http"
	"testing"
)

// What：验证两个 Alpha Search 公开别名都注册到同一个 handler。
// Why：GPT-5.6 的两种 base URL 必须共享认证、转发和错误处理边界。
func TestCodexAlphaSearchRoutesShareHandler(t *testing.T) {
	server := newTestServer(t)
	want := map[string]string{
		"/v1/alpha/search":                "",
		"/backend-api/codex/alpha/search": "",
	}
	for _, route := range server.engine.Routes() {
		if route.Method == http.MethodPost {
			if _, ok := want[route.Path]; ok {
				want[route.Path] = route.Handler
			}
		}
	}
	if want["/v1/alpha/search"] == "" || want["/backend-api/codex/alpha/search"] == "" {
		t.Fatalf("Alpha Search routes missing: %#v", want)
	}
	if want["/v1/alpha/search"] != want["/backend-api/codex/alpha/search"] {
		t.Fatalf("Alpha Search handlers differ: %#v", want)
	}
}
