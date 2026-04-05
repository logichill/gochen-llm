package router

import (
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	restapi "gochen/api/restapi"
	dataquery "gochen/db/query"
	"gochen/errorx"
	"gochen/httpx"
)

type llmMetricsQueryFields struct {
	Provider  string    `query:"type=enum,ops=eq"`
	Model     string    `query:"type=enum,ops=eq"`
	Status    string    `query:"type=enum,ops=eq"`
	ABVariant string    `query:"type=enum,ops=eq"`
	Outcome   string    `query:"type=enum,ops=eq"`
	ABTestID  int64     `query:"ops=eq"`
	UserID    int64     `query:"ops=eq"`
	CreatedAt time.Time `query:"ops=gte|lte"`
}

var llmMetricsQuerySchema = dataquery.MustInferQuerySchema[llmMetricsQueryFields](nil)
var llmMetricsQueryConfig = restapi.NewQueryRouteConfig[int64](llmMetricsQuerySchema, 50, 500)

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
	if err := rejectLegacyQueryParams(ctx,
		"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	params, err := parseMetricsQueryParams(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(params.CriteriaView().Filters)

	group := ctx.GetRequest().URL.Query().Get("group_by")
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
	if err := rejectLegacyQueryParams(ctx,
		"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end", "limit", "offset",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	opts, err := parseMetricsPaginationOptions(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(opts.CriteriaView().Filters)
	limit, offset := resolvePageBounds(opts, 50)

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
	if err := rejectLegacyQueryParams(ctx,
		"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end",
	); err != nil {
		return httpx.WriteError(ctx, err)
	}

	params, err := parseMetricsQueryParams(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(params.CriteriaView().Filters)
	if err := requireABTestID(filter); err != nil {
		return httpx.WriteError(ctx, err)
	}

	report, err := r.metrics.Significance(ctx.GetContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

func parseMetricsQueryParams(ctx httpx.IContext) (*dataquery.QueryParams, error) {
	return restapi.ParseQueryParams(ctx, llmMetricsQueryConfig)
}

func parseMetricsPaginationOptions(ctx httpx.IContext) (*dataquery.PaginationOptions, error) {
	return restapi.ParsePaginationOptions(ctx, llmMetricsQueryConfig)
}

func decodeMetricsFilter(decoded []dataquery.DecodedFilter) entity.MetricsFilter {
	var filter entity.MetricsFilter
	for _, item := range decoded {
		switch item.Field.Name {
		case "provider":
			filter.Provider = item.Value.String
		case "model":
			filter.Model = item.Value.String
		case "status":
			filter.Status = item.Value.String
		case "ab_variant":
			filter.ABVariant = item.Value.String
		case "outcome":
			filter.Outcome = item.Value.String
		case "ab_test_id":
			value := item.Value.Int
			filter.ABTestID = &value
		case "user_id":
			value := item.Value.Int
			filter.UserID = &value
		case "created_at":
			value := item.Value.Time
			if item.Op == dataquery.FilterOpGte {
				filter.StartAt = &value
			}
			if item.Op == dataquery.FilterOpLte {
				filter.EndAt = &value
			}
		}
	}
	return filter
}

func requireABTestID(filter entity.MetricsFilter) error {
	if filter.ABTestID != nil {
		return nil
	}
	return errorx.New(errorx.InvalidInput, "ab_test_id 不能为空")
}

func resolvePageBounds(opts *dataquery.PaginationOptions, defaultSize int) (int, int) {
	limit := defaultSize
	offset := 0
	if opts != nil {
		if opts.Size > 0 {
			limit = opts.Size
		}
		if opts.Page > 1 && limit > 0 {
			offset = (opts.Page - 1) * limit
		}
	}
	return limit, offset
}
