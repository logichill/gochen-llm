package repo

import (
	"sort"
	"testing"

	"gochen-llm/entity"
)

func TestEffectivePromptLessUsesLowerPriorityFirst(t *testing.T) {
	scope := entity.PromptScopeProject
	scopeID := int64(42)
	templates := []*entity.PromptTemplate{
		{Name: "answer", Scope: scope, ScopeID: scopeID, Priority: 100, Version: 9},
		{Name: "answer", Scope: scope, ScopeID: scopeID, Priority: 10, Version: 1},
		{Name: "answer", Scope: entity.PromptScopeGlobal, ScopeID: 0, Priority: 1, Version: 99},
	}

	sort.Slice(templates, func(i, j int) bool {
		return effectivePromptLess(templates[i], templates[j], scope, scopeID)
	})

	if got := templates[0]; got.Scope != scope || got.ScopeID != scopeID || got.Priority != 10 {
		t.Fatalf("unexpected first template: %#v", got)
	}
}
