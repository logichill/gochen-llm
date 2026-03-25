package client

import (
	"context"
	"time"

	"gochen/errorx"
)

// Provider 定义提供者。
type Provider string

const (
	ProviderOpenAI           Provider = "openai"
	ProviderOpenAICompatible Provider = "openai_compatible"
	ProviderAnthropic        Provider = "anthropic"
	ProviderGemini           Provider = "gemini"
	ProviderMock             Provider = "mock"
)

// Config 定义相关配置。
type Config struct {
	Provider          Provider
	APIKey            string
	BaseURL           string
	Model             string
	Timeout           time.Duration
	AnthropicVersion  string
	GeminiAPIEndpoint string
}

// ChatMessage 定义对话消息结构。
type ChatMessage struct {
	Role    string
	Content string
}

// ChatRequest 定义对话请求参数。
type ChatRequest struct {
	System      string
	Messages    []ChatMessage
	Temperature float32
	MaxTokens   int
}

// ChatResponse 定义对话响应结果。
type ChatResponse struct {
	Content string
}

// IClient 定义客户端能力接口。
type IClient interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// NewClient 创建客户端。
func NewClient(cfg *Config) (IClient, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, errorx.New(errorx.InvalidInput, "llm.Config 不能为空且 provider 必须设置")
	}
	switch cfg.Provider {
	case ProviderOpenAI, ProviderOpenAICompatible:
		return newOpenAIClient(cfg), nil
	case ProviderAnthropic:
		return newAnthropicClient(cfg), nil
	case ProviderGemini:
		return newGeminiClient(cfg), nil
	case ProviderMock:
		return &mockClient{}, nil
	default:
		return nil, errorx.New(errorx.Unsupported, "不支持的 LLM provider").WithContext("provider", string(cfg.Provider))
	}
}
