package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

// IABTestRepository 负责提示词 A/B 测试持久化。
type IABTestRepository interface {
	Save(ctx context.Context, test *entity.ABTest) error
	Update(ctx context.Context, test *entity.ABTest) error
	Get(ctx context.Context, id int64) (*entity.ABTest, error)
}

type abTestRepoImpl struct {
	orm         orm.IOrm
	abTestModel ormModel
}

// NewABTestRepo 创建A/B测试仓储。
func NewABTestRepo(o orm.IOrm) IABTestRepository {
	return &abTestRepoImpl{
		orm:         o,
		abTestModel: newOrmModel(&entity.ABTest{}, (entity.ABTest{}).TableName()),
	}
}

// Save 保存数据。
func (r *abTestRepoImpl) Save(ctx context.Context, test *entity.ABTest) error {
	model, err := r.abTestModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 A/B 测试 model 失败")
	}
	if err := model.Create(ctx, test); err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 A/B 测试失败")
	}
	return nil
}

// Update 更新记录。
func (r *abTestRepoImpl) Update(ctx context.Context, test *entity.ABTest) error {
	model, err := r.abTestModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 A/B 测试 model 失败")
	}
	if err := model.UpdateValues(ctx, map[string]any{
		"status":        test.Status,
		"traffic_split": test.TrafficSplit,
		"result_json":   test.ResultJSON,
		"end_at":        test.EndAt,
	}, orm.WithWhere("id = ?", test.ID)); err != nil {
		return errorx.Wrap(err, errorx.Database, "更新 A/B 测试失败")
	}
	return nil
}

// Get 返回当前值。
func (r *abTestRepoImpl) Get(ctx context.Context, id int64) (*entity.ABTest, error) {
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
