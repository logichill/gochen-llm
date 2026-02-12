package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"gochen-llm/client"
	"gochen-llm/entity"
	"gochen-llm/repo"
	"gochen/errorx"
	runtime "gochen/task"
)

type ChatService interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatWithPrompt(ctx context.Context, req *PromptChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error)
	BatchChat(ctx context.Context, reqs []*ChatRequest) ([]*ChatResponse, error)
}

type chatServiceImpl struct {
	manager     ProviderManager
	prompt      PromptService
	safety      SafetyService
	metricsRepo repo.MetricsRepo
	costCalc    CostCalculator
}

func NewChatService(manager ProviderManager, prompt PromptService, safety SafetyService, metrics repo.MetricsRepo, costCalc CostCalculator) ChatService {
	return &chatServiceImpl{
		manager:     manager,
		prompt:      prompt,
		safety:      safety,
		metricsRepo: metrics,
		costCalc:    costCalc,
	}
}

func (s *chatServiceImpl) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, errorx.New(errorx.InvalidInput, "ChatRequest 不能为空")
	}
	if s.manager == nil {
		return nil, errorx.New(errorx.Internal, "LLM ProviderManager 未配置")
	}

	// 安全策略：输入验证与系统提示拼接
	finalSystem := strings.TrimSpace(req.System)
	if s.safety != nil {
		if _, err := s.safety.CheckRateLimit(ctx, req.UserID); err != nil {
			return nil, err
		}
		if _, err := s.safety.ValidateInput(ctx, joinMessages(req.Messages)); err != nil {
			return nil, err
		}
		safetyPrompt, err := s.safety.BuildSystemPrompt(ctx)
		if err != nil {
			return nil, err
		}
		if safetyPrompt != "" {
			if finalSystem != "" {
				finalSystem = safetyPrompt + "\n\n" + finalSystem
			} else {
				finalSystem = safetyPrompt
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	temperature := req.Temperature
	if temperature < 0 {
		temperature = 0.7
	}

	clientReq := &client.ChatRequest{
		System:      finalSystem,
		Messages:    convertMessages(req.Messages),
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}
	resp, provider, model, latencyMs, inPricePer1k, outPricePer1k, err := s.manager.ChatForUser(ctx, req.UserID, clientReq)
	if err != nil {
		if s.metricsRepo != nil {
			var abTestID int64
			var abVariant string
			if v, ok := req.Metadata["ab_test_id"].(int64); ok {
				abTestID = v
			}
			if v, ok := req.Metadata["ab_variant"].(string); ok {
				abVariant = v
			}
			_ = s.metricsRepo.Save(ctx, &entity.Metrics{
				Provider:  provider,
				Model:     model,
				UserID:    req.UserID,
				ABTestID:  abTestID,
				ABVariant: abVariant,
				Status:    "error",
				ErrorType: err.Error(),
				CreatedAt: time.Now(),
			})
		}
		return nil, err
	}

	content := resp.Content
	if s.safety != nil {
		filtered, err := s.safety.FilterContent(ctx, content)
		if err != nil && filtered == "" {
			return nil, err
		}
		if filtered != "" {
			content = filtered
		}
	}

	result := &ChatResponse{
		Content:  content,
		Usage:    estimateUsage(finalSystem, req.Messages, content),
		Metadata: req.Metadata,
	}

	if s.metricsRepo != nil && result.Usage != nil {
		var abTestID int64
		var abVariant string
		var promptTemplateID int64
		if v, ok := req.Metadata["ab_test_id"].(int64); ok {
			abTestID = v
		}
		if v, ok := req.Metadata["ab_variant"].(string); ok {
			abVariant = v
		}
		if v, ok := req.Metadata["prompt_template_id"].(int64); ok {
			promptTemplateID = v
		}
		cost := 0.0
		if s.costCalc != nil {
			cost = s.costCalc.EstimateCost(provider, model, result.Usage.RequestTokens, result.Usage.ResponseTokens, inPricePer1k, outPricePer1k)
		}
		_ = s.metricsRepo.Save(ctx, &entity.Metrics{
			Provider:       provider,
			Model:          model,
			UserID:         req.UserID,
			ABTestID:       abTestID,
			ABVariant:      abVariant,
			PromptTemplate: promptTemplateID,
			RequestTokens:  result.Usage.RequestTokens,
			ResponseTokens: result.Usage.ResponseTokens,
			TotalTokens:    result.Usage.TotalTokens,
			LatencyMs:      int(latencyMs),
			Status:         "ok",
			ErrorType:      "",
			CreatedAt:      time.Now(),
			CostUSD:        cost,
		})
	}

	if s.safety != nil {
		body := map[string]any{
			"system":   finalSystem,
			"messages": req.Messages,
		}
		bodyJSON, _ := json.Marshal(body)
		respJSON, _ := json.Marshal(result)
		_ = s.safety.RecordAuditLog(ctx, &entity.AuditLog{
			UserID:       req.UserID,
			Action:       "llm.chat",
			RequestJSON:  string(bodyJSON),
			ResponseJSON: string(respJSON),
			Status:       "ok",
		})
	}

	return result, nil
}

func (s *chatServiceImpl) ChatWithPrompt(ctx context.Context, req *PromptChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, errorx.New(errorx.InvalidInput, "PromptChatRequest 不能为空")
	}
	if s.prompt == nil {
		return nil, errorx.New(errorx.Internal, "PromptService 未配置")
	}

	tmpl, err := s.prompt.GetPrompt(ctx, req.PromptName, req.PromptScope, req.PromptScopeID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, errorx.New(errorx.NotFound, "提示词不存在")
	}

	// A/B 分配（可选）
	var abVariant string
	if req.ABTestID > 0 {
		if abTmpl, variant, err := s.prompt.AssignABVariant(ctx, req.ABTestID, req.UserID); err == nil && abTmpl != nil {
			tmpl = abTmpl
			abVariant = variant
		}
	}

	systemPrompt, err := s.prompt.RenderPrompt(ctx, tmpl, req.Variables)
	if err != nil {
		return nil, err
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if abVariant != "" {
		metadata["ab_test_id"] = req.ABTestID
		metadata["ab_variant"] = abVariant
		metadata["prompt_template_id"] = tmpl.ID
	}

	resp, err := s.Chat(ctx, &ChatRequest{
		UserID:      req.UserID,
		System:      systemPrompt,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Metadata:    metadata,
	})
	if err != nil {
		return nil, err
	}

	if abVariant != "" {
		if resp.Metadata == nil {
			resp.Metadata = map[string]interface{}{}
		}
		resp.Metadata["ab_test_id"] = req.ABTestID
		resp.Metadata["ab_variant"] = abVariant
		resp.Metadata["prompt_template_id"] = tmpl.ID
	}
	return resp, nil
}

func (s *chatServiceImpl) StreamChat(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	if req == nil {
		return nil, errorx.New(errorx.InvalidInput, "ChatRequest 不能为空")
	}

	ch := make(chan *ChatChunk, 8)
	super := runtime.NewTaskSupervisor("llm.stream_chat")
	super.Go(ctx, "stream", func(ctx context.Context) {
		defer close(ch)

		resp, err := s.Chat(ctx, req)
		if err != nil {
			return
		}

		segments := chunkContent(resp.Content, 200)
		for _, seg := range segments {
			select {
			case <-ctx.Done():
				return
			case ch <- &ChatChunk{Content: seg}:
			}
		}
	})
	return ch, nil
}

func (s *chatServiceImpl) BatchChat(ctx context.Context, reqs []*ChatRequest) ([]*ChatResponse, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	result := make([]*ChatResponse, len(reqs))
	errCh := make(chan error, len(reqs))

	concurrency := 4
	if len(reqs) < concurrency {
		concurrency = len(reqs)
	}

	var wg sync.WaitGroup
	idxCh := make(chan int, len(reqs))
	for i := range reqs {
		idxCh <- i
	}
	close(idxCh)

	super := runtime.NewTaskSupervisor("llm.batch_chat")
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		workerID := w
		super.Go(ctx, fmt.Sprintf("worker_%d", workerID), func(ctx context.Context) {
			defer wg.Done()
			for idx := range idxCh {
				r := reqs[idx]
				// 每个请求单独超时，避免批处理阻塞
				cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				resp, err := s.Chat(cctx, r)
				cancel()
				if err != nil {
					errCh <- err
					return
				}
				result[idx] = resp
			}
		})
	}

	wg.Wait()
	close(errCh)
	super.Stop()
	if err := <-errCh; err != nil {
		return nil, err
	}
	return result, nil
}

func convertMessages(msgs []Message) []client.ChatMessage {
	result := make([]client.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "" {
			role = "user"
		}
		result = append(result, client.ChatMessage{
			Role:    role,
			Content: m.Content,
		})
	}
	return result
}

func joinMessages(msgs []Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(m.Role)
		sb.WriteString(":")
		sb.WriteString(m.Content)
	}
	return sb.String()
}

// estimateUsage 基于字符数的粗略 token 估算，避免缺少 provider usage 时完全空白。
func estimateUsage(system string, msgs []Message, content string) *TokenUsage {
	countRunes := func(s string) int {
		return len([]rune(s))
	}
	reqTokens := countRunes(system)
	for _, m := range msgs {
		reqTokens += countRunes(m.Content)
	}
	// 粗略估算 4 字符约等于 1 token，避免除零
	reqTokens = (reqTokens + 3) / 4
	respTokens := (countRunes(content) + 3) / 4
	return &TokenUsage{
		RequestTokens:  reqTokens,
		ResponseTokens: respTokens,
		TotalTokens:    reqTokens + respTokens,
	}
}

// chunkContent 将文本按指定大小分段，用于模拟流式输出。
func chunkContent(text string, size int) []string {
	if size <= 0 || len(text) == 0 {
		return []string{text}
	}
	runes := []rune(text)
	var chunks []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
