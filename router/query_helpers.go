package router

import (
	"math"
	"slices"
	"sort"
	"strings"

	"gochen-llm/repo"
	"gochen-runtime/api/rest"
	"gochen/errors"
	"gochen/httpx"
)

// queryDSLParams 是 rest 查询 DSL 认识的全部参数名；其余一律视为已废弃的扁平参数。
var queryDSLParams = map[string]struct{}{
	"filter": {}, "sorts": {}, "fields": {}, "page": {}, "size": {}, "page_size": {},
}

// rejectUnknownQueryParams 拒绝 DSL 之外的查询参数。
//
// rest 解析器只读取自己认识的键，多余参数会被静默忽略；对审计/指标查询而言，
// 把 user_id=5 忽略掉意味着返回全量数据而不是报错，这里必须显式拒绝。
func rejectUnknownQueryParams(ctx httpx.IContext, routeParams ...string) error {
	var unknown []string
	for key := range ctx.QueryParams() {
		if _, ok := queryDSLParams[key]; !ok && !slices.Contains(routeParams, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return errors.NewCode(errors.InvalidInput, "unsupported query parameters; use filter=field:op:value").
		WithContext("params", strings.Join(unknown, ","))
}

func writePaginatedList(ctx httpx.IContext, list any, total int64, page, size int) error {
	totalPages := 0
	if size > 0 && total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(size)))
	}
	return httpx.WriteSuccess(ctx, map[string]any{
		"data":        list,
		"total":       total,
		"page":        page,
		"size":        size,
		"total_pages": totalPages,
		"has_next":    totalPages > 0 && page < totalPages,
		"has_prev":    page > 1,
	})
}

func writeLLMMetricsAggregate(ctx httpx.IContext, metrics repo.IMetricsRepo) error {
	if metrics == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM metrics repo 未配置")
	}

	if err := rejectUnknownQueryParams(ctx, "group_by"); err != nil {
		return httpx.WriteError(ctx, err)
	}
	params, err := rest.ParseQueryParams(ctx, llmMetricsQueryConfig)
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
		rows, err := metrics.AggregateByVariant(ctx.RequestContext(), filter)
		if err != nil {
			return httpx.WriteError(ctx, err)
		}
		return httpx.WriteSuccess(ctx, map[string]any{"variants": rows})
	}

	report, err := metrics.Aggregate(ctx.RequestContext(), filter)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return httpx.WriteSuccess(ctx, map[string]any{"report": report})
}

func writeLLMMetricsList(ctx httpx.IContext, metrics repo.IMetricsRepo) error {
	if metrics == nil {
		return httpx.WriteErrorCode(ctx, errors.Internal, "LLM metrics repo 未配置")
	}

	if err := rejectUnknownQueryParams(ctx); err != nil {
		return httpx.WriteError(ctx, err)
	}
	opts, err := rest.ParsePaginationOptions(ctx, llmMetricsQueryConfig)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	filter, err := decodeMetricsFilter(opts.Filters)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	limit, offset := opts.Size, opts.Offset()
	list, total, err := metrics.List(ctx.RequestContext(), filter, limit, offset)
	if err != nil {
		return httpx.WriteError(ctx, err)
	}
	return writePaginatedList(ctx, list, total, opts.Page, opts.Size)
}
