package llm

import (
	"context"
	moduleruntime "gochen/host/module/runtime"
	"testing"

	llmauthz "gochen-llm/moduleauthz"
	auth "gochen/auth"
	"gochen/host/module"
)

func TestNewModule_RegistersPermissionCatalog(t *testing.T) {
	registry := auth.NewRegistry()
	host := moduleruntime.NewHost([]module.ModuleCtor{NewModule}, moduleruntime.WithAuthzRegistry(registry))

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
