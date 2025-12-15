package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/data/orm"
	"gochen/errors"
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
	err := r.model.model(r.orm).First(ctx, &policy, orm.WithWhere("id = ?", 1))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WrapDbError(ctx, err, "查询 LLM 安全配置失败")
	}
	return &policy, nil
}

func (r *safetyPolicyRepoImpl) Save(ctx context.Context, policy *entity.SafetyPolicy) error {
	if policy == nil {
		return nil
	}
	policy.ID = 1
	if err := r.model.model(r.orm).Save(ctx, policy); err != nil {
		return errors.WrapDbError(ctx, err, "保存 LLM 安全配置失败")
	}
	return nil
}
