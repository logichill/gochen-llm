package llm

import (
	"context"
	"testing"

	llmauthz "gochen-llm/moduleauthz"
	auth "gochen-runtime/host/authz"
	hostconfig "gochen-runtime/host/config"
	"gochen-runtime/host/module"
	moduleruntime "gochen-runtime/host/module/runtime"
)

func TestNewModule_RegistersPermissionCatalog(t *testing.T) {
	registry := auth.NewRegistry()
	moduleCtor := func() (module.IModule, error) {
		return NewModule()
	}
	host := moduleruntime.NewHost([]module.ModuleCtor{moduleCtor}, hostconfig.WithCatalogRegistrar(auth.NewCatalogRegistrar(registry)))

	if err := host.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	module, ok := registry.Module("llm")
	if !ok {
		t.Fatal("expected llm module catalog to be registered")
	}
	if module.ModuleName != "LLM" {
		t.Fatalf("unexpected module name: %#v", module)
	}

	expected := []string{
		llmauthz.PermissionSet.Code(auth.PermissionActionRead),
		llmauthz.PermissionSet.Code(auth.PermissionActionWrite),
	}
	if got := registry.Permissions(); len(got) != len(expected) || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("unexpected llm permissions: %#v", got)
	}
}
