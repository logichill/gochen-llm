package entity

import "time"

// AuditLog 表示单次 LLM 调用的审计日志记录
// 主要用于安全审计与问题排查，记录用户、资源、请求与响应等信息。
type AuditLog struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`                           // 主键 ID
	UserID       int64     `gorm:"index:idx_llm_audit_logs_user_id"`                   // 触发调用的用户 ID
	Action       string    `gorm:"size:50;not null;index:idx_llm_audit_logs_action"`   // 操作类型，如 "chat"、"admin.update_config"
	ResourceType string    `gorm:"size:50"`                                            // 资源类型，如 "prompt"、"provider_config"
	ResourceID   int64     `gorm:""`                                                   // 资源 ID
	RequestJSON  string    `gorm:"type:text"`                                          // 请求内容序列化（含参数、上下文）
	ResponseJSON string    `gorm:"type:text"`                                          // 响应内容序列化
	IPAddress    string    `gorm:"size:50"`                                            // 客户端 IP 地址
	UserAgent    string    `gorm:"type:text"`                                          // 客户端 User-Agent
	Status       string    `gorm:"size:20"`                                            // 结果状态，如 "success"、"error"
	ErrorMessage string    `gorm:"type:text"`                                          // 错误信息（如有）
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_llm_audit_logs_created_at"` // 创建时间
}

func (AuditLog) TableName() string {
	return "llm_audit_logs"
}

// Metrics 表示 LLM 调用的指标统计记录
// 用于存储单次调用的 Provider、模型、token 用量、时延、成本与结果状态等信息。
type Metrics struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`                        // 主键 ID
	Provider       string    `gorm:"size:50;not null;index:idx_llm_metrics_provider"` // Provider 名称
	Model          string    `gorm:"size:100"`                                        // 模型名称
	UserID         int64     `gorm:"index:idx_llm_metrics_user_id"`                   // 用户 ID
	ABTestID       int64     `gorm:"index:idx_llm_metrics_ab_test_id"`                // A/B 测试 ID
	ABVariant      string    `gorm:"size:5"`                                          // A/B 测试变体标识，如 "A"/"B"
	PromptTemplate int64     `gorm:"index:idx_llm_metrics_prompt_template_id"`        // 使用的提示词模板 ID
	RequestTokens  int       `gorm:""`                                                // 请求 token 数
	ResponseTokens int       `gorm:""`                                                // 响应 token 数
	TotalTokens    int       `gorm:""`                                                // 总 token 数
	LatencyMs      int       `gorm:""`                                                // 调用耗时（毫秒）
	CostUSD        float64   `gorm:"type:decimal(10,6)"`                              // 估算花费（USD）
	Status         string    `gorm:"size:20"`                                         // 调用状态，如 "success"/"error"
	ErrorType      string    `gorm:"size:50"`                                         // 错误类型，如超时、配额不足等
	Outcome        string    `gorm:"size:50"`                                         // 额外事件，如 conversion
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_llm_metrics_created_at"` // 创建时间
}

func (Metrics) TableName() string {
	return "llm_metrics"
}

// RateLimit 表示在特定时间窗口内的限流统计记录
// 按用户与资源类型维度记录请求次数与已消费令牌数，用于实现令牌桶限流策略。
type RateLimit struct {
	ID                int64     `gorm:"primaryKey;autoIncrement"` // 主键 ID
	UserID            int64     `gorm:"not null"`                 // 用户 ID
	ResourceType      string    `gorm:"size:50;not null"`         // 资源类型，如 "chat"、"admin"
	WindowStart       time.Time `gorm:"not null"`                 // 限流窗口起始时间
	WindowSizeSeconds int       `gorm:"not null"`                 // 限流窗口大小（秒）
	RequestCount      int       `gorm:"not null;default:0"`       // 窗口内请求次数
	TokenCount        int       `gorm:"not null;default:0"`       // 窗口内已消费 token 数
	CreatedAt         time.Time `gorm:"autoCreateTime"`           // 记录创建时间
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`           // 记录更新时间
}

func (RateLimit) TableName() string {
	return "llm_rate_limits"
}
