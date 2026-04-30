package router

import (
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/api/restapi"
	"gochen/db/query"
	"gochen/db/query/querybind"
	"gochen/errors"
	"gochen/httpx"
)

type llmMetricsQueryFields struct {
	Provider  string                 `query:"type=enum,ops=eq"`
	Model     string                 `query:"type=enum,ops=eq"`
	Status    string                 `query:"type=enum,ops=eq"`
	ABVariant string                 `query:"type=enum,ops=eq"`
	Outcome   string                 `query:"type=enum,ops=eq"`
	ABTestID  *int64                 `query:"field=ab_test_id,ops=eq"`
	UserID    *int64                 `query:"field=user_id,ops=eq"`
	CreatedAt query.Range[time.Time] `query:"field=created_at,ops=gte|lte"`
}

var llmMetricsQueryContract = querybind.MustNewContract[llmMetricsQueryFields](nil)
var llmMetricsQuerySchema = llmMetricsQueryContract.Schema()
var llmMetricsQueryConfig = restapi.NewQueryRouteConfig[int64](llmMetricsQuerySchema, 50, 500)

type metricsAggregateQuery struct {
	GroupBy string `query:"group_by"`
}

// MetricsRoutes 提供指标看板接口（时间窗口聚合与原始日志分页）
type MetricsRoutes struct {
	metrics repo.IMetricsRepo
}

// NewMetricsRoutes 创建指标路由集合。
func NewMetricsRoutes(metrics repo.IMetricsRepo) *MetricsRoutes {
	return &MetricsRoutes{metrics: metrics}
}

// Name 返回名称。
func (r *MetricsRoutes) Name() string { return "llm_metrics" }

// Priority 返回优先级。
func (r *MetricsRoutes) Priority() int { return 310 }

// RegisterRoutes 注册路由集合。
func (r *MetricsRoutes) RegisterRoutes(group httpx.IRouteGroup) error {
	metricsGroup := group.Group("/admin/llm/metrics")
	metricsGroup.Use(ReadPermissionMiddleware())
	metricsGroup.GET("/agg", r.aggregate)
	metricsGroup.GET("/list", r.list)
	metricsGroup.GET("/significance", r.significance)
	return nil
}

// aggregate 聚合数据。
func (r *MetricsRoutes) aggregate(ctx httpx.IContext) error {
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
		return httpx.WriteSuccess(ctx, map[string]any{"variants": rows})
	}

	report, err := r.metrics.Aggregate(ctx.RequestContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

// list 列出数据。
func (r *MetricsRoutes) list(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM metrics repo 未配置")
	}
	if err := restapi.RejectLegacyQueryParams(ctx,
		"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end", "limit", "offset",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	opts, err := restapi.ParsePaginationOptions(ctx, llmMetricsQueryConfig)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter, err := decodeMetricsFilter(opts.Filters)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	limit, offset := opts.Size, opts.Offset()

	list, total, err := r.metrics.List(ctx.RequestContext(), filter, limit, offset)
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

// significance 处理 significance。
func (r *MetricsRoutes) significance(ctx httpx.IContext) error {
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
	if err := requireABTestID(filter); err != nil {
		return httpx.WriteError(ctx, err)
	}

	report, err := r.metrics.Significance(ctx.RequestContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

func decodeMetricsFilter(filters query.QueryFilters) (entity.MetricsFilter, error) {
	bound, err := llmMetricsQueryContract.Decode(filters)
	if err != nil {
		return entity.MetricsFilter{}, err
	}
	return entity.MetricsFilter{
		Provider:  bound.Provider,
		Model:     bound.Model,
		UserID:    bound.UserID,
		Status:    bound.Status,
		ABTestID:  bound.ABTestID,
		ABVariant: bound.ABVariant,
		StartAt:   bound.CreatedAt.LowerPtr(),
		EndAt:     bound.CreatedAt.UpperPtr(),
		Outcome:   bound.Outcome,
	}, nil
}

func requireABTestID(filter entity.MetricsFilter) error {
	if filter.ABTestID != nil {
		return nil
	}
	return errors.NewCode(errors.InvalidInput, "ab_test_id 不能为空")
}

func parseMetricsAggregateGroupBy(ctx httpx.IContext) (string, error) {
	quer, err := restapi.ParseQuery[metricsAggregateQuery](ctx)
	if err != nil {
		return "", err
	}
	return quer.GroupBy, nil
}
