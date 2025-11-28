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
	ID       int64  `gorm:"primaryKey;autoIncrement"`
	UserID   int64  `gorm:"not null;index:idx_user_id"`
	ParentID *int64 `gorm:"index:idx_parent_id"`

	// Type 会话类型：chat（普通聊天）/ story（故事会话）
	Type string `gorm:"size:20;not null;default:'chat';index:idx_type"`

	Title  string `gorm:"size:200"`
	Status string `gorm:"size:20;not null;default:'active';index:idx_status"`

	// PromptTemplateID 关联的提示词模板（如故事世界）
	PromptTemplateID *int64 `gorm:"index:idx_prompt_template_id"`

	MetadataJSON string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
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
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	ConversationID int64     `gorm:"not null;index:idx_conversation_id"`
	Role           string    `gorm:"size:20;not null"`
	Content        string    `gorm:"type:text;not null"`
	Tokens         int       `gorm:""`
	MetadataJSON   string    `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_created_at"`
}

func (Message) TableName() string {
	return "llm_messages"
}

// StoryMessageMetadata 故事消息的元数据结构（存储在 MetadataJSON 中）
// 替代原 StorySegmentRecord 的 HighlightTaskIDsJSON
type StoryMessageMetadata struct {
	HighlightTaskIDs []int64 `json:"highlight_task_ids"` // 高亮的任务 ID
}
