package entity

import "time"

type AuditLog struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	UserID       int64     `gorm:"index:idx_user_id"`
	Action       string    `gorm:"size:50;not null;index:idx_action"`
	ResourceType string    `gorm:"size:50"`
	ResourceID   int64     `gorm:""`
	RequestJSON  string    `gorm:"type:text"`
	ResponseJSON string    `gorm:"type:text"`
	IPAddress    string    `gorm:"size:50"`
	UserAgent    string    `gorm:"type:text"`
	Status       string    `gorm:"size:20"`
	ErrorMessage string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_created_at"`
}

func (AuditLog) TableName() string {
	return "llm_audit_logs"
}

type Metrics struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	Provider       string    `gorm:"size:50;not null;index:idx_provider"`
	Model          string    `gorm:"size:100"`
	UserID         int64     `gorm:"index:idx_user_id"`
	ABTestID       int64     `gorm:"index:idx_ab_test_id"`
	ABVariant      string    `gorm:"size:5"`
	PromptTemplate int64     `gorm:"index:idx_prompt_template_id"`
	RequestTokens  int       `gorm:""`
	ResponseTokens int       `gorm:""`
	TotalTokens    int       `gorm:""`
	LatencyMs      int       `gorm:""`
	CostUSD        float64   `gorm:"type:decimal(10,6)"`
	Status         string    `gorm:"size:20"`
	ErrorType      string    `gorm:"size:50"`
	Outcome        string    `gorm:"size:50"` // 额外事件，如 conversion
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_created_at"`
}

func (Metrics) TableName() string {
	return "llm_metrics"
}

type RateLimit struct {
	ID                int64     `gorm:"primaryKey;autoIncrement"`
	UserID            int64     `gorm:"not null"`
	ResourceType      string    `gorm:"size:50;not null"`
	WindowStart       time.Time `gorm:"not null"`
	WindowSizeSeconds int       `gorm:"not null"`
	RequestCount      int       `gorm:"not null;default:0"`
	TokenCount        int       `gorm:"not null;default:0"`
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

func (RateLimit) TableName() string {
	return "llm_rate_limits"
}
