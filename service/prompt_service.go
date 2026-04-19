package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errors"
)

// IPromptService 定义提示词服务能力接口。
type IPromptService interface {
	FindPrompt(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error)
	FindPromptByID(ctx context.Context, id int64) (*entity.PromptTemplate, error)
	RenderPrompt(ctx context.Context, tmpl *entity.PromptTemplate, vars map[string]any) (string, error)
	ComposePrompts(ctx context.Context, names []string, scope entity.PromptScope, scopeID int64, vars map[string]any) (string, error)
	SavePrompt(ctx context.Context, tmpl *entity.PromptTemplate) error
	ListPrompts(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error)
	CreateVersion(ctx context.Context, templateID int64, changeLog string) (*entity.PromptVersion, error)
	RollbackVersion(ctx context.Context, templateID int64, version int) error
	ExportPrompts(ctx context.Context, filter repo.PromptFilter) ([]byte, error)
	ImportPrompts(ctx context.Context, data []byte) error
	StartABTest(ctx context.Context, test *entity.ABTest) error
	FindABTestResult(ctx context.Context, testID int64) (*entity.ABTest, error)
	AssignABVariant(ctx context.Context, testID int64, userID int64) (*entity.PromptTemplate, string, error)
}

type promptServiceImpl struct {
	templates repo.IPromptTemplateRepository
	versions  repo.IPromptVersionRepository
	abTests   repo.IABTestRepository
}

// NewPromptService 创建提示词服务。
func NewPromptService(templates repo.IPromptTemplateRepository, versions repo.IPromptVersionRepository, abTests repo.IABTestRepository) IPromptService {
	return &promptServiceImpl{templates: templates, versions: versions, abTests: abTests}
}

// FindPrompt 返回提示词。
func (s *promptServiceImpl) FindPrompt(ctx context.Context, name string, scope entity.PromptScope, scopeID int64) (*entity.PromptTemplate, error) {
	return s.templates.FindEffective(ctx, name, scope, scopeID)
}

// FindPromptByID 返回提示词按ID。
func (s *promptServiceImpl) FindPromptByID(ctx context.Context, id int64) (*entity.PromptTemplate, error) {
	return s.templates.Get(ctx, id)
}

// RenderPrompt 渲染提示词。
func (s *promptServiceImpl) RenderPrompt(ctx context.Context, tmpl *entity.PromptTemplate, vars map[string]any) (string, error) {
	if tmpl == nil {
		return "", errors.NewCode(errors.InvalidInput, "模板不能为空")
	}
	t, err := template.New("prompt").Parse(tmpl.Content)
	if err != nil {
		return "", errors.Wrap(err, errors.Internal, "解析提示词模板失败")
	}
	var buf bytes.Buffer
	if vars == nil {
		vars = map[string]any{}
	}
	if err := t.Execute(&buf, vars); err != nil {
		return "", errors.Wrap(err, errors.Internal, "渲染提示词模板失败")
	}
	return buf.String(), nil
}

// ComposePrompts 组合提示词列表。
func (s *promptServiceImpl) ComposePrompts(ctx context.Context, names []string, scope entity.PromptScope, scopeID int64, vars map[string]any) (string, error) {
	var buf bytes.Buffer
	for idx, name := range names {
		tmpl, err := s.FindPrompt(ctx, name, scope, scopeID)
		if err != nil {
			return "", err
		}
		if tmpl == nil {
			continue
		}
		rendered, err := s.RenderPrompt(ctx, tmpl, vars)
		if err != nil {
			return "", err
		}
		if idx > 0 && buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(rendered)
	}
	return buf.String(), nil
}

// SavePrompt 保存提示词。
func (s *promptServiceImpl) SavePrompt(ctx context.Context, tmpl *entity.PromptTemplate) error {
	if tmpl == nil {
		return errors.NewCode(errors.InvalidInput, "提示词模板不能为空")
	}
	if tmpl.Scope == "" {
		tmpl.Scope = entity.PromptScopeGlobal
	}
	if tmpl.Category == "" {
		tmpl.Category = "system"
	}
	if tmpl.Priority == 0 {
		tmpl.Priority = 100
	}
	if tmpl.Version == 0 {
		tmpl.Version = 1
	}

	if err := s.templates.Upsert(ctx, tmpl); err != nil {
		return err
	}

	// 记录版本历史
	version := &entity.PromptVersion{
		TemplateID:    tmpl.ID,
		Version:       tmpl.Version,
		Content:       tmpl.Content,
		VariablesJSON: tmpl.VariablesJSON,
		CreatedAt:     time.Now(),
	}
	return s.versions.Save(ctx, version)
}

// ListPrompts 列出提示词列表。
func (s *promptServiceImpl) ListPrompts(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error) {
	return s.listFilteredPrompts(ctx, filter)
}

// listFilteredPrompts 列出Filtered提示词列表。
func (s *promptServiceImpl) listFilteredPrompts(ctx context.Context, filter repo.PromptFilter) ([]*entity.PromptTemplate, error) {
	total, err := s.templates.Count(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(total)
	if limit <= 0 {
		return []*entity.PromptTemplate{}, nil
	}
	items, err := s.templates.List(ctx, 0, limit)
	if err != nil {
		return nil, err
	}
	filtered := make([]*entity.PromptTemplate, 0, len(items))
	for _, tmpl := range items {
		if tmpl == nil {
			continue
		}
		if filter.Name != "" && tmpl.Name != filter.Name {
			continue
		}
		if filter.Category != "" && tmpl.Category != filter.Category {
			continue
		}
		if filter.Scope != nil && tmpl.Scope != *filter.Scope {
			continue
		}
		if filter.ScopeID != nil && tmpl.ScopeID != *filter.ScopeID {
			continue
		}
		if filter.Enabled != nil && tmpl.Enabled != *filter.Enabled {
			continue
		}
		filtered = append(filtered, tmpl)
	}
	return filtered, nil
}

// CreateVersion 创建版本。
func (s *promptServiceImpl) CreateVersion(ctx context.Context, templateID int64, changeLog string) (*entity.PromptVersion, error) {
	if templateID <= 0 {
		return nil, errors.NewCode(errors.InvalidInput, "templateID 无效")
	}
	tmpl, err := s.templates.Get(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, errors.NewCode(errors.NotFound, "提示词模板不存在")
	}

	newVersion := tmpl.Version + 1
	version := &entity.PromptVersion{
		TemplateID:    tmpl.ID,
		Version:       newVersion,
		Content:       tmpl.Content,
		VariablesJSON: tmpl.VariablesJSON,
		ChangeLog:     changeLog,
		CreatedAt:     time.Now(),
	}

	if err := s.versions.Save(ctx, version); err != nil {
		return nil, err
	}

	// 将模板版本号推进，以便后续更新保持一致
	tmpl.Version = newVersion
	if err := s.templates.Upsert(ctx, tmpl); err != nil {
		return nil, err
	}

	return version, nil
}

// RollbackVersion 回滚版本。
func (s *promptServiceImpl) RollbackVersion(ctx context.Context, templateID int64, version int) error {
	if templateID <= 0 || version <= 0 {
		return errors.NewCode(errors.InvalidInput, "templateID 或 version 无效")
	}

	target, err := s.versions.Get(ctx, templateID, version)
	if err != nil {
		return err
	}
	if target == nil {
		return errors.NewCode(errors.NotFound, "指定版本不存在")
	}

	tmpl, err := s.templates.Get(ctx, templateID)
	if err != nil {
		return err
	}
	if tmpl == nil {
		return errors.NewCode(errors.NotFound, "提示词模板不存在")
	}

	// 回滚内容并创建新的版本记录，便于审计
	tmpl.Content = target.Content
	tmpl.VariablesJSON = target.VariablesJSON
	tmpl.Version = target.Version + 1

	if err := s.templates.Upsert(ctx, tmpl); err != nil {
		return err
	}

	rollbackVersion := &entity.PromptVersion{
		TemplateID:    tmpl.ID,
		Version:       tmpl.Version,
		Content:       tmpl.Content,
		VariablesJSON: tmpl.VariablesJSON,
		ChangeLog:     fmt.Sprintf("rollback to version %d", version),
		CreatedAt:     time.Now(),
	}
	return s.versions.Save(ctx, rollbackVersion)
}

// ExportPrompts 导出提示词列表。
func (s *promptServiceImpl) ExportPrompts(ctx context.Context, filter repo.PromptFilter) ([]byte, error) {
	list, err := s.ListPrompts(ctx, filter)
	if err != nil {
		return nil, err
	}
	return json.Marshal(list)
}

// ImportPrompts 导入提示词列表。
func (s *promptServiceImpl) ImportPrompts(ctx context.Context, data []byte) error {
	var list []*entity.PromptTemplate
	if err := json.Unmarshal(data, &list); err != nil {
		return errors.Wrap(err, errors.InvalidInput, "解析导入数据失败")
	}
	for _, tmpl := range list {
		if err := s.SavePrompt(ctx, tmpl); err != nil {
			return err
		}
	}
	return nil
}

// StartABTest 启动A/B测试。
func (s *promptServiceImpl) StartABTest(ctx context.Context, test *entity.ABTest) error {
	if test == nil {
		return errors.NewCode(errors.InvalidInput, "A/B 测试不能为空")
	}
	if test.TemplateAID <= 0 || test.TemplateBID <= 0 {
		return errors.NewCode(errors.Validation, "A/B 测试模板 ID 无效")
	}

	// 校验模板存在
	tmplA, err := s.templates.Get(ctx, test.TemplateAID)
	if err != nil {
		return err
	}
	if tmplA == nil {
		return errors.NewCode(errors.NotFound, "A/B 测试模板 A 不存在")
	}
	tmplB, err := s.templates.Get(ctx, test.TemplateBID)
	if err != nil {
		return err
	}
	if tmplB == nil {
		return errors.NewCode(errors.NotFound, "A/B 测试模板 B 不存在")
	}

	test.Status = "running"
	test.StartAt = time.Now()
	return s.abTests.Save(ctx, test)
}

// FindABTestResult 返回A/B测试结果。
func (s *promptServiceImpl) FindABTestResult(ctx context.Context, testID int64) (*entity.ABTest, error) {
	test, err := s.abTests.Get(ctx, testID)
	if err != nil || test == nil {
		return test, err
	}
	return test, nil
}

// AssignABVariant 分配A/B实验分组。
func (s *promptServiceImpl) AssignABVariant(ctx context.Context, testID int64, userID int64) (*entity.PromptTemplate, string, error) {
	if testID <= 0 {
		return nil, "", errors.NewCode(errors.InvalidInput, "ab_test_id 无效")
	}
	test, err := s.abTests.Get(ctx, testID)
	if err != nil {
		return nil, "", err
	}
	if test == nil || test.Status != "running" {
		return nil, "", errors.NewCode(errors.NotFound, "A/B 测试不可用")
	}

	traffic := test.TrafficSplit
	if traffic <= 0 || traffic >= 100 {
		traffic = 50
	}
	// 简单 hash 分配，保证同一 user 稳定
	hash := userID
	if hash < 0 {
		hash = -hash
	}
	slot := hash % 100
	var chosenID int64
	var variant string
	if slot < int64(traffic) {
		chosenID = test.TemplateAID
		variant = "A"
	} else {
		chosenID = test.TemplateBID
		variant = "B"
	}

	tmpl, err := s.templates.Get(ctx, chosenID)
	if err != nil {
		return nil, "", err
	}
	if tmpl == nil {
		return nil, "", errors.NewCode(errors.NotFound, "A/B 变体模板不存在")
	}

	// 记录简单曝光计数到 ResultJSON
	var result struct {
		TemplateAUses int `json:"template_a_uses"`
		TemplateBUses int `json:"template_b_uses"`
	}
	if strings.TrimSpace(test.ResultJSON) != "" {
		_ = json.Unmarshal([]byte(test.ResultJSON), &result)
	}
	if variant == "A" {
		result.TemplateAUses++
	} else {
		result.TemplateBUses++
	}
	data, _ := json.Marshal(result)
	test.ResultJSON = string(data)
	_ = s.abTests.Update(ctx, test)

	return tmpl, variant, nil
}
