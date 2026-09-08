package llm

import (
	"context"

	llmauthz "gochen-llm/moduleauthz"
	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen-runtime/host"
	auth "gochen-runtime/host/authz"
	"gochen-runtime/host/module"
)

// NewModule 创建模块。
func NewModule() (module.IModule, error) {
	return host.Module("llm").
		Name("LLM").
		Extension(auth.Catalog{
			PermissionDefinitions: llmauthz.PermissionDefinitions(),
		}).
		Provide(
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
		).
		RouteRegistrar(
			router.NewLLMAdminRoutes,
			router.NewMetricsRoutes,
		).
		RuntimeComponent(
			newProviderManagerRuntime,
			newChatServiceRuntime,
		).
		Build()
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

type chatServiceRuntime struct {
	chat service.IChatService
}

func newChatServiceRuntime(chat service.IChatService) *chatServiceRuntime {
	return &chatServiceRuntime{chat: chat}
}

func (r *chatServiceRuntime) Start(context.Context) error {
	return nil
}

func (r *chatServiceRuntime) Stop(ctx context.Context) error {
	if r == nil || r.chat == nil {
		return nil
	}
	return r.chat.Stop(ctx)
}
