package llm

import (
	"context"

	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/runtime/di"
	"gochen/runtime/errorx"
	"gochen/server"
)

// Module LLM 通用能力模块
type Module struct {
	container di.IContainer
	opts      server.ModuleInitOptions
}

func NewModule() (server.IModule, error) {
	return &Module{}, nil
}

func (m *Module) Name() string {
	return "LLM"
}

func (m *Module) ID() string { return "llm" }

func (m *Module) Init(opts server.ModuleInitOptions) error {
	m.opts = opts
	m.container = opts.Container
	return m.registerProviders()
}

// RegisterRoutes 仅挂载 HTTP 路由，不进入运行期。
func (m *Module) RegisterRoutes(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if m.opts.HTTP == nil {
		return nil
	}
	group := m.opts.HTTP.MountGroup()
	if group == nil {
		return nil
	}

	if err := m.container.Invoke(func(
		manager service.ProviderManager,
		safety repo.SafetyPolicyRepo,
		metrics repo.MetricsRepo,
		cfgRepo repo.ProviderConfigRepo,
		audit repo.AuditLogRepo,
		rate repo.RateLimitRepo,
		safetySvc service.SafetyService,
	) {
		router.NewLLMAdminRoutes(manager, safety, metrics, cfgRepo, audit, rate, safetySvc).RegisterRoutes(group)
		router.NewMetricsRoutes(metrics).RegisterRoutes(group)
	}); err != nil {
		return errorx.WrapError(err, errorx.Dependency, "failed to build llm routes")
	}
	return nil
}

func (m *Module) Start(ctx context.Context) (server.ModuleStopFunc, error) {
	if m == nil {
		return nil, nil
	}
	if m.container == nil {
		return nil, errorx.NewInternalError("container not initialized")
	}

	// 运行期任务：启动 ProviderManager 的健康探测等后台循环（构造阶段不启动，避免泄露与测试语义漂移）。
	if err := m.container.Invoke(func(pm service.ProviderManager) error {
		return pm.Start(ctx)
	}); err != nil {
		return nil, errorx.WrapError(err, errorx.Dependency, "failed to start llm background tasks")
	}

	return func(stopCtx context.Context) error {
		if m.container == nil {
			return nil
		}
		if err := m.container.Invoke(func(pm service.ProviderManager) error {
			return pm.Stop(stopCtx)
		}); err != nil {
			return errorx.WrapError(err, errorx.Internal, "failed to stop llm background tasks")
		}
		return nil
	}, nil
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
