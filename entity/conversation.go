package entity

import "time"

// ConversationType 会话类型常量
const (
	// ConversationTypeChat 普通聊天会话
	ConversationTypeChat = "chat"
	// ConversationTypeStory 故事会话（原 StorySegmentRecord）
	ConversationTypeStory = "story"
)

// Conversation 会话实体
// 支持普通聊天和故事生成两种场景
type Conversation struct {
	ID       int64  `gorm:"primaryKey;autoIncrement"`                     // 会话主键 ID
	UserID   int64  `gorm:"not null;index:idx_llm_conversations_user_id"` // 归属用户 ID
	ParentID *int64 `gorm:"index:idx_llm_conversations_parent_id"`        // 父会话 ID（用于会话分支）

	// Type 会话类型：chat（普通聊天）/ story（故事会话）
	Type string `gorm:"size:20;not null;default:'chat';index:idx_llm_conversations_type"`

	Title  string `gorm:"size:200"`                                                             // 会话标题
	Status string `gorm:"size:20;not null;default:'active';index:idx_llm_conversations_status"` // 会话状态，如 active/archived

	// PromptTemplateID 关联的提示词模板（如故事世界）
	PromptTemplateID *int64 `gorm:"index:idx_llm_conversations_prompt_template_id"` // 关联的提示词模板 ID

	MetadataJSON string    `gorm:"type:text"`      // 额外元数据（JSON）
	CreatedAt    time.Time `gorm:"autoCreateTime"` // 创建时间
	UpdatedAt    time.Time `gorm:"autoUpdateTime"` // 更新时间
}

func (Conversation) TableName() string {
	return "llm_conversations"
}

// StoryConversationMetadata 故事会话的元数据结构（存储在 MetadataJSON 中）
// 替代原 StorySegmentRecord 的 Chapter/Scene 等字段
type StoryConversationMetadata struct {
	Chapter string `json:"chapter"` // 当前章节
	Scene   string `json:"scene"`   // 当前场景
}

// Message 消息实体
type Message struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`                         // 消息主键 ID
	ConversationID int64     `gorm:"not null;index:idx_llm_messages_conversation_id"`  // 所属会话 ID
	Role           string    `gorm:"size:20;not null"`                                 // 角色，如 user/system/assistant
	Content        string    `gorm:"type:text;not null"`                               // 消息内容
	Tokens         int       `gorm:""`                                                 // 消息 token 数（可选）
	MetadataJSON   string    `gorm:"type:text"`                                        // 额外元数据（JSON）
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_llm_messages_created_at"` // 创建时间
}

func (Message) TableName() string {
	return "llm_messages"
}

// StoryMessageMetadata 故事消息的元数据结构（存储在 MetadataJSON 中）
// 替代原 StorySegmentRecord 的 HighlightTaskIDsJSON
type StoryMessageMetadata struct {
	HighlightTaskIDs []int64 `json:"highlight_task_ids"` // 高亮的任务 ID
}
