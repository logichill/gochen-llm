package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
)

type testPromptRepo struct {
	upsertFn        func(ctx context.Context, tmpl *entity.PromptTemplate) error
	getByIDFn       func(ctx context.Context, id int64) (*entity.PromptTemplate, error)
	findEffectiveFn func(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error)
	listFn          func(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error)
	saveVersionFn   func(ctx context.Context, version *entity.PromptVersion) error
	getVersionFn    func(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error)
	saveABTestFn    func(ctx context.Context, test *entity.ABTest) error
	updateABTestFn  func(ctx context.Context, test *entity.ABTest) error
	getABTestFn     func(ctx context.Context, id int64) (*entity.ABTest, error)
}

func (r *testPromptRepo) Upsert(ctx context.Context, tmpl *entity.PromptTemplate) error {
	if r.upsertFn != nil {
		return r.upsertFn(ctx, tmpl)
	}
	return nil
}

func (r *testPromptRepo) GetByID(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (r *testPromptRepo) FindEffective(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
	if r.findEffectiveFn != nil {
		return r.findEffectiveFn(ctx, name, scope, scopeID)
	}
	return nil, nil
}

func (r *testPromptRepo) List(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error) {
	if r.listFn != nil {
		return r.listFn(ctx, filter)
	}
	return nil, nil
}

func (r *testPromptRepo) SaveVersion(ctx context.Context, version *entity.PromptVersion) error {
	if r.saveVersionFn != nil {
		return r.saveVersionFn(ctx, version)
	}
	return nil
}

func (r *testPromptRepo) GetVersion(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error) {
	if r.getVersionFn != nil {
		return r.getVersionFn(ctx, templateID, version)
	}
	return nil, nil
}

func (r *testPromptRepo) SaveABTest(ctx context.Context, test *entity.ABTest) error {
	if r.saveABTestFn != nil {
		return r.saveABTestFn(ctx, test)
	}
	return nil
}

func (r *testPromptRepo) UpdateABTest(ctx context.Context, test *entity.ABTest) error {
	if r.updateABTestFn != nil {
		return r.updateABTestFn(ctx, test)
	}
	return nil
}

func (r *testPromptRepo) GetABTest(ctx context.Context, id int64) (*entity.ABTest, error) {
	if r.getABTestFn != nil {
		return r.getABTestFn(ctx, id)
	}
	return nil, nil
}

func TestPromptServiceRenderAndCompose(t *testing.T) {
	repoStub := &testPromptRepo{
		findEffectiveFn: func(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
			switch name {
			case "base":
				return &entity.PromptTemplate{Content: "hello {{.name}}"}, nil
			case "tail":
				return &entity.PromptTemplate{Content: "tail"}, nil
			default:
				return nil, nil
			}
		},
	}
	svc := NewPromptService(repoStub)

	if _, err := svc.RenderPrompt(context.Background(), nil, nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid input for nil template, got %v", err)
	}

	if _, err := svc.RenderPrompt(context.Background(), &entity.PromptTemplate{Content: "{{ .x"}, nil); err == nil || !errorx.Is(err, errorx.Internal) {
		t.Fatalf("expected parse error, got %v", err)
	}

	rendered, err := svc.RenderPrompt(context.Background(), &entity.PromptTemplate{Content: "hello {{.name}}"}, map[string]any{"name": "alex"})
	if err != nil {
		t.Fatalf("render prompt failed: %v", err)
	}
	if rendered != "hello alex" {
		t.Fatalf("unexpected rendered content: %q", rendered)
	}

	composed, err := svc.ComposePrompts(context.Background(), []string{"base", "missing", "tail"}, entity.PromptScopeGlobal, 0, map[string]any{"name": "alex"})
	if err != nil {
		t.Fatalf("compose prompts failed: %v", err)
	}
	if composed != "hello alex\n\ntail" {
		t.Fatalf("unexpected composed prompt: %q", composed)
	}
}

func TestPromptServiceSaveAndVersionLifecycle(t *testing.T) {
	var upserted []*entity.PromptTemplate
	var savedVersions []*entity.PromptVersion

	currentTemplate := &entity.PromptTemplate{ID: 10, Version: 2, Content: "v2", VariablesJSON: "{}"}
	repoStub := &testPromptRepo{
		upsertFn: func(ctx context.Context, tmpl *entity.PromptTemplate) error {
			clone := *tmpl
			upserted = append(upserted, &clone)
			return nil
		},
		saveVersionFn: func(ctx context.Context, version *entity.PromptVersion) error {
			clone := *version
			savedVersions = append(savedVersions, &clone)
			return nil
		},
		getByIDFn: func(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
			if id == 10 {
				clone := *currentTemplate
				return &clone, nil
			}
			return nil, nil
		},
		getVersionFn: func(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error) {
			if templateID == 10 && version == 1 {
				return &entity.PromptVersion{TemplateID: 10, Version: 1, Content: "v1", VariablesJSON: "{\"x\":1}"}, nil
			}
			return nil, nil
		},
	}
	svc := NewPromptService(repoStub)

	if err := svc.SavePrompt(context.Background(), nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid input when saving nil prompt, got %v", err)
	}

	tmpl := &entity.PromptTemplate{Name: "n", Content: "c"}
	if err := svc.SavePrompt(context.Background(), tmpl); err != nil {
		t.Fatalf("save prompt failed: %v", err)
	}
	if tmpl.Scope != entity.PromptScopeGlobal || tmpl.Category != "system" || tmpl.Priority != 100 || tmpl.Version != 1 {
		t.Fatalf("expected defaults applied, got %+v", tmpl)
	}
	if len(savedVersions) == 0 {
		t.Fatalf("expected save version called")
	}

	if _, err := svc.CreateVersion(context.Background(), 0, "bad"); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid input for template id, got %v", err)
	}
	if _, err := svc.CreateVersion(context.Background(), 404, "none"); err == nil || !errorx.Is(err, errorx.NotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	version, err := svc.CreateVersion(context.Background(), 10, "bump")
	if err != nil {
		t.Fatalf("create version failed: %v", err)
	}
	if version.Version != 3 {
		t.Fatalf("expected new version 3, got %d", version.Version)
	}

	if err := svc.RollbackVersion(context.Background(), 0, 1); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid input rollback, got %v", err)
	}
	if err := svc.RollbackVersion(context.Background(), 10, 999); err == nil || !errorx.Is(err, errorx.NotFound) {
		t.Fatalf("expected missing rollback version, got %v", err)
	}

	if err := svc.RollbackVersion(context.Background(), 10, 1); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if len(upserted) < 3 {
		t.Fatalf("expected upsert called for save/create/rollback")
	}
	lastVersion := savedVersions[len(savedVersions)-1]
	if !strings.Contains(lastVersion.ChangeLog, "rollback") {
		t.Fatalf("expected rollback changelog, got %q", lastVersion.ChangeLog)
	}
}

func TestPromptServiceImportExportAndABFlow(t *testing.T) {
	var saveCount int
	var updatedTest *entity.ABTest

	abTest := &entity.ABTest{ID: 1, TemplateAID: 11, TemplateBID: 22, TrafficSplit: 30, Status: "running"}
	repoStub := &testPromptRepo{
		listFn: func(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error) {
			return []*entity.PromptTemplate{{ID: 11, Name: "a"}}, nil
		},
		upsertFn: func(ctx context.Context, tmpl *entity.PromptTemplate) error {
			saveCount++
			return nil
		},
		saveVersionFn: func(ctx context.Context, version *entity.PromptVersion) error {
			return nil
		},
		getByIDFn: func(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
			switch id {
			case 11:
				return &entity.PromptTemplate{ID: 11, Name: "A", Content: "A"}, nil
			case 22:
				return &entity.PromptTemplate{ID: 22, Name: "B", Content: "B"}, nil
			case 33:
				return nil, errors.New("db")
			default:
				return nil, nil
			}
		},
		saveABTestFn: func(ctx context.Context, test *entity.ABTest) error {
			abTest = test
			return nil
		},
		getABTestFn: func(ctx context.Context, id int64) (*entity.ABTest, error) {
			if id == 1 {
				clone := *abTest
				return &clone, nil
			}
			if id == 2 {
				return &entity.ABTest{ID: 2, Status: "stopped"}, nil
			}
			return nil, nil
		},
		updateABTestFn: func(ctx context.Context, test *entity.ABTest) error {
			clone := *test
			updatedTest = &clone
			return nil
		},
	}
	svc := NewPromptService(repoStub)

	data, err := svc.ExportPrompts(context.Background(), repo.PromptFilter{})
	if err != nil {
		t.Fatalf("export prompts failed: %v", err)
	}
	var exported []*entity.PromptTemplate
	if err := json.Unmarshal(data, &exported); err != nil || len(exported) != 1 {
		t.Fatalf("unexpected export payload: %v len=%d", err, len(exported))
	}

	if err := svc.ImportPrompts(context.Background(), []byte("bad-json")); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid import error, got %v", err)
	}
	if err := svc.ImportPrompts(context.Background(), []byte(`[{"name":"x","content":"y"}]`)); err != nil {
		t.Fatalf("import prompts failed: %v", err)
	}
	if saveCount == 0 {
		t.Fatalf("expected upsert invoked during import")
	}

	if err := svc.StartABTest(context.Background(), nil); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected nil ab test error, got %v", err)
	}
	if err := svc.StartABTest(context.Background(), &entity.ABTest{TemplateAID: 0, TemplateBID: 1}); err == nil || !errorx.Is(err, errorx.Validation) {
		t.Fatalf("expected invalid template id error, got %v", err)
	}
	if err := svc.StartABTest(context.Background(), &entity.ABTest{TemplateAID: 33, TemplateBID: 22}); err == nil {
		t.Fatalf("expected upstream get template error")
	}

	start := &entity.ABTest{TemplateAID: 11, TemplateBID: 22, TrafficSplit: 40}
	if err := svc.StartABTest(context.Background(), start); err != nil {
		t.Fatalf("start ab test failed: %v", err)
	}
	if start.Status != "running" || start.StartAt.IsZero() {
		t.Fatalf("expected running status and start time, got %+v", start)
	}

	if _, _, err := svc.AssignABVariant(context.Background(), 0, 10); err == nil || !errorx.Is(err, errorx.InvalidInput) {
		t.Fatalf("expected invalid test id, got %v", err)
	}
	if _, _, err := svc.AssignABVariant(context.Background(), 2, 10); err == nil || !errorx.Is(err, errorx.NotFound) {
		t.Fatalf("expected unavailable ab test error, got %v", err)
	}

	tmplA, variantA, err := svc.AssignABVariant(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("assign variant A failed: %v", err)
	}
	if tmplA.ID != 11 || variantA != "A" {
		t.Fatalf("expected variant A template, got %d/%s", tmplA.ID, variantA)
	}

	tmplB, variantB, err := svc.AssignABVariant(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("assign variant B failed: %v", err)
	}
	if tmplB.ID != 22 || variantB != "B" {
		t.Fatalf("expected variant B template, got %d/%s", tmplB.ID, variantB)
	}

	if updatedTest == nil || updatedTest.ResultJSON == "" {
		t.Fatalf("expected ab test result updated")
	}
}
