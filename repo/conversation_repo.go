package repo

import (
	"context"

	"gochen-llm/entity"
	"gochen/db/orm"
	"gochen/errorx"
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
	model, err := r.conversationModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 conversation model 失败")
	}
	if err := model.Create(ctx, conv); err != nil {
		return errorx.Wrap(err, errorx.Database, "创建会话失败")
	}
	return nil
}

func (r *conversationRepoImpl) GetConversation(ctx context.Context, id int64) (*entity.Conversation, error) {
	var conv entity.Conversation
	model, err := r.conversationModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 conversation model 失败")
	}
	err = model.First(ctx, &conv, orm.WithWhere("id = ?", id))
	if err != nil {
		if errorx.Is(err, errorx.NotFound) {
			return nil, nil
		}
		return nil, errorx.Wrap(err, errorx.Database, "查询会话失败")
	}
	return &conv, nil
}

func (r *conversationRepoImpl) UpdateConversation(ctx context.Context, conv *entity.Conversation) error {
	model, err := r.conversationModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 conversation model 失败")
	}
	if err := model.Save(ctx, conv, orm.WithWhere("id = ?", conv.ID)); err != nil {
		return errorx.Wrap(err, errorx.Database, "更新会话失败")
	}
	return nil
}

func (r *conversationRepoImpl) AddMessage(ctx context.Context, msg *entity.Message) error {
	model, err := r.messageModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 message model 失败")
	}
	if err := model.Create(ctx, msg); err != nil {
		return errorx.Wrap(err, errorx.Database, "添加消息失败")
	}
	return nil
}

func (r *conversationRepoImpl) GetMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var messages []*entity.Message
	model, err := r.messageModel.model(r.orm)
	if err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "创建 message model 失败")
	}
	if err := model.Find(ctx, &messages,
		orm.WithWhere("conversation_id = ?", conversationID),
		orm.WithOrderBy("created_at", true),
		orm.WithLimit(limit),
	); err != nil {
		return nil, errorx.Wrap(err, errorx.Database, "查询消息列表失败")
	}
	return messages, nil
}

func (r *conversationRepoImpl) TrimMessages(ctx context.Context, conversationID int64, keepLast int) error {
	if keepLast <= 0 {
		keepLast = 100
	}

	model, err := r.messageModel.model(r.orm)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "创建 message model 失败")
	}

	var ids []int64
	err = model.Find(ctx, &ids,
		orm.WithSelect("id"),
		orm.WithWhere("conversation_id = ?", conversationID),
		orm.WithOrderBy("created_at", true),
		orm.WithOffset(keepLast),
	)
	if err != nil {
		return errorx.Wrap(err, errorx.Database, "查询待删除消息失败")
	}
	if len(ids) == 0 {
		return nil
	}

	if err := model.Delete(ctx, orm.WithWhere("id IN ?", ids)); err != nil {
		return errorx.Wrap(err, errorx.Database, "压缩会话消息失败")
	}
	return nil
}
