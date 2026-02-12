package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type geminiClient struct {
	*httpClient
}

func newGeminiClient(cfg *Config) *geminiClient {
	return &geminiClient{httpClient: newHTTPClient(cfg)}
}

type geminiGenerateRequest struct {
	Contents         []geminiContent  `json:"contents"`
	GenerationConfig *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	Temperature     float32 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

func (c *geminiClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key 未配置")
	}

	model := c.cfg.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}

	baseURL := c.cfg.GeminiAPIEndpoint
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, model, c.cfg.APIKey)

	var promptBuilder strings.Builder
	if req.System != "" {
		promptBuilder.WriteString("[System]\n")
		promptBuilder.WriteString(req.System)
		promptBuilder.WriteString("\n\n")
	}
	for _, m := range req.Messages {
		promptBuilder.WriteString("[")
		if m.Role == "" {
			promptBuilder.WriteString("user")
		} else {
			promptBuilder.WriteString(m.Role)
		}
		promptBuilder.WriteString("]\n")
		promptBuilder.WriteString(m.Content)
		promptBuilder.WriteString("\n\n")
	}

	body := geminiGenerateRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: promptBuilder.String()},
				},
			},
		},
	}

	if req.Temperature != 0 || req.MaxTokens > 0 {
		body.GenerationConfig = &geminiGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	return c.doRequest(ctx, url, body, func(respBytes []byte) (*ChatResponse, error) {
		var gr geminiGenerateResponse
		if err := json.Unmarshal(respBytes, &gr); err != nil {
			return nil, fmt.Errorf("解析 Gemini 响应失败: %w", err)
		}
		if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("gemini 响应中不包含内容")
		}
		return &ChatResponse{Content: gr.Candidates[0].Content.Parts[0].Text}, nil
	})
}
