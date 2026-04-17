package moduleauthz

import (
	goauthz "gochen/authz"
	authzhttp "gochen/authzhttp"
	"gochen/httpx"
)

var (
	ReadPermission = goauthz.APIPermission("llm", goauthz.PermissionActionRead).
			Label("LLM Read").
			Desc("View LLM configuration, status, metrics, and audit data.").
			Scope(goauthz.PermissionScopePlatform, goauthz.PermissionScopeTenant).
			Risk(goauthz.PermissionRiskMedium)
	WritePermission = goauthz.APIPermission("llm", goauthz.PermissionActionWrite).
			Label("LLM Write").
			Desc("Manage LLM configuration, pricing, safety policy, and operational actions.").
			Scope(goauthz.PermissionScopePlatform, goauthz.PermissionScopeTenant).
			Risk(goauthz.PermissionRiskHigh)
	PermissionSet = goauthz.NewPermissionSet(
		ReadPermission,
		WritePermission,
	)
)

func PermissionDefinitions() []goauthz.PermissionDefinition {
	return goauthz.PermissionDefinitions(ReadPermission, WritePermission)
}

func ReadMiddleware() httpx.Middleware {
	return authzhttp.PermissionMiddleware(PermissionSet.Must(goauthz.PermissionActionRead))
}

func WriteMiddleware() httpx.Middleware {
	return authzhttp.PermissionMiddleware(PermissionSet.Must(goauthz.PermissionActionWrite))
}
