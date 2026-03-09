package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
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

func NewPromptVersionRepo(o orm.IOrm) IPromptVersionRepository {
	return &promptVersionRepoImpl{
		orm:          o,
		versionModel: newOrmModel(&entity.PromptVersion{}, (entity.PromptVersion{}).TableName()),
	}
}

func (r *promptVersionRepoImpl) Save(ctx context.Context, version *entity.PromptVersion) error {
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建提示词版本 model 失败")
	}
	if err := model.Create(ctx, version); err != nil {
		return errorx.Wrap(err, errorx.Database, "创建提示词版本失败")
	}
	return nil
}

func (r *promptVersionRepoImpl) Get(ctx context.Context, templateID int64, version int) (*entity.PromptVersion, error) {
	var v entity.PromptVersion
	model, err := r.versionModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建提示词版本 model 失败")
	}
	err = model.First(ctx, &v, orm.WithWhere("template_id = ? AND version = ?", templateID, version))
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询提示词版本失败")
	}
	return &v, nil
}
