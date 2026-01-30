package llm

import (
	"context"
	"reflect"
	"sort"

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

type routeRegistrar = server.RouteRegistrar

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

	registrars, err := m.resolveRouteRegistrars()
	if err != nil {
		return err
	}
	sort.Slice(registrars, func(i, j int) bool {
		pi, pj := registrars[i].GetPriority(), registrars[j].GetPriority()
		if pi == pj {
			return registrars[i].GetName() < registrars[j].GetName()
		}
		return pi < pj
	})

	for _, r := range registrars {
		if r == nil {
			continue
		}
		if err := server.SafeRegisterRoutes(r, group); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) Start(ctx context.Context) (server.ModuleStopFunc, error) {
	return nil, nil
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

func (m *Module) resolveRouteRegistrars() ([]routeRegistrar, error) {
	if m == nil || m.container == nil {
		return nil, nil
	}

	types := []reflect.Type{
		server.ElemType((*router.LLMAdminRoutes)(nil)),
		server.ElemType((*router.MetricsRoutes)(nil)),
	}

	out := make([]routeRegistrar, 0, len(types))
	for _, t := range types {
		inst, err := server.ResolveByType(m.container, t)
		if err != nil {
			return nil, err
		}
		r, ok := inst.(routeRegistrar)
		if !ok || server.IsTypedNil(r) {
			return nil, errorx.NewInternalError("resolved route registrar has invalid type").
				WithContext("type", t.String()).
				WithContext("value_type", server.TypeString(inst))
		}
		out = append(out, r)
	}
	return out, nil
}
