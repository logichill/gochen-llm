package moduleauthz

import (
	"gochen-runtime/api/rest/action"
	"gochen-runtime/host/authz"
	"gochen/httpx"
)

var (
	ReadPermission = authz.APIPermission("llm", authz.PermissionActionRead).
			Label("LLM Read").
			Desc("View LLM configuration, status, metrics, and audit data.").
			Scope(authz.PermissionScopePlatform, authz.PermissionScopeTenant).
			Risk(authz.PermissionRiskMedium)
	WritePermission = authz.APIPermission("llm", authz.PermissionActionWrite).
			Label("LLM Write").
			Desc("Manage LLM configuration, pricing, safety policy, and operational actions.").
			Scope(authz.PermissionScopePlatform, authz.PermissionScopeTenant).
			Risk(authz.PermissionRiskHigh)
	PermissionSet = authz.NewPermissionSet(
		ReadPermission,
		WritePermission,
	)

	readMiddleware  = mustFastDeny(authz.PermissionActionRead)
	writeMiddleware = mustFastDeny(authz.PermissionActionWrite)
)

func mustFastDeny(actionType authz.PermissionAction) httpx.Middleware {
	code := PermissionSet.Must(actionType).Code
	mw, err := action.FastDeny(authz.NewActionChecker(), code)
	if err != nil {
		panic(err)
	}
	return mw
}

func PermissionDefinitions() []authz.PermissionDefinition {
	return authz.PermissionDefinitions(ReadPermission, WritePermission)
}

func ReadMiddleware() httpx.Middleware {
	return readMiddleware
}

func WriteMiddleware() httpx.Middleware {
	return writeMiddleware
}
