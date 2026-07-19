package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	cliproxyAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type failingCodexAlphaSearchWriter struct{ gin.ResponseWriter }

func (w failingCodexAlphaSearchWriter) Write([]byte) (int, error) {
	return 0, errors.New("downstream disconnected")
}

// What：验证 Alpha Search 的 session affinity、上游白名单 headers 与响应 relay。
// Why：这些边界必须保留客户端契约，同时阻止内部 headers 泄露并原样传回上游结果。
func TestCodexAlphaSearchHTTPBoundaries(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6"}`)
	clientHeaders := http.Header{"User-Agent": {"client-agent"}, "Version": {"1"}, "X-Internal": {"secret"}}
	opts := buildCodexAlphaSearchSelectionOptions(clientHeaders, " session-1 ", body)
	if got := opts.Headers.Get("X-Session-ID"); got != "session-1" || !bytes.Equal(opts.OriginalRequest, body) {
		t.Fatalf("selection options lost session/body: header=%q body=%q", got, opts.OriginalRequest)
	}
	selected := &cliproxyAuth.Auth{Metadata: map[string]any{"account_id": "acct-1"}}
	upstream := buildCodexAlphaSearchUpstreamHeaders(clientHeaders, selected)
	if upstream.Get("Originator") != "codex_cli_rs" || upstream.Get("Chatgpt-Account-Id") != "acct-1" || upstream.Get("Version") != "1" {
		t.Fatalf("upstream headers missing contract fields: %#v", upstream)
	}
	if upstream.Get("X-Internal") != "" || upstream.Get("Content-Type") != "application/json" {
		t.Fatalf("upstream headers leaked or missed defaults: %#v", upstream)
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{StatusCode: http.StatusMultiStatus, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`))}
	relayed, readErr, writeErr := (&Server{}).relayCodexAlphaSearchResponse(context.Background(), c, resp)
	if readErr != nil || writeErr != nil {
		t.Fatalf("relay response: read=%v write=%v", readErr, writeErr)
	}
	if !bytes.Equal(relayed, recorder.Body.Bytes()) || recorder.Code != http.StatusMultiStatus || recorder.Header().Get("Content-Type") != "application/json" || recorder.Body.String() != `{"ok":true}` {
		t.Fatalf("relayed response = status %d headers %#v body %q", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

// What：验证 Alpha Search 结果回写可触发 GPT-5.6 的默认 limit_50 与 usage-limit freeze。
// Why：该路由绕过通用 Execute，必须单独证明成功计数和上游 429 都进入 MarkResult。
func TestCodexAlphaSearchResultUpdatesFreezeState(t *testing.T) {
	ctx := cliproxyAuth.WithSkipPersist(context.Background())
	manager := cliproxyAuth.NewManager(nil, nil, nil)
	selected := &cliproxyAuth.Auth{ID: "gpt-5.6-auth", Provider: "codex", Metadata: map[string]any{"email": "alpha@example.com"}}
	if _, err := manager.Register(ctx, selected); err != nil {
		t.Fatalf("register selected auth: %v", err)
	}
	freezeStarted := time.Now()
	for i := 0; i < 50; i++ {
		manager.MarkResult(ctx, codexAlphaSearchResult(selected, "gpt-5.6-sol", http.StatusOK, []byte(`{"ok":true}`), nil))
	}
	frozen, _ := manager.GetByID(selected.ID)
	remaining := frozen.Quota.NextRecoverAt.Sub(freezeStarted)
	if frozen.Success != 50 || !frozen.Unavailable || frozen.Quota.Reason != "limit_50" {
		t.Fatalf("limit_50 state = %#v", frozen)
	}
	if remaining < 721*time.Hour-time.Minute || remaining > 721*time.Hour+time.Minute {
		t.Fatalf("limit_50 duration = %v", remaining)
	}

	usage := &cliproxyAuth.Auth{ID: "usage-limit-auth", Provider: "codex", Metadata: map[string]any{"email": "usage@example.com"}}
	if _, err := manager.Register(ctx, usage); err != nil {
		t.Fatalf("register usage auth: %v", err)
	}
	manager.MarkResult(ctx, codexAlphaSearchResult(usage, "", http.StatusTooManyRequests, []byte(`{"error":{"type":"usage_limit_reached"}}`), nil))
	usageFrozen, _ := manager.GetByID(usage.ID)
	if !usageFrozen.Unavailable || usageFrozen.Quota.Reason != "usage_limit_reached" {
		t.Fatalf("usage-limit state = %#v", usageFrozen)
	}

	failureRecorder := httptest.NewRecorder()
	failureContext, _ := gin.CreateTestContext(failureRecorder)
	failureContext.Writer = failingCodexAlphaSearchWriter{ResponseWriter: failureContext.Writer}
	failureResp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`))}
	body, upstreamReadErr, downstreamWriteErr := (&Server{}).relayCodexAlphaSearchResponse(context.Background(), failureContext, failureResp)
	result := codexAlphaSearchResult(selected, "gpt-5.6-sol", http.StatusOK, body, upstreamReadErr)
	if upstreamReadErr != nil || downstreamWriteErr == nil || !result.Success {
		t.Fatalf("downstream error accounting = read %v write %v result %#v", upstreamReadErr, downstreamWriteErr, result)
	}
}
