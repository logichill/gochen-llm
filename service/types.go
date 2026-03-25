package service

import "gochen-llm/entity"

// Message 定义消息结构。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 通用聊天请求
type ChatRequest struct {
	UserID      int64                  `json:"user_id"`
	System      string                 `json:"system"`
	Messages    []Message              `json:"messages"`
	Temperature float32                `json:"temperature"`
	MaxTokens   int                    `json:"max_tokens"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PromptChatRequest 基于提示词的聊天请求
type PromptChatRequest struct {
	UserID        int64                  `json:"user_id"`
	PromptName    string                 `json:"prompt_name"`
	PromptScope   entity.PromptScope     `json:"prompt_scope"`
	PromptScopeID int64                  `json:"prompt_scope_id"`
	ABTestID      int64                  `json:"ab_test_id,omitempty"`
	Variables     map[string]interface{} `json:"variables"`
	Messages      []Message              `json:"messages"`
	Temperature   float32                `json:"temperature"`
	MaxTokens     int                    `json:"max_tokens"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ChatResponse 定义对话响应结果。
type ChatResponse struct {
	Content      string                 `json:"content"`
	FinishReason string                 `json:"finish_reason"`
	Usage        *TokenUsage            `json:"usage,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ChatChunk 定义对话片段。
type ChatChunk struct {
	Content string `json:"content"`
}

// TokenUsage 定义令牌用量。
type TokenUsage struct {
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

// SafetyResult 定义安全处理结果。
type SafetyResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// RateLimitResult 定义速率限额处理结果。
type RateLimitResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// RateLimitSettings 定义速率限额设置。
type RateLimitSettings struct {
	PerMinute int `json:"per_minute"`
	Burst     int `json:"burst"`
}

// CostFilter 定义成本过滤条件。
type CostFilter struct {
	Provider string
	Model    string
	UserID   *int64
}

// CostReport 定义成本报告。
type CostReport struct {
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	TotalTokens      int     `json:"total_tokens"`
}

// ICostCalculator 抽象成本计算器能力接口。
type ICostCalculator interface {
	EstimateCost(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64
}
