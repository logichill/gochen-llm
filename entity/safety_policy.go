package entity

import "time"

// SafetyPolicy 表示系统级的大模型安全策略配置
type SafetyPolicy struct {
	ID int64 `gorm:"primaryKey;autoIncrement"` // 主键 ID

	// 是否启用安全策略
	Enabled bool `gorm:"not null;default:true"` // 是否启用安全策略

	// 全局 System Prompt，优先级高于业务提示词
	GlobalSystemPrompt string `gorm:"type:text"` // 全局 System Prompt

	// 屏蔽类别（JSON 数组），预留给不同 Provider 的安全设置适配
	BlockedCategoriesJSON string `gorm:"type:text"` // 屏蔽类别配置 JSON

	// 屏蔽关键词（JSON 数组），用于输入/输出的简单文本过滤
	BlockedKeywordsJSON string `gorm:"type:text"` // 屏蔽关键词配置 JSON

	// 生成内容的最大长度（字符数，0 表示不限制）
	MaxContentLength int `gorm:"not null;default:0"` // 最大内容长度限制

	// 日志级别：none / summary / full_violation 等（首版仅记录占位）
	LogLevel string `gorm:"size:20;not null;default:'none'"` // 日志级别

	CreatedAt time.Time `gorm:"autoCreateTime"` // 创建时间
	UpdatedAt time.Time `gorm:"autoUpdateTime"` // 更新时间
}

func (SafetyPolicy) TableName() string {
	return "llm_safety_policies"
}
