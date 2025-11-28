package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errors"
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
	mu             sync.Mutex
	rateCounters   map[int64]*rateState
	rateLimitPerM  int
	rateLimitBurst int
}

func NewSafetyService(repo repo.SafetyPolicyRepo, audit repo.AuditLogRepo, rate repo.RateLimitRepo) SafetyService {
	return &safetyServiceImpl{
		repo:           repo,
		auditRepo:      audit,
		rateRepo:       rate,
		rateCounters:   make(map[int64]*rateState),
		rateLimitPerM:  60,
		rateLimitBurst: 30,
	}
}

type rateState struct {
	start      time.Time
	count      int
	tokens     float64
	lastRefill time.Time
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
	allowed, retryAfter := s.takeUserToken(userID, now)
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
		}, errors.NewError(errors.ErrCodeValidation, msg)
	}

	return &RateLimitResult{Allowed: true}, nil
}

func (s *safetyServiceImpl) RecordAuditLog(ctx context.Context, log *entity.AuditLog) error {
	if log == nil {
		return errors.NewError(errors.ErrCodeInvalidInput, "audit log 不能为空")
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
		return &SafetyResult{Allowed: false, Reason: "pii_detected"}, errors.NewError(errors.ErrCodeValidation, "内容包含敏感信息")
	}
	return &SafetyResult{Allowed: true}, nil
}

func (s *safetyServiceImpl) MaskPII(ctx context.Context, content string) (string, error) {
	piiRegex := regexp.MustCompile(`(?i)([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\d{3,4}[- ]?\d{6,8})`)
	masked := piiRegex.ReplaceAllString(content, "[PII]")
	return masked, nil
}

func (s *safetyServiceImpl) takeUserToken(userID int64, now time.Time) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensureRateState(userID, now)

	capacity := float64(s.rateLimitPerM + s.rateLimitBurst)
	if capacity <= 0 {
		capacity = float64(s.rateLimitPerM)
	}
	refillPerSec := float64(s.rateLimitPerM) / 60.0

	if now.Sub(state.start) >= time.Minute {
		state.start = now.Truncate(time.Minute)
		state.count = 0
	}

	if refillPerSec > 0 {
		elapsed := now.Sub(state.lastRefill).Seconds()
		if elapsed > 0 {
			state.tokens += elapsed * refillPerSec
			if state.tokens > capacity {
				state.tokens = capacity
			}
			state.lastRefill = now
		}
	}

	if state.tokens >= 1 {
		state.tokens -= 1
		state.count++
		return true, 0
	}

	retryAfter := 1
	if refillPerSec > 0 {
		need := 1 - state.tokens
		if need < 0 {
			need = 0
		}
		retryAfter = int(math.Ceil(need / refillPerSec))
		if retryAfter < 1 {
			retryAfter = 1
		}
	}
	return false, retryAfter
}

func (s *safetyServiceImpl) ensureRateState(userID int64, now time.Time) *rateState {
	state, ok := s.rateCounters[userID]
	if !ok {
		capacity := float64(s.rateLimitPerM + s.rateLimitBurst)
		if capacity <= 0 {
			capacity = float64(s.rateLimitPerM)
		}
		state = &rateState{
			start:      now.Truncate(time.Minute),
			tokens:     capacity,
			lastRefill: now,
		}
		s.rateCounters[userID] = state
	}
	return state
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
			}, errors.NewError(errors.ErrCodeValidation, "内容命中敏感词")
		}
	}
	return &SafetyResult{Allowed: true}, nil
}
