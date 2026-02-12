package repo

import (
	"context"
	"math"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
)

// MetricsRepo 持久化 LLM 调用指标
type MetricsRepo interface {
	Save(ctx context.Context, m *entity.Metrics) error
	Aggregate(ctx context.Context, filter entity.MetricsFilter) (*entity.MetricsReport, error)
	AggregateByVariant(ctx context.Context, filter entity.MetricsFilter) ([]*entity.VariantMetricsReport, error)
	List(ctx context.Context, filter entity.MetricsFilter, limit, offset int) ([]*entity.Metrics, int64, error)
	Significance(ctx context.Context, filter entity.MetricsFilter) (*entity.ABSignificanceReport, error)
}

type metricsRepoImpl struct {
	orm   orm.IOrm
	model ormModel
}

func NewMetricsRepo(o orm.IOrm) MetricsRepo {
	return &metricsRepoImpl{
		orm:   o,
		model: newOrmModel(&entity.Metrics{}, (entity.Metrics{}).TableName()),
	}
}

func (r *metricsRepoImpl) Save(ctx context.Context, m *entity.Metrics) error {
	if m == nil {
		return errorx.New(errorx.InvalidInput, "metrics 不能为空")
	}
	model, err := r.model.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 metrics model 失败")
	}
	if err := model.Create(ctx, m); err != nil {
		return errorx.Wrap(err, errorx.Database, "保存 LLM 指标失败")
	}
	return nil
}

func (r *metricsRepoImpl) Aggregate(ctx context.Context, filter entity.MetricsFilter) (*entity.MetricsReport, error) {
	report := &entity.MetricsReport{}

	selects := []string{
		"COUNT(*) as total_calls",
		"SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END) AS success_calls",
		"SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_calls",
		"SUM(CASE WHEN status = 'converted' THEN 1 ELSE 0 END) AS conversion_calls",
		"SUM(request_tokens) as total_request_tokens",
		"SUM(response_tokens) as total_response_tokens",
		"SUM(total_tokens) as total_tokens",
		"AVG(latency_ms) as avg_latency_ms",
		"SUM(cost_usd) as total_cost_usd",
	}

	opts := append(buildMetricsOptions(filter), orm.WithSelect(selects...))

	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 metrics model 失败")
	}
	if err := model.First(ctx, report, opts...); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "汇总 LLM 指标失败")
	}

	if report.TotalCalls > 0 {
		report.SuccessRate = float64(report.SuccessCalls) / float64(report.TotalCalls)
		report.ConversionRate = float64(report.ConversionCalls) / float64(report.TotalCalls)
	}

	return report, nil
}

func (r *metricsRepoImpl) AggregateByVariant(ctx context.Context, filter entity.MetricsFilter) ([]*entity.VariantMetricsReport, error) {
	if filter.ABTestID == nil {
		return nil, errorx.New(errorx.InvalidInput, "ab_test_id 不能为空")
	}

	opts := buildMetricsOptions(filter)
	opts = append(opts, orm.WithWhere("ab_test_id = ?", *filter.ABTestID))

	type row struct {
		Variant string
		entity.MetricsReport
	}
	var rows []row
	selects := []string{
		"ab_variant as variant",
		"COUNT(*) as total_calls",
		"SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END) AS success_calls",
		"SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_calls",
		"SUM(CASE WHEN status = 'converted' THEN 1 ELSE 0 END) AS conversion_calls",
		"SUM(request_tokens) as total_request_tokens",
		"SUM(response_tokens) as total_response_tokens",
		"SUM(total_tokens) as total_tokens",
		"AVG(latency_ms) as avg_latency_ms",
		"SUM(cost_usd) as total_cost_usd",
	}

	queryOpts := append(opts, orm.WithSelect(selects...), orm.WithGroupBy("ab_variant"))

	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 metrics model 失败")
	}
	if err := model.Find(ctx, &rows, queryOpts...); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "按变体汇总 LLM 指标失败")
	}

	result := make([]*entity.VariantMetricsReport, 0, len(rows))
	for _, rrow := range rows {
		if rrow.MetricsReport.TotalCalls > 0 {
			rrow.MetricsReport.SuccessRate = float64(rrow.MetricsReport.SuccessCalls) / float64(rrow.MetricsReport.TotalCalls)
		}
		result = append(result, &entity.VariantMetricsReport{
			Variant: rrow.Variant,
			Metrics: rrow.MetricsReport,
		})
	}
	return result, nil
}

func (r *metricsRepoImpl) Significance(ctx context.Context, filter entity.MetricsFilter) (*entity.ABSignificanceReport, error) {
	if filter.ABTestID == nil {
		return nil, errorx.New(errorx.InvalidInput, "ab_test_id 不能为空")
	}

	// 基于成功调用作为曝光，转换事件为 status=converted（可按 outcome 过滤）
	exposureFilter := filter
	exposureFilter.Status = "ok"
	exposureFilter.ABVariant = ""
	exposureFilter.Outcome = ""
	exposures, err := r.queryVariantCount(ctx, exposureFilter)
	if err != nil {
		return nil, err
	}

	convFilter := filter
	convFilter.Status = "converted"
	convFilter.ABVariant = ""
	conversions, err := r.queryVariantCount(ctx, convFilter)
	if err != nil {
		return nil, err
	}

	aTotal := exposures["A"]
	bTotal := exposures["B"]
	aConv := conversions["A"]
	bConv := conversions["B"]

	report := &entity.ABSignificanceReport{
		ABTestID: *filter.ABTestID,
		Outcome:  filter.Outcome,
	}

	report.VariantA = buildVariantReport("A", aTotal, aConv)
	report.VariantB = buildVariantReport("B", bTotal, bConv)

	if aTotal == 0 || bTotal == 0 {
		report.Note = "样本不足，无法计算显著性"
		report.PValue = 1
		report.Confidence = 0
		return report, nil
	}

	pValue := calcPValue(aConv, aTotal, bConv, bTotal)
	report.PValue = pValue
	report.Confidence = maxFloat(0, 1-pValue)

	rateA := 0.0
	rateB := 0.0
	if aTotal > 0 {
		rateA = float64(aConv) / float64(aTotal)
	}
	if bTotal > 0 {
		rateB = float64(bConv) / float64(bTotal)
	}
	report.Lift = rateB - rateA

	if rateA > rateB {
		report.Winner = "A"
	} else if rateB > rateA {
		report.Winner = "B"
	} else {
		report.Winner = "tie"
	}
	return report, nil
}

func (r *metricsRepoImpl) queryVariantCount(ctx context.Context, filter entity.MetricsFilter) (map[string]int64, error) {
	type row struct {
		Variant string
		Count   int64
	}
	var rows []row

	opts := append(buildMetricsOptions(filter),
		orm.WithSelect("ab_variant as variant", "COUNT(*) as count"),
		orm.WithGroupBy("ab_variant"),
	)

	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 metrics model 失败")
	}
	if err := model.Find(ctx, &rows, opts...); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "统计 A/B 指标失败")
	}

	result := map[string]int64{
		"A": 0,
		"B": 0,
	}
	for _, rrow := range rows {
		if rrow.Variant == "" {
			continue
		}
		result[rrow.Variant] = rrow.Count
	}
	return result, nil
}

func calcPValue(aConv, aTotal, bConv, bTotal int64) float64 {
	if aTotal == 0 || bTotal == 0 {
		return 1
	}
	pA := float64(aConv) / float64(aTotal)
	pB := float64(bConv) / float64(bTotal)

	pooled := float64(aConv+bConv) / float64(aTotal+bTotal)
	denominator := pooled * (1 - pooled) * (1/float64(aTotal) + 1/float64(bTotal))
	if denominator <= 0 {
		return 1
	}
	z := (pA - pB) / math.Sqrt(denominator)
	// 双侧检验
	p := 2 * (1 - 0.5*(1+math.Erf(math.Abs(z)/math.Sqrt2)))
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

func buildVariantReport(variant string, total, conv int64) *entity.VariantMetricsReport {
	report := &entity.VariantMetricsReport{
		Variant: variant,
		Metrics: entity.MetricsReport{
			TotalCalls:      int(total),
			ConversionCalls: int(conv),
		},
	}
	if total > 0 {
		report.Metrics.ConversionRate = float64(conv) / float64(total)
	}
	return report
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (r *metricsRepoImpl) List(ctx context.Context, filter entity.MetricsFilter, limit, offset int) ([]*entity.Metrics, int64, error) {
	opts := buildMetricsOptions(filter)
	model, err := r.model.model(r.orm)
	if err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "创建 metrics model 失败")
	}

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := model.Count(ctx, opts...)
	if err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "统计 LLM 指标总数失败")
	}

	listOpts := append(opts,
		orm.WithOrderBy("created_at", true),
		orm.WithLimit(limit),
		orm.WithOffset(offset),
	)

	var list []*entity.Metrics
	if err := model.Find(ctx, &list, listOpts...); err != nil {
		return nil, 0, errorx.Wrap(err, errorx.Database, "查询 LLM 指标列表失败")
	}
	return list, total, nil
}

func buildMetricsOptions(filter entity.MetricsFilter) []orm.QueryOption {
	opts := []orm.QueryOption{}
	if filter.Provider != "" {
		opts = append(opts, orm.WithWhere("provider = ?", filter.Provider))
	}
	if filter.Model != "" {
		opts = append(opts, orm.WithWhere("model = ?", filter.Model))
	}
	if filter.UserID != nil {
		opts = append(opts, orm.WithWhere("user_id = ?", *filter.UserID))
	}
	if filter.Status != "" {
		opts = append(opts, orm.WithWhere("status = ?", filter.Status))
	}
	if filter.ABTestID != nil {
		opts = append(opts, orm.WithWhere("ab_test_id = ?", *filter.ABTestID))
	}
	if filter.ABVariant != "" {
		opts = append(opts, orm.WithWhere("ab_variant = ?", filter.ABVariant))
	}
	if filter.StartAt != nil {
		opts = append(opts, orm.WithWhere("created_at >= ?", *filter.StartAt))
	}
	if filter.EndAt != nil {
		opts = append(opts, orm.WithWhere("created_at <= ?", *filter.EndAt))
	}
	if filter.Outcome != "" {
		opts = append(opts, orm.WithWhere("outcome = ?", filter.Outcome))
	}
	return opts
}
