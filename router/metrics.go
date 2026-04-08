package router

import (
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	restapi "gochen/api/restapi"
	queryhelper "gochen/app/helper/query"
	dataquery "gochen/db/query"
	"gochen/errorx"
	"gochen/httpx"
)

type llmMetricsQueryFields struct {
	Provider  string                     `query:"type=enum,ops=eq"`
	Model     string                     `query:"type=enum,ops=eq"`
	Status    string                     `query:"type=enum,ops=eq"`
	ABVariant string                     `query:"type=enum,ops=eq"`
	Outcome   string                     `query:"type=enum,ops=eq"`
	ABTestID  *int64                     `query:"field=ab_test_id,ops=eq"`
	UserID    *int64                     `query:"field=user_id,ops=eq"`
	CreatedAt dataquery.Range[time.Time] `query:"field=created_at,ops=gte|lte"`
}

var llmMetricsQueryContract = queryhelper.MustNewContract[llmMetricsQueryFields](nil)
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

// GetName 返回名称。
func (r *MetricsRoutes) GetName() string { return "llm_metrics" }

// GetPriority 返回优先级。
func (r *MetricsRoutes) GetPriority() int { return 310 }

// RegisterRoutes 注册路由集合。
func (r *MetricsRoutes) RegisterRoutes(group httpx.IRouteGroup) error {
	metricsGroup := group.Group("/admin/llm/metrics")
	metricsGroup.GET("/agg", r.aggregate)
	metricsGroup.GET("/list", r.list)
	metricsGroup.GET("/significance", r.significance)
	return nil
}

// aggregate 聚合数据。
func (r *MetricsRoutes) aggregate(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errorx.Internal, "LLM metrics repo 未配置")
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
		rows, err := r.metrics.AggregateByVariant(ctx.GetContext(), filter)
		if err != nil {
			return httpx.WriteError(ctx, err)
		}
		return httpx.WriteSuccess(ctx, map[string]any{"variants": rows})
	}

	report, err := r.metrics.Aggregate(ctx.GetContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

// list 列出数据。
func (r *MetricsRoutes) list(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errorx.Internal, "LLM metrics repo 未配置")
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

	list, total, err := r.metrics.List(ctx.GetContext(), filter, limit, offset)
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
		return httpx.WriteErrorCode(ctx, errorx.Internal, "LLM metrics repo 未配置")
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

	report, err := r.metrics.Significance(ctx.GetContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

func decodeMetricsFilter(filters dataquery.QueryFilters) (entity.MetricsFilter, error) {
	bound, err := queryhelper.DecodeContract(llmMetricsQueryContract, filters)
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
	return errorx.New(errorx.InvalidInput, "ab_test_id 不能为空")
}

func parseMetricsAggregateGroupBy(ctx httpx.IContext) (string, error) {
	query, err := restapi.ParseQuery[metricsAggregateQuery](ctx)
	if err != nil {
		return "", err
	}
	return query.GroupBy, nil
}
