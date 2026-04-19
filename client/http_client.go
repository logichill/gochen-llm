package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gochen/errors"
)

type httpClient struct {
	http *http.Client
	cfg  *Config
}

// newHTTPClient 创建HTTP客户端。
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

// doRequest 处理do请求。
func (c *httpClient) doRequest(ctx context.Context, url string, payload any, parse func([]byte) (*ChatResponse, error)) (*ChatResponse, error) {
	if ctx == nil {
		return nil, errors.NewCode(errors.InvalidInput, "ctx is nil")
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, wrapClientInternal(err, "序列化请求失败")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, wrapClientInternal(err, "创建 HTTP 请求失败")
	}

	req.Header.Set("Content-Type", "application/json")

	switch c.cfg.Provider {
	case ProviderOpenAI, ProviderOpenAICompatible:
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapClientNetwork(err, "调用 LLM 接口失败")
	}
	defer resp.Body.Close()

	respBytes, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, wrapClientNetwork(err, "读取 LLM 响应失败")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newUpstreamStatusError(string(c.cfg.Provider), resp.StatusCode, respBytes)
	}

	return parse(respBytes)
}

// ioReadAll 处理io读取全部。
func ioReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
