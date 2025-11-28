package repo

import "gochen/orm"

type ormModel struct {
	meta *orm.ModelMeta
}

func newOrmModel(model any, table string) ormModel {
	return ormModel{
		meta: &orm.ModelMeta{
			Model: model,
			Table: table,
		},
	}
}

func (m ormModel) model(o orm.IOrm) orm.IModel {
	return o.Model(m.meta)
}

func anySlice[T any](items []T) []any {
	if len(items) == 0 {
		return nil
	}
	result := make([]any, len(items))
	for i := range items {
		result[i] = items[i]
	}
	return result
}

func anyPtrSlice[T any](items []*T) []any {
	if len(items) == 0 {
		return nil
	}
	result := make([]any, len(items))
	for i := range items {
		result[i] = items[i]
	}
	return result
}
