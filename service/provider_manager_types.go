package service

import "gochen-llm/client"

// ChatExecution 表示一次 LLM 调用的执行结果与计费元数据。
type ChatExecution struct {
	Response         *client.ChatResponse
	Provider         string
	Model            string
	LatencyMs        int64
	InputPricePer1k  float64
	OutputPricePer1k float64
}
