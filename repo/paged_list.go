package repo

import (
	"context"

	"gochen/db/orm"
	"gochen/errors"
)

type pagedListOptions struct {
	DefaultLimit int
	MaxLimit     int
	OrderBy      string
	Desc         bool
	CountError   string
	ListError    string
}

func findPagedList[T any](
	ctx context.Context,
	model orm.IModel,
	filterOptions []orm.QueryOption,
	limit int,
	offset int,
	opts pagedListOptions,
) ([]*T, int64, error) {
	if limit <= 0 || (opts.MaxLimit > 0 && limit > opts.MaxLimit) {
		limit = opts.DefaultLimit
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := model.Count(ctx, filterOptions...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.Database, opts.CountError)
	}

	listOptions := append([]orm.QueryOption{}, filterOptions...)
	if opts.OrderBy != "" {
		listOptions = append(listOptions, orm.WithOrderBy(opts.OrderBy, opts.Desc))
	}
	listOptions = append(listOptions, orm.WithLimit(limit), orm.WithOffset(offset))

	var list []*T
	if err := model.Find(ctx, &list, listOptions...); err != nil {
		return nil, 0, errors.Wrap(err, errors.Database, opts.ListError)
	}
	return list, total, nil
}
