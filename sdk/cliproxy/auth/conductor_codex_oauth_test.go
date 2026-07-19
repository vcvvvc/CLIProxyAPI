package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type codexOAuthHomeDispatcher struct {
	auths  map[int]Auth
	counts []int
}

func (d *codexOAuthHomeDispatcher) HeartbeatOK() bool { return true }

func (d *codexOAuthHomeDispatcher) RPopAuth(_ context.Context, _, _ string, _ http.Header, count int) ([]byte, error) {
	d.counts = append(d.counts, count)
	return json.Marshal(homeAuthDispatchResponse{Auth: d.auths[count]})
}

type codexOAuthFreezeStore struct {
	countingStore
	freezes []RuntimeFreezeState
	saved   []RuntimeFreezeState
}

func (s *codexOAuthFreezeStore) ListRuntimeFreezes(context.Context) ([]RuntimeFreezeState, error) {
	return s.freezes, nil
}

func (s *codexOAuthFreezeStore) SaveRuntimeFreeze(_ context.Context, state RuntimeFreezeState) error {
	s.saved = append(s.saved, state)
	return nil
}

func (s *codexOAuthFreezeStore) DeleteRuntimeFreeze(context.Context, string) error { return nil }

// What：验证运行时冻结原因只接受上游限额标识或带正整数的本地上限标识。
// Why：freeze.json 是外部持久化边界，必须拒绝含糊或非法的 limit_xx 值。
func TestRuntimeFreezeReasonValidation(t *testing.T) {
	tests := map[string]bool{
		"usage_limit_reached": true,
		"limit_50":            true,
		"limit_120":           true,
		"limit_xx":            false,
		"limit_0":             false,
		"limit_-1":            false,
		"limit_":              false,
	}
	for reason, want := range tests {
		if got := isRuntimeFreezeReason(reason); got != want {
			t.Errorf("isRuntimeFreezeReason(%q) = %t, want %t", reason, got, want)
		}
	}
}

// What：验证 Codex Alpha Search 只会选中可用的 OAuth 凭据。
// Why：API Key 与 active freeze 都必须沿用现有 selector 状态被排除。
func TestSelectCodexOAuthAuthSkipsAPIKeyAndRuntimeFreeze(t *testing.T) {
	ctx := WithSkipPersist(context.Background())
	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	until := time.Now().Add(time.Hour)
	auths := []*Auth{
		{ID: "a-api-key", Provider: "codex", Attributes: map[string]string{"api_key": "secret"}},
		{ID: "b-frozen-oauth", Provider: "codex", Metadata: map[string]any{"email": "frozen@example.com"}, Unavailable: true, NextRetryAfter: until, Quota: QuotaState{Exceeded: true, Reason: "limit_50", NextRecoverAt: until}},
		{ID: "c-active-oauth", Provider: "codex", Metadata: map[string]any{"email": "active@example.com"}},
	}
	for _, candidate := range auths {
		if _, err := manager.Register(ctx, candidate); err != nil {
			t.Fatalf("register %s: %v", candidate.ID, err)
		}
	}

	selected, err := manager.SelectCodexOAuthAuth(ctx, "", cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("SelectCodexOAuthAuth() error = %v", err)
	}
	if selected == nil || selected.ID != "c-active-oauth" {
		t.Fatalf("SelectCodexOAuthAuth() = %#v, want c-active-oauth", selected)
	}
}

// What：验证 Home 选择会累计冻结跳过数，并拒绝非 Codex OAuth 与 API Key。
// Why：计数失步会重复返回已尝试凭据，provider 漏检则可能把其他 OAuth 发送到 ChatGPT。
func TestSelectCodexOAuthAuthHomeMaintainsCountAndProvider(t *testing.T) {
	until := time.Now().Add(time.Hour)
	dispatcher := &codexOAuthHomeDispatcher{auths: map[int]Auth{
		1: {ID: "frozen", Provider: "codex", Metadata: map[string]any{"email": "frozen@example.com"}},
		2: {ID: "claude-oauth", Provider: "claude", Metadata: map[string]any{"email": "claude@example.com"}},
		3: {ID: "codex-api-key", Provider: "codex", Attributes: map[string]string{"api_key": "secret"}},
		4: {ID: "codex-oauth", Provider: "codex", Metadata: map[string]any{"email": "codex@example.com"}},
	}}
	oldDispatcher := currentHomeDispatcher
	currentHomeDispatcher = func() homeAuthDispatcher { return dispatcher }
	t.Cleanup(func() { currentHomeDispatcher = oldDispatcher })

	store := &codexOAuthFreezeStore{freezes: []RuntimeFreezeState{{AuthID: "frozen", Reason: "limit_50", NextRecoverAt: until}}}
	manager := NewManager(store, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "claude"})

	selected, err := manager.SelectCodexOAuthAuth(context.Background(), "", cliproxyexecutor.Options{})
	if err != nil || selected == nil || selected.ID != "codex-oauth" {
		t.Fatalf("SelectCodexOAuthAuth() = %#v, %v; want codex-oauth", selected, err)
	}
	wantCounts := []int{1, 2, 3, 4}
	if len(dispatcher.counts) != len(wantCounts) {
		t.Fatalf("Home counts = %v, want %v", dispatcher.counts, wantCounts)
	}
	for i := range wantCounts {
		if dispatcher.counts[i] != wantCounts[i] {
			t.Fatalf("Home counts = %v, want %v", dispatcher.counts, wantCounts)
		}
	}
}

// What：验证 GPT-5.5 成功请求在配置次数触发 limit_<N> 并从后续选择中排除。
// Why：Search 复用 Responses 执行链，必须证明配置阈值不会绕过冻结和 sidecar 持久化。
func TestGPT55SuccessLimitFreezeExcludesAuth(t *testing.T) {
	store := &codexOAuthFreezeStore{}
	manager := NewManager(store, &RoundRobinSelector{}, nil)
	configuredLimit := int64(120)
	manager.SetConfig(&internalconfig.Config{SuccessFreezeLimit: configuredLimit})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	ctx := WithSkipPersist(context.Background())
	for _, candidate := range []*Auth{
		{ID: "a-primary", Provider: "codex", Metadata: map[string]any{"email": "primary@example.com"}},
		{ID: "b-backup", Provider: "codex", Metadata: map[string]any{"email": "backup@example.com"}},
	} {
		if _, err := manager.Register(ctx, candidate); err != nil {
			t.Fatalf("register %s: %v", candidate.ID, err)
		}
	}
	for i := int64(0); i < configuredLimit-1; i++ {
		manager.MarkResult(ctx, Result{AuthID: "a-primary", Provider: "codex", Model: "gpt-5.5", Success: true})
	}
	before, _ := manager.GetByID("a-primary")
	if before.Success != configuredLimit-1 || before.successFreezeCount != configuredLimit-1 || before.Unavailable {
		t.Fatalf("before threshold = success %d count %d unavailable %t", before.Success, before.successFreezeCount, before.Unavailable)
	}
	freezeStarted := time.Now()
	manager.MarkResult(ctx, Result{AuthID: "a-primary", Provider: "codex", Model: "gpt-5.5", Success: true})
	frozen, _ := manager.GetByID("a-primary")
	remaining := frozen.Quota.NextRecoverAt.Sub(freezeStarted)
	if frozen.Success != configuredLimit || frozen.successFreezeCount != 0 || !frozen.Unavailable || frozen.Quota.Reason != successFreezeReasonForLimit(configuredLimit) {
		t.Fatalf("threshold state = %#v", frozen)
	}
	if remaining < usageLimitFreezeDuration-time.Minute || remaining > usageLimitFreezeDuration+time.Minute {
		t.Fatalf("freeze duration = %v, want %v", remaining, usageLimitFreezeDuration)
	}
	if len(store.saved) != 1 || store.saved[0].AuthID != "a-primary" || store.saved[0].Reason != successFreezeReasonForLimit(configuredLimit) {
		t.Fatalf("saved freezes = %#v", store.saved)
	}
	selected, _, err := manager.pickNext(ctx, "codex", "", cliproxyexecutor.Options{}, nil)
	if err != nil || selected == nil || selected.ID != "b-backup" {
		t.Fatalf("pickNext() = %#v, %v; want b-backup", selected, err)
	}
}

// What：验证热加载降低成功冻结上限后，已超过新上限的 auth 会立即冻结。
// Why：私有周期计数会跨配置热加载保留，严格相等判断会永久跳过已越过的上限。
func TestSuccessFreezeLimitLoweringFreezesExistingCount(t *testing.T) {
	store := &codexOAuthFreezeStore{}
	manager := NewManager(store, nil, nil)
	ctx := WithSkipPersist(context.Background())
	manager.SetConfig(&internalconfig.Config{SuccessFreezeLimit: 120})
	if _, err := manager.Register(ctx, &Auth{ID: "lowered-limit", Provider: "codex"}); err != nil {
		t.Fatalf("register auth: %v", err)
	}
	for range 60 {
		manager.MarkResult(ctx, Result{AuthID: "lowered-limit", Success: true})
	}

	manager.SetConfig(&internalconfig.Config{SuccessFreezeLimit: 50})
	manager.MarkResult(ctx, Result{AuthID: "lowered-limit", Success: true})
	frozen, _ := manager.GetByID("lowered-limit")
	if frozen.Success != 61 || frozen.successFreezeCount != 0 || !frozen.Unavailable || frozen.Quota.Reason != successFreezeReasonForLimit(50) {
		t.Fatalf("lowered limit state = %#v", frozen)
	}
	if len(store.saved) != 1 || store.saved[0].AuthID != "lowered-limit" {
		t.Fatalf("saved freezes = %#v", store.saved)
	}
}
