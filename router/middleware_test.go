package router

import (
	"context"
	"testing"
	"time"

	goauthz "gochen/authz"
	"gochen/errorx"
	"gochen/httpx"
)

type llmMiddlewareRequestContext struct {
	context.Context
}

func (c llmMiddlewareRequestContext) WithContext(ctx context.Context) httpx.IRequestContext {
	return llmMiddlewareRequestContext{Context: ctx}
}
func (c llmMiddlewareRequestContext) WithValue(key any, value any) httpx.IRequestContext {
	return llmMiddlewareRequestContext{Context: context.WithValue(c.Context, key, value)}
}
func (c llmMiddlewareRequestContext) WithTimeout(timeout time.Duration) (httpx.IRequestContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.Context, timeout)
	return llmMiddlewareRequestContext{Context: ctx}, cancel
}
func (c llmMiddlewareRequestContext) WithCancel() (httpx.IRequestContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.Context)
	return llmMiddlewareRequestContext{Context: ctx}, cancel
}
func (c llmMiddlewareRequestContext) WithDeadline(deadline time.Time) (httpx.IRequestContext, context.CancelFunc) {
	ctx, cancel := context.WithDeadline(c.Context, deadline)
	return llmMiddlewareRequestContext{Context: ctx}, cancel
}
func (c llmMiddlewareRequestContext) Clone() httpx.IRequestContext {
	return llmMiddlewareRequestContext{Context: c.Context}
}

func newLLMAdminContext(t *testing.T, principal goauthz.Principal) *routerTestContext {
	t.Helper()
	ctx := newRouterTestContext("GET", "/admin/llm/config")
	bound, err := goauthz.WithPrincipal(context.Background(), principal)
	if err != nil {
		t.Fatalf("bind llm principal: %v", err)
	}
	ctx.requestCtx = llmMiddlewareRequestContext{Context: bound}
	return ctx
}

func TestAdminOnlyMiddlewareRejectsUnauthenticatedRequest(t *testing.T) {
	ctx := newRouterTestContext("GET", "/admin/llm/config")
	called := false

	err := AdminOnlyMiddleware()(ctx, func() error {
		called = true
		return nil
	})
	if !errorx.Is(err, errorx.Unauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if called {
		t.Fatal("next handler should not be called")
	}
}

func TestAdminOnlyMiddlewareRejectsNonAdminPrincipal(t *testing.T) {
	ctx := newLLMAdminContext(t, goauthz.Principal{
		SubjectID:   7,
		TenantID:    "default",
		Permissions: []string{"api:llm:read"},
	})
	called := false

	err := AdminOnlyMiddleware()(ctx, func() error {
		called = true
		return nil
	})
	if !errorx.Is(err, errorx.Forbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if called {
		t.Fatal("next handler should not be called")
	}
}

func TestAdminOnlyMiddlewareAllowsAdminPrincipal(t *testing.T) {
	ctx := newLLMAdminContext(t, goauthz.Principal{
		SubjectID:   7,
		TenantID:    "default",
		Permissions: []string{"*:*:*"},
	})
	called := false

	err := AdminOnlyMiddleware()(ctx, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !called {
		t.Fatal("next handler should be called")
	}
}
