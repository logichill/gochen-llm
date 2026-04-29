package moduleauthz

import (
	authhttp "gochen/auth/adapters/http"
	auth "gochen/auth/core"
	"gochen/httpx"
)

var (
	ReadPermission = auth.APIPermission("llm", auth.PermissionActionRead).
			Label("LLM Read").
			Desc("View LLM configuration, status, metrics, and audit data.").
			Scope(auth.PermissionScopePlatform, auth.PermissionScopeTenant).
			Risk(auth.PermissionRiskMedium)
	WritePermission = auth.APIPermission("llm", auth.PermissionActionWrite).
			Label("LLM Write").
			Desc("Manage LLM configuration, pricing, safety policy, and operational actions.").
			Scope(auth.PermissionScopePlatform, auth.PermissionScopeTenant).
			Risk(auth.PermissionRiskHigh)
	PermissionSet = auth.NewPermissionSet(
		ReadPermission,
		WritePermission,
	)
)

func PermissionDefinitions() []auth.PermissionDefinition {
	return auth.PermissionDefinitions(ReadPermission, WritePermission)
}

func ReadMiddleware() httpx.Middleware {
	return authhttp.PermissionMiddleware(PermissionSet.Must(auth.PermissionActionRead))
}

func WriteMiddleware() httpx.Middleware {
	return authhttp.PermissionMiddleware(PermissionSet.Must(auth.PermissionActionWrite))
}
