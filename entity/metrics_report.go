package entity

import "time"

type MetricsFilter struct {
	Provider  string
	Model     string
	UserID    *int64
	Status    string
	ABTestID  *int64
	ABVariant string
	StartAt   *time.Time
	EndAt     *time.Time
	Outcome   string
}

type MetricsReport struct {
	TotalCalls          int     `json:"total_calls"`
	SuccessCalls        int     `json:"success_calls"`
	ErrorCalls          int     `json:"error_calls"`
	SuccessRate         float64 `json:"success_rate"`
	ConversionCalls     int     `json:"conversion_calls"`
	ConversionRate      float64 `json:"conversion_rate"`
	TotalRequestTokens  int     `json:"total_request_tokens"`
	TotalResponseTokens int     `json:"total_response_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	TotalCostUSD        float64 `json:"total_cost_usd"`
}

type VariantMetricsReport struct {
	Variant string        `json:"variant"`
	Metrics MetricsReport `json:"metrics"`
}

type ABSignificanceReport struct {
	ABTestID   int64                 `json:"ab_test_id"`
	Outcome    string                `json:"outcome,omitempty"`
	VariantA   *VariantMetricsReport `json:"variant_a,omitempty"`
	VariantB   *VariantMetricsReport `json:"variant_b,omitempty"`
	PValue     float64               `json:"p_value"`
	Confidence float64               `json:"confidence"`
	Winner     string                `json:"winner,omitempty"`
	Lift       float64               `json:"lift,omitempty"`
	Note       string                `json:"note,omitempty"`
}
