package provider

// KnownModels contains pricing and capability info for known models.
// This is used for cost calculation and model selection.
var KnownModels = map[string]Model{
	// Anthropic models
	"claude-sonnet-4-20250514": {
		ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic",
		ContextWindow: 200000, MaxOutput: 16384,
		InputCostPer1M: 3.0, OutputCostPer1M: 15.0,
		SupportsThinking: true, SupportsTools: true,
	},
	"claude-opus-4-20250514": {
		ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Provider: "anthropic",
		ContextWindow: 200000, MaxOutput: 32768,
		InputCostPer1M: 15.0, OutputCostPer1M: 75.0,
		SupportsThinking: true, SupportsTools: true,
	},
	"claude-3-5-haiku-20241022": {
		ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Provider: "anthropic",
		ContextWindow: 200000, MaxOutput: 8192,
		InputCostPer1M: 0.80, OutputCostPer1M: 4.0,
		SupportsThinking: false, SupportsTools: true,
	},

	// OpenAI models
	"gpt-4.1": {
		ID: "gpt-4.1", Name: "GPT-4.1", Provider: "openai",
		ContextWindow: 1047576, MaxOutput: 32768,
		InputCostPer1M: 2.0, OutputCostPer1M: 8.0,
		SupportsThinking: false, SupportsTools: true,
	},
	"gpt-4.1-mini": {
		ID: "gpt-4.1-mini", Name: "GPT-4.1 Mini", Provider: "openai",
		ContextWindow: 1047576, MaxOutput: 32768,
		InputCostPer1M: 0.40, OutputCostPer1M: 1.60,
		SupportsThinking: false, SupportsTools: true,
	},
	"o3": {
		ID: "o3", Name: "o3", Provider: "openai",
		ContextWindow: 200000, MaxOutput: 100000,
		InputCostPer1M: 2.0, OutputCostPer1M: 8.0,
		SupportsThinking: true, SupportsTools: true,
	},
	"o4-mini": {
		ID: "o4-mini", Name: "o4-mini", Provider: "openai",
		ContextWindow: 200000, MaxOutput: 100000,
		InputCostPer1M: 1.10, OutputCostPer1M: 4.40,
		SupportsThinking: true, SupportsTools: true,
	},

	// Google models
	"gemini-2.5-pro": {
		ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "google",
		ContextWindow: 1048576, MaxOutput: 65536,
		InputCostPer1M: 1.25, OutputCostPer1M: 10.0,
		SupportsThinking: true, SupportsTools: true,
	},
	"gemini-2.5-flash": {
		ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google",
		ContextWindow: 1048576, MaxOutput: 65536,
		InputCostPer1M: 0.15, OutputCostPer1M: 0.60,
		SupportsThinking: true, SupportsTools: true,
	},
}

// GetModel looks up a model by ID from the known models table.
// Returns an empty Model if not found.
func GetModel(modelID string) (Model, bool) {
	m, ok := KnownModels[modelID]
	return m, ok
}
