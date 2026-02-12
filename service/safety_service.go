package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/clock"
	"gochen/errorx"
	"gochen/policy/ratelimit"
)

// SafetyService 聚合安全与审计能力（首版提供关键词过滤与系统安全提示）
type SafetyService interface {
	GetActivePolicy(ctx context.Context) (*entity.SafetyPolicy, error)
	BuildSystemPrompt(ctx context.Context) (string, error)
	ValidateInput(ctx context.Context, input string) (*SafetyResult, error)
	ValidateOutput(ctx context.Context, output string) (*SafetyResult, error)
	FilterContent(ctx context.Context, content string) (string, error)
	CheckRateLimit(ctx context.Context, userID int64) (*RateLimitResult, error)
	RecordAuditLog(ctx context.Context, log *entity.AuditLog) error
	DetectPII(ctx context.Context, content string) (*SafetyResult, error)
	MaskPII(ctx context.Context, content string) (string, error)
	GetRateLimitSettings() RateLimitSettings
}

type safetyServiceImpl struct {
	repo           repo.SafetyPolicyRepo
	auditRepo      repo.AuditLogRepo
	rateRepo       repo.RateLimitRepo
	rateLimitPerM  int
	rateLimitBurst int
	rateLimiter    *ratelimit.Limiter
}

func NewSafetyService(repo repo.SafetyPolicyRepo, audit repo.AuditLogRepo, rate repo.RateLimitRepo) SafetyService {
	svc := &safetyServiceImpl{
		repo:           repo,
		auditRepo:      audit,
		rateRepo:       rate,
		rateLimitPerM:  60,
		rateLimitBurst: 30,
	}
	svc.initRateLimiter()
	return svc
}

func (s *safetyServiceImpl) initRateLimiter() {
	if s.rateLimitPerM <= 0 {
		return
	}
	burst := s.rateLimitPerM + s.rateLimitBurst
	if burst <= 0 {
		burst = s.rateLimitPerM
	}

	baseClock := clock.NewRealClock()
	cleanupWindow := 10 * time.Minute

	if s.rateLimitPerM%60 == 0 {
		s.rateLimiter = ratelimit.New(ratelimit.Config{
			RequestsPerSecond: s.rateLimitPerM / 60,
			BurstSize:         burst,
			WindowSize:        cleanupWindow,
			Clock:             baseClock,
		})
		return
	}

	scaleFactor := 60.0
	scaledClock := newScaledClock(baseClock, scaleFactor)
	scaledCleanupWindow := time.Duration(float64(cleanupWindow) / scaleFactor)
	if scaledCleanupWindow <= 0 {
		scaledCleanupWindow = time.Second
	}

	s.rateLimiter = ratelimit.New(ratelimit.Config{
		RequestsPerSecond: s.rateLimitPerM,
		BurstSize:         burst,
		WindowSize:        scaledCleanupWindow,
		Clock:             scaledClock,
	})
}

func (s *safetyServiceImpl) GetActivePolicy(ctx context.Context) (*entity.SafetyPolicy, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.GetActive(ctx)
}

func (s *safetyServiceImpl) BuildSystemPrompt(ctx context.Context) (string, error) {
	policy, err := s.GetActivePolicy(ctx)
	if err != nil || policy == nil || !policy.Enabled {
		return "", err
	}
	return strings.TrimSpace(policy.GlobalSystemPrompt), nil
}

func (s *safetyServiceImpl) GetRateLimitSettings() RateLimitSettings {
	return RateLimitSettings{
		PerMinute: s.rateLimitPerM,
		Burst:     s.rateLimitBurst,
	}
}

func (s *safetyServiceImpl) ValidateInput(ctx context.Context, input string) (*SafetyResult, error) {
	return s.validateText(ctx, input)
}

func (s *safetyServiceImpl) ValidateOutput(ctx context.Context, output string) (*SafetyResult, error) {
	return s.validateText(ctx, output)
}

func (s *safetyServiceImpl) FilterContent(ctx context.Context, content string) (string, error) {
	res, err := s.validateText(ctx, content)
	if err != nil || res == nil || res.Allowed {
		return content, err
	}
	return "内容涉及不适宜主题，已被过滤。", nil
}

func (s *safetyServiceImpl) CheckRateLimit(ctx context.Context, userID int64) (*RateLimitResult, error) {
	if userID <= 0 {
		return &RateLimitResult{Allowed: true}, nil
	}

	if s.rateLimitPerM <= 0 {
		return &RateLimitResult{Allowed: true}, nil
	}

	now := time.Now()
	allowed, retryAfter := s.allowUser(userID)
	windowStart := now.Truncate(time.Minute)

	if s.rateRepo != nil {
		state, err := s.rateRepo.Increment(ctx, userID, "chat", windowStart, 60, 1, 0)
		if err != nil {
			return nil, err
		}
		// DB 计数作为兜底，超过 (perMin+burst) 视为超限
		if state != nil {
			limitCap := s.rateLimitPerM + s.rateLimitBurst
			if limitCap <= 0 {
				limitCap = s.rateLimitPerM
			}
			if state.RequestCount > limitCap {
				allowed = false
			}
		}
	}

	if !allowed {
		msg := "请求过于频繁，请稍后再试"
		if retryAfter > 0 {
			msg = fmt.Sprintf("请求过于频繁，请在 %d 秒后再试", retryAfter)
		}
		return &RateLimitResult{
			Allowed: false,
			Reason:  "rate_limited",
		}, errorx.New(errorx.Validation, msg)
	}

	return &RateLimitResult{Allowed: true}, nil
}

func (s *safetyServiceImpl) RecordAuditLog(ctx context.Context, log *entity.AuditLog) error {
	if log == nil {
		return errorx.New(errorx.InvalidInput, "audit log 不能为空")
	}
	if s.auditRepo == nil {
		// 兜底：无持久化时不阻断主流程
		return nil
	}
	return s.auditRepo.Save(ctx, log)
}

func (s *safetyServiceImpl) DetectPII(ctx context.Context, content string) (*SafetyResult, error) {
	piiRegex := regexp.MustCompile(`(?i)([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\d{3,4}[- ]?\d{6,8})`)
	if piiRegex.MatchString(content) {
		return &SafetyResult{Allowed: false, Reason: "pii_detected"}, errorx.New(errorx.Validation, "内容包含敏感信息")
	}
	return &SafetyResult{Allowed: true}, nil
}

func (s *safetyServiceImpl) MaskPII(ctx context.Context, content string) (string, error) {
	piiRegex := regexp.MustCompile(`(?i)([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\d{3,4}[- ]?\d{6,8})`)
	masked := piiRegex.ReplaceAllString(content, "[PII]")
	return masked, nil
}

func (s *safetyServiceImpl) allowUser(userID int64) (bool, int) {
	if s.rateLimiter == nil {
		return true, 0
	}
	key := fmt.Sprintf("%d", userID)
	if s.rateLimiter.Allow(key) {
		return true, 0
	}
	return false, s.estimateRetryAfter()
}

func (s *safetyServiceImpl) estimateRetryAfter() int {
	if s.rateLimitPerM <= 0 {
		return 1
	}
	refillPerSec := float64(s.rateLimitPerM) / 60.0
	if refillPerSec <= 0 {
		return 1
	}
	retryAfter := int(math.Ceil(1.0 / refillPerSec))
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter
}

func (s *safetyServiceImpl) validateText(ctx context.Context, text string) (*SafetyResult, error) {
	policy, err := s.GetActivePolicy(ctx)
	if err != nil || policy == nil || !policy.Enabled {
		return &SafetyResult{Allowed: true}, err
	}

	var kws []string
	if strings.TrimSpace(policy.BlockedKeywordsJSON) != "" {
		_ = json.Unmarshal([]byte(policy.BlockedKeywordsJSON), &kws)
	}

	lower := strings.ToLower(text)
	for _, kw := range kws {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return &SafetyResult{
				Allowed: false,
				Reason:  "命中敏感词",
			}, errorx.New(errorx.Validation, "内容命中敏感词")
		}
	}
	return &SafetyResult{Allowed: true}, nil
}

type scaledClock struct {
	base   clock.Clock
	factor float64
	origin time.Time
}

func newScaledClock(base clock.Clock, factor float64) clock.Clock {
	if base == nil {
		base = clock.NewRealClock()
	}
	if factor <= 0 {
		factor = 1
	}
	return &scaledClock{
		base:   base,
		factor: factor,
		origin: base.Now(),
	}
}

func (c *scaledClock) Now() time.Time {
	if c == nil || c.base == nil {
		return time.Now()
	}
	now := c.base.Now()
	elapsed := now.Sub(c.origin)
	scaled := time.Duration(float64(elapsed) / c.factor)
	return c.origin.Add(scaled)
}

func (c *scaledClock) NewTimer(d time.Duration) clock.Timer {
	if c == nil || c.base == nil {
		return clock.NewRealClock().NewTimer(d)
	}
	return c.base.NewTimer(time.Duration(float64(d) * c.factor))
}

func (c *scaledClock) NewTicker(d time.Duration) clock.Ticker {
	if c == nil || c.base == nil {
		return clock.NewRealClock().NewTicker(d)
	}
	return c.base.NewTicker(time.Duration(float64(d) * c.factor))
}
