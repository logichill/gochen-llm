package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type anthropicClient struct {
	*httpClient
}

func newAnthropicClient(cfg *Config) *anthropicClient {
	return &anthropicClient{httpClient: newHTTPClient(cfg)}
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicTextContent `json:"content"`
}

type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicChatRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float32            `json:"temperature,omitempty"`
}

type anthropicChatResponse struct {
	Content []anthropicTextContent `json:"content"`
}

func (c *anthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic API key 未配置")
	}
	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := fmt.Sprintf("%s/v1/messages", baseURL)

	var messages []anthropicMessage
	var userText strings.Builder
	for _, m := range req.Messages {
		if userText.Len() > 0 {
			userText.WriteString("\n\n")
		}
		userText.WriteString(fmt.Sprintf("[%s]\n%s", m.Role, m.Content))
	}
	userMsg := anthropicMessage{
		Role: "user",
		Content: []anthropicTextContent{
			{Type: "text", Text: userText.String()},
		},
	}
	messages = append(messages, userMsg)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	body := anthropicChatRequest{
		Model:       c.cfg.Model,
		MaxTokens:   maxTokens,
		System:      req.System,
		Messages:    messages,
		Temperature: req.Temperature,
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化 Anthropic 请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("创建 Anthropic 请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	version := c.cfg.AnthropicVersion
	if version == "" {
		version = "2023-06-01"
	}
	httpReq.Header.Set("anthropic-version", version)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 Anthropic 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Anthropic 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic 响应错误: status=%d, body=%s", resp.StatusCode, string(respBytes))
	}

	var ar anthropicChatResponse
	if err := json.Unmarshal(respBytes, &ar); err != nil {
		return nil, fmt.Errorf("解析 Anthropic 响应失败: %w", err)
	}
	if len(ar.Content) == 0 {
		return nil, fmt.Errorf("anthropic 响应中不包含内容")
	}
	return &ChatResponse{Content: ar.Content[0].Text}, nil
}
