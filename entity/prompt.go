package entity

import "time"

// PromptScope 定义提示词模板的作用域维度
// 用于区分全局、组织、项目与用户级的配置边界。
type PromptScope string

const (
	PromptScopeGlobal  PromptScope = "global"
	PromptScopeOrg     PromptScope = "org"
	PromptScopeProject PromptScope = "project"
	PromptScopeUser    PromptScope = "user"
)

// PromptTemplate 提示词模板定义
// 用于存储和管理 LLM 的 Prompt 模板，支持多级作用域（Scope）和版本控制。
// 核心属性包括作用域（Scope/ScopeID）、内容（Content）和变量定义（VariablesJSON）。
type PromptTemplate struct {
	// ID 唯一标识符
	ID int64 `gorm:"primaryKey;autoIncrement"`

	// Name 模板名称
	// 在同一作用域（Scope + ScopeID）下应唯一。
	// 用于在代码中引用特定的 Prompt。
	Name string `gorm:"size:200;not null;index:idx_llm_prompt_templates_name_scope,priority:1"`

	// Scope 作用域类型
	// 取值：global (全局), org (组织), project (项目), user (用户)
	// 决定了模板的可见性和生效范围。
	Scope PromptScope `gorm:"size:20;not null;index:idx_llm_prompt_templates_name_scope,priority:2"`

	// ScopeID 作用域实体 ID
	// 根据 Scope 的不同，分别对应 0 (Global), OrgID, ProjectID, UserID。
	ScopeID int64 `gorm:"not null;default:0;index:idx_llm_prompt_templates_name_scope,priority:3"`

	// Category 分类
	// 用于 UI 分组或业务逻辑分类（如 "chat", "summary", "extraction"）。
	Category string `gorm:"size:50;not null"`

	// Content 模板内容
	// 实际的 Prompt 文本，支持 Go Template 语法或自定义变量占位符。
	Content string `gorm:"type:text;not null"`

	// VariablesJSON 变量定义
	// 存储 JSON 数组，描述 Content 中使用的变量及其元数据（类型、描述、默认值）。
	// 示例：[{"name": "input", "type": "string", "description": "用户输入"}]
	VariablesJSON string `gorm:"type:text"`

	// Version 当前版本号
	// 每次更新 Content 或 VariablesJSON 时递增。
	Version int `gorm:"not null;default:1"`

	// ParentID 父模板 ID
	// 用于实现模板继承或分叉（Fork）。
	// 如果不为 nil，表示该模板是基于 ParentID 模板创建的。
	ParentID *int64

	// Priority 优先级
	// 数值越小优先级越高（或越低，需结合业务逻辑确认，通常用于同名模板在不同作用域的覆盖策略）。
	// 默认 100。
	Priority int `gorm:"not null;default:100"`

	// Enabled 是否启用
	// false 表示逻辑删除或暂时禁用。
	Enabled bool `gorm:"not null;default:true;index:idx_llm_prompt_templates_enabled"`

	// TagsJSON 标签集合
	// 存储 JSON 字符串数组，用于灵活检索和分类。
	TagsJSON string `gorm:"type:text"`

	// MetadataJSON 扩展元数据
	// 存储额外的配置信息，如推荐的模型参数（Temperature, MaxTokens 等）。
	MetadataJSON string `gorm:"type:text"`

	CreatedAt time.Time `gorm:"autoCreateTime"` // 创建时间
	UpdatedAt time.Time `gorm:"autoUpdateTime"` // 更新时间
}

// TableName 设置表名为 llm_prompt_templates
func (PromptTemplate) TableName() string {
	return "llm_prompt_templates"
}

// PromptVersion 提示词版本记录
type PromptVersion struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`                                           // 版本记录主键 ID
	TemplateID int64  `gorm:"index:idx_llm_prompt_versions_template_version,priority:1"`          // 关联的模板 ID
	Version    int    `gorm:"not null;index:idx_llm_prompt_versions_template_version,priority:2"` // 版本号
	Content    string `gorm:"type:text;not null"`                                                 // 版本对应的模板内容

	VariablesJSON string    `gorm:"type:text"` // 版本对应的变量定义 JSON
	ChangeLog     string    `gorm:"type:text"` // 版本变更说明
	CreatedBy     int64     // 创建人用户 ID
	CreatedAt     time.Time `gorm:"autoCreateTime"` // 创建时间
}

func (PromptVersion) TableName() string {
	return "llm_prompt_versions"
}

// ABTest 提示词 A/B 测试配置
type ABTest struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`                                         // A/B 测试主键 ID
	Name         string    `gorm:"size:200;not null"`                                                // 测试名称
	TemplateAID  int64     `gorm:"not null"`                                                         // 变体 A 使用的模板 ID
	TemplateBID  int64     `gorm:"not null"`                                                         // 变体 B 使用的模板 ID
	TrafficSplit int       `gorm:"not null;default:50"`                                              // 流量分配比例（A 百分比）
	Status       string    `gorm:"size:20;not null;default:'running';index:idx_llm_ab_tests_status"` // 状态：running/stopped 等
	StartAt      time.Time `gorm:""`                                                                 // 开始时间
	EndAt        time.Time `gorm:""`                                                                 // 结束时间
	ResultJSON   string    `gorm:"type:text"`                                                        // 统计与分析结果 JSON
	CreatedAt    time.Time `gorm:"autoCreateTime"`                                                   // 创建时间
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`                                                   // 更新时间
}

func (ABTest) TableName() string {
	return "llm_ab_tests"
}

// PromptCategory 预定义的提示词分类常量
const (
	// PromptCategoryStoryWorld 故事世界提示词（原 StoryWorld）
	PromptCategoryStoryWorld = "story_world"
	// PromptCategoryStoryGeneration 故事生成提示词
	PromptCategoryStoryGeneration = "story_generation"
	// PromptCategoryUserPreferences 用户偏好配置（原 GrowthProfile）
	PromptCategoryUserPreferences = "user_preferences"
	// PromptCategoryChat 通用聊天
	PromptCategoryChat = "chat"
	// PromptCategorySummary 摘要生成
	PromptCategorySummary = "summary"
)

// StoryWorldMetadata 故事世界的元数据结构（存储在 MetadataJSON 中）
type StoryWorldMetadata struct {
	DisplayName string `json:"display_name"` // 展示名称，如 "森林大冒险"
	WorldKey    string `json:"world_key"`    // 唯一标识，如 "forest"
	Theme       string `json:"theme"`        // 主题标签
	Description string `json:"description"`  // 世界观描述
	Config      any    `json:"config"`       // NPC/地点等额外配置
	IconURL     string `json:"icon_url"`     // 图标 URL
	SortOrder   int    `json:"sort_order"`   // 排序权重
}

// UserPreferencesMetadata 用户偏好的元数据结构（原 GrowthProfile，存储在 MetadataJSON 中）
type UserPreferencesMetadata struct {
	Age         int      `json:"age"`          // 年龄
	Grade       string   `json:"grade"`        // 年级或学段
	FocusAreas  []string `json:"focus_areas"`  // 希望强化的方向
	AvoidThemes []string `json:"avoid_themes"` // 希望回避的主题
}
