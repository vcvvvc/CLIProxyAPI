package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type gpt56IntegrationExecutor struct {
	responsesAuthIDs []string
	searchAuthIDs    []string
}

func (e *gpt56IntegrationExecutor) Identifier() string { return "codex" }

func (e *gpt56IntegrationExecutor) Execute(_ context.Context, auth *cliproxyAuth.Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.responsesAuthIDs = append(e.responsesAuthIDs, auth.ID)
	return cliproxyexecutor.Response{Payload: []byte(`{"id":"response-ok"}`)}, nil
}

func (e *gpt56IntegrationExecutor) ExecuteStream(context.Context, *cliproxyAuth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e *gpt56IntegrationExecutor) Refresh(_ context.Context, auth *cliproxyAuth.Auth) (*cliproxyAuth.Auth, error) {
	return auth, nil
}

func (e *gpt56IntegrationExecutor) CountTokens(context.Context, *cliproxyAuth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e *gpt56IntegrationExecutor) PrepareRequest(_ *http.Request, _ *cliproxyAuth.Auth) error {
	return nil
}

func (e *gpt56IntegrationExecutor) HttpRequest(_ context.Context, auth *cliproxyAuth.Auth, _ *http.Request) (*http.Response, error) {
	e.searchAuthIDs = append(e.searchAuthIDs, auth.ID)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"search":"ok"}`))}, nil
}

// What：整体验证 GPT-5.6 正常 Responses 与 Alpha Search 共用计数、freeze 和备用 auth。
// Why：两条协议路径实现不同，但第 50 次成功后的选择结果必须保持一致。
func TestGPT56ConversationAndSearchShareLimitFreeze(t *testing.T) {
	server := newTestServer(t)
	manager := server.handlers.AuthManager
	executor := &gpt56IntegrationExecutor{}
	manager.RegisterExecutor(executor)
	primary := &cliproxyAuth.Auth{ID: "a-primary", Provider: "codex", Metadata: map[string]any{"email": "primary@example.com"}}
	if _, err := manager.Register(cliproxyAuth.WithSkipPersist(context.Background()), primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(primary.ID, "codex", []*registry.ModelInfo{{ID: "gpt-5.6-sol"}})
	t.Cleanup(func() { modelRegistry.UnregisterClient(primary.ID) })

	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		server.engine.ServeHTTP(recorder, req)
		return recorder
	}
	for i := 0; i < 49; i++ {
		if recorder := request("/v1/responses", `{"model":"gpt-5.6-sol","input":"hello"}`); recorder.Code != http.StatusOK {
			t.Fatalf("conversation %d status = %d body=%s", i+1, recorder.Code, recorder.Body.String())
		}
	}
	if recorder := request("/v1/alpha/search", `{"id":"search-1","model":"gpt-5.6-sol"}`); recorder.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	frozen, _ := manager.GetByID(primary.ID)
	if frozen.Success != 50 || !frozen.Unavailable || frozen.Quota.Reason != "limit_50" {
		t.Fatalf("primary freeze state = %#v", frozen)
	}

	backup := &cliproxyAuth.Auth{ID: "b-backup", Provider: "codex", Metadata: map[string]any{"email": "backup@example.com"}}
	if _, err := manager.Register(cliproxyAuth.WithSkipPersist(context.Background()), backup); err != nil {
		t.Fatalf("register backup: %v", err)
	}
	modelRegistry.RegisterClient(backup.ID, "codex", []*registry.ModelInfo{{ID: "gpt-5.6-sol"}})
	t.Cleanup(func() { modelRegistry.UnregisterClient(backup.ID) })
	if recorder := request("/v1/responses", `{"model":"gpt-5.6-sol","input":"after freeze"}`); recorder.Code != http.StatusOK {
		t.Fatalf("backup conversation status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := request("/v1/alpha/search", `{"id":"search-2","model":"gpt-5.6-sol"}`); recorder.Code != http.StatusOK {
		t.Fatalf("backup search status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if executor.responsesAuthIDs[len(executor.responsesAuthIDs)-1] != backup.ID || executor.searchAuthIDs[len(executor.searchAuthIDs)-1] != backup.ID {
		t.Fatalf("post-freeze auths = responses %v search %v", executor.responsesAuthIDs, executor.searchAuthIDs)
	}
}
