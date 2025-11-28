package client

import "context"

type mockClient struct{}

func (m *mockClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{
		Content: `{"story_segment":"这是一个本地 mock 的故事片段，用于开发环境。","highlight_task_ids":[],"proposals":[]}`,
	}, nil
}
