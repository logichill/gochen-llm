package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gochen/errorx"
)

func TestHTTPClient_DoRequest_NilContext(t *testing.T) {
	hc := newHTTPClient(&Config{Provider: ProviderOpenAI, APIKey: "x"})
	_, err := hc.doRequest(nil, "http://example.invalid", map[string]any{"x": 1}, func([]byte) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected InvalidInput, got %v", err)
	}
}

func TestHTTPClient_DoRequest_HTTPStatusMappedToErrorCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer ts.Close()

	hc := newHTTPClient(&Config{Provider: ProviderOpenAI, APIKey: "x"})
	_, err := hc.doRequest(context.Background(), ts.URL, map[string]any{"x": 1}, func([]byte) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errorx.Is(err, errorx.TooManyRequests) {
		t.Fatalf("expected TooManyRequests, got %v", err)
	}
	appErr, ok := err.(*errorx.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if got := appErr.Details()["provider"]; got != string(ProviderOpenAI) {
		t.Fatalf("expected provider context %q, got %#v", ProviderOpenAI, got)
	}
	if got := appErr.Details()["upstream_status"]; got != http.StatusTooManyRequests {
		t.Fatalf("expected upstream_status=%d, got %#v", http.StatusTooManyRequests, got)
	}
}

func TestOpenAIClient_Chat_MissingAPIKey(t *testing.T) {
	client := newOpenAIClient(&Config{Provider: ProviderOpenAI})
	_, err := client.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected Internal, got %v", err)
	}
}

func TestGeminiClient_Chat_MissingAPIKey(t *testing.T) {
	client := newGeminiClient(&Config{Provider: ProviderGemini})
	_, err := client.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected Internal, got %v", err)
	}
}

func TestAnthropicClient_Chat_MissingAPIKey(t *testing.T) {
	client := newAnthropicClient(&Config{Provider: ProviderAnthropic})
	_, err := client.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected Internal, got %v", err)
	}
}

func TestNewClient_ValidationAndUnsupported(t *testing.T) {
	if _, err := NewClient(nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected InvalidInput for nil config, got %v", err)
	}
	if _, err := NewClient(&Config{}); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected InvalidInput for empty provider, got %v", err)
	}
	if _, err := NewClient(&Config{Provider: Provider("bad")}); err == nil || !errorx.Is(err, errorx.Unsupported) {
		t.Fatalf("expected Unsupported for bad provider, got %v", err)
	}
}

func TestHTTPClient_DoRequest_Upstream4xxErrorsKeepDeterministicCodes(t *testing.T) {
	tests := []struct {
		status int
		code   errorx.ErrorCode
	}{
		{status: http.StatusBadRequest, code: errorx.InvalidInput},
		{status: http.StatusUnauthorized, code: errorx.Unauthorized},
		{status: http.StatusForbidden, code: errorx.Forbidden},
		{status: http.StatusNotFound, code: errorx.NotFound},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, `{"error":"upstream"}`)
			}))
			defer ts.Close()

			hc := newHTTPClient(&Config{Provider: ProviderGemini, APIKey: "x"})
			_, err := hc.doRequest(context.Background(), ts.URL, map[string]any{"x": 1}, func([]byte) (*ChatResponse, error) {
				return &ChatResponse{Content: "ok"}, nil
			})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errorx.Is(err, tt.code) {
				t.Fatalf("expected %s, got %v", tt.code, err)
			}
		})
	}
}
