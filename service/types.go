package service

import "gochen-llm/entity"

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

type ChatResponse struct {
	Content      string                 `json:"content"`
	FinishReason string                 `json:"finish_reason"`
	Usage        *TokenUsage            `json:"usage,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type ChatChunk struct {
	Content string `json:"content"`
}

type TokenUsage struct {
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

type SafetyResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type RateLimitResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type RateLimitSettings struct {
	PerMinute int `json:"per_minute"`
	Burst     int `json:"burst"`
}

type CostFilter struct {
	Provider string
	Model    string
	UserID   *int64
}

type CostReport struct {
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	TotalTokens      int     `json:"total_tokens"`
}

// CostCalculator 估算成本（简化：按 provider/model 的固定单价）
type CostCalculator interface {
	EstimateCost(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64
}
