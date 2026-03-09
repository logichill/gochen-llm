package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gochen-llm/client"
	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
)

type testChatManager struct {
	chatForUserFn func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error)
	lastReq       *client.ChatRequest
}

func (m *testChatManager) Start(ctx context.Context) error { return nil }
func (m *testChatManager) Stop(ctx context.Context) error  { return nil }
func (m *testChatManager) Reload(ctx context.Context) error {
	return nil
}
func (m *testChatManager) ListEffectiveConfigs(ctx context.Context) ([]*entity.ProviderConfig, error) {
	return nil, nil
}
func (m *testChatManager) ReplaceConfigs(ctx context.Context, configs []*entity.ProviderConfig) error {
	return nil
}
func (m *testChatManager) ListStatus(ctx context.Context) ([]*EndpointStatus, error) {
	return nil, nil
}
func (m *testChatManager) ChatForUser(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
	m.lastReq = req
	if m.chatForUserFn != nil {
		return m.chatForUserFn(ctx, userID, req)
	}
	return &ChatExecution{Response: &client.ChatResponse{Content: "ok"}, Provider: "mock", Model: "mock", LatencyMs: 1}, nil
}

type testPromptService struct {
	getPromptFn       func(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error)
	renderPromptFn    func(ctx context.Context, tmpl *entity.PromptTemplate, vars map[string]any) (string, error)
	assignABVariantFn func(ctx context.Context, testID int64, userID int64) (*entity.PromptTemplate, string, error)
}

func (s *testPromptService) GetPrompt(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
	if s.getPromptFn != nil {
		return s.getPromptFn(ctx, name, scope, scopeID)
	}
	return nil, nil
}

func (s *testPromptService) GetPromptByID(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
	return nil, nil
}

func (s *testPromptService) RenderPrompt(ctx context.Context, tmpl *entity.PromptTemplate, vars map[string]any) (string, error) {
	if s.renderPromptFn != nil {
		return s.renderPromptFn(ctx, tmpl, vars)
	}
	return tmpl.Content, nil
}

func (s *testPromptService) ComposePrompts(ctx context.Context, names []string, scope entity.PromptScope, scopeID int64, vars map[string]any) (string, error) {
	return "", nil
}

func (s *testPromptService) SavePrompt(ctx context.Context, tmpl *entity.PromptTemplate) error {
	return nil
}

func (s *testPromptService) ListPrompts(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error) {
	return nil, nil
}

func (s *testPromptService) CreateVersion(ctx context.Context, templateID int64, changeLog string) (*entity.PromptVersion, error) {
	return nil, nil
}

func (s *testPromptService) RollbackVersion(ctx context.Context, templateID int64, version int) error {
	return nil
}

func (s *testPromptService) ExportPrompts(ctx context.Context, filter repo.PromptFilter) ([]byte, error) {
	return nil, nil
}

func (s *testPromptService) ImportPrompts(ctx context.Context, data []byte) error {
	return nil
}

func (s *testPromptService) StartABTest(ctx context.Context, test *entity.ABTest) error {
	return nil
}

func (s *testPromptService) GetABTestResult(ctx context.Context, testID int64) (*entity.ABTest, error) {
	return nil, nil
}

func (s *testPromptService) AssignABVariant(ctx context.Context, testID int64, userID int64) (*entity.PromptTemplate, string, error) {
	if s.assignABVariantFn != nil {
		return s.assignABVariantFn(ctx, testID, userID)
	}
	return nil, "", nil
}

type testSafetyService struct {
	checkRateLimitFn    func(ctx context.Context, userID int64) (*RateLimitResult, error)
	validateInputFn     func(ctx context.Context, input string) (*SafetyResult, error)
	buildSystemPromptFn func(ctx context.Context) (string, error)
	filterContentFn     func(ctx context.Context, content string) (string, error)
	recordAuditLogFn    func(ctx context.Context, log *entity.AuditLog) error
}

func (s *testSafetyService) GetActivePolicy(ctx context.Context) (*entity.SafetyPolicy, error) {
	return nil, nil
}

func (s *testSafetyService) BuildSystemPrompt(ctx context.Context) (string, error) {
	if s.buildSystemPromptFn != nil {
		return s.buildSystemPromptFn(ctx)
	}
	return "", nil
}

func (s *testSafetyService) ValidateInput(ctx context.Context, input string) (*SafetyResult, error) {
	if s.validateInputFn != nil {
		return s.validateInputFn(ctx, input)
	}
	return &SafetyResult{Allowed: true}, nil
}

func (s *testSafetyService) ValidateOutput(ctx context.Context, output string) (*SafetyResult, error) {
	return &SafetyResult{Allowed: true}, nil
}

func (s *testSafetyService) FilterContent(ctx context.Context, content string) (string, error) {
	if s.filterContentFn != nil {
		return s.filterContentFn(ctx, content)
	}
	return content, nil
}

func (s *testSafetyService) CheckRateLimit(ctx context.Context, userID int64) (*RateLimitResult, error) {
	if s.checkRateLimitFn != nil {
		return s.checkRateLimitFn(ctx, userID)
	}
	return &RateLimitResult{Allowed: true}, nil
}

func (s *testSafetyService) RecordAuditLog(ctx context.Context, log *entity.AuditLog) error {
	if s.recordAuditLogFn != nil {
		return s.recordAuditLogFn(ctx, log)
	}
	return nil
}

func (s *testSafetyService) DetectPII(ctx context.Context, content string) (*SafetyResult, error) {
	return &SafetyResult{Allowed: true}, nil
}

func (s *testSafetyService) MaskPII(ctx context.Context, content string) (string, error) {
	return content, nil
}

func (s *testSafetyService) GetRateLimitSettings() RateLimitSettings {
	return RateLimitSettings{}
}

type testMetricsRepo struct {
	saveFn func(ctx context.Context, m *entity.Metrics) error
}

func (r *testMetricsRepo) Save(ctx context.Context, m *entity.Metrics) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, m)
	}
	return nil
}

func (r *testMetricsRepo) Aggregate(ctx context.Context, filter entity.MetricsFilter) (*entity.MetricsReport, error) {
	return nil, nil
}

func (r *testMetricsRepo) AggregateByVariant(ctx context.Context, filter entity.MetricsFilter) ([]*entity.VariantMetricsReport, error) {
	return nil, nil
}

func (r *testMetricsRepo) List(ctx context.Context, filter entity.MetricsFilter, limit, offset int) ([]*entity.Metrics, int64, error) {
	return nil, 0, nil
}

func (r *testMetricsRepo) Significance(ctx context.Context, filter entity.MetricsFilter) (*entity.ABSignificanceReport, error) {
	return nil, nil
}

type testCostCalc struct {
	estimateFn func(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64
}

func (c *testCostCalc) EstimateCost(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64 {
	if c.estimateFn != nil {
		return c.estimateFn(provider, model, requestTokens, responseTokens, inputPer1k, outputPer1k)
	}
	return 0
}

func TestChatServiceChatValidationAndHappyPath(t *testing.T) {
	if _, err := NewChatService(nil, nil, nil, nil, nil).Chat(context.Background(), nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid input for nil chat req, got %v", err)
	}

	nilManagerSvc := &chatServiceImpl{}
	if _, err := nilManagerSvc.Chat(context.Background(), &ChatRequest{}); err == nil || !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected manager missing error, got %v", err)
	}

	var savedMetrics []*entity.Metrics
	var auditLogs []*entity.AuditLog

	manager := &testChatManager{
		chatForUserFn: func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
			return &ChatExecution{Response: &client.ChatResponse{Content: "origin-response"}, Provider: "openai", Model: "gpt-x", LatencyMs: 12, InputPricePer1k: 0.1, OutputPricePer1k: 0.2}, nil
		},
	}
	safety := &testSafetyService{
		checkRateLimitFn: func(ctx context.Context, userID int64) (*RateLimitResult, error) {
			return &RateLimitResult{Allowed: true}, nil
		},
		validateInputFn: func(ctx context.Context, input string) (*SafetyResult, error) {
			if !strings.Contains(input, "hello") {
				t.Fatalf("expected joined input messages")
			}
			return &SafetyResult{Allowed: true}, nil
		},
		buildSystemPromptFn: func(ctx context.Context) (string, error) {
			return "safety", nil
		},
		filterContentFn: func(ctx context.Context, content string) (string, error) {
			return "filtered-response", nil
		},
		recordAuditLogFn: func(ctx context.Context, log *entity.AuditLog) error {
			auditLogs = append(auditLogs, log)
			return nil
		},
	}
	metrics := &testMetricsRepo{
		saveFn: func(ctx context.Context, m *entity.Metrics) error {
			savedMetrics = append(savedMetrics, m)
			return nil
		},
	}
	cost := &testCostCalc{estimateFn: func(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64 {
		if provider != "openai" || model != "gpt-x" {
			t.Fatalf("unexpected provider/model in cost calc")
		}
		return 1.23
	}}

	svc := NewChatService(manager, nil, safety, metrics, cost)
	req := &ChatRequest{
		UserID:      9,
		System:      "business",
		Messages:    []Message{{Role: "user", Content: "hello"}},
		Temperature: -1,
		MaxTokens:   0,
		Metadata: map[string]any{
			"ab_test_id":         int64(1),
			"ab_variant":         "B",
			"prompt_template_id": int64(8),
		},
	}

	resp, err := svc.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if resp.Content != "filtered-response" || resp.Usage == nil || resp.Usage.TotalTokens == 0 {
		t.Fatalf("unexpected chat response: %#v", resp)
	}
	if manager.lastReq == nil || manager.lastReq.System != "safety\n\nbusiness" {
		t.Fatalf("expected safety prompt prepended, req=%#v", manager.lastReq)
	}
	if len(savedMetrics) != 1 || savedMetrics[0].Status != "ok" || savedMetrics[0].CostUSD != 1.23 {
		t.Fatalf("expected success metrics saved, got %#v", savedMetrics)
	}
	if len(auditLogs) != 1 || auditLogs[0].Action != "llm.chat" {
		t.Fatalf("expected audit log recorded")
	}
}

func TestChatServiceErrorAndFilterPaths(t *testing.T) {
	var saved []*entity.Metrics
	manager := &testChatManager{
		chatForUserFn: func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
			return nil, errors.New("boom")
		},
	}
	metrics := &testMetricsRepo{saveFn: func(ctx context.Context, m *entity.Metrics) error {
		saved = append(saved, m)
		return nil
	}}
	svc := NewChatService(manager, nil, nil, metrics, nil)

	_, err := svc.Chat(context.Background(), &ChatRequest{UserID: 2, Messages: []Message{{Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected manager error")
	}
	if len(saved) != 1 || saved[0].Status != "error" {
		t.Fatalf("expected error metric saved, got %#v", saved)
	}

	svc2 := NewChatService(&testChatManager{
		chatForUserFn: func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
			return &ChatExecution{Response: &client.ChatResponse{Content: "raw"}, Provider: "mock", Model: "m", LatencyMs: 1}, nil
		},
	}, nil, &testSafetyService{
		filterContentFn: func(ctx context.Context, content string) (string, error) {
			return "", errors.New("filter failed")
		},
	}, nil, nil)

	_, err = svc2.Chat(context.Background(), &ChatRequest{UserID: 1, Messages: []Message{{Content: "x"}}})
	if err == nil {
		t.Fatalf("expected hard filter error")
	}
}

func TestChatServicePromptStreamAndBatch(t *testing.T) {
	prompt := &testPromptService{
		getPromptFn: func(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
			if name == "missing" {
				return nil, nil
			}
			return &entity.PromptTemplate{ID: 22, Content: "tmpl"}, nil
		},
		renderPromptFn: func(ctx context.Context, tmpl *entity.PromptTemplate, vars map[string]any) (string, error) {
			return "rendered-system", nil
		},
		assignABVariantFn: func(ctx context.Context, testID int64, userID int64) (*entity.PromptTemplate, string, error) {
			return &entity.PromptTemplate{ID: 33, Content: "ab"}, "B", nil
		},
	}

	manager := &testChatManager{
		chatForUserFn: func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
			if len(req.Messages) > 0 && req.Messages[0].Content == "fail" {
				return nil, errors.New("chat failed")
			}
			return &ChatExecution{Response: &client.ChatResponse{Content: strings.Repeat("a", 450)}, Provider: "mock", Model: "mock", LatencyMs: 5}, nil
		},
	}

	svc := NewChatService(manager, prompt, nil, nil, nil)
	if _, err := svc.ChatWithPrompt(context.Background(), nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid prompt chat request")
	}

	svcNoPrompt := NewChatService(manager, nil, nil, nil, nil)
	if _, err := svcNoPrompt.ChatWithPrompt(context.Background(), &PromptChatRequest{}); err == nil || !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected prompt service missing error")
	}

	if _, err := svc.ChatWithPrompt(context.Background(), &PromptChatRequest{PromptName: "missing", PromptScope: entity.PromptScopeGlobal}); err == nil || !errorx.Is(err, errorx.NotFound) {
		t.Fatalf("expected prompt not found error")
	}

	resp, err := svc.ChatWithPrompt(context.Background(), &PromptChatRequest{
		UserID:      42,
		PromptName:  "ok",
		PromptScope: entity.PromptScopeGlobal,
		ABTestID:    9,
		Messages:    []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("chat with prompt failed: %v", err)
	}
	if resp.Metadata["ab_variant"] != "B" || resp.Metadata["prompt_template_id"].(int64) != 33 {
		t.Fatalf("expected ab metadata in response, got %#v", resp.Metadata)
	}

	stream, err := svc.StreamChat(context.Background(), &ChatRequest{UserID: 1, Messages: []Message{{Content: "ok"}}})
	if err != nil {
		t.Fatalf("stream chat failed: %v", err)
	}
	var parts []string
	for ch := range stream {
		parts = append(parts, ch.Content)
	}
	if got := strings.Join(parts, ""); len(parts) != 3 || len(got) != 450 {
		t.Fatalf("expected 3 chunks and full content restored, chunks=%d len=%d", len(parts), len(got))
	}

	if out, err := svc.BatchChat(context.Background(), nil); err != nil || out != nil {
		t.Fatalf("empty batch should return nil,nil, got %#v %v", out, err)
	}

	_, err = svc.BatchChat(context.Background(), []*ChatRequest{
		{UserID: 1, Messages: []Message{{Content: "ok"}}},
		{UserID: 2, Messages: []Message{{Content: "fail"}}},
	})
	if err == nil {
		t.Fatalf("expected batch error when one request fails")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	outs, err := svc.BatchChat(ctx, []*ChatRequest{{UserID: 3, Messages: []Message{{Content: "ok"}}}})
	if err != nil || len(outs) != 1 {
		t.Fatalf("expected single batch success, outs=%#v err=%v", outs, err)
	}
}

func TestChatHelpers(t *testing.T) {
	msgs := []Message{{Role: "", Content: "a"}, {Role: "assistant", Content: "b"}}
	converted := convertMessages(msgs)
	if converted[0].Role != "user" || converted[1].Role != "assistant" {
		t.Fatalf("unexpected converted messages: %#v", converted)
	}

	joined := joinMessages([]Message{{Role: "u", Content: "x"}, {Role: "a", Content: "y"}})
	if joined != "u:x\na:y" {
		t.Fatalf("unexpected joined messages: %q", joined)
	}

	usage := estimateUsage("sys", []Message{{Content: "abcd"}}, "zzzz")
	if usage.TotalTokens <= 0 || usage.RequestTokens <= 0 || usage.ResponseTokens <= 0 {
		t.Fatalf("unexpected usage: %#v", usage)
	}

	if got := chunkContent("abc", 0); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("unexpected chunk for size<=0: %#v", got)
	}
	if got := chunkContent("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("unexpected chunk for empty string: %#v", got)
	}
}

func TestChatServiceErrorMetricsUseErrorCode(t *testing.T) {
	var saved []*entity.Metrics
	manager := &testChatManager{
		chatForUserFn: func(ctx context.Context, userID int64, req *client.ChatRequest) (*ChatExecution, error) {
			return &ChatExecution{Provider: "openai", Model: "gpt"}, errorx.New(errorx.TooManyRequests, "rate limited")
		},
	}
	metrics := &testMetricsRepo{saveFn: func(ctx context.Context, m *entity.Metrics) error {
		saved = append(saved, m)
		return nil
	}}

	svc := NewChatService(manager, nil, nil, metrics, nil)
	_, err := svc.Chat(context.Background(), &ChatRequest{UserID: 9, Messages: []Message{{Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected manager error")
	}
	if len(saved) != 1 {
		t.Fatalf("expected one metric saved, got %#v", saved)
	}
	if saved[0].ErrorType != string(errorx.TooManyRequests) {
		t.Fatalf("expected error type code, got %#v", saved[0])
	}
}
