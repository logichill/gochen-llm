package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gochen-llm/client"
	"gochen-llm/entity"
	"gochen/errors"
)

type fakeProviderConfigRepo struct {
	listAllFn    func(ctx context.Context) ([]*entity.ProviderConfig, error)
	replaceAllFn func(ctx context.Context, configs []*entity.ProviderConfig) error

	replaceAllCalls int
	lastConfigs     []*entity.ProviderConfig
}

func (f *fakeProviderConfigRepo) ListAll(ctx context.Context) ([]*entity.ProviderConfig, error) {
	if f.listAllFn == nil {
		return nil, nil
	}
	return f.listAllFn(ctx)
}

func (f *fakeProviderConfigRepo) ReplaceAll(ctx context.Context, configs []*entity.ProviderConfig) error {
	f.replaceAllCalls++
	f.lastConfigs = configs
	if f.replaceAllFn == nil {
		return nil
	}
	return f.replaceAllFn(ctx, configs)
}

func (f *fakeProviderConfigRepo) UpdatePricing(ctx context.Context, updates []entity.ProviderPricing) error {
	return nil
}

type fakeLLMClient struct {
	resp   *client.ChatResponse
	err    error
	chatFn func(ctx context.Context, req *client.ChatRequest) (*client.ChatResponse, error)
	calls  int32
}

func (f *fakeLLMClient) Chat(ctx context.Context, req *client.ChatRequest) (*client.ChatResponse, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.chatFn != nil {
		return f.chatFn(ctx, req)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func newTestEndpoint(priority int, weight int, cooldownSeconds int) *endpointState {
	if weight == 0 {
		weight = 100
	}
	if cooldownSeconds == 0 {
		cooldownSeconds = 30
	}
	return &endpointState{
		cfg: &entity.ProviderConfig{
			Name:            "ep",
			Provider:        "mock",
			Model:           "mock-model",
			Enabled:         true,
			Priority:        priority,
			Weight:          weight,
			CooldownSeconds: cooldownSeconds,
			MaxErrorStreak:  2,
		},
	}
}

func TestProviderManagerSelectCandidates(t *testing.T) {
	m := &providerManagerImpl{}
	now := time.Now()

	e0 := newTestEndpoint(200, 100, 30)
	e1 := newTestEndpoint(100, 100, 30)
	e2 := newTestEndpoint(100, 100, 30)
	e3 := newTestEndpoint(100, 100, 30)

	atomic.StoreInt64(&e2.cooldownUntil, now.Add(2*time.Minute).UnixNano())
	atomic.StoreUint32(&e3.inCircuitOpen, 1)

	eps := []*endpointState{e0, e1, e2, e3}

	candidates := m.selectCandidates(eps, now)
	if len(candidates) != 1 || candidates[0] != 1 {
		t.Fatalf("unexpected candidates: %v", candidates)
	}

	allMinPri := m.selectAllByMinPriority(eps)
	if len(allMinPri) != 2 || allMinPri[0] != 1 || allMinPri[1] != 2 {
		t.Fatalf("unexpected min-priority candidates: %v", allMinPri)
	}
}

func TestProviderManagerTakeRateTokenAndWindow(t *testing.T) {
	m := &providerManagerImpl{}
	now := time.Now()
	ep := &endpointState{cfg: &entity.ProviderConfig{RateLimitPerMin: 60, RateLimitBurst: 0}}

	if !m.takeRateToken(ep, now) {
		t.Fatalf("first token should be available")
	}

	ep.rateMu.Lock()
	ep.rateTokens = 0
	ep.rateLastRefill = now
	ep.rateMu.Unlock()
	if m.takeRateToken(ep, now) {
		t.Fatalf("token should be exhausted without elapsed time")
	}
	if !m.takeRateToken(ep, now.Add(2*time.Second)) {
		t.Fatalf("token should refill after elapsed time")
	}

	m.bumpRateWindow(ep, now)
	if got := atomic.LoadInt64(&ep.rateCount); got != 1 {
		t.Fatalf("unexpected rate count: %d", got)
	}
	m.bumpRateWindow(ep, now.Add(61*time.Second))
	if got := atomic.LoadInt64(&ep.rateCount); got != 1 {
		t.Fatalf("rate count should reset for new minute, got: %d", got)
	}
}

func TestProviderManagerReplaceConfigsValidationAndDefaults(t *testing.T) {
	repo := &fakeProviderConfigRepo{}
	m := &providerManagerImpl{repo: repo}

	cfg := &entity.ProviderConfig{Provider: "mock"}
	if err := m.ReplaceConfigs(context.Background(), []*entity.ProviderConfig{cfg}); err != nil {
		t.Fatalf("replace configs failed: %v", err)
	}
	if repo.replaceAllCalls != 1 {
		t.Fatalf("expected replace all called once, got %d", repo.replaceAllCalls)
	}
	if cfg.Priority != 100 || cfg.TimeoutSeconds != 30 || cfg.CooldownSeconds != 30 || cfg.Weight != 100 {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if cfg.Name != "mock" {
		t.Fatalf("expected name fallback to provider, got %q", cfg.Name)
	}

	err := m.ReplaceConfigs(context.Background(), []*entity.ProviderConfig{{Provider: "mock", InputPricePer1k: -1}})
	if err == nil {
		t.Fatalf("expected validation error for negative price")
	}
	err = m.ReplaceConfigs(context.Background(), []*entity.ProviderConfig{{Provider: "mock", OutputPricePer1k: 101}})
	if err == nil {
		t.Fatalf("expected validation error for abnormal price")
	}
}

func TestProviderManagerListEffectiveConfigsMaskAPIKey(t *testing.T) {
	m := &providerManagerImpl{}
	ep1 := &endpointState{cfg: &entity.ProviderConfig{Name: "a", APIKey: "abcdef"}}
	ep2 := &endpointState{cfg: &entity.ProviderConfig{Name: "b", APIKey: "abc"}}
	m.endpoints.Store([]*endpointState{ep1, ep2})

	cfgs, err := m.ListEffectiveConfigs(context.Background())
	if err != nil {
		t.Fatalf("list effective configs: %v", err)
	}
	if len(cfgs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(cfgs))
	}
	if cfgs[0].APIKey != "****cdef" {
		t.Fatalf("unexpected masked key: %q", cfgs[0].APIKey)
	}
	if cfgs[1].APIKey != "****" {
		t.Fatalf("unexpected short masked key: %q", cfgs[1].APIKey)
	}
	if ep1.cfg.APIKey != "abcdef" {
		t.Fatalf("original config should not be mutated")
	}
}

func TestProviderManagerChatForUser(t *testing.T) {
	successClient := &fakeLLMClient{resp: &client.ChatResponse{Content: "ok"}}
	ep := newTestEndpoint(100, 100, 30)
	ep.client = successClient
	ep.cfg.Provider = "openai"
	ep.cfg.Model = "gpt-test"
	ep.cfg.InputPricePer1k = 0.1
	ep.cfg.OutputPricePer1k = 0.2

	m := &providerManagerImpl{}
	m.endpoints.Store([]*endpointState{ep})

	if _, err := m.ChatForUser(nil, 1, &client.ChatRequest{}); err == nil {
		t.Fatalf("expected nil ctx validation error")
	}
	if _, err := m.ChatForUser(context.Background(), 1, nil); err == nil {
		t.Fatalf("expected nil request validation error")
	}

	result, err := m.ChatForUser(context.Background(), 1, &client.ChatRequest{Messages: []client.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if result == nil || result.Response == nil || result.Response.Content != "ok" {
		t.Fatalf("unexpected response: %#v", result)
	}
	if result.Provider != "openai" || result.Model != "gpt-test" {
		t.Fatalf("unexpected provider/model: %s/%s", result.Provider, result.Model)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("unexpected negative latency: %d", result.LatencyMs)
	}
	if result.InputPricePer1k != 0.1 || result.OutputPricePer1k != 0.2 {
		t.Fatalf("unexpected prices: %v/%v", result.InputPricePer1k, result.OutputPricePer1k)
	}
}

func TestProviderManagerChatForUserAllFailed(t *testing.T) {
	failClient := &fakeLLMClient{err: errors.New("boom")}
	ep := newTestEndpoint(100, 100, 30)
	ep.client = failClient

	m := &providerManagerImpl{}
	m.endpoints.Store([]*endpointState{ep})

	_, err := m.ChatForUser(context.Background(), 1, &client.ChatRequest{Messages: []client.ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected all endpoints failed")
	}
	if atomic.LoadInt64(&ep.cooldownUntil) == 0 {
		t.Fatalf("expected endpoint entered cooldown")
	}
	if got := atomic.LoadUint64(&ep.stats.failures); got == 0 {
		t.Fatalf("expected failures incremented")
	}
}

func TestProviderManagerReloadAndListStatus(t *testing.T) {
	repo := &fakeProviderConfigRepo{
		listAllFn: func(ctx context.Context) ([]*entity.ProviderConfig, error) {
			return []*entity.ProviderConfig{
				{Provider: "invalid", Enabled: true, Name: "bad", Model: "x", TimeoutSeconds: 1},
				{Provider: "mock", Enabled: false, Name: "disabled", Model: "x", TimeoutSeconds: 1},
				{Provider: "mock", Enabled: true, Name: "ok", Model: "m", TimeoutSeconds: 1, Priority: 10, Weight: 20, RateLimitPerMin: 60, RateLimitBurst: 10},
			}, nil
		},
	}
	m := &providerManagerImpl{repo: repo}

	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	eps, err := m.getOrLoadEndpoints(context.Background())
	if err != nil {
		t.Fatalf("get endpoints failed: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected only one valid enabled endpoint, got %d", len(eps))
	}

	ep := eps[0]
	atomic.StoreUint64(&ep.stats.totalRequests, 10)
	atomic.StoreUint64(&ep.stats.failures, 2)
	atomic.StoreInt64(&ep.stats.lastErrorAt, time.Now().UnixNano())
	ep.stats.lastError.Store("last boom")
	m.recordHealthSample(ep, healthSample{Success: true, StatusCode: 200, LatencyMs: 12})

	status, err := m.ListStatus(context.Background())
	if err != nil {
		t.Fatalf("list status failed: %v", err)
	}
	if len(status) != 1 {
		t.Fatalf("expected one status, got %d", len(status))
	}
	if status[0].Name != "ok" {
		t.Fatalf("unexpected status name: %s", status[0].Name)
	}
	if status[0].SuccessRate <= 0 || status[0].SuccessRate >= 1 {
		t.Fatalf("unexpected success rate: %v", status[0].SuccessRate)
	}
	if status[0].LastError == "" || status[0].LastErrorAt == "" {
		t.Fatalf("expected error fields populated")
	}
	if len(status[0].HealthHistory) != 1 {
		t.Fatalf("expected health history")
	}
}

func TestProviderManagerChatForUserPreservesDependencyFailureCode(t *testing.T) {
	failClient := &fakeLLMClient{err: errors.NewCode(errors.ServiceUnavailable, "upstream auth failed")}
	ep := newTestEndpoint(100, 100, 30)
	ep.client = failClient

	m := &providerManagerImpl{}
	m.endpoints.Store([]*endpointState{ep})

	_, err := m.ChatForUser(context.Background(), 1, &client.ChatRequest{Messages: []client.ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected all endpoints failed")
	}
	if !errors.Is(err, errors.ServiceUnavailable) {
		t.Fatalf("expected ServiceUnavailable preserved, got %v", err)
	}
}

func TestProviderManagerChatForUser_DoesNotFailoverOnDeterministic4xx(t *testing.T) {
	badReqClient := &fakeLLMClient{err: errors.NewCode(errors.InvalidInput, "bad request")}
	successClient := &fakeLLMClient{resp: &client.ChatResponse{Content: "ok"}}

	first := newTestEndpoint(100, 100, 30)
	first.client = badReqClient
	first.cfg.Name = "first"
	first.cfg.Provider = "openai"
	second := newTestEndpoint(100, 100, 30)
	second.client = successClient
	second.cfg.Name = "second"
	second.cfg.Provider = "gemini"

	m := &providerManagerImpl{}
	m.endpoints.Store([]*endpointState{first, second})

	_, err := m.ChatForUser(context.Background(), 1, &client.ChatRequest{Messages: []client.ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected deterministic provider error")
	}
	if !errors.Is(err, errors.InvalidInput) {
		t.Fatalf("expected InvalidInput, got %v", err)
	}
	if got := atomic.LoadInt32(&badReqClient.calls); got != 1 {
		t.Fatalf("expected first client called once, got %d", got)
	}
	if got := atomic.LoadInt32(&successClient.calls); got != 0 {
		t.Fatalf("expected second client not to be called, got %d", got)
	}
	if got := atomic.LoadInt64(&first.cooldownUntil); got != 0 {
		t.Fatalf("expected deterministic error not to trigger cooldown, got %d", got)
	}
}

func TestProviderManagerChatForUser_PrefersCurrentDeterministicErrorOverEarlierTransient(t *testing.T) {
	transientClient := &fakeLLMClient{err: errors.NewCode(errors.ServiceUnavailable, "temporary upstream error")}
	deterministicClient := &fakeLLMClient{err: errors.NewCode(errors.InvalidInput, "bad request")}

	first := newTestEndpoint(100, 100, 30)
	first.client = transientClient
	first.cfg.Name = "first"
	first.cfg.Provider = "openai"
	second := newTestEndpoint(100, 100, 30)
	second.client = deterministicClient
	second.cfg.Name = "second"
	second.cfg.Provider = "gemini"

	m := &providerManagerImpl{}
	m.endpoints.Store([]*endpointState{first, second})

	_, err := m.ChatForUser(context.Background(), 1, &client.ChatRequest{Messages: []client.ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected provider error")
	}
	if !errors.Is(err, errors.InvalidInput) {
		t.Fatalf("expected latest deterministic error, got %v", err)
	}
	if got := atomic.LoadInt32(&transientClient.calls); got != 1 {
		t.Fatalf("expected transient client called once, got %d", got)
	}
	if got := atomic.LoadInt32(&deterministicClient.calls); got != 1 {
		t.Fatalf("expected deterministic client called once, got %d", got)
	}
}
