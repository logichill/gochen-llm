package client

import (
	"context"
	"fmt"
	"time"
)

type Provider string

const (
	ProviderOpenAI           Provider = "openai"
	ProviderOpenAICompatible Provider = "openai_compatible"
	ProviderAnthropic        Provider = "anthropic"
	ProviderGemini           Provider = "gemini"
	ProviderMock             Provider = "mock"
)

type Config struct {
	Provider          Provider
	APIKey            string
	BaseURL           string
	Model             string
	Timeout           time.Duration
	AnthropicVersion  string
	GeminiAPIEndpoint string
}

type ChatMessage struct {
	Role    string
	Content string
}

type ChatRequest struct {
	System      string
	Messages    []ChatMessage
	Temperature float32
	MaxTokens   int
}

type ChatResponse struct {
	Content string
}

type Client interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

func NewClient(cfg *Config) (Client, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, fmt.Errorf("llm.Config 不能为空且 provider 必须设置")
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
		return nil, fmt.Errorf("不支持的 LLM provider: %s", cfg.Provider)
	}
}
