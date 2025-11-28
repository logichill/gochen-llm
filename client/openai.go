package client

import (
	"context"
	"encoding/json"
	"fmt"
)

type openAIClient struct {
	*httpClient
}

func newOpenAIClient(cfg *Config) *openAIClient {
	return &openAIClient{httpClient: newHTTPClient(cfg)}
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float32             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}

func (c *openAIClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API Key 未配置")
	}

	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	url := fmt.Sprintf("%s/v1/chat/completions", baseURL)

	var messages []openAIChatMessage
	if req.System != "" {
		messages = append(messages, openAIChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		messages = append(messages, openAIChatMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	body := openAIChatRequest{
		Model:       c.cfg.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	return c.doRequest(ctx, url, body, func(respBytes []byte) (*ChatResponse, error) {
		var resp openAIChatResponse
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			return nil, fmt.Errorf("解析 OpenAI 响应失败: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("OpenAI 响应中不包含 choices")
		}
		return &ChatResponse{Content: resp.Choices[0].Message.Content}, nil
	})
}
