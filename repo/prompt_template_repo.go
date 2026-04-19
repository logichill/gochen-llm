package repo

import (
	"context"
	"sort"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/db/orm/repo"
	domaincrud "gochen/domain/crud"
	"gochen/errors"
	"gochen/ident"
)

// PromptFilter 定义提示词过滤条件。
type PromptFilter struct {
	Name     string
	Category string
	Scope    *entity.PromptScope
	ScopeID  *int64
	Enabled  *bool
}

// IPromptTemplateRepository 负责提示词模板的主表读写。
type IPromptTemplateRepository interface {
	domaincrud.IRepository[*entity.PromptTemplate, int64]
	domaincrud.IQueryRepository[*entity.PromptTemplate, int64]
	Upsert(ctx context.Context, tmpl *entity.PromptTemplate) error
	FindEffective(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error)
}

type promptTemplateRepoImpl struct {
	*repo.Repo[*entity.PromptTemplate, int64]
	orm           orm.IOrm
	templateModel ormModel
}

// NewPromptTemplateRepo 创建提示词Template仓储。
func NewPromptTemplateRepo(o orm.IOrm) (IPromptTemplateRepository, error) {
	base, err := repo.NewRepo[*entity.PromptTemplate, int64](
		o,
		(entity.PromptTemplate{}).TableName(),
		repo.WithIDGenerator[*entity.PromptTemplate, int64](ident.DefaultInt64Generator()),
	)
	if err != nil {
		return nil, err
	}
	return &promptTemplateRepoImpl{
		Repo:          base,
		orm:           o,
		templateModel: newOrmModel(&entity.PromptTemplate{}, (entity.PromptTemplate{}).TableName()),
	}, nil
}

// Get 遵循通用仓储语义，未命中时返回 nil。
func (r *promptTemplateRepoImpl) Get(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
	tmpl, err := r.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil
		}
		return nil, err
	}
	return tmpl, nil
}

// Upsert 依据 name+scope+scope_id 覆盖或新增模板。
func (r *promptTemplateRepoImpl) Upsert(ctx context.Context, tmpl *entity.PromptTemplate) error {
	session, err := r.orm.Begin(ctx)
	if err != nil {
		return errors.Wrap(err, errors.Database, "开启提示词模板事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.templateModel.model(session)
	if err != nil {
		return errors.Wrap(err, errors.Database, "创建提示词模板 model 失败")
	}

	var existing entity.PromptTemplate
	err = model.First(ctx, &existing,
		orm.WithWhere("name = ? AND scope = ? AND scope_id = ?", tmpl.Name, tmpl.Scope, tmpl.ScopeID),
		orm.WithForUpdate(),
	)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Wrap(err, errors.Database, "查询提示词模板失败")
	}

	if errors.Is(err, errors.NotFound) {
		if tmpl.Version <= 0 {
			tmpl.Version = 1
		}
		if err := model.Create(ctx, tmpl); err != nil {
			return errors.Wrap(err, errors.Database, "创建提示词模板失败")
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
			return errors.Wrap(err, errors.Database, "更新提示词模板失败")
		}
	}

	if err := session.Commit(); err != nil {
		return errors.Wrap(err, errors.Database, "提交提示词模板事务失败")
	}
	committed = true
	return nil
}

// FindEffective 获取作用域内优先级最高的提示词模板（避免跨作用域串租）。
func (r *promptTemplateRepoImpl) FindEffective(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
	model, err := r.templateModel.model(r.orm)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "创建提示词模板 model 失败")
	}
	var templates []*entity.PromptTemplate
	err = model.Find(ctx, &templates,
		orm.WithWhere("name = ? AND enabled = ?", name, true),
		orm.WithWhere("(scope = ? AND scope_id = 0) OR (scope = ? AND scope_id = ?)", entity.PromptScopeGlobal, scope, scopeID),
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "查询生效提示词模板失败")
	}
	if len(templates) == 0 {
		return nil, nil
	}
	sort.Slice(templates, func(i, j int) bool {
		rank := func(t *entity.PromptTemplate) int {
			if t.Scope == scope && t.ScopeID == scopeID {
				return 1
			}
			if t.Scope == entity.PromptScopeGlobal && t.ScopeID == 0 {
				return 2
			}
			return 3
		}
		ri, rj := rank(templates[i]), rank(templates[j])
		if ri != rj {
			return ri < rj
		}
		if templates[i].Priority != templates[j].Priority {
			return templates[i].Priority < templates[j].Priority
		}
		return templates[i].Version > templates[j].Version
	})
	return templates[0], nil
}
