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
	"gochen/errors"
	runtime "gochen/process/task"
)

// IChatService 抽象对话服务能力接口。
type IChatService interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatWithPrompt(ctx context.Context, req *PromptChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error)
	BatchChat(ctx context.Context, reqs []*ChatRequest) ([]*ChatResponse, error)
	Stop(ctx context.Context) error
}

type chatServiceImpl struct {
	manager      IProviderManager
	prompt       IPromptService
	safety       ISafetyService
	metricsRepo  repo.IMetricsRepo
	costCalc     ICostCalculator
	streamSuper  *runtime.TaskSupervisor
	streamCtx    context.Context
	cancelStream context.CancelFunc
	lifecycleMu  sync.Mutex
	stopped      bool
}

// NewChatService 创建对话服务。
func NewChatService(manager IProviderManager, prompt IPromptService, safety ISafetyService, metrics repo.IMetricsRepo, costCalc ICostCalculator) IChatService {
	streamCtx, cancelStream := context.WithCancel(context.Background())
	return &chatServiceImpl{
		manager:      manager,
		prompt:       prompt,
		safety:       safety,
		metricsRepo:  metrics,
		costCalc:     costCalc,
		streamSuper:  runtime.NewTaskSupervisor("llm.stream_chat"),
		streamCtx:    streamCtx,
		cancelStream: cancelStream,
	}
}

// Chat 发起对话请求。
func (s *chatServiceImpl) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, errors.NewCode(errors.InvalidInput, "ChatRequest 不能为空")
	}
	if s.manager == nil {
		return nil, errors.NewCode(errors.Internal, "LLM ProviderManager 未配置")
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
	execResult, err := s.manager.ChatForUser(ctx, req.UserID, clientReq)
	provider, model := "", ""
	var latencyMs int64
	var inPricePer1k, outPricePer1k float64
	if execResult != nil {
		provider = execResult.Provider
		model = execResult.Model
		latencyMs = execResult.LatencyMs
		inPricePer1k = execResult.InputPricePer1k
		outPricePer1k = execResult.OutputPricePer1k
	}
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
				ErrorType: errorLabel(err),
				CreatedAt: time.Now(),
			})
		}
		return nil, err
	}

	if execResult == nil || execResult.Response == nil {
		return nil, errors.NewCode(errors.Internal, "LLM 调用未返回响应")
	}

	content := execResult.Response.Content
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

// ChatWithPrompt 为带提示词发起对话请求。
func (s *chatServiceImpl) ChatWithPrompt(ctx context.Context, req *PromptChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, errors.NewCode(errors.InvalidInput, "PromptChatRequest 不能为空")
	}
	if s.prompt == nil {
		return nil, errors.NewCode(errors.Internal, "PromptService 未配置")
	}

	tmpl, err := s.prompt.FindPrompt(ctx, req.PromptName, req.PromptScope, req.PromptScopeID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, errors.NewCode(errors.NotFound, "提示词不存在")
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

// StreamChat 处理Stream对话。
func (s *chatServiceImpl) StreamChat(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	if s == nil {
		return nil, errors.NewCode(errors.Internal, "ChatService 未配置")
	}
	if ctx == nil {
		return nil, errors.NewCode(errors.InvalidInput, "ctx 不能为空")
	}
	if req == nil {
		return nil, errors.NewCode(errors.InvalidInput, "ChatRequest 不能为空")
	}

	ch := make(chan *ChatChunk, 8)
	streamCtx, cancel := context.WithCancel(ctx)
	var stopLink func() bool

	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		cancel()
		return nil, errors.NewCode(errors.Internal, "ChatService 已停止，无法发起流式对话")
	}
	if s.streamSuper == nil {
		s.streamSuper = runtime.NewTaskSupervisor("llm.stream_chat")
	}
	if s.streamCtx == nil {
		s.streamCtx, s.cancelStream = context.WithCancel(context.Background())
	}
	stopLink = context.AfterFunc(s.streamCtx, cancel)
	if err := s.streamSuper.Go(streamCtx, "stream", func(ctx context.Context) {
		defer stopLink()
		defer cancel()
		defer close(ch)

		resp, err := s.Chat(ctx, req)
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- &ChatChunk{Error: err.Error()}:
			}
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
	}); err != nil {
		s.lifecycleMu.Unlock()
		stopLink()
		cancel()
		close(ch)
		return nil, err
	}
	s.lifecycleMu.Unlock()
	return ch, nil
}

// BatchChat 处理批量对话。
func (s *chatServiceImpl) BatchChat(ctx context.Context, reqs []*ChatRequest) ([]*ChatResponse, error) {
	if ctx == nil {
		return nil, errors.NewCode(errors.InvalidInput, "ctx 不能为空")
	}
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
		workerID := w
		wg.Add(1)
		if err := super.Go(ctx, fmt.Sprintf("worker_%d", workerID), func(ctx context.Context) {
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
		}); err != nil {
			wg.Done()
			_ = super.StopWithinParentDeadline(ctx, supervisorStopFallback)
			return nil, err
		}
	}

	wg.Wait()
	close(errCh)
	_ = super.StopWithinParentDeadline(ctx, supervisorStopFallback)
	if err := <-errCh; err != nil {
		return nil, err
	}
	return result, nil
}

// Stop 停止 ChatService 托管的流式任务。
func (s *chatServiceImpl) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return errors.NewCode(errors.InvalidInput, "ctx 不能为空")
	}

	s.lifecycleMu.Lock()
	if !s.stopped {
		s.stopped = true
	}
	cancelStream := s.cancelStream
	streamSuper := s.streamSuper
	s.lifecycleMu.Unlock()

	if cancelStream != nil {
		cancelStream()
	}
	if streamSuper == nil {
		return nil
	}
	if err := streamSuper.StopWithinParentDeadline(ctx, supervisorStopFallback); err != nil {
		s.lifecycleMu.Lock()
		if s.streamSuper == streamSuper {
			s.streamSuper = nil
			s.streamCtx = nil
			s.cancelStream = nil
		}
		s.lifecycleMu.Unlock()
		return errors.Wrap(err, errors.Timeout, "停止 ChatService 流式任务超时")
	}
	s.lifecycleMu.Lock()
	if s.streamSuper == streamSuper {
		s.streamSuper = nil
		s.streamCtx = nil
		s.cancelStream = nil
	}
	s.lifecycleMu.Unlock()
	return nil
}

// convertMessages 转换消息集合。
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

// joinMessages 处理join消息集合。
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

const supervisorStopFallback = 5 * time.Second

// errorLabel 处理错误标签。
func errorLabel(err error) string {
	if err == nil {
		return ""
	}
	if code := errors.Code(err); code != "" {
		return string(code)
	}
	return err.Error()
}
