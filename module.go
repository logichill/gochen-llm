package llm

import (
	"context"

	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/errorx"
	"gochen/httpx"
	"gochen/server"
)

func NewModule() (server.IModule, error) {
	var container server.ModuleContainer
	return server.BuildModule(server.ModuleConfig{
		ID:   "llm",
		Name: "LLM",
		Constructors: []any{
			// Repos
			repo.NewProviderConfigRepo,
			repo.NewSafetyPolicyRepo,
			repo.NewPromptTemplateRepo,
			repo.NewAuditLogRepo,
			repo.NewRateLimitRepo,
			repo.NewConversationRepo,
			repo.NewMetricsRepo,
			// Services
			service.NewProviderManager,
			service.NewSafetyService,
			service.NewPromptService,
			service.NewConversationService,
			service.NewCostCalculator,
			service.NewChatService,
		},
		RouteRegistrars: []any{
			router.NewLLMAdminRoutes,
			router.NewMetricsRoutes,
		},
		OnInit: func(c server.ModuleContainer) error {
			container = c
			return nil
		},
		OnStart: func(ctx context.Context) error {
			if container == nil {
				return errorx.New(errorx.Internal, "container is nil")
			}
			return container.Invoke(func(pm service.ProviderManager) error {
				return pm.Start(ctx)
			})
		},
		OnStop: func(ctx context.Context) error {
			if container == nil {
				return nil
			}
			return container.Invoke(func(pm service.ProviderManager) error {
				return pm.Stop(ctx)
			})
		},
		// LLM 模块的路由主要是管理端/监控端点；鉴权由上层应用按需挂载。
		Middlewares: []httpx.Middleware{},
	}), nil
}
