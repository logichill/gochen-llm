package repo

import (
	"context"
	"time"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

// AuditLogRepo 持久化审计日志
type AuditLogRepo interface {
	Save(ctx context.Context, log *entity.AuditLog) error
	List(ctx context.Context, filter AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error)
}

// RateLimitRepo 持久化限流窗口
type RateLimitRepo interface {
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

type AuditLogFilter struct {
	UserID       *int64
	Action       string
	Status       string
	ResourceType string
	StartAt      *time.Time
	EndAt        *time.Time
}

func NewAuditLogRepo(o orm.IOrm) AuditLogRepo {
	return &auditLogRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.AuditLog{}, (entity.AuditLog{}).TableName()),
	}
}

func NewRateLimitRepo(o orm.IOrm) RateLimitRepo {
	return &rateLimitRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.RateLimit{}, (entity.RateLimit{}).TableName()),
	}
}

func (r *auditLogRepoImpl) Save(ctx context.Context, log *entity.AuditLog) error {
	if log == nil {
		return errorx.New(errorx.InvalidInput, "audit log 不能为空")
	}
	model, err := r.model.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建审计日志 model 失败")
	}
	if err := model.Create(ctx, log); err != nil {
		return errorx.Wrap(err, errorx.Database, "保存审计日志失败")
	}
	return nil
}

func (r *auditLogRepoImpl) List(ctx context.Context, filter AuditLogFilter, limit, offset int) ([]*entity.AuditLog, int64, error) {
	filterOptions := buildAuditOptions(filter)
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "创建审计日志 model 失败")
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := model.Count(ctx, filterOptions...)
	if err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "统计审计日志失败")
	}

	listOptions := append(filterOptions,
		orm.WithOrderBy("created_at", true),
		orm.WithLimit(limit),
		orm.WithOffset(offset),
	)

	var list []*entity.AuditLog
	if err := model.Find(ctx, &list, listOptions...); err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "查询审计日志失败")
	}
	return list, total, nil
}

func (r *rateLimitRepoImpl) Increment(ctx context.Context, userID int64, resourceType string, windowStart time.Time, windowSizeSeconds int, deltaReq int, deltaTokens int) (*entity.RateLimit, error) {
	if userID <= 0 {
		return nil, errorx.New(errorx.InvalidInput, "userID 无效")
	}
	if resourceType == "" {
		resourceType = "default"
	}
	if windowSizeSeconds <= 0 {
		windowSizeSeconds = 60
	}

	session, err := r.orm.Begin(ctx)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "开启限流事务失败")
	}
	committed := false
	defer func() {
		if !committed {
			_ = session.Rollback()
		}
	}()

	model, err := r.model.model(session)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建限流 model 失败")
	}

	var result entity.RateLimit
	err = model.First(ctx, &result,
		orm.WithWhere("user_id = ? AND resource_type = ? AND window_start = ?", userID, resourceType, windowStart),
		orm.WithForUpdate(),
	)
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			result = entity.RateLimit{
				UserID:            userID,
				ResourceType:      resourceType,
				WindowStart:       windowStart,
				WindowSizeSeconds: windowSizeSeconds,
				RequestCount:      deltaReq,
				TokenCount:        deltaTokens,
			}
			if err := model.Create(ctx, &result); err != nil {
				return nil, errorx.Wrap(err, errorx.Database, "创建限流窗口失败")
			}
		} else {
			return nil, errorx.Wrap(err, errorx.Database, "查询限流窗口失败")
		}
	} else {
		result.RequestCount += deltaReq
		result.TokenCount += deltaTokens
		if err := model.Save(ctx, &result, orm.WithWhere("id = ?", result.ID)); err != nil {
			return nil, errorx.Wrap(err, errorx.Database, "更新限流计数失败")
		}
	}

	if err := session.Commit(); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "提交限流事务失败")
	}
	committed = true
	return &result, nil
}

func (r *rateLimitRepoImpl) ListRecent(ctx context.Context, resourceType string, limit int) ([]*entity.RateLimit, error) {
	opts := []orm.QueryOption{}
	if resourceType != "" {
		opts = append(opts, orm.WithWhere("resource_type = ?", resourceType))
	}
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
		return nil, errorx.Wrap(err, errorx.Database, "创建限流 model 失败")
	}
	if err := model.Find(ctx, &list, opts...); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "查询限流窗口失败")
	}
	return list, nil
}

func (r *rateLimitRepoImpl) SumSince(ctx context.Context, resourceType string, since time.Time) (int64, error) {
	opts := []orm.QueryOption{}
	if resourceType != "" {
		opts = append(opts, orm.WithWhere("resource_type = ?", resourceType))
	}
	if !since.IsZero() {
		opts = append(opts, orm.WithWhere("window_start >= ?", since))
	}

	var row struct {
		Total int64 `json:"total"`
	}
	model, err := r.model.model(r.orm)
	if err != nil {
		return 0, errorx.Wrap(err, errorx.Database, "创建限流 model 失败")
	}
	if err := model.First(ctx, &row, append(opts, orm.WithSelect("COALESCE(SUM(request_count), 0) as total"))...); err != nil {
		return 0, errorx.Wrap(err, errorx.Database, "统计限流请求数失败")
	}
	return row.Total, nil
}

func buildAuditOptions(filter AuditLogFilter) []orm.QueryOption {
	opts := []orm.QueryOption{}
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
