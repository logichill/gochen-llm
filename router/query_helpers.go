package router

import (
	"math"

	"gochen-llm/repo"
	"gochen/api/rest"
	"gochen/errors"
	"gochen/httpx"
)

var llmMetricsLegacyQueryParams = []string{
	"provider", "model", "status", "ab_variant", "outcome", "conversion_type", "ab_test_id", "user_id", "start", "end",
}

var llmMetricsLegacyListQueryParams = appendLegacyQueryParams(llmMetricsLegacyQueryParams, "limit", "offset")

var llmAuditLogLegacyListQueryParams = []string{
	"user_id", "action", "status", "resource_type", "start", "end", "limit", "offset",
}

func rejectLLMMetricsLegacyQueryParams(ctx httpx.IContext) error {
	return rest.RejectLegacyQueryParams(ctx, llmMetricsLegacyQueryParams...)
}

func rejectLLMMetricsLegacyListQueryParams(ctx httpx.IContext) error {
	return rest.RejectLegacyQueryParams(ctx, llmMetricsLegacyListQueryParams...)
}

func rejectLLMAuditLogLegacyListQueryParams(ctx httpx.IContext) error {
	return rest.RejectLegacyQueryParams(ctx, llmAuditLogLegacyListQueryParams...)
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
	if err := rejectLLMMetricsLegacyQueryParams(ctx); err != nil {
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
	if err := rejectLLMMetricsLegacyListQueryParams(ctx); err != nil {
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

func appendLegacyQueryParams(base []string, extra ...string) []string {
	out := make([]string, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}
