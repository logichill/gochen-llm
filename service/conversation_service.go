package service

import (
	"context"
	"encoding/json"
	"strings"

	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
)

// ConversationService 会话服务
type ConversationService interface {
	CreateConversation(ctx context.Context, userID int64, metadata map[string]any) (*entity.Conversation, error)
	GetConversation(ctx context.Context, conversationID int64) (*entity.Conversation, error)
	AddMessage(ctx context.Context, conversationID int64, msg *entity.Message) error
	GetMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error)
	SummarizeConversation(ctx context.Context, conversationID int64) (string, error)
	CreateBranch(ctx context.Context, conversationID int64, fromMessageID int64) (*entity.Conversation, error)
	CompressHistory(ctx context.Context, conversationID int64) error
}

type conversationServiceImpl struct {
	repo repo.ConversationRepo
}

func NewConversationService(repo repo.ConversationRepo) ConversationService {
	return &conversationServiceImpl{repo: repo}
}

func (s *conversationServiceImpl) CreateConversation(ctx context.Context, userID int64, metadata map[string]any) (*entity.Conversation, error) {
	if userID <= 0 {
		return nil, errorx.New(errorx.Validation, "userID 无效")
	}

	conv := &entity.Conversation{
		UserID: userID,
		Status: "active",
	}

	// 处理 metadata
	if metadata != nil {
		if t, ok := metadata["type"].(string); ok {
			conv.Type = t
		}
		if title, ok := metadata["title"].(string); ok {
			conv.Title = title
		}
		metaJSON, _ := json.Marshal(metadata)
		conv.MetadataJSON = string(metaJSON)
	}

	if conv.Type == "" {
		conv.Type = entity.ConversationTypeChat
	}

	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *conversationServiceImpl) GetConversation(ctx context.Context, conversationID int64) (*entity.Conversation, error) {
	return s.repo.GetConversation(ctx, conversationID)
}

func (s *conversationServiceImpl) AddMessage(ctx context.Context, conversationID int64, msg *entity.Message) error {
	if msg == nil {
		return errorx.New(errorx.Validation, "消息不能为空")
	}
	msg.ConversationID = conversationID
	return s.repo.AddMessage(ctx, msg)
}

func (s *conversationServiceImpl) GetMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetMessages(ctx, conversationID, limit)
}

func (s *conversationServiceImpl) SummarizeConversation(ctx context.Context, conversationID int64) (string, error) {
	msgs, err := s.repo.GetMessages(ctx, conversationID, 50)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
		if sb.Len() > 800 {
			break
		}
	}
	summary := sb.String()
	if len(summary) > 800 {
		summary = summary[:800]
	}
	return summary, nil
}

func (s *conversationServiceImpl) CreateBranch(ctx context.Context, conversationID int64, fromMessageID int64) (*entity.Conversation, error) {
	base, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return nil, errorx.New(errorx.NotFound, "会话不存在")
	}

	meta := map[string]any{}
	if strings.TrimSpace(base.MetadataJSON) != "" {
		_ = json.Unmarshal([]byte(base.MetadataJSON), &meta)
	}
	meta["branch_from_message_id"] = fromMessageID
	metaJSON, _ := json.Marshal(meta)

	branch := &entity.Conversation{
		UserID:           base.UserID,
		ParentID:         &base.ID,
		Type:             base.Type,
		Title:            base.Title + " (branch)",
		Status:           "active",
		PromptTemplateID: base.PromptTemplateID,
		MetadataJSON:     string(metaJSON),
	}
	if err := s.repo.CreateConversation(ctx, branch); err != nil {
		return nil, err
	}
	return branch, nil
}

func (s *conversationServiceImpl) CompressHistory(ctx context.Context, conversationID int64) error {
	// 默认保留最近 100 条消息
	return s.repo.TrimMessages(ctx, conversationID, 100)
}
