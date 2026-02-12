package service

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gochen-llm/client"
	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
	"gochen/logging"
	"gochen/policy/retry"
	runtime "gochen/task"
)

// ProviderManager 抽象多源 LLM 管理器，负责端点选择与简单故障切换。
type ProviderManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	ChatForUser(ctx context.Context, userID int64, req *client.ChatRequest) (*client.ChatResponse, string, string, int64, float64, float64, error)
	Reload(ctx context.Context) error
	ListEffectiveConfigs(ctx context.Context) ([]*entity.ProviderConfig, error)
	ReplaceConfigs(ctx context.Context, configs []*entity.ProviderConfig) error
	ListStatus(ctx context.Context) ([]*EndpointStatus, error)
}

type endpointState struct {
	cfg           *entity.ProviderConfig
	client        client.Client
	cooldownUntil int64 // UnixNano，原子访问；0 表示无冷却
	// 健康与熔断
	healthFailedStreak  uint32
	healthSuccessStreak uint32
	inCircuitOpen       uint32 // 0/1
	lastPingAt          int64  // UnixNano
	healthMu            sync.Mutex
	healthHistory       []healthSample

	// 运行时限流（令牌桶 + 窗口计数）
	rateWindowStart int64
	rateCount       int64
	rateMu          sync.Mutex
	rateTokens      float64
	rateLastRefill  time.Time

	// 运行时统计数据
	stats endpointStats
}

type endpointStats struct {
	totalRequests uint64       // 总请求数
	failures      uint64       // 失败次数
	lastErrorAt   int64        // UnixNano
	lastLatencyMs int64        // 最近一次成功响应的耗时
	failureStreak uint32       // 连续失败次数，用于退避
	lastError     atomic.Value // string
}

type healthSample struct {
	Timestamp  time.Time
	Success    bool
	StatusCode int
	LatencyMs  int64
	Error      string
}

type providerManagerImpl struct {
	repo   repo.ProviderConfigRepo
	logger logging.ILogger
	super  *runtime.TaskSupervisor

	endpoints atomic.Value // []*endpointState
	pingEvery time.Duration

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	cancel      context.CancelFunc
}

func NewProviderManager(repo repo.ProviderConfigRepo, logger logging.ILogger) (ProviderManager, error) {
	m := &providerManagerImpl{
		repo:      repo,
		logger:    logger,
		super:     runtime.NewTaskSupervisor("gochen-llm.provider_manager"),
		pingEvery: 30 * time.Second,
	}
	return m, nil
}

func (m *providerManagerImpl) Start(ctx context.Context) error {
	if m == nil {
		return nil
	}

	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if m.stopped {
		return errorx.New(errorx.Internal, "LLM ProviderManager 已停止，无法再次启动")
	}
	if m.started {
		return nil
	}

	if ctx == nil {
		return errorx.New(errorx.InvalidInput, "ctx 不能为空")
	}
	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.started = true

	m.super.GoLoop(loopCtx, "health_loop", m.pingEvery, func(ctx context.Context) error {
		m.runHealthCheckOnce(ctx)
		return nil
	})

	return nil
}

func (m *providerManagerImpl) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}

	m.lifecycleMu.Lock()
	if !m.started || m.stopped {
		m.lifecycleMu.Unlock()
		return nil
	}
	m.stopped = true
	cancel := m.cancel
	m.lifecycleMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if m.super != nil {
		m.super.Stop()
	}
	if m.logger != nil {
		if ctx != nil {
			m.logger.Info(ctx, "[LLMProviderManager] stopped")
		}
	}
	if ctx == nil {
		return errorx.New(errorx.InvalidInput, "ctx 不能为空")
	}
	return nil
}

func (m *providerManagerImpl) ChatForUser(ctx context.Context, userID int64, req *client.ChatRequest) (*client.ChatResponse, string, string, int64, float64, float64, error) {
	if ctx == nil {
		return nil, "", "", 0, 0, 0, errorx.New(errorx.InvalidInput, "ctx 不能为空")
	}
	if req == nil {
		return nil, "", "", 0, 0, 0, errorx.New(errorx.InvalidInput, "LLM 请求不能为空")
	}

	eps, err := m.getOrLoadEndpoints(ctx)
	if err != nil {
		return nil, "", "", 0, 0, 0, err
	}
	if len(eps) == 0 {
		return nil, "", "", 0, 0, 0, errorx.New(errorx.Internal, "LLM 未配置")
	}

	now := time.Now()
	candidates := m.selectCandidates(eps, now)
	if len(candidates) == 0 {
		candidates = m.selectAllByMinPriority(eps)
	}
	if len(candidates) == 0 {
		return nil, "", "", 0, 0, 0, errorx.New(errorx.Internal, "没有可用的 LLM 端点")
	}

	var firstErr error
	startPos := m.chooseWeightedStart(eps, candidates, userID, now)

	for i := 0; i < len(candidates); i++ {
		idx := candidates[(startPos+i)%len(candidates)]
		ep := eps[idx]

		// 熔断检查
		if atomic.LoadUint32(&ep.inCircuitOpen) == 1 {
			// 定期尝试半开
			if time.Since(time.Unix(0, ep.lastPingAt)) < time.Duration(maxInt(ep.cfg.HealthTimeoutSeconds, 1))*time.Second {
				continue
			}
		}

		// 健康 ping（按配置 URL，避免频繁）
		if ep.cfg.HealthPingURL != "" && time.Since(time.Unix(0, ep.lastPingAt)) > time.Duration(maxInt(ep.cfg.HealthTimeoutSeconds, 1))*time.Second {
			atomic.StoreInt64(&ep.lastPingAt, time.Now().UnixNano())
			if err := m.pingEndpoint(ctx, ep); err != nil {
				continue
			}
		}

		// 令牌桶限流：平滑突发
		if ep.cfg.RateLimitPerMin > 0 {
			if !m.takeRateToken(ep, now) {
				continue
			}
			m.bumpRateWindow(ep, now)
		}

		start := time.Now()
		resp, err := ep.client.Chat(ctx, req)

		atomic.AddUint64(&ep.stats.totalRequests, 1)
		if err == nil {
			atomic.StoreUint32(&ep.stats.failureStreak, 0)
			latency := time.Since(start).Milliseconds()
			if latency < 0 {
				latency = 0
			}
			atomic.StoreInt64(&ep.stats.lastLatencyMs, latency)
			atomic.StoreInt64(&ep.lastPingAt, time.Now().UnixNano())
			if atomic.LoadUint32(&ep.inCircuitOpen) == 1 {
				// 半开成功计数
				atomic.AddUint32(&ep.healthSuccessStreak, 1)
				if int(atomic.LoadUint32(&ep.healthSuccessStreak)) >= maxInt(ep.cfg.RecoverySuccesses, 1) {
					atomic.StoreUint32(&ep.inCircuitOpen, 0)
					atomic.StoreUint32(&ep.healthFailedStreak, 0)
					atomic.StoreUint32(&ep.healthSuccessStreak, 0)
				}
			} else {
				atomic.StoreUint32(&ep.healthFailedStreak, 0)
			}
			return resp, ep.cfg.Provider, ep.cfg.Model, latency, ep.cfg.InputPricePer1k, ep.cfg.OutputPricePer1k, nil
		}

		atomic.AddUint64(&ep.stats.failures, 1)
		atomic.StoreInt64(&ep.stats.lastErrorAt, time.Now().UnixNano())
		ep.stats.lastError.Store(err.Error())
		atomic.StoreUint32(&ep.healthSuccessStreak, 0)
		failStreak := atomic.AddUint32(&ep.healthFailedStreak, 1)
		if int(failStreak) >= maxInt(ep.cfg.MaxErrorStreak, 1) {
			atomic.StoreUint32(&ep.inCircuitOpen, 1)
		}

		if firstErr == nil {
			firstErr = err
		}

		base := ep.cfg.CooldownSeconds
		if base <= 0 {
			base = 30
		}
		streak := atomic.AddUint32(&ep.stats.failureStreak, 1)
		if streak == 0 {
			streak = 1
		}
		factorExp := streak - 1
		if factorExp > 3 {
			factorExp = 3
		}
		factor := 1 << factorExp
		cd := time.Duration(base*factor) * time.Second
		maxCooldown := 5 * time.Minute
		if cd > maxCooldown {
			cd = maxCooldown
		}

		atomic.StoreInt64(&ep.cooldownUntil, time.Now().Add(cd).UnixNano())
		if m.logger != nil {
			m.logger.Warn(ctx, "[LLMProviderManager] 端点失败进入冷却",
				logging.String("name", ep.cfg.Name),
				logging.String("provider", ep.cfg.Provider),
				logging.String("cooldown", cd.String()),
				logging.Error(err),
			)
		}
	}

	if firstErr == nil {
		return nil, "", "", 0, 0, 0, errorx.New(errorx.Internal, "LLM 调用失败但未返回具体错误")
	}
	return nil, "", "", 0, 0, 0, errorx.Wrap(firstErr, errorx.Internal, "所有 LLM 端点调用失败")
}

func (m *providerManagerImpl) pingEndpoint(ctx context.Context, ep *endpointState) error {
	if ep == nil || ep.cfg == nil {
		return errorx.New(errorx.Internal, "端点未初始化")
	}
	timeout := time.Duration(maxInt(ep.cfg.HealthTimeoutSeconds, 1)) * time.Second
	client := &http.Client{Timeout: timeout}

	attempts := maxInt(ep.cfg.RecoverySuccesses, 1)
	if attempts < 2 {
		attempts = 2
	}
	if attempts > 5 {
		attempts = 5
	}

	var lastErr error
	var statusCode int
	var latencyMs int64
	retryCfg := retry.Config{
		MaxAttempts:   attempts,
		InitialDelay:  150 * time.Millisecond,
		BackoffFactor: 1.5,
		MaxDelay:      600 * time.Millisecond,
		JitterRatio:   0,
	}
	err := retry.DoWithInfo(ctx, func(ctx context.Context, attempt int) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.cfg.HealthPingURL, nil)
		if err != nil {
			lastErr = err
			return err
		}
		resp, err := client.Do(req)
		latencyMs = time.Since(start).Milliseconds()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp != nil {
			statusCode = resp.StatusCode
		}

		if err == nil && resp != nil && resp.StatusCode < 400 {
			lastErr = nil
			return nil
		}

		if err != nil {
			lastErr = err
		} else if resp != nil {
			lastErr = fmt.Errorf("status=%d", resp.StatusCode)
		} else {
			lastErr = errorx.New(errorx.Internal, "未知健康探测错误")
		}
		return lastErr
	}, retryCfg)

	if err == nil {
		m.recordHealthSample(ep, healthSample{
			Timestamp:  time.Now(),
			Success:    true,
			StatusCode: statusCode,
			LatencyMs:  latencyMs,
		})
		// ping 成功，尝试恢复熔断状态
		atomic.StoreUint32(&ep.healthFailedStreak, 0)
		atomic.StoreUint32(&ep.healthSuccessStreak, 0)
		atomic.StoreUint32(&ep.inCircuitOpen, 0)
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastErr == nil {
		lastErr = err
	}

	atomic.StoreUint32(&ep.healthSuccessStreak, 0)
	failStreak := atomic.AddUint32(&ep.healthFailedStreak, 1)
	if int(failStreak) >= maxInt(ep.cfg.MaxErrorStreak, 1) {
		atomic.StoreUint32(&ep.inCircuitOpen, 1)
	}
	m.recordHealthSample(ep, healthSample{
		Timestamp: time.Now(),
		Success:   false,
		Error:     errToString(lastErr),
	})
	if m.logger != nil {
		m.logger.Warn(ctx, "[LLMProviderManager] 健康探测失败",
			logging.String("name", ep.cfg.Name),
			logging.String("provider", ep.cfg.Provider),
			logging.Int("attempts", attempts),
			logging.Int("fail_streak", int(failStreak)),
			logging.Error(lastErr),
		)
	}
	return errorx.New(errorx.Internal, "health ping failed")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// takeRateToken 使用令牌桶平滑限流，支持突发余量
func (m *providerManagerImpl) takeRateToken(ep *endpointState, now time.Time) bool {
	if ep == nil || ep.cfg == nil {
		return false
	}
	perMin := ep.cfg.RateLimitPerMin
	if perMin <= 0 {
		return true
	}
	burst := ep.cfg.RateLimitBurst
	capacity := float64(perMin + burst)
	if capacity <= 0 {
		capacity = float64(perMin)
	}
	refillPerSec := float64(perMin) / 60.0

	ep.rateMu.Lock()
	defer ep.rateMu.Unlock()

	if ep.rateLastRefill.IsZero() {
		ep.rateLastRefill = now
		if ep.rateTokens == 0 {
			ep.rateTokens = capacity
		}
	}

	if refillPerSec > 0 {
		elapsed := now.Sub(ep.rateLastRefill).Seconds()
		if elapsed > 0 {
			ep.rateTokens += elapsed * refillPerSec
			if ep.rateTokens > capacity {
				ep.rateTokens = capacity
			}
			ep.rateLastRefill = now
		}
	}

	if ep.rateTokens >= 1 {
		ep.rateTokens -= 1
		return true
	}
	return false
}

// bumpRateWindow 保留原分钟窗口计数，便于状态看板
func (m *providerManagerImpl) bumpRateWindow(ep *endpointState, now time.Time) {
	if ep == nil {
		return
	}
	nowMin := now.Unix() / 60
	if atomic.LoadInt64(&ep.rateWindowStart) != nowMin {
		atomic.StoreInt64(&ep.rateWindowStart, nowMin)
		atomic.StoreInt64(&ep.rateCount, 0)
	}
	atomic.AddInt64(&ep.rateCount, 1)
}

func (m *providerManagerImpl) recordHealthSample(ep *endpointState, sample healthSample) {
	if ep == nil {
		return
	}
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now()
	}
	ep.healthMu.Lock()
	defer ep.healthMu.Unlock()

	ep.healthHistory = append(ep.healthHistory, sample)
	if len(ep.healthHistory) > 10 {
		ep.healthHistory = ep.healthHistory[len(ep.healthHistory)-10:]
	}
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func formatTimeUTC(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(0, ts).UTC().Format(time.RFC3339)
}

// runHealthCheckOnce 对具备 HealthPingURL 的端点做一次 ping，更新健康状态。
func (m *providerManagerImpl) runHealthCheckOnce(ctx context.Context) {
	if m == nil {
		return
	}
	if ctx == nil {
		panic("providerManagerImpl.runHealthCheckOnce: ctx is nil")
	}

	eps, err := m.getOrLoadEndpoints(ctx)
	if err != nil || len(eps) == 0 {
		return
	}
	for _, ep := range eps {
		if ep == nil || ep.cfg == nil || ep.cfg.HealthPingURL == "" {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, time.Duration(maxInt(ep.cfg.HealthTimeoutSeconds, 1))*time.Second)
		atomic.StoreInt64(&ep.lastPingAt, time.Now().UnixNano())
		_ = m.pingEndpoint(pctx, ep)
		cancel()
	}
}

func (m *providerManagerImpl) Reload(ctx context.Context) error {
	eps, err := m.loadEndpoints(ctx)
	if err != nil {
		return err
	}
	m.endpoints.Store(eps)
	if m.logger != nil {
		if len(eps) == 0 {
			m.logger.Warn(ctx, "[LLMProviderManager] Reload 后没有任何 LLM 端点配置（将回退到环境变量配置）")
		} else {
			m.logger.Info(ctx, "[LLMProviderManager] LLM 端点已重载",
				logging.Int("count", len(eps)),
			)
		}
	}
	return nil
}

func (m *providerManagerImpl) ListEffectiveConfigs(ctx context.Context) ([]*entity.ProviderConfig, error) {
	eps, err := m.getOrLoadEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*entity.ProviderConfig, 0, len(eps))
	for _, ep := range eps {
		cfg := *ep.cfg
		if len(cfg.APIKey) > 4 {
			cfg.APIKey = "****" + cfg.APIKey[len(cfg.APIKey)-4:]
		} else if cfg.APIKey != "" {
			cfg.APIKey = "****"
		}
		result = append(result, &cfg)
	}
	return result, nil
}

// EndpointStatus 用于对外暴露端点状态与统计信息（仅运维查看）
type EndpointStatus struct {
	Name                  string             `json:"name"`
	Provider              string             `json:"provider"`
	Model                 string             `json:"model"`
	Enabled               bool               `json:"enabled"`
	Priority              int                `json:"priority"`
	Weight                int                `json:"weight"`
	CooldownSeconds       int                `json:"cooldown_seconds"`
	InCooldown            bool               `json:"in_cooldown"`
	CooldownRemainingSecs int64              `json:"cooldown_remaining_seconds"`
	TotalRequests         uint64             `json:"total_requests"`
	Failures              uint64             `json:"failures"`
	SuccessRate           float64            `json:"success_rate"`
	LastLatencyMs         int64              `json:"last_latency_ms"`
	LastErrorAt           string             `json:"last_error_at,omitempty"`
	LastError             string             `json:"last_error,omitempty"`
	InCircuitOpen         bool               `json:"in_circuit_open"`
	HealthFailedStreak    int                `json:"health_failed_streak"`
	HealthSuccessStreak   int                `json:"health_success_streak"`
	LastPingAt            string             `json:"last_ping_at,omitempty"`
	HealthScore           float64            `json:"health_score"`
	HealthHistory         []HealthSampleView `json:"health_history,omitempty"`
	RateWindowStart       int64              `json:"rate_window_start"`
	RateWindowCount       int64              `json:"rate_window_count"`
	RateLimitPerMin       int                `json:"rate_limit_per_min"`
	RateLimitBurst        int                `json:"rate_limit_burst"`
	RateTokensRemaining   float64            `json:"rate_tokens_remaining"`
	RateBucketCapacity    float64            `json:"rate_bucket_capacity"`
	RateRefillPerSec      float64            `json:"rate_refill_per_sec"`
}

type HealthSampleView struct {
	At         string `json:"at"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (m *providerManagerImpl) ListStatus(ctx context.Context) ([]*EndpointStatus, error) {
	eps, err := m.getOrLoadEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	result := make([]*EndpointStatus, 0, len(eps))
	for _, ep := range eps {
		if ep == nil || ep.cfg == nil {
			continue
		}
		cfg := ep.cfg
		stats := &ep.stats

		total := atomic.LoadUint64(&stats.totalRequests)
		failures := atomic.LoadUint64(&stats.failures)
		lastLatency := atomic.LoadInt64(&stats.lastLatencyMs)
		lastErrAt := atomic.LoadInt64(&stats.lastErrorAt)
		lastErr, _ := stats.lastError.Load().(string)

		var successRate float64
		if total > 0 {
			successRate = float64(total-failures) / float64(total)
		}

		cdUntil := atomic.LoadInt64(&ep.cooldownUntil)
		inCooldown := false
		var remainSecs int64
		if cdUntil > 0 {
			cdTime := time.Unix(0, cdUntil)
			if now.Before(cdTime) {
				inCooldown = true
				diff := cdTime.Sub(now)
				if diff < 0 {
					diff = 0
				}
				remainSecs = int64(diff.Seconds())
			}
		}

		inCircuit := atomic.LoadUint32(&ep.inCircuitOpen) == 1
		healthStreak := atomic.LoadUint32(&ep.healthFailedStreak)
		healthSuccess := atomic.LoadUint32(&ep.healthSuccessStreak)
		rateStart := atomic.LoadInt64(&ep.rateWindowStart)
		rateCount := atomic.LoadInt64(&ep.rateCount)
		lastPing := atomic.LoadInt64(&ep.lastPingAt)

		// 复制健康历史，避免锁持有过久
		var history []HealthSampleView
		var healthScore float64
		ep.healthMu.Lock()
		if len(ep.healthHistory) > 0 {
			history = make([]HealthSampleView, 0, len(ep.healthHistory))
			success := 0
			for _, h := range ep.healthHistory {
				if h.Success {
					success++
				}
				history = append(history, HealthSampleView{
					At:         h.Timestamp.UTC().Format(time.RFC3339),
					Success:    h.Success,
					StatusCode: h.StatusCode,
					LatencyMs:  h.LatencyMs,
					Error:      h.Error,
				})
			}
			healthScore = float64(success) / float64(len(ep.healthHistory))
		}
		ep.healthMu.Unlock()

		var rateTokens float64
		var rateCapacity float64
		var rateRefillPerSec float64
		ep.rateMu.Lock()
		rateTokens = ep.rateTokens
		rateCapacity = float64(cfg.RateLimitPerMin + cfg.RateLimitBurst)
		if rateCapacity <= 0 {
			rateCapacity = float64(cfg.RateLimitPerMin)
		}
		rateRefillPerSec = float64(cfg.RateLimitPerMin) / 60.0
		ep.rateMu.Unlock()

		status := &EndpointStatus{
			Name:                  cfg.Name,
			Provider:              cfg.Provider,
			Model:                 cfg.Model,
			Enabled:               cfg.Enabled,
			Priority:              cfg.Priority,
			Weight:                cfg.Weight,
			CooldownSeconds:       cfg.CooldownSeconds,
			InCooldown:            inCooldown,
			CooldownRemainingSecs: remainSecs,
			TotalRequests:         total,
			Failures:              failures,
			SuccessRate:           successRate,
			LastLatencyMs:         lastLatency,
			InCircuitOpen:         inCircuit,
			HealthFailedStreak:    int(healthStreak),
			HealthSuccessStreak:   int(healthSuccess),
			LastPingAt:            formatTimeUTC(lastPing),
			HealthScore:           healthScore,
			HealthHistory:         history,
			RateWindowStart:       rateStart,
			RateWindowCount:       rateCount,
			RateLimitPerMin:       cfg.RateLimitPerMin,
			RateLimitBurst:        cfg.RateLimitBurst,
			RateTokensRemaining:   rateTokens,
			RateBucketCapacity:    rateCapacity,
			RateRefillPerSec:      rateRefillPerSec,
		}

		if lastErrAt > 0 {
			status.LastErrorAt = time.Unix(0, lastErrAt).UTC().Format(time.RFC3339)
		}
		if lastErr != "" {
			status.LastError = lastErr
		}

		result = append(result, status)
	}

	return result, nil
}

func (m *providerManagerImpl) ReplaceConfigs(ctx context.Context, configs []*entity.ProviderConfig) error {
	for _, cfg := range configs {
		if cfg.Priority == 0 {
			cfg.Priority = 100
		}
		if cfg.TimeoutSeconds <= 0 {
			cfg.TimeoutSeconds = 30
		}
		if cfg.CooldownSeconds <= 0 {
			cfg.CooldownSeconds = 30
		}
		if cfg.Weight <= 0 {
			cfg.Weight = 100
		}
		if cfg.Name == "" {
			cfg.Name = cfg.Provider
		}
		if cfg.InputPricePer1k < 0 || cfg.OutputPricePer1k < 0 {
			return errorx.New(errorx.Validation, "LLM 单价不能为负数")
		}
		if cfg.InputPricePer1k > 100 || cfg.OutputPricePer1k > 100 {
			return errorx.New(errorx.Validation, "LLM 单价疑似异常（>100 USD/1k tokens）")
		}
	}
	if err := m.repo.ReplaceAll(ctx, configs); err != nil {
		return err
	}
	return nil
}

func (m *providerManagerImpl) getOrLoadEndpoints(ctx context.Context) ([]*endpointState, error) {
	if v := m.endpoints.Load(); v != nil {
		if eps, ok := v.([]*endpointState); ok {
			return eps, nil
		}
	}
	if err := m.Reload(ctx); err != nil {
		return nil, err
	}
	v := m.endpoints.Load()
	if v == nil {
		return nil, nil
	}
	eps, _ := v.([]*endpointState)
	return eps, nil
}

func (m *providerManagerImpl) loadEndpoints(ctx context.Context) ([]*endpointState, error) {
	var cfgs []*entity.ProviderConfig
	var err error

	if m.repo != nil {
		cfgs, err = m.repo.ListAll(ctx)
		if err != nil {
			return nil, err
		}
	}

	eps := make([]*endpointState, 0, len(cfgs))
	for _, c := range cfgs {
		if c == nil || !c.Enabled {
			continue
		}
		timeout := time.Duration(c.TimeoutSeconds) * time.Second
		clientCfg := &client.Config{
			Provider:          client.Provider(c.Provider),
			APIKey:            c.APIKey,
			BaseURL:           c.BaseURL,
			Model:             c.Model,
			Timeout:           timeout,
			AnthropicVersion:  c.AnthropicVersion,
			GeminiAPIEndpoint: c.GeminiAPIEndpoint,
		}
		cl, err := client.NewClient(clientCfg)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn(ctx, "[LLMProviderManager] 跳过无效端点",
					logging.String("name", c.Name),
					logging.String("provider", c.Provider),
					logging.Error(err),
				)
			}
			continue
		}
		capacity := float64(c.RateLimitPerMin + c.RateLimitBurst)
		if capacity <= 0 {
			capacity = float64(c.RateLimitPerMin)
		}
		if capacity < 0 {
			capacity = 0
		}
		now := time.Now()
		ep := &endpointState{
			cfg:            c,
			client:         cl,
			cooldownUntil:  0,
			rateTokens:     capacity,
			rateLastRefill: now,
		}
		eps = append(eps, ep)
	}

	return eps, nil
}

// selectCandidates 选择当前未处于冷却状态的、优先级最高的一批端点索引。
func (m *providerManagerImpl) selectCandidates(eps []*endpointState, now time.Time) []int {
	minPri := math.MaxInt32
	candidates := make([]int, 0, len(eps))

	for i, ep := range eps {
		// 跳过熔断中的端点
		if atomic.LoadUint32(&ep.inCircuitOpen) == 1 {
			continue
		}
		cd := atomic.LoadInt64(&ep.cooldownUntil)
		if cd > 0 && now.Before(time.Unix(0, cd)) {
			continue
		}

		p := ep.cfg.Priority
		if p == 0 {
			p = 100
		}

		if p < minPri {
			minPri = p
			candidates = candidates[:0]
			candidates = append(candidates, i)
		} else if p == minPri {
			candidates = append(candidates, i)
		}
	}

	return candidates
}

// selectAllByMinPriority 忽略冷却，选出优先级最高的一批端点。
func (m *providerManagerImpl) selectAllByMinPriority(eps []*endpointState) []int {
	if len(eps) == 0 {
		return nil
	}
	minPri := math.MaxInt32
	for _, ep := range eps {
		if atomic.LoadUint32(&ep.inCircuitOpen) == 1 {
			continue
		}
		p := ep.cfg.Priority
		if p == 0 {
			p = 100
		}
		if p < minPri {
			minPri = p
		}
	}
	candidates := make([]int, 0, len(eps))
	for i, ep := range eps {
		if atomic.LoadUint32(&ep.inCircuitOpen) == 1 {
			continue
		}
		p := ep.cfg.Priority
		if p == 0 {
			p = 100
		}
		if p == minPri {
			candidates = append(candidates, i)
		}
	}
	return candidates
}

// chooseWeightedStart 在候选端点中基于权重和 userID 选择起始位置。
func (m *providerManagerImpl) chooseWeightedStart(eps []*endpointState, candidates []int, userID int64, now time.Time) int {
	if len(candidates) == 0 {
		return 0
	}

	totalWeight := 0
	for _, idx := range candidates {
		w := eps[idx].cfg.Weight
		if w <= 0 {
			w = 100
		}
		totalWeight += w
	}
	if totalWeight <= 0 {
		return 0
	}

	var seed int64
	if userID > 0 {
		seed = userID
	} else {
		seed = now.UnixNano()
	}

	h := uint64(seed)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33

	point := int(h % uint64(totalWeight))

	for i, idx := range candidates {
		w := eps[idx].cfg.Weight
		if w <= 0 {
			w = 100
		}
		if point < w {
			return i
		}
		point -= w
	}

	return 0
}
