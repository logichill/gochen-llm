package router

import (
	"gochen/errors"
	httpx "gochen/http"
)

// AdminOnlyMiddleware 默认仅检查用户已认证，角色校验由上层应用按需追加
func AdminOnlyMiddleware() httpx.Middleware {
	return func(ctx httpx.IHttpContext, next func() error) error {
		reqCtx := ctx.GetContext()
		if reqCtx == nil || reqCtx.GetUserID() == 0 {
			return errors.NewError(errors.Unauthorized, "用户未认证")
		}
		return next()
	}
}
