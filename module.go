package llm

import (
	"context"

	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/boot"
	"gochen/server"
)

// NewModule 创建模块。
func NewModule() (server.IModule, error) {
	return boot.BuildModule(boot.ModuleConfig{
		ID:   "llm",
		Name: "LLM",
		Providers: []any{
			// Repos
			repo.NewProviderConfigRepo,
			repo.NewSafetyPolicyRepo,
			repo.NewPromptTemplateRepo,
			repo.NewPromptVersionRepo,
			repo.NewABTestRepo,
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
		RuntimeComponents: []any{
			newProviderManagerRuntime,
		},
	}), nil
}

type providerManagerRuntime struct {
	manager service.IProviderManager
}

func newProviderManagerRuntime(manager service.IProviderManager) *providerManagerRuntime {
	return &providerManagerRuntime{manager: manager}
}

func (r *providerManagerRuntime) Start(ctx context.Context) error {
	return r.manager.Start(ctx)
}

func (r *providerManagerRuntime) Stop(ctx context.Context) error {
	return r.manager.Stop(ctx)
}
