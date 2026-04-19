package client

import (
	"fmt"

	"gochen/errors"
)

// newClientConfigError 创建客户端配置错误。
func newClientConfigError(message string) error {
	return errors.NewCode(errors.Internal, message)
}

// wrapClientInternal 包装客户端Internal。
func wrapClientInternal(err error, message string) error {
	return errors.Wrap(err, errors.Internal, message)
}

// wrapClientNetwork 包装客户端Network。
func wrapClientNetwork(err error, message string) error {
	return errors.Wrap(err, errors.Network, message)
}

// newUpstreamStatusError 创建Upstream状态错误。
func newUpstreamStatusError(provider string, statusCode int, body []byte) error {
	message := fmt.Sprintf("%s 上游响应错误", provider)
	errCode := errors.ServiceUnavailable
	switch {
	case statusCode == 408:
		errCode = errors.Timeout
	case statusCode == 429:
		errCode = errors.TooManyRequests
	case statusCode == 401:
		errCode = errors.Unauthorized
	case statusCode == 403:
		errCode = errors.Forbidden
	case statusCode == 404:
		errCode = errors.NotFound
	case statusCode == 409:
		errCode = errors.Conflict
	case statusCode == 422:
		errCode = errors.Validation
	case statusCode >= 400 && statusCode < 500:
		errCode = errors.InvalidInput
	}
	return errors.NewCode(errCode, message).
		WithContext("provider", provider).
		WithContext("upstream_status", statusCode).
		WithContext("body", string(body))
}
