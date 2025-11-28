package service

// simpleCostCalculator 根据 provider/model 提供粗略单价估算成本（USD）
type simpleCostCalculator struct {
	// 单价映射，key: provider:model 或 provider
	// 单价为每 1 token 的 USD 估算，区分 input/output，未命中则使用 provider 级或 0
	unit map[string]pricePair
	// 默认单价兜底（可由外部配置扩展）
	defaultInputPer1k  float64
	defaultOutputPer1k float64
}

type pricePair struct {
	Input  float64
	Output float64
}

func NewCostCalculator() CostCalculator {
	calc := &simpleCostCalculator{
		unit:               map[string]pricePair{},
		defaultInputPer1k:  0,
		defaultOutputPer1k: 0,
	}
	// 价格示例，实际应从配置或外部来源下发
	calc.unit["openai:gpt-4"] = pricePair{Input: 0.00003, Output: 0.00006}
	calc.unit["openai:gpt-3.5"] = pricePair{Input: 0.000002, Output: 0.000004}
	calc.unit["anthropic:claude"] = pricePair{Input: 0.00003, Output: 0.00006}
	calc.unit["gemini:gemini-1.5"] = pricePair{Input: 0.000004, Output: 0.000004}
	calc.unit["mock"] = pricePair{Input: 0, Output: 0}
	return calc
}

func (c *simpleCostCalculator) EstimateCost(provider string, model string, requestTokens int, responseTokens int, inputPer1k float64, outputPer1k float64) float64 {
	if requestTokens < 0 {
		requestTokens = 0
	}
	if responseTokens < 0 {
		responseTokens = 0
	}
	if requestTokens+responseTokens == 0 {
		return 0
	}
	// 先使用端点配置的单价
	in := inputPer1k
	out := outputPer1k
	// 若端点未提供，使用预置映射
	key := provider + ":" + model
	if unit, ok := c.unit[key]; ok {
		if in == 0 {
			in = unit.Input * 1000 // unit is per token, convert to per 1k
		}
		if out == 0 {
			out = unit.Output * 1000
		}
	}
	if unit, ok := c.unit[provider]; ok {
		if in == 0 {
			in = unit.Input * 1000
		}
		if out == 0 {
			out = unit.Output * 1000
		}
	}
	if in == 0 {
		in = c.defaultInputPer1k
	}
	if out == 0 {
		out = c.defaultOutputPer1k
	}
	return in*float64(requestTokens)/1000 + out*float64(responseTokens)/1000
}
