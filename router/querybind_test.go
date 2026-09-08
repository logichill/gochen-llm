package router

import (
	"testing"
	"time"

	"gochen/app/query"
)

func TestDecodeMetricsFilter_UsesDefaultBindingForInitialisms(t *testing.T) {
	start := time.Now().UTC().Round(0)
	end := start.Add(2 * time.Hour)

	filter, err := decodeMetricsFilter(query.QueryFilters{
		"provider": {{
			Op:    query.FilterOpEq,
			Value: query.StringValue("openai"),
		}},
		"ab_test_id": {{
			Op:    query.FilterOpEq,
			Value: query.IntValue(7),
		}},
		"user_id": {{
			Op:    query.FilterOpEq,
			Value: query.IntValue(11),
		}},
		"created_at": {
			{
				Op:    query.FilterOpGte,
				Value: query.TimeValue(start),
			},
			{
				Op:    query.FilterOpLte,
				Value: query.TimeValue(end),
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeMetricsFilter returned error: %v", err)
	}

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

	filter, err := decodeAuditLogFilter(query.QueryFilters{
		"user_id": {{
			Op:    query.FilterOpEq,
			Value: query.IntValue(99),
		}},
		"resource_type": {{
			Op:    query.FilterOpEq,
			Value: query.StringValue("prompt"),
		}},
		"created_at": {{
			Op:    query.FilterOpGte,
			Value: query.TimeValue(start),
		}},
	})
	if err != nil {
		t.Fatalf("decodeAuditLogFilter returned error: %v", err)
	}

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
