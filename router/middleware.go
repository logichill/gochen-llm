package router

import (
	"context"

	goauthz "gochen/authz"
	"gochen/errorx"
	"gochen/httpx"
)

const adminEntryPermission = "*:*:*"

// AdminOnlyMiddleware 要求请求上下文里存在管理员 principal。
func AdminOnlyMiddleware() httpx.Middleware {
	return func(ctx httpx.IContext, next func() error) error {
		reqCtx := ctx.RequestContext()
		if reqCtx != nil {
			var baseCtx context.Context = reqCtx
			if bound, err := goauthz.WithConsistencyMode(baseCtx, goauthz.ConsistencyModeStrong); err == nil {
				baseCtx = bound
			}
			if bound, err := goauthz.WithHighRiskAuthorization(baseCtx); err == nil {
				baseCtx = bound
			}
			reqCtx = reqCtx.WithContext(baseCtx)
			ctx.SetContext(reqCtx)
		}

		principal, ok := goauthz.PrincipalFromContext(reqCtx)
		if !ok {
			return errorx.New(errorx.Unauthorized, "用户未认证")
		}
		if !principal.IsSystem && !principal.HasPermission(adminEntryPermission) {
			return errorx.New(errorx.Forbidden, "需要管理员权限")
		}
		return next()
	}
}
