package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
)

type testSafetyPolicyRepo struct {
	getActiveFn func(ctx context.Context) (*entity.SafetyPolicy, error)
	saveFn      func(ctx context.Context, policy *entity.SafetyPolicy) error
}

func (r *testSafetyPolicyRepo) GetActive(ctx context.Context) (*entity.SafetyPolicy, error) {
	if r.getActiveFn != nil {
		return r.getActiveFn(ctx)
	}
	return nil, nil
}

func (r *testSafetyPolicyRepo) Save(ctx context.Context, policy *entity.SafetyPolicy) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, policy)
	}
	return nil
}

type testAuditLogRepo struct {
	saveFn func(ctx context.Context, log *entity.AuditLog) error
}

func (r *testAuditLogRepo) Save(ctx context.Context, log *entity.AuditLog) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, log)
	}
	return nil
}

func (r *testAuditLogRepo) List(ctx context.Context, filter repo.AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error) {
	_ = ctx
	_ = filter
	_ = limit
	_ = offset
	return nil, 0, nil
}

type testRateLimitRepo struct {
	incrementFn func(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error)
}

func (r *testRateLimitRepo) Increment(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error) {
	if r.incrementFn != nil {
		return r.incrementFn(ctx, userID, resourceType, windowStart, windowSizeSeconds, deltaReq, deltaTokens)
	}
	return &entity.RateLimit{RequestCount: 1}, nil
}

func (r *testRateLimitRepo) ListRecent(ctx context.Context, resourceType string, limit int) ([]*entity.RateLimit, error) {
	_ = ctx
	_ = resourceType
	_ = limit
	return nil, nil
}

func (r *testRateLimitRepo) SumSince(ctx context.Context, resourceType string, since time.Time) (int64, error) {
	_ = ctx
	_ = resourceType
	_ = since
	return 0, nil
}

func TestSafetyServicePolicyAndPII(t *testing.T) {
	repo := &testSafetyPolicyRepo{
		getActiveFn: func(ctx context.Context) (*entity.SafetyPolicy, error) {
			return &entity.SafetyPolicy{
				Enabled:             true,
				GlobalSystemPrompt:  "  safe policy  ",
				BlockedKeywordsJSON: `["hack", "暴力"]`,
			}, nil
		},
	}

	svc := NewSafetyService(repo, nil, nil)

	prompt, err := svc.BuildSystemPrompt(context.Background())
	if err != nil {
		t.Fatalf("build system prompt failed: %v", err)
	}
	if prompt != "safe policy" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}

	res, err := svc.ValidateInput(context.Background(), "please hack now")
	if err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected validation error for blocked keyword, got %v", err)
	}
	if res == nil || res.Allowed {
		t.Fatalf("expected blocked safety result, got %#v", res)
	}

	filtered, err := svc.FilterContent(context.Background(), "这包含暴力内容")
	if err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected blocked content returns validation error, got %v", err)
	}
	if filtered != "这包含暴力内容" {
		t.Fatalf("blocked content should remain original when validation returns error, got %q", filtered)
	}

	resOut, err := svc.ValidateOutput(context.Background(), "safe output")
	if err != nil || resOut == nil || !resOut.Allowed {
		t.Fatalf("expected safe output result, got %#v err=%v", resOut, err)
	}

	piiRes, err := svc.DetectPII(context.Background(), "mail: a@test.com")
	if err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected pii validation error, got %v", err)
	}
	if piiRes == nil || piiRes.Allowed {
		t.Fatalf("expected pii blocked result, got %#v", piiRes)
	}

	masked, err := svc.MaskPII(context.Background(), "phone 010-12345678")
	if err != nil {
		t.Fatalf("mask pii failed: %v", err)
	}
	if !strings.Contains(masked, "[PII]") {
		t.Fatalf("expected pii marker in masked text, got %q", masked)
	}
}

func TestSafetyServiceRateLimitAndAudit(t *testing.T) {
	baseSvc := NewSafetyService(nil, nil, nil)
	impl, ok := baseSvc.(*safetyServiceImpl)
	if !ok {
		t.Fatalf("unexpected safety service type")
	}
	impl.rateLimitPerM = 1
	impl.rateLimitBurst = 0
	impl.initRateLimiter()

	if res, err := baseSvc.CheckRateLimit(context.Background(), 0); err != nil || !res.Allowed {
		t.Fatalf("user 0 should bypass rate limit, res=%#v err=%v", res, err)
	}

	if res, err := baseSvc.CheckRateLimit(context.Background(), 101); err != nil || !res.Allowed {
		t.Fatalf("first request should pass, res=%#v err=%v", res, err)
	}

	res, err := baseSvc.CheckRateLimit(context.Background(), 101)
	if err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected rate limited error on second request, got %v", err)
	}
	if res == nil || res.Allowed || res.Reason != "rate_limited" {
		t.Fatalf("unexpected rate limit result: %#v", res)
	}

	impl.rateLimitPerM = 0
	impl.initRateLimiter()
	if res, err := baseSvc.CheckRateLimit(context.Background(), 101); err != nil || !res.Allowed {
		t.Fatalf("rate limit disabled should allow, res=%#v err=%v", res, err)
	}

	dbLimitSvc := NewSafetyService(nil, nil, &testRateLimitRepo{
		incrementFn: func(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error) {
			return &entity.RateLimit{RequestCount: 999}, nil
		},
	})
	if _, err := dbLimitSvc.CheckRateLimit(context.Background(), 1); err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected db fallback rate limit error, got %v", err)
	}

	repoErrSvc := NewSafetyService(nil, nil, &testRateLimitRepo{
		incrementFn: func(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error) {
			return nil, errors.New("db down")
		},
	})
	if _, err := repoErrSvc.CheckRateLimit(context.Background(), 1); err == nil {
		t.Fatalf("expected increment error propagated")
	}

	var saved *entity.AuditLog
	auditSvc := NewSafetyService(nil, &testAuditLogRepo{
		saveFn: func(ctx context.Context, log *entity.AuditLog) error {
			saved = log
			return nil
		},
	}, nil)
	if err := auditSvc.RecordAuditLog(context.Background(), nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid audit log error, got %v", err)
	}
	if err := auditSvc.RecordAuditLog(context.Background(), &entity.AuditLog{Action: "llm.chat"}); err != nil {
		t.Fatalf("record audit log failed: %v", err)
	}
	if saved == nil || saved.Action != "llm.chat" {
		t.Fatalf("expected audit log persisted")
	}

	if err := baseSvc.RecordAuditLog(context.Background(), &entity.AuditLog{Action: "noop"}); err != nil {
		t.Fatalf("record audit without repo should not fail: %v", err)
	}

	settings := baseSvc.GetRateLimitSettings()
	if settings.PerMinute != impl.rateLimitPerM || settings.Burst != impl.rateLimitBurst {
		t.Fatalf("unexpected rate limit settings: %#v", settings)
	}
}
