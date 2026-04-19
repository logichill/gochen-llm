package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errors"
)

// IPromptVersionRepository 负责提示词版本记录持久化。
type IPromptVersionRepository interface {
	Save(ctx context.Context, version *entity.PromptVersion) error
	Get(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error)
}

type promptVersionRepoImpl struct {
	orm          orm.IOrm
	versionModel ormModel
}

// NewPromptVersionRepo 创建提示词版本仓储。
func NewPromptVersionRepo(o orm.IOrm) IPromptVersionRepository {
	return &promptVersionRepoImpl{
		orm:          o,
		versionModel: newOrmModel(&entity.PromptVersion{}, (entity.PromptVersion{}).TableName()),
	}
}

// Save 保存数据。
func (r *promptVersionRepoImpl) Save(ctx context.Context, version *entity.PromptVersion) error {
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return errors.Wrap(err, errors.Database, "创建提示词版本 model 失败")
	}
	if err := model.Create(ctx, version); err != nil {
		return errors.Wrap(err, errors.Database, "创建提示词版本失败")
	}
	return nil
}

// Get 返回当前值。
func (r *promptVersionRepoImpl) Get(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error) {
	var v entity.PromptVersion
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "创建提示词版本 model 失败")
	}
	err = model.First(ctx, &v, orm.WithWhere("template_id = ? AND version = ?", templateID, version))
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, errors.Database, "查询提示词版本失败")
	}
	return &v, nil
}
