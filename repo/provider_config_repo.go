package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

// ProviderConfigRepo 管理多源 LLM 端点配置
type ProviderConfigRepo interface {
	// ListAll 返回所有配置（包括未启用的），按 Priority 升序、ID 升序排序
	ListAll(ctx context.Context) ([]*entity.ProviderConfig, error)
	// ReplaceAll 用新的配置集合替换现有配置（用于运维批量更新）
	ReplaceAll(ctx context.Context, configs []*entity.ProviderConfig) error
	// UpdatePricing 仅更新单价，避免误改敏感字段
	UpdatePricing(ctx context.Context, updates []entity.ProviderPricing) error
}

type providerConfigRepoImpl struct {
	orm   orm.IOrm
	model ormModel
}

func NewProviderConfigRepo(o orm.IOrm) ProviderConfigRepo {
	return &providerConfigRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.ProviderConfig{}, (entity.ProviderConfig{}).TableName()),
	}
}

func (r *providerConfigRepoImpl) ListAll(ctx context.Context) ([]*entity.ProviderConfig, error) {
	var cfgs []*entity.ProviderConfig
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 LLM provider model 失败")
	}
	if err := model.Find(ctx, &cfgs,
		orm.WithOrderBy("priority", false),
		orm.WithOrderBy("id", false),
	); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "查询 LLM provider 配置失败")
	}
	return cfgs, nil
}

func (r *providerConfigRepoImpl) ReplaceAll(ctx context.Context, configs []*entity.ProviderConfig) error {
	session, err := r.orm.Begin(ctx)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "开启 LLM provider 配置事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.model.model(session)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 LLM provider model 失败")
	}

	if err := model.Delete(ctx, orm.WithWhere("1 = 1")); err != nil {
		return errorx.Wrap(err, errorx.Database, "清空 LLM provider 配置失败")
	}

	if len(configs) > 0 {
		if err := model.Create(ctx, anyPtrSlice(configs)...); err != nil {
			return errorx.Wrap(err, errorx.Database, "批量保存 LLM provider 配置失败")
		}
	}

	if err := session.Commit(); err != nil {
		return errorx.Wrap(err, errorx.Database, "提交 LLM provider 配置事务失败")
	}
	committed = true
	return nil
}

func (r *providerConfigRepoImpl) UpdatePricing(ctx context.Context, updates []entity.ProviderPricing) error {
	if len(updates) == 0 {
		return nil
	}
	session, err := r.orm.Begin(ctx)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "开启更新 LLM 单价事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.model.model(session)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 LLM provider model 失败")
	}

	for _, up := range updates {
		if up.ID <= 0 {
			return errorx.New(errorx.InvalidInput, "pricing id 无效")
		}
		if up.InputPricePer1k < 0 || up.OutputPricePer1k < 0 {
			return errorx.New(errorx.Validation, "单价不能为负数")
		}

		updateValues := map[string]any{
			"input_price_per1k":  up.InputPricePer1k,
			"output_price_per1k": up.OutputPricePer1k,
		}
		if err := model.UpdateValues(ctx, updateValues, orm.WithWhere("id = ?", up.ID)); err != nil {
			return errorx.Wrap(err, errorx.Database, "更新 LLM 单价失败")
		}
	}

	if err := session.Commit(); err != nil {
		return errorx.Wrap(err, errorx.Database, "提交更新 LLM 单价事务失败")
	}
	committed = true
	return nil
}
