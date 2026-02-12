package entity

import "time"

// ProviderConfig 持久化单个 LLM 提供商端点配置，支持多源与运行时重载。
type ProviderConfig struct {
	ID int64 `gorm:"primaryKey;autoIncrement"` // 主键 ID

	// 便于运维识别的名称（如 "primary-openai"、"backup-gemini"）
	Name string `gorm:"size:100;not null;default:''"` // 端点名称

	// Provider 类型：openai/openai_compatible/anthropic/gemini/mock
	Provider string `gorm:"size:50;not null"` // Provider 类型

	// 访问密钥，真实环境可配合加密/脱敏
	APIKey string `gorm:"size:500;not null"` // 访问密钥

	// 自定义 BaseURL（如兼容网关）
	BaseURL string `gorm:"size:200"` // 自定义 BaseURL

	// 模型名称
	Model string `gorm:"size:100;not null"` // 模型名称

	// 是否启用此端点
	Enabled bool `gorm:"not null;default:true"` // 是否启用

	// 端点优先级，数值越小优先级越高（用于主备）
	Priority int `gorm:"not null;default:100"` // 端点优先级（越小越优先）

	// 同一优先级组内的权重，用于加权分流（数值越大流量占比越高）
	Weight int `gorm:"not null;default:100"` // 同优先级内的流量权重

	// 单次请求超时时间（秒）
	TimeoutSeconds int `gorm:"not null;default:30"` // 请求超时时间（秒）

	// 调用失败后进入冷却的时间（秒），在冷却期内不会被选中
	CooldownSeconds int `gorm:"not null;default:30"` // 熔断冷却时间（秒）

	AnthropicVersion  string `gorm:"size:50"`  // Anthropic API 版本号
	GeminiAPIEndpoint string `gorm:"size:200"` // Gemini 特定 API 端点

	// 单价（USD 每 1000 tokens），可选，未设置则使用全局默认或成本表兜底
	InputPricePer1k  float64 `gorm:"type:decimal(10,6)"` // 输入端价格（每 1k tokens）
	OutputPricePer1k float64 `gorm:"type:decimal(10,6)"` // 输出端价格（每 1k tokens）

	// 健康探测与熔断配置
	HealthPingURL        string `gorm:"size:200"`           // 健康检查 URL（为空则跳过 ping）
	HealthTimeoutSeconds int    `gorm:"not null;default:5"` // 健康检查超时时间（秒）
	MaxErrorStreak       int    `gorm:"not null;default:3"` // 连续错误阈值，触发熔断
	RecoverySuccesses    int    `gorm:"not null;default:2"` // 连续成功次数，解除熔断

	// 限流配置（令牌桶）：0 表示不限制
	RateLimitPerMin int `gorm:"not null;default:0"` // 每分钟令牌发放速率
	RateLimitBurst  int `gorm:"not null;default:0"` // 桶容量（突发上限）

	CreatedAt time.Time `gorm:"autoCreateTime"` // 创建时间
	UpdatedAt time.Time `gorm:"autoUpdateTime"` // 更新时间
}

func (ProviderConfig) TableName() string {
	return "llm_provider_configs"
}

// ProviderPricing 仅用于后台调整单价，避免误改敏感字段
type ProviderPricing struct {
	ID               int64   `json:"id"`                  // ProviderConfig ID
	InputPricePer1k  float64 `json:"input_price_per_1k"`  // 输入端价格（每 1k tokens）
	OutputPricePer1k float64 `json:"output_price_per_1k"` // 输出端价格（每 1k tokens）
}
