package router

import (
	"fmt"
	"strconv"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen-llm/service"
	"gochen/httpx"
	hbasic "gochen/httpx/nethttp"
)

// LLMAdminRoutes 提供 LLM 模块的管理接口
type LLMAdminRoutes struct {
	manager    service.ProviderManager
	safetyRepo repo.SafetyPolicyRepo
	safetySvc  service.SafetyService
	metrics    repo.MetricsRepo
	cfgRepo    repo.ProviderConfigRepo
	auditRepo  repo.AuditLogRepo
	rateRepo   repo.RateLimitRepo
	utils      *hbasic.Utils
}

func NewLLMAdminRoutes(manager service.ProviderManager, safety repo.SafetyPolicyRepo, metrics repo.MetricsRepo, cfgRepo repo.ProviderConfigRepo, audit repo.AuditLogRepo, rate repo.RateLimitRepo, safetySvc service.SafetyService) *LLMAdminRoutes {
	return &LLMAdminRoutes{
		manager:    manager,
		safetyRepo: safety,
		safetySvc:  safetySvc,
		metrics:    metrics,
		cfgRepo:    cfgRepo,
		auditRepo:  audit,
		rateRepo:   rate,
		utils:      &hbasic.Utils{},
	}
}

func (r *LLMAdminRoutes) RegisterRoutes(group httpx.IRouteGroup) error {
	admin := group.Group("/admin")
	admin.Use(AdminOnlyMiddleware())

	admin.GET("/llm/config", r.getLLMConfig)
	admin.PUT("/llm/config", r.updateLLMConfig)
	admin.PUT("/llm/pricing", r.updatePricing)
	admin.POST("/llm/reload", r.reloadLLMConfig)
	admin.GET("/llm/safety", r.getLLMSafetyConfig)
	admin.PUT("/llm/safety", r.updateLLMSafetyConfig)
	admin.GET("/llm/security/overview", r.getSecurityOverview)
	admin.GET("/llm/status", r.getLLMStatus)
	admin.GET("/llm/metrics", r.getLLMMetrics)
	admin.POST("/llm/metrics/convert", r.markConversion)
	admin.GET("/llm/audit", r.listAuditLogs)
	// TODO: 接口文档补充健康/限流字段说明
	return nil
}

func (r *LLMAdminRoutes) GetName() string {
	return "llm_admin"
}

func (r *LLMAdminRoutes) GetPriority() int {
	return 305
}

func (r *LLMAdminRoutes) getLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM manager 未配置"})
	}

	cfgs, err := r.manager.ListEffectiveConfigs(ctx.GetContext())
	if err != nil {
		return r.respondError(ctx, 500, err)
	}

	return ctx.JSON(200, map[string]interface{}{
		"configs": cfgs,
	})
}

func (r *LLMAdminRoutes) updateLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM manager 未配置"})
	}

	var body struct {
		Configs []*entity.ProviderConfig `json:"configs"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return r.respondError(ctx, 400, err)
	}

	if err := r.manager.ReplaceConfigs(ctx.GetContext(), body.Configs); err != nil {
		return r.respondError(ctx, 500, err)
	}

	if err := r.manager.Reload(ctx.GetContext()); err != nil {
		return r.respondError(ctx, 500, err)
	}

	return ctx.JSON(200, map[string]string{"message": "ok", "reload": "applied"})
}

func (r *LLMAdminRoutes) updatePricing(ctx httpx.IContext) error {
	if r.cfgRepo == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM config repo 未配置"})
	}
	var body struct {
		Pricing []entity.ProviderPricing `json:"pricing"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return r.respondError(ctx, 400, err)
	}
	if len(body.Pricing) == 0 {
		return r.respondError(ctx, 400, fmt.Errorf("pricing 不能为空"))
	}
	for _, p := range body.Pricing {
		if err := r.validatePricing(p); err != nil {
			return r.respondError(ctx, 400, err)
		}
	}
	if err := r.cfgRepo.UpdatePricing(ctx.GetContext(), body.Pricing); err != nil {
		return r.respondError(ctx, 500, err)
	}
	if r.manager != nil {
		_ = r.manager.Reload(ctx.GetContext())
	}
	return ctx.JSON(200, map[string]string{"message": "ok"})
}

func (r *LLMAdminRoutes) reloadLLMConfig(ctx httpx.IContext) error {
	if r.manager == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM manager 未配置"})
	}

	if err := r.manager.Reload(ctx.GetContext()); err != nil {
		return r.respondError(ctx, 500, err)
	}

	return ctx.JSON(200, map[string]string{"message": "reloaded"})
}

func (r *LLMAdminRoutes) getLLMSafetyConfig(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM safety repo 未配置"})
	}

	cfg, err := r.safetyRepo.GetActive(ctx.GetContext())
	if err != nil {
		return r.respondError(ctx, 500, err)
	}
	return ctx.JSON(200, map[string]any{
		"config": cfg,
	})
}

func (r *LLMAdminRoutes) updateLLMSafetyConfig(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM safety repo 未配置"})
	}

	var body struct {
		Config *entity.SafetyPolicy `json:"config"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return r.respondError(ctx, 400, err)
	}
	if body.Config == nil {
		return r.respondError(ctx, 400, fmt.Errorf("config 不能为空"))
	}

	cfg := &entity.SafetyPolicy{
		Enabled:               body.Config.Enabled,
		GlobalSystemPrompt:    body.Config.GlobalSystemPrompt,
		BlockedCategoriesJSON: body.Config.BlockedCategoriesJSON,
		BlockedKeywordsJSON:   body.Config.BlockedKeywordsJSON,
		MaxContentLength:      body.Config.MaxContentLength,
		LogLevel:              body.Config.LogLevel,
	}

	if err := r.safetyRepo.Save(ctx.GetContext(), cfg); err != nil {
		return r.respondError(ctx, 500, err)
	}

	return ctx.JSON(200, map[string]string{"message": "ok"})
}

func (r *LLMAdminRoutes) getLLMStatus(ctx httpx.IContext) error {
	if r.manager == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM manager 未配置"})
	}

	status, err := r.manager.ListStatus(ctx.GetContext())
	if err != nil {
		return r.respondError(ctx, 500, err)
	}

	return ctx.JSON(200, map[string]interface{}{
		"status": status,
	})
}

func (r *LLMAdminRoutes) getLLMMetrics(ctx httpx.IContext) error {
	if r.metrics == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM metrics repo 未配置"})
	}

	var filter entity.MetricsFilter
	if provider := ctx.GetRequest().URL.Query().Get("provider"); provider != "" {
		filter.Provider = provider
	}
	if model := ctx.GetRequest().URL.Query().Get("model"); model != "" {
		filter.Model = model
	}
	if abTest := ctx.GetRequest().URL.Query().Get("ab_test_id"); abTest != "" {
		if v, err := strconv.ParseInt(abTest, 10, 64); err == nil {
			filter.ABTestID = &v
		}
	}
	if variant := ctx.GetRequest().URL.Query().Get("ab_variant"); variant != "" {
		filter.ABVariant = variant
	}

	group := ctx.GetRequest().URL.Query().Get("group_by")
	if group == "variant" && filter.ABTestID != nil {
		rows, err := r.metrics.AggregateByVariant(ctx.GetContext(), filter)
		if err != nil {
			return r.respondError(ctx, 500, err)
		}
		return ctx.JSON(200, map[string]interface{}{
			"variants": rows,
		})
	}

	report, err := r.metrics.Aggregate(ctx.GetContext(), filter)
	if err != nil {
		return r.respondError(ctx, 500, err)
	}
	return ctx.JSON(200, map[string]interface{}{
		"report": report,
	})
}

// markConversion 记录一次转化事件（例如 A/B 测试的成功/点击）
func (r *LLMAdminRoutes) markConversion(ctx httpx.IContext) error {
	if r.metrics == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM metrics repo 未配置"})
	}
	var body struct {
		UserID           int64  `json:"user_id"`
		ABTestID         int64  `json:"ab_test_id"`
		ABVariant        string `json:"ab_variant"`
		PromptTemplateID int64  `json:"prompt_template_id"`
		Provider         string `json:"provider"`
		Model            string `json:"model"`
		Outcome          string `json:"outcome"`
		ConversionType   string `json:"conversion_type"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		return r.respondError(ctx, 400, err)
	}
	if body.Outcome == "" {
		body.Outcome = body.ConversionType
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
	if err := r.metrics.Save(ctx.GetContext(), record); err != nil {
		return r.respondError(ctx, 500, err)
	}
	return ctx.JSON(200, map[string]string{"message": "ok"})
}

func (r *LLMAdminRoutes) listAuditLogs(ctx httpx.IContext) error {
	if r.auditRepo == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM audit repo 未配置"})
	}

	var filter repo.AuditLogFilter
	q := ctx.GetRequest().URL.Query()
	if v := q.Get("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.UserID = &id
		}
	}
	if v := q.Get("action"); v != "" {
		filter.Action = v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = v
	}
	if v := q.Get("resource_type"); v != "" {
		filter.ResourceType = v
	}
	if v := q.Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.StartAt = &t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.EndAt = &t
		}
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	list, total, err := r.auditRepo.List(ctx.GetContext(), filter, limit, offset)
	if err != nil {
		return r.respondError(ctx, 500, err)
	}
	return ctx.JSON(200, map[string]any{
		"total":  total,
		"list":   list,
		"limit":  limit,
		"offset": offset,
	})
}

func (r *LLMAdminRoutes) getSecurityOverview(ctx httpx.IContext) error {
	if r.safetyRepo == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM safety repo 未配置"})
	}
	policy, err := r.safetyRepo.GetActive(ctx.GetContext())
	if err != nil {
		return r.respondError(ctx, 500, err)
	}

	rateSummary := map[string]any{
		"resource_type": "chat",
	}
	if r.safetySvc != nil {
		settings := r.safetySvc.GetRateLimitSettings()
		rateSummary["per_minute"] = settings.PerMinute
		rateSummary["burst"] = settings.Burst
	}
	if r.rateRepo != nil {
		since := time.Now().Add(-1 * time.Hour)
		if total, err := r.rateRepo.SumSince(ctx.GetContext(), "chat", since); err == nil {
			rateSummary["requests_last_hour"] = total
		}
		if recent, err := r.rateRepo.ListRecent(ctx.GetContext(), "chat", 20); err == nil {
			rateSummary["recent_windows"] = recent
		}
	}

	return ctx.JSON(200, map[string]any{
		"policy":     policy,
		"rate_limit": rateSummary,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *LLMAdminRoutes) respondError(ctx httpx.IContext, status int, err error) error {
	return ctx.JSON(status, map[string]string{"message": err.Error()})
}

func (r *LLMAdminRoutes) validatePricing(p entity.ProviderPricing) error {
	if p.ID <= 0 {
		return fmt.Errorf("pricing id 无效")
	}
	if p.InputPricePer1k < 0 || p.OutputPricePer1k < 0 {
		return fmt.Errorf("单价不能为负数")
	}
	if p.InputPricePer1k > 100 || p.OutputPricePer1k > 100 {
		return fmt.Errorf("单价超出合理范围，请检查输入")
	}
	return nil
}
