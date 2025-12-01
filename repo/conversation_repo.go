package repo

import (
	"context"
	stdErrors "errors"

	"gochen-llm/entity"
	"gochen/data/orm"
	"gochen/errors"
)

// ConversationRepo 会话仓储
type ConversationRepo interface {
	CreateConversation(ctx context.Context, conv *entity.Conversation) error
	GetConversation(ctx context.Context, id int64) (*entity.Conversation, error)
	UpdateConversation(ctx context.Context, conv *entity.Conversation) error
	AddMessage(ctx context.Context, msg *entity.Message) error
	GetMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error)
	TrimMessages(ctx context.Context, conversationID int64, keepLast int) error
}

type conversationRepoImpl struct {
	orm               orm.IOrm
	conversationModel ormModel
	messageModel      ormModel
}

func NewConversationRepo(o orm.IOrm) ConversationRepo {
	return &conversationRepoImpl{
		orm:               o,
		conversationModel: newOrmModel(&entity.Conversation{}, (entity.Conversation{}).TableName()),
		messageModel:      newOrmModel(&entity.Message{}, (entity.Message{}).TableName()),
	}
}

func (r *conversationRepoImpl) CreateConversation(ctx context.Context, conv *entity.Conversation) error {
	if err := r.conversationModel.model(r.orm).Create(ctx, conv); err != nil {
		return errors.WrapDbError(ctx, err, "创建会话失败")
	}
	return nil
}

func (r *conversationRepoImpl) GetConversation(ctx context.Context, id int64) (*entity.Conversation, error) {
	var conv entity.Conversation
	err := r.conversationModel.model(r.orm).First(ctx, &conv, orm.WithWhere("id = ?", id))
	if err != nil {
		if stdErrors.Is(err, orm.ErrNotFound) {
			return nil, nil
		}
		return nil, errors.WrapDbError(ctx, err, "查询会话失败")
	}
	return &conv, nil
}

func (r *conversationRepoImpl) UpdateConversation(ctx context.Context, conv *entity.Conversation) error {
	if err := r.conversationModel.model(r.orm).Save(ctx, conv, orm.WithWhere("id = ?", conv.ID)); err != nil {
		return errors.WrapDbError(ctx, err, "更新会话失败")
	}
	return nil
}

func (r *conversationRepoImpl) AddMessage(ctx context.Context, msg *entity.Message) error {
	if err := r.messageModel.model(r.orm).Create(ctx, msg); err != nil {
		return errors.WrapDbError(ctx, err, "添加消息失败")
	}
	return nil
}

func (r *conversationRepoImpl) GetMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var messages []*entity.Message
	if err := r.messageModel.model(r.orm).Find(ctx, &messages,
		orm.WithWhere("conversation_id = ?", conversationID),
		orm.WithOrderBy("created_at", true),
		orm.WithLimit(limit),
	); err != nil {
		return nil, errors.WrapDbError(ctx, err, "查询消息列表失败")
	}
	return messages, nil
}

func (r *conversationRepoImpl) TrimMessages(ctx context.Context, conversationID int64, keepLast int) error {
	if keepLast <= 0 {
		keepLast = 100
	}

	var ids []int64
	err := r.messageModel.model(r.orm).Find(ctx, &ids,
		orm.WithSelect("id"),
		orm.WithWhere("conversation_id = ?", conversationID),
		orm.WithOrderBy("created_at", true),
		orm.WithOffset(keepLast),
	)
	if err != nil {
		return errors.WrapDbError(ctx, err, "查询待删除消息失败")
	}
	if len(ids) == 0 {
		return nil
	}

	if err := r.messageModel.model(r.orm).Delete(ctx, orm.WithWhere("id IN ?", ids)); err != nil {
		return errors.WrapDbError(ctx, err, "压缩会话消息失败")
	}
	return nil
}
