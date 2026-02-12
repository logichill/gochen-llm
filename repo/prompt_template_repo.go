package repo

import (
	"context"
	"fmt"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

type PromptFilter struct {
	Name     string
	Category string
	Scope    *entity.PromptScope
	ScopeID  *int64
	Enabled  *bool
}

// PromptTemplateRepo 持久化提示词模板与版本
type PromptTemplateRepo interface {
	Upsert(ctx context.Context, tmpl *entity.PromptTemplate) error
	GetByID(ctx context.Context, id int64) (*entity.PromptTemplate, error)
	FindEffective(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error)
	List(ctx context.Context, filter PromptFilter) ([]*entity.PromptTemplate, error)
	SaveVersion(ctx context.Context, version *entity.PromptVersion) error
	GetVersion(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error)
	SaveABTest(ctx context.Context, test *entity.ABTest) error
	UpdateABTest(ctx context.Context, test *entity.ABTest) error
	GetABTest(ctx context.Context, id int64) (*entity.ABTest, error)
}

type promptTemplateRepoImpl struct {
	orm           orm.IOrm
	templateModel ormModel
	versionModel  ormModel
	abTestModel   ormModel
}

func NewPromptTemplateRepo(o orm.IOrm) PromptTemplateRepo {
	return &promptTemplateRepoImpl{
		orm:           o,
		templateModel: newOrmModel(&entity.PromptTemplate{}, (entity.PromptTemplate{}).TableName()),
		versionModel:  newOrmModel(&entity.PromptVersion{}, (entity.PromptVersion{}).TableName()),
		abTestModel:   newOrmModel(&entity.ABTest{}, (entity.ABTest{}).TableName()),
	}
}

func (r *promptTemplateRepoImpl) GetByID(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
	var tmpl entity.PromptTemplate
	model, err := r.templateModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建提示词模板 model 失败")
	}
	err = model.First(ctx, &tmpl, orm.WithWhere("id = ?", id))
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询提示词模板失败")
	}
	return &tmpl, nil
}

// Upsert 依据 name+scope+scope_id 覆盖或新增模板
func (r *promptTemplateRepoImpl) Upsert(ctx context.Context, tmpl *entity.PromptTemplate) error {
	session, err := r.orm.Begin(ctx)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "开启提示词模板事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.templateModel.model(session)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建提示词模板 model 失败")
	}

	var existing entity.PromptTemplate
	err = model.First(ctx, &existing,
		orm.WithWhere("name = ? AND scope = ? AND scope_id = ?", tmpl.Name, tmpl.Scope, tmpl.ScopeID),
		orm.WithForUpdate(),
	)
	if err != nil && !errorx.Is(err, errorx.NotFound) {
		return errorx.Wrap(err, errorx.Database, "查询提示词模板失败")
	}

	if errorx.Is(err, errorx.NotFound) {
		if tmpl.Version <= 0 {
			tmpl.Version = 1
		}
		if err := model.Create(ctx, tmpl); err != nil {
			return errorx.Wrap(err, errorx.Database, "创建提示词模板失败")
		}
	} else {
		tmpl.ID = existing.ID
		if tmpl.Version <= existing.Version {
			tmpl.Version = existing.Version + 1
		}
		updateValues := map[string]any{
			"category":       tmpl.Category,
			"content":        tmpl.Content,
			"variables_json": tmpl.VariablesJSON,
			"version":        tmpl.Version,
			"parent_id":      tmpl.ParentID,
			"priority":       tmpl.Priority,
			"enabled":        tmpl.Enabled,
			"tags_json":      tmpl.TagsJSON,
			"metadata_json":  tmpl.MetadataJSON,
		}
		if err := model.UpdateValues(ctx, updateValues, orm.WithWhere("id = ?", existing.ID)); err != nil {
			return errorx.Wrap(err, errorx.Database, "更新提示词模板失败")
		}
	}

	if err := session.Commit(); err != nil {
		return errorx.Wrap(err, errorx.Database, "提交提示词模板事务失败")
	}
	committed = true
	return nil
}

// FindEffective 获取作用域内优先级最高的提示词模板（避免跨作用域串租）
// 仅在当前作用域与全局作用域中查找，防止 user/project/org 之间因相同 ID 误匹配。
func (r *promptTemplateRepoImpl) FindEffective(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
	scopeOrder := fmt.Sprintf(`
		CASE 
			WHEN scope = '%s' AND scope_id = %d THEN 1
			WHEN scope = '%s' THEN 2
			ELSE 3
		END`,
		scope, scopeID,
		entity.PromptScopeGlobal,
	)

	var tmpl entity.PromptTemplate
	model, err := r.templateModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建提示词模板 model 失败")
	}
	err = model.First(ctx, &tmpl,
		orm.WithWhere("name = ? AND enabled = ?", name, true),
		orm.WithWhere("(scope = ? AND scope_id = 0) OR (scope = ? AND scope_id = ?)", entity.PromptScopeGlobal, scope, scopeID),
		orm.WithOrderBy(scopeOrder, false),
		orm.WithOrderBy("priority", false),
		orm.WithOrderBy("id", false),
	)
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询提示词模板失败")
	}
	return &tmpl, nil
}

// List 列出提示词模板
func (r *promptTemplateRepoImpl) List(ctx context.Context, filter PromptFilter) ([]*entity.PromptTemplate, error) {
	opts := []orm.QueryOption{}
	if filter.Name != "" {
		opts = append(opts, orm.WithWhere("name = ?", filter.Name))
	}
	if filter.Category != "" {
		opts = append(opts, orm.WithWhere("category = ?", filter.Category))
	}
	if filter.Scope != nil {
		opts = append(opts, orm.WithWhere("scope = ?", *filter.Scope))
	}
	if filter.ScopeID != nil {
		opts = append(opts, orm.WithWhere("scope_id = ?", *filter.ScopeID))
	}
	if filter.Enabled != nil {
		opts = append(opts, orm.WithWhere("enabled = ?", *filter.Enabled))
	}
	opts = append(opts,
		orm.WithOrderBy("name", false),
		orm.WithOrderBy("priority", false),
		orm.WithOrderBy("id", false),
	)

	var list []*entity.PromptTemplate
	model, err := r.templateModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建提示词模板 model 失败")
	}
	if err := model.Find(ctx, &list, opts...); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "查询提示词模板列表失败")
	}
	return list, nil
}

func (r *promptTemplateRepoImpl) SaveVersion(ctx context.Context, version *entity.PromptVersion) error {
	if version == nil {
		return nil
	}
	if version.Version == 0 {
		version.Version = 1
	}
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建提示词版本 model 失败")
	}
	if err := model.Create(ctx, version); err != nil {
		return errorx.Wrap(err, errorx.Database, "保存提示词版本失败")
	}
	return nil
}

func (r *promptTemplateRepoImpl) GetVersion(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error) {
	var v entity.PromptVersion
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建提示词版本 model 失败")
	}
	err = model.First(ctx, &v,
		orm.WithWhere("template_id = ? AND version = ?", templateID, version),
	)
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询提示词版本失败")
	}
	return &v, nil
}

func (r *promptTemplateRepoImpl) SaveABTest(ctx context.Context, test *entity.ABTest) error {
	if test == nil {
		return errorx.New(errorx.InvalidInput, "A/B 测试不能为空")
	}
	model, err := r.abTestModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 A/B 测试 model 失败")
	}
	if err := model.Create(ctx, test); err != nil {
		return errorx.Wrap(err, errorx.Database, "保存 A/B 测试失败")
	}
	return nil
}

func (r *promptTemplateRepoImpl) UpdateABTest(ctx context.Context, test *entity.ABTest) error {
	if test == nil || test.ID == 0 {
		return errorx.New(errorx.InvalidInput, "A/B 测试 ID 无效")
	}
	model, err := r.abTestModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 A/B 测试 model 失败")
	}
	if err := model.Save(ctx, test, orm.WithWhere("id = ?", test.ID)); err != nil {
		return errorx.Wrap(err, errorx.Database, "更新 A/B 测试失败")
	}
	return nil
}

func (r *promptTemplateRepoImpl) GetABTest(ctx context.Context, id int64) (*entity.ABTest, error) {
	if id <= 0 {
		return nil, errorx.New(errorx.InvalidInput, "A/B 测试 ID 无效")
	}
	var test entity.ABTest
	model, err := r.abTestModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 A/B 测试 model 失败")
	}
	err = model.First(ctx, &test, orm.WithWhere("id = ?", id))
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询 A/B 测试失败")
	}
	return &test, nil
}
