package llm

import (
	"context"
	"testing"

	llmauthz "gochen-llm/moduleauthz"
	"gochen/auth"
	"gochen/server"
)

func TestNewModule_RegistersPermissionCatalog(t *testing.T) {
	registry := auth.NewRegistry()
	srv := server.NewServer([]server.ModuleCtor{NewModule}, server.WithServerAuthzRegistry(registry))

	if err := srv.SetupDependencies(context.Background()); err != nil {
		t.Fatalf("SetupDependencies: %v", err)
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
