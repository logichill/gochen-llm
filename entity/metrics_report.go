package entity

import "time"

// MetricsFilter 定义统计查询时可用的筛选条件
// 支持按 Provider/模型/用户/A-B 实验/时间范围等维度过滤指标数据。
type MetricsFilter struct {
	Provider  string     // Provider 名称
	Model     string     // 模型名称
	UserID    *int64     // 用户 ID（可选）
	Status    string     // 调用状态过滤，如 success/error
	ABTestID  *int64     // A/B 测试 ID（可选）
	ABVariant string     // A/B 变体标识，如 "A"/"B"
	StartAt   *time.Time // 起始时间（可选）
	EndAt     *time.Time // 结束时间（可选）
	Outcome   string     // 目标事件过滤，如 conversion
}

// MetricsReport 汇总后的核心指标统计结果
// 聚合调用次数、成功率、转化率、token 用量、平均时延与总成本等。
type MetricsReport struct {
	TotalCalls          int     `json:"total_calls"`           // 总调用次数
	SuccessCalls        int     `json:"success_calls"`         // 成功调用次数
	ErrorCalls          int     `json:"error_calls"`           // 失败调用次数
	SuccessRate         float64 `json:"success_rate"`          // 成功率
	ConversionCalls     int     `json:"conversion_calls"`      // 产生目标事件的调用次数
	ConversionRate      float64 `json:"conversion_rate"`       // 转化率
	TotalRequestTokens  int     `json:"total_request_tokens"`  // 请求 token 总数
	TotalResponseTokens int     `json:"total_response_tokens"` // 响应 token 总数
	TotalTokens         int     `json:"total_tokens"`          // token 总数
	AvgLatencyMs        float64 `json:"avg_latency_ms"`        // 平均延迟（毫秒）
	TotalCostUSD        float64 `json:"total_cost_usd"`        // 总成本（USD）
}

// VariantMetricsReport 表示单个实验变体的指标报告
// 一般用于 A/B 测试中对比不同模板或配置的效果。
type VariantMetricsReport struct {
	Variant string        `json:"variant"` // 变体标识，如 "A"/"B"
	Metrics MetricsReport `json:"metrics"` // 对应变体的汇总指标
}

// ABSignificanceReport 表示 A/B 测试的显著性分析结果
// 包含各变体指标、p 值、置信度、胜出方与提升比例等信息。
type ABSignificanceReport struct {
	ABTestID   int64                 `json:"ab_test_id"`          // A/B 测试 ID
	Outcome    string                `json:"outcome,omitempty"`   // 关注的结果事件名称
	VariantA   *VariantMetricsReport `json:"variant_a,omitempty"` // 变体 A 指标
	VariantB   *VariantMetricsReport `json:"variant_b,omitempty"` // 变体 B 指标
	PValue     float64               `json:"p_value"`             // p 值
	Confidence float64               `json:"confidence"`          // 置信度（0-1）
	Winner     string                `json:"winner,omitempty"`    // 胜出变体标识
	Lift       float64               `json:"lift,omitempty"`      // 指标提升比例
	Note       string                `json:"note,omitempty"`      // 备注说明
}
