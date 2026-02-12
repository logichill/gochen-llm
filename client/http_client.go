package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type httpClient struct {
	http *http.Client
	cfg  *Config
}

func newHTTPClient(cfg *Config) *httpClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &httpClient{
		http: &http.Client{Timeout: timeout},
		cfg:  cfg,
	}
}

func (c *httpClient) doRequest(ctx context.Context, url string, payload any, parse func([]byte) (*ChatResponse, error)) (*ChatResponse, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	switch c.cfg.Provider {
	case ProviderOpenAI, ProviderOpenAICompatible:
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 LLM 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LLM 响应错误: status=%d, body=%s", resp.StatusCode, string(respBytes))
	}

	return parse(respBytes)
}

func ioReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
