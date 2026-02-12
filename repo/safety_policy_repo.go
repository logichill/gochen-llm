package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

// SafetyPolicyRepo 管理系统级 LLM 安全策略
type SafetyPolicyRepo interface {
	GetActive(ctx context.Context) (*entity.SafetyPolicy, error)
	Save(ctx context.Context, policy *entity.SafetyPolicy) error
}

type safetyPolicyRepoImpl struct {
	orm   orm.IOrm
	model ormModel
}

func NewSafetyPolicyRepo(o orm.IOrm) SafetyPolicyRepo {
	return &safetyPolicyRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.SafetyPolicy{}, (entity.SafetyPolicy{}).TableName()),
	}
}

func (r *safetyPolicyRepoImpl) GetActive(ctx context.Context) (*entity.SafetyPolicy, error) {
	var policy entity.SafetyPolicy
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 LLM safety policy model 失败")
	}
	err = model.First(ctx, &policy, orm.WithWhere("id = ?", 1))
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询 LLM 安全配置失败")
	}
	return &policy, nil
}

func (r *safetyPolicyRepoImpl) Save(ctx context.Context, policy *entity.SafetyPolicy) error {
	if policy == nil {
		return nil
	}
	policy.ID = 1
	model, err := r.model.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 LLM safety policy model 失败")
	}
	if err := model.Save(ctx, policy); err != nil {
		return errorx.Wrap(err, errorx.Database, "保存 LLM 安全配置失败")
	}
	return nil
}
