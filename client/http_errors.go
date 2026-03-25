package client

import (
	"fmt"

	"gochen/errorx"
)

// newClientConfigError 创建客户端配置错误。
func newClientConfigError(message string) error {
	return errorx.New(errorx.Internal, message)
}

// wrapClientInternal 包装客户端Internal。
func wrapClientInternal(err error, message string) error {
	return errorx.Wrap(err, errorx.Internal, message)
}

// wrapClientNetwork 包装客户端Network。
func wrapClientNetwork(err error, message string) error {
	return errorx.Wrap(err, errorx.Network, message)
}

// newUpstreamStatusError 创建Upstream状态错误。
func newUpstreamStatusError(provider string, statusCode int, body []byte) error {
	message := fmt.Sprintf("%s 上游响应错误", provider)
	errCode := errorx.ServiceUnavailable
	switch {
	case statusCode == 408:
		errCode = errorx.Timeout
	case statusCode == 429:
		errCode = errorx.TooManyRequests
	case statusCode == 401:
		errCode = errorx.Unauthorized
	case statusCode == 403:
		errCode = errorx.Forbidden
	case statusCode == 404:
		errCode = errorx.NotFound
	case statusCode == 409:
		errCode = errorx.Conflict
	case statusCode == 422:
		errCode = errorx.Validation
	case statusCode >= 400 && statusCode < 500:
		errCode = errorx.InvalidInput
	}
	return errorx.New(errCode, message).
		WithContext("provider", provider).
		WithContext("upstream_status", statusCode).
		WithContext("body", string(body))
}
