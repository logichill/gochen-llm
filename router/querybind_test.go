package router

import (
	"testing"
	"time"

	dataquery "gochen/db/query"
)

func TestDecodeMetricsFilter_UsesDefaultBindingForInitialisms(t *testing.T) {
	start := time.Now().UTC().Round(0)
	end := start.Add(2 * time.Hour)

	filter := decodeMetricsFilter(dataquery.QueryFilters{
		"provider": {{
			Op:    dataquery.FilterOpEq,
			Value: dataquery.StringValue("openai"),
		}},
		"ab_test_id": {{
			Op:    dataquery.FilterOpEq,
			Value: dataquery.IntValue(7),
		}},
		"user_id": {{
			Op:    dataquery.FilterOpEq,
			Value: dataquery.IntValue(11),
		}},
		"created_at": {
			{
				Op:    dataquery.FilterOpGte,
				Value: dataquery.TimeValue(start),
			},
			{
				Op:    dataquery.FilterOpLte,
				Value: dataquery.TimeValue(end),
			},
		},
	})

	if filter.Provider != "openai" {
		t.Fatalf("expected provider to bind, got %+v", filter)
	}
	if filter.ABTestID == nil || *filter.ABTestID != 7 {
		t.Fatalf("expected ab_test_id to bind, got %+v", filter.ABTestID)
	}
	if filter.UserID == nil || *filter.UserID != 11 {
		t.Fatalf("expected user_id to bind, got %+v", filter.UserID)
	}
	if filter.StartAt == nil || !filter.StartAt.Equal(start) {
		t.Fatalf("expected start time to bind, got %+v", filter.StartAt)
	}
	if filter.EndAt == nil || !filter.EndAt.Equal(end) {
		t.Fatalf("expected end time to bind, got %+v", filter.EndAt)
	}
}

func TestDecodeAuditLogFilter_UsesDefaultBindingForInitialisms(t *testing.T) {
	start := time.Now().UTC().Round(0)

	filter := decodeAuditLogFilter(dataquery.QueryFilters{
		"user_id": {{
			Op:    dataquery.FilterOpEq,
			Value: dataquery.IntValue(99),
		}},
		"resource_type": {{
			Op:    dataquery.FilterOpEq,
			Value: dataquery.StringValue("prompt"),
		}},
		"created_at": {{
			Op:    dataquery.FilterOpGte,
			Value: dataquery.TimeValue(start),
		}},
	})

	if filter.UserID == nil || *filter.UserID != 99 {
		t.Fatalf("expected user_id to bind, got %+v", filter.UserID)
	}
	if filter.ResourceType != "prompt" {
		t.Fatalf("expected resource type to bind, got %+v", filter)
	}
	if filter.StartAt == nil || !filter.StartAt.Equal(start) {
		t.Fatalf("expected start time to bind, got %+v", filter.StartAt)
	}
}

func TestParseMetricsAggregateGroupBy(t *testing.T) {
	ctx := newRouterTestContext("GET", "/admin/llm/metrics?group_by=variant")

	groupBy, err := parseMetricsAggregateGroupBy(ctx)
	if err != nil {
		t.Fatalf("parseMetricsAggregateGroupBy returned error: %v", err)
	}
	if groupBy != "variant" {
		t.Fatalf("expected group_by=variant, got %q", groupBy)
	}
}
