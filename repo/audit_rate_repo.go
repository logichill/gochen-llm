package repo

import (
	"context"
	"time"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errors"
)

// IAuditLogRepo 持久化审计日志
type IAuditLogRepo interface {
	Save(ctx context.Context, log *entity.AuditLog) error
	List(ctx context.Context, filter AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error)
}

// IRateLimitRepo 持久化限流窗口
type IRateLimitRepo interface {
	Increment(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error)
	ListRecent(ctx context.Context, resourceType string, limit int) ([]*entity.RateLimit, error)
	SumSince(ctx context.Context, resourceType string, since time.Time) (int64, error)
}

type auditLogRepoImpl struct {
	orm   orm.IOrm
	model ormModel
}

type rateLimitRepoImpl struct {
	orm   orm.IOrm
	model ormModel
}

// AuditLogFilter 定义审计日志列表查询的过滤条件。
type AuditLogFilter struct {
	UserID       *int64
	Action       string
	Status       string
	ResourceType string
	StartAt      *time.Time
	EndAt        *time.Time
}

// NewAuditLogRepo 创建审计日志仓储。
func NewAuditLogRepo(o orm.IOrm) IAuditLogRepo {
	return &auditLogRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.AuditLog{}, (entity.AuditLog{}).TableName()),
	}
}

// NewRateLimitRepo 创建限流窗口仓储。
func NewRateLimitRepo(o orm.IOrm) IRateLimitRepo {
	return &rateLimitRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.RateLimit{}, (entity.RateLimit{}).TableName()),
	}
}

// Save 持久化一条审计日志。
func (r *auditLogRepoImpl) Save(ctx context.Context, log *entity.AuditLog) error {
	if log == nil {
		return errors.NewCode(errors.InvalidInput, "audit log 不能为空")
	}
	model, err := r.model.model(r.orm)
	if err != nil {
		return errors.Wrap(err, errors.Database, "创建审计日志 model 失败")
	}
	if err := model.Create(ctx, log); err != nil {
		return errors.Wrap(err, errors.Database, "保存审计日志失败")
	}
	return nil
}

// List 按过滤条件分页查询审计日志。
func (r *auditLogRepoImpl) List(ctx context.Context, filter AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error) {
	// 1. 先构建查询条件并准备 model。
	filterOptions := buildAuditOptions(filter)
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.Database, "创建审计日志 model 失败")
	}

	return findPagedList[entity.AuditLog](ctx, model, filterOptions, limit, offset, pagedListOptions{
		DefaultLimit: 50,
		MaxLimit:     200,
		OrderBy:      "created_at",
		Desc:         true,
		CountError:   "统计审计日志失败",
		ListError:    "查询审计日志失败",
	})
}

// Increment 对指定用户和资源类型的限流窗口做累加更新。
func (r *rateLimitRepoImpl) Increment(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error) {
	// 1. 先规范化调用参数，保证窗口键和默认窗口大小稳定。
	if userID <= 0 {
		return nil, errors.NewCode(errors.InvalidInput, "userID 无效")
	}
	if resourceType == "" {
		resourceType = "default"
	}
	if windowSizeSeconds <= 0 {
		windowSizeSeconds = 60
	}

	// 2. 开启事务，确保“查找窗口 + 创建/累加”是一个原子过程。
	session, err := r.orm.Begin(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "开启限流事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.model.model(session)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "创建限流 model 失败")
	}

	// 3. 先尝试锁定已有窗口；没有就创建，有就原地累加计数。
	var result entity.RateLimit
	err = model.First(ctx, &result,
		orm.WithWhere("user_id = ? AND resource_type = ? AND window_start = ?", userID, resourceType, windowStart),
		orm.WithForUpdate(),
	)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			result = entity.RateLimit{
				UserID:            userID,
				ResourceType:      resourceType,
				WindowStart:       windowStart,
				WindowSizeSeconds: windowSizeSeconds,
				RequestCount:      deltaReq,
				TokenCount:        deltaTokens,
			}
			if err := model.Create(ctx, &result); err != nil {
				return nil, errors.Wrap(err, errors.Database, "创建限流窗口失败")
			}
		} else {
			return nil, errors.Wrap(err, errors.Database, "查询限流窗口失败")
		}
	} else {
		result.RequestCount += deltaReq
		result.TokenCount += deltaTokens
		if err := model.Save(ctx, &result, orm.WithWhere("id = ?", result.ID)); err != nil {
			return nil, errors.Wrap(err, errors.Database, "更新限流计数失败")
		}
	}

	// 4. 事务提交成功后再返回最终窗口快照。
	if err := session.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.Database, "提交限流事务失败")
	}
	committed = true
	return &result, nil
}

// ListRecent 查询最近的限流窗口记录。
func (r *rateLimitRepoImpl) ListRecent(ctx context.Context, resourceType string, limit int) ([]*entity.RateLimit, error) {
	// 1. 先拼装可选资源过滤条件。
	opts := []orm.QueryOption{}
	if resourceType != "" {
		opts = append(opts, orm.WithWhere("resource_type = ?", resourceType))
	}

	// 2. 再统一排序与分页边界，保证最近窗口按时间倒序返回。
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	opts = append(opts,
		orm.WithOrderBy("window_start", true),
		orm.WithLimit(limit),
	)

	var list []*entity.RateLimit
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errors.Wrap(err, errors.Database, "创建限流 model 失败")
	}
	if err := model.Find(ctx, &list, opts...); err != nil {
		return nil, errors.Wrap(err, errors.Database, "查询限流窗口失败")
	}
	return list, nil
}

// SumSince 统计指定时间点之后的请求总量。
func (r *rateLimitRepoImpl) SumSince(ctx context.Context, resourceType string, since time.Time) (int64, error) {
	// 1. 先根据资源类型和起始时间拼装聚合过滤条件。
	opts := []orm.QueryOption{}
	if resourceType != "" {
		opts = append(opts, orm.WithWhere("resource_type = ?", resourceType))
	}
	if !since.IsZero() {
		opts = append(opts, orm.WithWhere("window_start >= ?", since))
	}

	// 2. 再用聚合查询返回 request_count 的累计值。
	var row struct {
		Total int64 `json:"total"`
	}
	model, err := r.model.model(r.orm)
	if err != nil {
		return 0, errors.Wrap(err, errors.Database, "创建限流 model 失败")
	}
	if err := model.First(ctx, &row, append(opts, orm.WithSelectExprUnsafe("COALESCE(SUM(request_count), 0) as total"))...); err != nil {
		return 0, errors.Wrap(err, errors.Database, "统计限流请求数失败")
	}
	return row.Total, nil
}

// buildAuditOptions 把审计日志过滤条件转换成 ORM 查询选项。
func buildAuditOptions(filter AuditLogFilter) []orm.QueryOption {
	// 1. 准备基础查询选项切片。
	opts := []orm.QueryOption{}

	// 2. 仅把非空过滤条件追加到 where 子句中，避免产生无效条件。
	if filter.UserID != nil {
		opts = append(opts, orm.WithWhere("user_id = ?", *filter.UserID))
	}
	if filter.Action != "" {
		opts = append(opts, orm.WithWhere("action = ?", filter.Action))
	}
	if filter.Status != "" {
		opts = append(opts, orm.WithWhere("status = ?", filter.Status))
	}
	if filter.ResourceType != "" {
		opts = append(opts, orm.WithWhere("resource_type = ?", filter.ResourceType))
	}
	if filter.StartAt != nil {
		opts = append(opts, orm.WithWhere("created_at >= ?", *filter.StartAt))
	}
	if filter.EndAt != nil {
		opts = append(opts, orm.WithWhere("created_at <= ?", *filter.EndAt))
	}
	return opts
}
