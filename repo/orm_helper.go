package repo

import "gochen/db/orm"

type ormModel struct {
	meta *orm.ModelMeta
}

// newOrmModel 创建Orm模型。
func newOrmModel(model any, table string) ormModel {
	return ormModel{
		meta: &orm.ModelMeta{
			ModelFactory: orm.NewModelFactoryFromSample(model),
			Table:        table,
		},
	}
}

// model 处理model。
func (m ormModel) model(o orm.IOrm) (orm.IModel, error) {
	return o.Model(m.meta)
}

// anySlice 处理anySlice。
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

// anyPtrSlice 处理anyPtrSlice。
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
