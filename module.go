package llm

import (
	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/runtime/di"
	"gochen/server"
)

// Module LLM 通用能力模块
type Module struct {
	container di.IContainer
}

func NewModule(container di.IContainer) (server.IModule, error) {
	m := &Module{container: container}
	if err := m.registerProviders(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Module) Name() string {
	return "LLM"
}

func (m *Module) registerProviders() error {
	ctors := []interface{}{
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
		// Routes
		router.NewLLMAdminRoutes,
		router.NewMetricsRoutes,
	}

	for _, ctor := range ctors {
		if err := m.container.RegisterConstructor(ctor); err != nil {
			return err
		}
	}
	return nil
}
