package llm

import (
	"context"

	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/eventing/bus"
	"gochen/eventing/projection"
	"gochen/runtime/di"
)

// Module LLM 通用能力模块
type Module struct{}

func NewModule() *Module {
	return &Module{}
}

func (m *Module) Name() string {
	return "LLM"
}

// RegisterProviders 注册仓储、服务与路由
func (m *Module) RegisterProviders(container di.IContainer) error {
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
		if err := container.RegisterConstructor(ctor); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) RegisterEventHandlers(ctx context.Context, eventBus bus.IEventBus, container di.IContainer) error {
	_ = ctx
	_ = eventBus
	_ = container
	return nil
}

func (m *Module) RegisterProjections(container di.IContainer) (*projection.ProjectionManager, []string, error) {
	_ = container
	return nil, nil, nil
}
