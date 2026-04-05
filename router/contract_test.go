package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gochen-llm/client"
	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen-llm/service"
	"gochen/httpx"
	"gochen/httpx/adaptersupport"
)

type routerTestContext struct {
	status  int
	jsonObj any
	values  map[string]httpx.ContextValue
	body    []byte
	request *http.Request
}

func newRouterTestContext(method, rawURL string) *routerTestContext {
	return newRouterTestContextWithBody(method, rawURL, nil)
}

func newRouterTestContextWithBody(method, rawURL string, body []byte) *routerTestContext {
	if rawURL == "" {
		rawURL = "/"
	}
	req := httptest.NewRequest(method, rawURL, bytes.NewReader(body)).WithContext(context.Background())
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return &routerTestContext{request: req, body: append([]byte(nil), body...)}
}

func (c *routerTestContext) GetMethod() string           { return c.request.Method }
func (c *routerTestContext) GetPath() string             { return c.request.URL.Path }
func (c *routerTestContext) GetHeader(key string) string { return c.request.Header.Get(key) }
func (c *routerTestContext) GetQuery(key string) string  { return c.request.URL.Query().Get(key) }
func (c *routerTestContext) GetParam(key string) string  { return "" }
func (c *routerTestContext) GetQueryParams() url.Values  { return c.request.URL.Query() }
func (c *routerTestContext) GetBody() ([]byte, error)    { return append([]byte(nil), c.body...), nil }
func (c *routerTestContext) GetRequest() *http.Request   { return c.request }
func (c *routerTestContext) ClientIP() string            { return "127.0.0.1" }
func (c *routerTestContext) UserAgent() string           { return "test" }
func (c *routerTestContext) BindJSON(obj any) error {
	if len(c.body) == 0 {
		return io.EOF
	}
	return json.Unmarshal(c.body, obj)
}
func (c *routerTestContext) BindQuery(obj any) error {
	return adaptersupport.BindQuery(obj, c.request.URL.Query())
}
func (c *routerTestContext) ShouldBindJSON(obj any) error { return c.BindJSON(obj) }
func (c *routerTestContext) SetStatus(code int)           { c.status = code }
func (c *routerTestContext) SetHeader(key, value string)  {}
func (c *routerTestContext) JSON(code int, obj httpx.JSONBody) error {
	c.status = code
	c.jsonObj, _ = httpx.JSONBodyAs[any](obj)
	return nil
}
func (c *routerTestContext) String(code int, text string) error { c.status = code; return nil }
func (c *routerTestContext) Data(code int, contentType string, data []byte) error {
	c.status = code
	return nil
}
func (c *routerTestContext) Set(key string, value httpx.ContextValue) {
	if c.values == nil {
		c.values = make(map[string]httpx.ContextValue)
	}
	c.values[key] = value
}
func (c *routerTestContext) Get(key string) (httpx.ContextValue, bool) {
	if c.values == nil {
		return httpx.ContextValue{}, false
	}
	value, ok := c.values[key]
	return value, ok
}
func (c *routerTestContext) GetRequired(key string) (httpx.ContextValue, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}
	return httpx.ContextValue{}, errors.New("missing")
}
func (c *routerTestContext) Abort()                   {}
func (c *routerTestContext) AbortWithStatus(code int) { c.status = code }
func (c *routerTestContext) AbortWithStatusJSON(code int, jsonObj httpx.JSONBody) {
	c.status = code
	c.jsonObj, _ = httpx.JSONBodyAs[any](jsonObj)
}
func (c *routerTestContext) IsAborted() bool                   { return false }
func (c *routerTestContext) GetContext() httpx.IRequestContext { return nil }
func (c *routerTestContext) SetContext(ctx httpx.IRequestContext) {
}

type stubProviderManager struct {
	configs []*entity.ProviderConfig
}

func (m *stubProviderManager) Start(ctx context.Context) error { return nil }
func (m *stubProviderManager) Stop(ctx context.Context) error  { return nil }
func (m *stubProviderManager) ChatForUser(ctx context.Context, userID int64, req *client.ChatRequest) (*service.ChatExecution, error) {
	return nil, nil
}
func (m *stubProviderManager) Reload(ctx context.Context) error { return nil }
func (m *stubProviderManager) ListEffectiveConfigs(ctx context.Context) ([]*entity.ProviderConfig, error) {
	return m.configs, nil
}
func (m *stubProviderManager) ReplaceConfigs(ctx context.Context, configs []*entity.ProviderConfig) error {
	m.configs = configs
	return nil
}
func (m *stubProviderManager) ListStatus(ctx context.Context) ([]*service.EndpointStatus, error) {
	return []*service.EndpointStatus{{Provider: "openai", Model: "gpt-4o-mini"}}, nil
}

type stubMetricsRepo struct {
	list  []*entity.Metrics
	total int64
}

func (r *stubMetricsRepo) Save(ctx context.Context, m *entity.Metrics) error { return nil }
func (r *stubMetricsRepo) Aggregate(ctx context.Context, filter entity.MetricsFilter) (*entity.MetricsReport, error) {
	return &entity.MetricsReport{TotalCalls: 3, SuccessCalls: 2}, nil
}
func (r *stubMetricsRepo) AggregateByVariant(ctx context.Context, filter entity.MetricsFilter) ([]*entity.VariantMetricsReport, error) {
	return []*entity.VariantMetricsReport{{Variant: "A"}}, nil
}
func (r *stubMetricsRepo) List(ctx context.Context, filter entity.MetricsFilter, limit, offset int) ([]*entity.Metrics, int64, error) {
	return r.list, r.total, nil
}
func (r *stubMetricsRepo) Significance(ctx context.Context, filter entity.MetricsFilter) (*entity.ABSignificanceReport, error) {
	return &entity.ABSignificanceReport{ABTestID: 1}, nil
}

func TestLLMAdminRoutesGetLLMConfig_UsesResponseMessage(t *testing.T) {
	routes := NewLLMAdminRoutes(&stubProviderManager{configs: []*entity.ProviderConfig{{Provider: "openai", Model: "gpt-4o-mini"}}}, nil, nil, nil, nil, nil, nil)
	ctx := newRouterTestContext(http.MethodGet, "/admin/llm/config")

	if err := routes.getLLMConfig(ctx); err != nil {
		t.Fatalf("getLLMConfig returned error: %v", err)
	}
	payload, ok := ctx.jsonObj.(*httpx.ResponseMessage)
	if !ok {
		t.Fatalf("expected ResponseMessage, got %T", ctx.jsonObj)
	}
	data, ok := payload.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", payload.Data)
	}
	configs, ok := data["configs"].([]*entity.ProviderConfig)
	if !ok || len(configs) != 1 {
		t.Fatalf("unexpected configs payload: %#v", data["configs"])
	}
}

func TestLLMAdminRoutesReloadLLMConfig_UsesTopLevelMessage(t *testing.T) {
	routes := NewLLMAdminRoutes(&stubProviderManager{}, nil, nil, nil, nil, nil, nil)
	ctx := newRouterTestContext(http.MethodPost, "/admin/llm/reload")

	if err := routes.reloadLLMConfig(ctx); err != nil {
		t.Fatalf("reloadLLMConfig returned error: %v", err)
	}
	payload, ok := ctx.jsonObj.(*httpx.ResponseMessage)
	if !ok {
		t.Fatalf("expected ResponseMessage, got %T", ctx.jsonObj)
	}
	if payload.Message != "reloaded" {
		t.Fatalf("expected top-level message 'reloaded', got %#v", payload)
	}
	if payload.Data != nil {
		t.Fatalf("expected no nested data payload, got %#v", payload.Data)
	}
}

func TestMetricsRoutesList_UsesPaginatedResponseMessage(t *testing.T) {
	routes := NewMetricsRoutes(&stubMetricsRepo{list: []*entity.Metrics{{Provider: "openai", Model: "gpt-4o-mini", CreatedAt: time.Now()}}, total: 1})
	ctx := newRouterTestContext(http.MethodGet, "/admin/llm/metrics/list?page=2&size=25")

	if err := routes.list(ctx); err != nil {
		t.Fatalf("metrics list returned error: %v", err)
	}
	payload, ok := ctx.jsonObj.(*httpx.ResponseMessage)
	if !ok {
		t.Fatalf("expected ResponseMessage, got %T", ctx.jsonObj)
	}
	data, ok := payload.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", payload.Data)
	}
	if data["total"] != int64(1) {
		t.Fatalf("expected total=1, got %#v", data)
	}
	if data["limit"] != 25 || data["offset"] != 25 {
		t.Fatalf("expected limit/offset in data, got %#v", data)
	}
	list, ok := data["list"].([]*entity.Metrics)
	if !ok || len(list) != 1 {
		t.Fatalf("unexpected metrics list payload: %#v", data["list"])
	}
}

func TestLLMAdminRoutesMarkConversion_RejectsLegacyConversionType(t *testing.T) {
	routes := NewLLMAdminRoutes(nil, nil, &stubMetricsRepo{}, nil, nil, nil, nil)
	ctx := newRouterTestContextWithBody(http.MethodPost, "/admin/llm/metrics/convert", []byte(`{"conversion_type":"signup"}`))

	if err := routes.markConversion(ctx); err != nil {
		t.Fatalf("markConversion returned error: %v", err)
	}
	if ctx.status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", ctx.status)
	}
	payload, ok := ctx.jsonObj.(*httpx.ResponseMessage)
	if !ok {
		t.Fatalf("expected ResponseMessage, got %T", ctx.jsonObj)
	}
	if payload.Code == "ok" {
		t.Fatalf("expected error payload, got %#v", payload)
	}
}

func TestMetricsRoutesList_RejectsLegacyFlatQueryContract(t *testing.T) {
	routes := NewMetricsRoutes(&stubMetricsRepo{})
	ctx := newRouterTestContext(http.MethodGet, "/admin/llm/metrics/list?provider=openai&limit=10")

	if err := routes.list(ctx); err != nil {
		t.Fatalf("metrics list returned error: %v", err)
	}
	if ctx.status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", ctx.status)
	}
}

func TestLLMAdminRoutesGetLLMMetrics_RejectsLegacyFlatQueryContract(t *testing.T) {
	routes := NewLLMAdminRoutes(nil, nil, &stubMetricsRepo{}, nil, nil, nil, nil)
	ctx := newRouterTestContext(http.MethodGet, "/admin/llm/metrics?provider=openai")

	if err := routes.getLLMMetrics(ctx); err != nil {
		t.Fatalf("getLLMMetrics returned error: %v", err)
	}
	if ctx.status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", ctx.status)
	}
}

func TestLLMAdminRoutesListAuditLogs_RejectsLegacyFlatQueryContract(t *testing.T) {
	routes := NewLLMAdminRoutes(nil, nil, nil, nil, &stubAuditLogRepo{}, nil, nil)
	ctx := newRouterTestContext(http.MethodGet, "/admin/llm/audit?action=reload&offset=10")

	if err := routes.listAuditLogs(ctx); err != nil {
		t.Fatalf("listAuditLogs returned error: %v", err)
	}
	if ctx.status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", ctx.status)
	}
}

type stubAuditLogRepo struct{}

func (r *stubAuditLogRepo) Save(ctx context.Context, log *entity.AuditLog) error { return nil }
func (r *stubAuditLogRepo) List(ctx context.Context, filter repo.AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error) {
	return nil, 0, nil
}

var _ httpx.IContext = (*routerTestContext)(nil)
var _ service.IProviderManager = (*stubProviderManager)(nil)
var _ repo.IMetricsRepo = (*stubMetricsRepo)(nil)
var _ repo.IAuditLogRepo = (*stubAuditLogRepo)(nil)
