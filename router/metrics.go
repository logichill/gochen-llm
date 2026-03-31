package router

import (
	"strconv"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	api "gochen/api/http"
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
var llmMetricsQueryConfig = api.NewQueryRouteConfig[int64](llmMetricsQuerySchema, 50, 500)

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
	api := group.Group("/admin/llm/metrics")
	api.GET("/agg", r.aggregate)
	api.GET("/list", r.list)
	api.GET("/significance", r.significance)
	return nil
}

// aggregate 聚合数据。
func (r *MetricsRoutes) aggregate(ctx httpx.IContext) error {
	if r.metrics == nil {
		return httpx.WriteErrorCode(ctx, errorx.Internal, "LLM metrics repo 未配置")
	}

	params, err := parseMetricsQueryParams(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(params.DecodedFilters)
	applyLegacyMetricsFilter(ctx, &filter)

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

	opts, err := parseMetricsPaginationOptions(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(opts.DecodedFilters)
	applyLegacyMetricsFilter(ctx, &filter)
	limit, offset := resolveLimitOffset(ctx, opts, 50, 500)

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

	params, err := parseMetricsQueryParams(ctx)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter := decodeMetricsFilter(params.DecodedFilters)
	applyLegacyMetricsFilter(ctx, &filter)
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
	return api.ParseQueryParams(ctx, llmMetricsQueryConfig)
}

func parseMetricsPaginationOptions(ctx httpx.IContext) (*dataquery.PaginationOptions, error) {
	return api.ParsePaginationOptions(ctx, llmMetricsQueryConfig)
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

func applyLegacyMetricsFilter(ctx httpx.IContext, filter *entity.MetricsFilter) {
	if filter == nil {
		return
	}
	q := ctx.GetRequest().URL.Query()
	if filter.Provider == "" {
		filter.Provider = q.Get("provider")
	}
	if filter.Model == "" {
		filter.Model = q.Get("model")
	}
	if filter.Status == "" {
		filter.Status = q.Get("status")
	}
	if filter.ABVariant == "" {
		filter.ABVariant = q.Get("ab_variant")
	}
	if filter.Outcome == "" {
		filter.Outcome = q.Get("outcome")
		if filter.Outcome == "" {
			filter.Outcome = q.Get("conversion_type")
		}
	}
	if filter.ABTestID == nil {
		if value, ok := parseInt64Query(q.Get("ab_test_id")); ok {
			filter.ABTestID = &value
		}
	}
	if filter.UserID == nil {
		if value, ok := parseInt64Query(q.Get("user_id")); ok {
			filter.UserID = &value
		}
	}
	if filter.StartAt == nil {
		if value, ok := parseRFC3339Query(q.Get("start")); ok {
			filter.StartAt = &value
		}
	}
	if filter.EndAt == nil {
		if value, ok := parseRFC3339Query(q.Get("end")); ok {
			filter.EndAt = &value
		}
	}
}

func requireABTestID(filter entity.MetricsFilter) error {
	if filter.ABTestID != nil {
		return nil
	}
	return errorx.New(errorx.InvalidInput, "ab_test_id 不能为空")
}

func resolveLimitOffset(ctx httpx.IContext, opts *dataquery.PaginationOptions, defaultLimit, maxLimit int) (int, int) {
	if ctx.GetQuery("page") != "" || ctx.GetQuery("size") != "" {
		limit := defaultLimit
		offset := 0
		if opts != nil {
			limit = opts.Size
			offset = (opts.Page - 1) * opts.Size
		}
		return limit, offset
	}
	q := ctx.GetRequest().URL.Query()
	limit := defaultLimit
	if value, err := strconv.Atoi(q.Get("limit")); err == nil && value > 0 {
		if maxLimit > 0 && value > maxLimit {
			value = maxLimit
		}
		limit = value
	}
	offset := 0
	if value, err := strconv.Atoi(q.Get("offset")); err == nil && value >= 0 {
		offset = value
	}
	return limit, offset
}

func parseInt64Query(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseRFC3339Query(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return value, true
}
