package router

import (
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen-llm/service"
	restapi "gochen/api/restapi"
	"gochen/db/query"
	"gochen/db/query/querybind"
	"gochen/errors"
	"gochen/httpx"
	"gochen/httpx/nethttp"
)

type llmAuditLogQueryFields struct {
	UserID       *int64                 `query:"field=user_id,ops=eq"`
	Action       string                 `query:"type=enum,ops=eq"`
	Status       string                 `query:"type=enum,ops=eq"`
	ResourceType string                 `query:"field=resource_type,type=enum,ops=eq"`
	CreatedAt    query.Range[time.Time] `query:"field=created_at,ops=gte|lte"`
}

var llmAuditLogQueryContract = querybind.MustNewContract[llmAuditLogQueryFields](nil)
var llmAuditLogQuerySchema = llmAuditLogQueryContract.Schema()
var llmAuditLogQueryConfig = restapi.NewQueryRouteConfig[int64](llmAuditLogQuerySchema, 50, 200)

// LLMAdminRoutes 提供 LLM 模块的管理接口
type LLMAdminRoutes struct {
	manager    service.IProviderManager
	safetyRepo repo.ISafetyPolicyRepo
	safetySvc  service.ISafetyService
	metrics    repo.IMetricsRepo
	cfgRepo    repo.IProviderConfigRepo
	auditRepo  repo.IAuditLogRepo
	rateRepo   repo.IRateLimitRepo
	utils      *nethttp.Utils
}

// NewLLMAdminRoutes 创建LLM管理端路由集合。
func NewLLMAdminRoutes(manager service.IProviderManager, safety repo.ISafetyPolicyRepo, metrics repo.IMetricsRepo, cfgRepo repo.IProviderConfigRepo, audit repo.IAuditLogRepo, rate repo.IRateLimitRepo, safetySvc service.ISafetyService) *LLMAdminRoutes {
	return &LLMAdminRoutes{
		manager:    manager,
		safetyRepo: safety,
		safetySvc:  safetySvc,
		metrics:    metrics,
		cfgRepo:    cfgRepo,
		auditRepo:  audit,
		rateRepo:   rate,
		utils:      &nethttp.Utils{},
	}
}

// RegisterRoutes 注册路由集合。
func (r *LLMAdminRoutes) RegisterRoutes(group httpx.IRouteGroup) error {
	read := group.Group("/admin")
	read.Use(ReadPermissionMiddleware())
	read.GET("/llm/config", r.getLLMConfig)
	read.GET("/llm/safety", r.getLLMSafetyConfig)
	read.GET("/llm/security/overview", r.getSecurityOverview)
	read.GET("/llm/status", r.getLLMStatus)
	read.GET("/llm/metrics", r.getLLMMetrics)
	read.GET("/llm/audit", r.listAuditLogs)

	write := group.Group("/admin")
	write.Use(WritePermissionMiddleware())
	write.PUT("/llm/config", r.updateLLMConfig)
	write.PUT("/llm/pricing", r.updatePricing)
	write.POST("/llm/reload", r.reloadLLMConfig)
	write.PUT("/llm/safety", r.updateLLMSafetyConfig)
	write.POST("/llm/metrics/convert", r.markConversion)
	return nil
}

// Name 返回名称。
func (r *LLMAdminRoutes) Name() string {
	return "llm_admin"
}

// Priority 返回优先级。
func (r *LLMAdminRoutes) Priority() int {
	return 305
}

// getLLMConfig 返回LLM配置。
func (r *LLMAdminRoutes) getLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM manager 未配置")
	}

	cfgs, err := r.manager.ListEffectiveConfigs(ctx.RequestContext())
	if err != nil {
		return httpx.WriteError(ctx, err)
	}

	return httpx.WriteSuccess(ctx, map[string]any{
		"configs": cfgs,
	})
}

// updateLLMConfig 更新LLM配置。
func (r *LLMAdminRoutes) updateLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM manager 未配置")
	}

	var body struct {
		Configs []*entity.ProviderConfig `json:"configs"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return httpx.WriteError(ctx, err)
	}

	if err := r.manager.ReplaceConfigs(ctx.RequestContext(), body.Configs); err != nil {
		return httpx.WriteError(ctx, err)
	}

	if err := r.manager.Reload(ctx.RequestContext()); err != nil {
		return httpx.WriteError(ctx, err)
	}

	return httpx.WriteSuccessMessage(ctx, 200, "ok", map[string]any{"reload": "applied"})
}

// updatePricing 更新定价。
func (r *LLMAdminRoutes) updatePricing(ctx httpx.IContext) error {
	if r.cfgRepo == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM config repo 未配置")
	}
	var body struct {
		Pricing []entity.ProviderPricing `json:"pricing"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return httpx.WriteError(ctx, err)
	}
	if len(body.Pricing) == 0 {
		return httpx.WriteErrorCode(ctx, errors.InvalidInput, "pricing 不能为空")
	}
	for _, p := range body.Pricing {
		if err := r.validatePricing(p); err != nil {
			return httpx.WriteError(ctx, err)
		}
	}
	if err := r.cfgRepo.UpdatePricing(ctx.RequestContext(), body.Pricing); err != nil {
		return httpx.WriteError(ctx, err)
	}
	if r.manager != nil {
		_ = r.manager.Reload(ctx.RequestContext())
	}
	return httpx.WriteSuccessMessage(ctx, 200, "ok", nil)
}

// reloadLLMConfig 处理reloadLLM配置。
func (r *LLMAdminRoutes) reloadLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM manager 未配置")
	}

	if err := r.manager.Reload(ctx.RequestContext()); err != nil {
		return httpx.WriteError(ctx, err)
	}

	return httpx.WriteSuccessMessage(ctx, 200, "reloaded", nil)
}

// getLLMSafetyConfig 返回LLM安全配置。
func (r *LLMAdminRoutes) getLLMSafetyConfig(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM safety repo 未配置")
	}

	cfg, err := r.safetyRepo.FindActive(ctx.RequestContext())
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{
		"config": cfg,
	})
}

// updateLLMSafetyConfig 更新LLM安全配置。
func (r *LLMAdminRoutes) updateLLMSafetyConfig(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM safety repo 未配置")
	}

	var body struct {
		Config *entity.SafetyPolicy `json:"config"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return httpx.WriteError(ctx, err)
	}
	if body.Config == nil {
		return httpx.WriteErrorCode(ctx, errors.InvalidInput, "config 不能为空")
	}

	cfg := &entity.SafetyPolicy{
		Enabled:               body.Config.Enabled,
		GlobalSystemPrompt:    body.Config.GlobalSystemPrompt,
		BlockedCategoriesJSON: body.Config.BlockedCategoriesJSON,
		BlockedKeywordsJSON:   body.Config.BlockedKeywordsJSON,
		MaxContentLength:      body.Config.MaxContentLength,
		LogLevel:              body.Config.LogLevel,
	}

	if err := r.safetyRepo.Save(ctx.RequestContext(), cfg); err != nil {
		return httpx.WriteError(ctx, err)
	}

	return httpx.WriteSuccessMessage(ctx, 200, "ok", nil)
}

// getLLMStatus 返回LLM状态。
func (r *LLMAdminRoutes) getLLMStatus(ctx httpx.IContext) error {
	if r.manager == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM manager 未配置")
	}

	status, err := r.manager.ListStatus(ctx.RequestContext())
	if err != nil {
		return httpx.WriteError(ctx, err)
	}

	return httpx.WriteSuccess(ctx, map[string]any{
		"status": status,
	})
}

// getLLMMetrics 返回LLM指标。
func (r *LLMAdminRoutes) getLLMMetrics(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM metrics repo 未配置")
	}
	if err := restapi.RejectLegacyQueryParams(ctx,
		"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	params, err := restapi.ParseQueryParams(ctx, llmMetricsQueryConfig)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter, err := decodeMetricsFilter(params.Filters)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	group, err := parseMetricsAggregateGroupBy(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	if group == "variant" && filter.ABTestID != nil {
		rows, err := r.metrics.AggregateByVariant(ctx.RequestContext(), filter)
		if err != nil {
			return httpx.WriteError(ctx, err)
		}
		return httpx.WriteSuccess(ctx, map[string]any{
			"variants": rows,
		})
	}

	report, err := r.metrics.Aggregate(ctx.RequestContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{
		"report": report,
	})
}

func decodeAuditLogFilter(filters query.QueryFilters) (repo.AuditLogFilter, error) {
	bound, err := llmAuditLogQueryContract.Decode(filters)
	if err != nil {
		return repo.AuditLogFilter{}, err
	}
	return repo.AuditLogFilter{
		UserID:       bound.UserID,
		Action:       bound.Action,
		Status:       bound.Status,
		ResourceType: bound.ResourceType,
		StartAt:      bound.CreatedAt.LowerPtr(),
		EndAt:        bound.CreatedAt.UpperPtr(),
	}, nil
}

// markConversion 记录一次转化事件（例如 A/B 测试的成功/点击）
func (r *LLMAdminRoutes) markConversion(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM metrics repo 未配置")
	}
	if err := rejectLegacyJSONFields(ctx, "conversion_type"); err != nil {
		return httpx.WriteError(ctx, err)
	}
	var body struct {
		UserID           int64  `json:"user_id"`
		ABTestID         int64  `json:"ab_test_id"`
		ABVariant        string `json:"ab_variant"`
		PromptTemplateID int64  `json:"prompt_template_id"`
		Provider         string `json:"provider"`
		Model            string `json:"model"`
		Outcome          string `json:"outcome"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return httpx.WriteError(ctx, err)
	}
	if body.Outcome == "" {
		body.Outcome = "conversion"
	}

	record := &entity.Metrics{
		UserID:         body.UserID,
		ABTestID:       body.ABTestID,
		ABVariant:      body.ABVariant,
		PromptTemplate: body.PromptTemplateID,
		Provider:       body.Provider,
		Model:          body.Model,
		Status:         "converted",
		Outcome:        body.Outcome,
	}
	if err := r.metrics.Save(ctx.RequestContext(), record); err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccessMessage(ctx, 200, "ok", nil)
}

// listAuditLogs 列出审计日志列表。
func (r *LLMAdminRoutes) listAuditLogs(ctx httpx.IContext) error {
	if r.auditRepo == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM audit repo 未配置")
	}
	if err := restapi.RejectLegacyQueryParams(ctx,
		"user_id", "action", "status", "resource_type", "start", "end", "limit", "offset",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	opts, err := restapi.ParsePaginationOptions(ctx, llmAuditLogQueryConfig)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter, err := decodeAuditLogFilter(opts.Filters)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	limit, offset := opts.Size, opts.Offset()

	list, total, err := r.auditRepo.List(ctx.RequestContext(), filter, limit, offset)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{
		"total":  total,
		"list":   list,
		"limit":  limit,
		"offset": offset,
	})
}

// getSecurityOverview 返回安全概览。
func (r *LLMAdminRoutes) getSecurityOverview(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM safety repo 未配置")
	}
	policy, err := r.safetyRepo.FindActive(ctx.RequestContext())
	if err != nil {
		return httpx.WriteError(ctx, err)
	}

	rateSummary := map[string]any{
		"resource_type": "chat",
	}
	if r.safetySvc != nil {
		settings := r.safetySvc.RateLimitSettings()
		rateSummary["per_minute"] = settings.PerMinute
		rateSummary["burst"] = settings.Burst
	}
	if r.rateRepo != nil {
		since := time.Now().Add(-1 * time.Hour)
		if total, err := r.rateRepo.SumSince(ctx.RequestContext(), "chat", since); err == nil {
			rateSummary["requests_last_hour"] = total
		}
		if recent, err := r.rateRepo.ListRecent(ctx.RequestContext(), "chat", 20); err == nil {
			rateSummary["recent_windows"] = recent
		}
	}

	return httpx.WriteSuccess(ctx, map[string]any{
		"policy":     policy,
		"rate_limit": rateSummary,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// validatePricing 校验定价。
func (r *LLMAdminRoutes) validatePricing(p entity.ProviderPricing) error {
	if p.ID <= 0 {
		return errors.NewCode(errors.InvalidInput, "pricing id 无效")
	}
	if p.InputPricePer1k < 0 || p.OutputPricePer1k < 0 {
		return errors.NewCode(errors.InvalidInput, "单价不能为负数")
	}
	if p.InputPricePer1k > 100 || p.OutputPricePer1k > 100 {
		return errors.NewCode(errors.InvalidInput, "单价超出合理范围，请检查输入")
	}
	return nil
}
