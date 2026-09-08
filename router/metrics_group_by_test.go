package router

import (
	"gochen-llm/entity"
	"gochen/httpx"
	"testing"
)

func TestMetricsAggregateAcceptsVariantGrouping(t *testing.T) {
	ctx := newRouterTestContext("GET", "/admin/llm/metrics/agg?group_by=variant&filter=ab_test_id:eq:1")
	if err := writeLLMMetricsAggregate(ctx, &stubMetricsRepo{}); err != nil {
		t.Fatal(err)
	}
	response, ok := ctx.jsonObj.(*httpx.ResponseMessage)
	if !ok {
		t.Fatalf("response=%T", ctx.jsonObj)
	}
	data, ok := response.Data.(map[string]any)
	if !ok {
		t.Fatalf("data=%#v", response.Data)
	}
	variants, ok := data["variants"].([]*entity.VariantMetricsReport)
	if !ok || len(variants) != 1 || variants[0].Variant != "A" {
		t.Fatalf("variants=%#v", data["variants"])
	}
	if err := rejectUnknownQueryParams(ctx); err == nil {
		t.Fatal("group_by must remain rejected on routes without grouping")
	}
	unknown := newRouterTestContext("GET", "/admin/llm/metrics/agg?group_by=variant&user_id=1")
	if err := rejectUnknownQueryParams(unknown, "group_by"); err == nil {
		t.Fatal("legacy user_id unexpectedly accepted")
	}
}
