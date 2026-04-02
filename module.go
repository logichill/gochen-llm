package llm

import (
	"context"

	"gochen-llm/repo"
	"gochen-llm/router"
	"gochen-llm/service"
	"gochen/di"
	"gochen/errorx"
	"gochen/server"
)

// Module 封装 LLM 模块的 providers、路由与运行期后台任务。
type Module struct {
	*server.BaseModule

	container server.IModuleContainer
	opts      server.ModuleInitOptions
}

// NewModule 创建模块。
func NewModule() (server.IModule, error) {
	return &Module{BaseModule: server.NewBaseModule("llm", "LLM")}, nil
}

// Init 注册模块 providers 并保存运行环境引用。
func (m *Module) Init(opts server.ModuleInitOptions) error {
	m.opts = opts
	container, err := server.ResolveModuleContainer(opts)
	if err != nil {
		return errorx.Wrap(err, errorx.Internal, "llm module init requires container capabilities")
	}
	m.container = container

	constructors := []any{
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
		// Routes
		router.NewLLMAdminRoutes,
		router.NewMetricsRoutes,
	}
	for _, ctor := range constructors {
		if err := m.container.RegisterConstructor(di.NewConstructor(ctor)); err != nil {
			return errorx.Wrap(err, errorx.Dependency, "register llm module constructor failed")
		}
	}
	return nil
}

// RegisterRoutes 仅挂载管理端与监控端路由。
func (m *Module) RegisterRoutes(_ context.Context) error {
	if m == nil || m.opts.HTTP == nil || m.container == nil {
		return nil
	}
	group := m.opts.HTTP.MountGroup()
	if group == nil {
		return nil
	}
	return m.container.Invoke(di.NewInvocation(func(
		adminRoutes *router.LLMAdminRoutes,
		metricsRoutes *router.MetricsRoutes,
	) error {
		if err := adminRoutes.RegisterRoutes(group); err != nil {
			return err
		}
		return metricsRoutes.RegisterRoutes(group)
	}))
}

// Start 启动 ProviderManager 的后台健康探测等运行期任务。
func (m *Module) Start(ctx context.Context) (server.ModuleStopFunc, error) {
	if m == nil || m.container == nil {
		return nil, nil
	}
	if err := m.container.Invoke(di.NewInvocation(func(pm service.IProviderManager) error {
		return pm.Start(ctx)
	})); err != nil {
		return nil, errorx.Wrap(err, errorx.Dependency, "start llm provider manager failed")
	}
	return func(stopCtx context.Context) error {
		return m.container.Invoke(di.NewInvocation(func(pm service.IProviderManager) error {
			return pm.Stop(stopCtx)
		}))
	}, nil
}

var _ server.IRouteModule = (*Module)(nil)
