package router

import (
	llmauthz "gochen-llm/moduleauthz"
	"gochen/httpx"
)

func ReadPermissionMiddleware() httpx.Middleware {
	return llmauthz.ReadMiddleware()
}

func WritePermissionMiddleware() httpx.Middleware {
	return llmauthz.WriteMiddleware()
}
