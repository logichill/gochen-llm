package router

import (
	"strconv"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/httpx"
)

// MetricsRoutes 提供指标看板接口（时间窗口聚合与原始日志分页）
type MetricsRoutes struct {
	metrics repo.MetricsRepo
}

func NewMetricsRoutes(metrics repo.MetricsRepo) *MetricsRoutes {
	return &MetricsRoutes{metrics: metrics}
}

func (r *MetricsRoutes) GetName() string { return "llm_metrics" }

func (r *MetricsRoutes) GetPriority() int { return 310 }

func (r *MetricsRoutes) RegisterRoutes(group httpx.IRouteGroup) error {
	api := group.Group("/admin/llm/metrics")
	api.GET("/agg", r.aggregate)
	api.GET("/list", r.list)
	api.GET("/significance", r.significance)
	return nil
}

func (r *MetricsRoutes) aggregate(ctx httpx.IContext) error {
	if r.metrics == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM metrics repo 未配置"})
	}

	var filter entity.MetricsFilter
	q := ctx.GetRequest().URL.Query()
	if v := q.Get("provider"); v != "" {
		filter.Provider = v
	}
	if v := q.Get("model"); v != "" {
		filter.Model = v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = v
	}
	if v := q.Get("ab_variant"); v != "" {
		filter.ABVariant = v
	}
	if v := q.Get("outcome"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("conversion_type"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("ab_test_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ABTestID = &id
		}
	}
	if v := q.Get("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.UserID = &id
		}
	}
	// 时间窗口，可选 start/end
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

	group := q.Get("group_by")
	if group == "variant" && filter.ABTestID != nil {
		rows, err := r.metrics.AggregateByVariant(ctx.GetContext(), filter)
		if err != nil {
			return ctx.JSON(500, map[string]string{"message": err.Error()})
		}
		return ctx.JSON(200, map[string]any{"variants": rows})
	}

	report, err := r.metrics.Aggregate(ctx.GetContext(), filter)
	if err != nil {
		return ctx.JSON(500, map[string]string{"message": err.Error()})
	}
	return ctx.JSON(200, map[string]any{"report": report})
}

func (r *MetricsRoutes) list(ctx httpx.IContext) error {
	if r.metrics == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM metrics repo 未配置"})
	}

	var filter entity.MetricsFilter
	q := ctx.GetRequest().URL.Query()
	if v := q.Get("provider"); v != "" {
		filter.Provider = v
	}
	if v := q.Get("model"); v != "" {
		filter.Model = v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = v
	}
	if v := q.Get("ab_variant"); v != "" {
		filter.ABVariant = v
	}
	if v := q.Get("outcome"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("conversion_type"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("ab_test_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ABTestID = &id
		}
	}
	if v := q.Get("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.UserID = &id
		}
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
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	list, total, err := r.metrics.List(ctx.GetContext(), filter, limit, offset)
	if err != nil {
		return ctx.JSON(500, map[string]string{"message": err.Error()})
	}

	return ctx.JSON(200, map[string]any{
		"total":  total,
		"list":   list,
		"limit":  limit,
		"offset": offset,
	})
}

func (r *MetricsRoutes) significance(ctx httpx.IContext) error {
	if r.metrics == nil {
		return ctx.JSON(500, map[string]string{"message": "LLM metrics repo 未配置"})
	}

	var filter entity.MetricsFilter
	q := ctx.GetRequest().URL.Query()
	if v := q.Get("provider"); v != "" {
		filter.Provider = v
	}
	if v := q.Get("model"); v != "" {
		filter.Model = v
	}
	if v := q.Get("outcome"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("conversion_type"); v != "" {
		filter.Outcome = v
	}
	if v := q.Get("ab_test_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ABTestID = &id
		}
	}
	if filter.ABTestID == nil {
		return ctx.JSON(400, map[string]string{"message": "ab_test_id 不能为空"})
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

	report, err := r.metrics.Significance(ctx.GetContext(), filter)
	if err != nil {
		return ctx.JSON(500, map[string]string{"message": err.Error()})
	}
	return ctx.JSON(200, map[string]any{
		"report": report,
	})
}
