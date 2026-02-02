package router

import (
	"gochen/errorx"
	httpx "gochen/httpx"
)

// AdminOnlyMiddleware 默认仅检查用户已认证，角色校验由上层应用按需追加
func AdminOnlyMiddleware() httpx.Middleware {
	return func(ctx httpx.IContext, next func() error) error {
		reqCtx := ctx.GetContext()
		if reqCtx == nil || reqCtx.GetUserID() == 0 {
			return errorx.NewError(errorx.Unauthorized, "用户未认证")
		}
		return next()
	}
}
