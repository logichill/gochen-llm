package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gochen-llm/entity"
	"gochen/errors"
)

type testConversationRepo struct {
	createConversationFn func(ctx context.Context, conv *entity.Conversation) error
	getConversationFn    func(ctx context.Context, id int64) (*entity.Conversation, error)
	updateConversationFn func(ctx context.Context, conv *entity.Conversation) error
	addMessageFn         func(ctx context.Context, msg *entity.Message) error
	getMessagesFn        func(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error)
	trimMessagesFn       func(ctx context.Context, conversationID int64, keepLast int) error
}

func (r *testConversationRepo) CreateConversation(ctx context.Context, conv *entity.Conversation) error {
	if r.createConversationFn != nil {
		return r.createConversationFn(ctx, conv)
	}
	return nil
}

func (r *testConversationRepo) FindConversation(ctx context.Context, id int64) (*entity.Conversation, error) {
	if r.getConversationFn != nil {
		return r.getConversationFn(ctx, id)
	}
	return nil, nil
}

func (r *testConversationRepo) UpdateConversation(ctx context.Context, conv *entity.Conversation) error {
	if r.updateConversationFn != nil {
		return r.updateConversationFn(ctx, conv)
	}
	return nil
}

func (r *testConversationRepo) AddMessage(ctx context.Context, msg *entity.Message) error {
	if r.addMessageFn != nil {
		return r.addMessageFn(ctx, msg)
	}
	return nil
}

func (r *testConversationRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
	if r.getMessagesFn != nil {
		return r.getMessagesFn(ctx, conversationID, limit)
	}
	return nil, nil
}

func (r *testConversationRepo) TrimMessages(ctx context.Context, conversationID int64, keepLast int) error {
	if r.trimMessagesFn != nil {
		return r.trimMessagesFn(ctx, conversationID, keepLast)
	}
	return nil
}

func TestConversationServiceCreateConversation(t *testing.T) {
	var created *entity.Conversation
	repo := &testConversationRepo{
		createConversationFn: func(ctx context.Context, conv *entity.Conversation) error {
			created = conv
			conv.ID = 100
			return nil
		},
	}
	svc := NewConversationService(repo)

	if _, err := svc.CreateConversation(context.Background(), 0, nil); err == nil || !errors.Is(err, errors.Validation) {
		t.Fatalf("expected validation error for invalid user id, got %v", err)
	}

	conv, err := svc.CreateConversation(context.Background(), 12, map[string]any{
		"title": "my title",
		"type":  "story",
		"foo":   "bar",
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if conv.ID != 100 {
		t.Fatalf("expected id assigned by repo")
	}
	if created == nil || created.Type != "story" || created.Title != "my title" || created.Status != "active" {
		t.Fatalf("unexpected created conversation: %#v", created)
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(created.MetadataJSON), &meta); err != nil {
		t.Fatalf("metadata json should be valid: %v", err)
	}
	if meta["foo"] != "bar" {
		t.Fatalf("expected foo in metadata, got %#v", meta)
	}

	conv2, err := svc.CreateConversation(context.Background(), 13, nil)
	if err != nil {
		t.Fatalf("create conversation with nil metadata failed: %v", err)
	}
	if conv2.Type != entity.ConversationTypeChat {
		t.Fatalf("expected default type chat, got %s", conv2.Type)
	}
}

func TestConversationServiceAddAndGetMessages(t *testing.T) {
	var gotMsg *entity.Message
	var gotLimit int
	repo := &testConversationRepo{
		addMessageFn: func(ctx context.Context, msg *entity.Message) error {
			gotMsg = msg
			return nil
		},
		getMessagesFn: func(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
			gotLimit = limit
			return []*entity.Message{{Role: "assistant", Content: "ok"}}, nil
		},
	}
	svc := NewConversationService(repo)

	if err := svc.AddMessage(context.Background(), 10, nil); err == nil || !errors.Is(err, errors.Validation) {
		t.Fatalf("expected validation error for nil message, got %v", err)
	}

	msg := &entity.Message{Role: "user", Content: "hello"}
	if err := svc.AddMessage(context.Background(), 10, msg); err != nil {
		t.Fatalf("add message failed: %v", err)
	}
	if gotMsg == nil || gotMsg.ConversationID != 10 {
		t.Fatalf("expected conversation id injected, got %#v", gotMsg)
	}

	msgs, err := svc.ListMessages(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 1 || gotLimit != 50 {
		t.Fatalf("expected default limit 50 and one message, got limit=%d len=%d", gotLimit, len(msgs))
	}
}

func TestConversationServiceSummarizeBranchAndCompress(t *testing.T) {
	basePromptID := int64(99)
	baseID := int64(88)
	var createdBranch *entity.Conversation
	var trimKeep int

	repo := &testConversationRepo{
		getMessagesFn: func(ctx context.Context, conversationID int64, limit int) ([]*entity.Message, error) {
			return []*entity.Message{
				{Role: "user", Content: "first"},
				{Role: "assistant", Content: "second"},
				{Role: "user", Content: strings.Repeat("x", 900)},
			}, nil
		},
		getConversationFn: func(ctx context.Context, id int64) (*entity.Conversation, error) {
			if id == 404 {
				return nil, nil
			}
			return &entity.Conversation{
				ID:               baseID,
				UserID:           7,
				Type:             "chat",
				Title:            "origin",
				PromptTemplateID: &basePromptID,
				MetadataJSON:     `{"trace":"x"}`,
			}, nil
		},
		createConversationFn: func(ctx context.Context, conv *entity.Conversation) error {
			createdBranch = conv
			conv.ID = 123
			return nil
		},
		trimMessagesFn: func(ctx context.Context, conversationID int64, keepLast int) error {
			trimKeep = keepLast
			return nil
		},
	}
	svc := NewConversationService(repo)

	summary, err := svc.SummarizeConversation(context.Background(), 1)
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if !strings.Contains(summary, "user: ") {
		t.Fatalf("unexpected summary content: %q", summary)
	}
	if len(summary) > 800 {
		t.Fatalf("summary should be capped at 800 chars, got %d", len(summary))
	}

	if _, err := svc.CreateBranch(context.Background(), 404, 10); err == nil || !errors.Is(err, errors.NotFound) {
		t.Fatalf("expected not found for missing base conversation, got %v", err)
	}

	branch, err := svc.CreateBranch(context.Background(), 88, 666)
	if err != nil {
		t.Fatalf("create branch failed: %v", err)
	}
	if branch.ID != 123 || createdBranch == nil {
		t.Fatalf("expected branch persisted")
	}
	if createdBranch.ParentID == nil || *createdBranch.ParentID != baseID {
		t.Fatalf("expected parent id inherited, got %#v", createdBranch.ParentID)
	}
	if !strings.Contains(createdBranch.Title, "(branch)") {
		t.Fatalf("expected branch title suffix, got %s", createdBranch.Title)
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(createdBranch.MetadataJSON), &meta); err != nil {
		t.Fatalf("branch metadata should be valid json: %v", err)
	}
	if meta["branch_from_message_id"] == nil {
		t.Fatalf("expected branch_from_message_id in metadata")
	}

	if err := svc.CompressHistory(context.Background(), 88); err != nil {
		t.Fatalf("compress history failed: %v", err)
	}
	if trimKeep != 100 {
		t.Fatalf("expected keepLast=100, got %d", trimKeep)
	}
}
