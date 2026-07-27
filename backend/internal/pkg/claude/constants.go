// Package claude provides constants and helpers for Claude API integration.
package claude

// Claude Code 客户端相关常量

// Beta header 常量
const (
	BetaOAuth                    = "oauth-2025-04-20"
	BetaClaudeCode               = "claude-code-20250219"
	BetaInterleavedThinking      = "interleaved-thinking-2025-05-14"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
	BetaTokenCounting            = "token-counting-2024-11-01"
	BetaContext1M                = "context-1m-2025-08-07"
	BetaFastMode                 = "fast-mode-2026-02-01"
)

// DroppedBetas 是转发时需要从 anthropic-beta header 中移除的 beta token 列表。
// 这些 token 是客户端特有的，不应透传给上游 API。
var DroppedBetas = []string{}

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// MessageBetaHeaderNoTools /v1/messages 在无工具时的 beta header
//
// NOTE: Claude Code OAuth credentials are scoped to Claude Code. When we "mimic"
// Claude Code for non-Claude-Code clients, we must include the claude-code beta
// even if the request doesn't use tools, otherwise upstream may reject the
// request as a non-Claude-Code API request.
const MessageBetaHeaderNoTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// MessageBetaHeaderWithTools /v1/messages 在有工具时的 beta header
const MessageBetaHeaderWithTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// CountTokensBetaHeader count_tokens 请求使用的 anthropic-beta header
const CountTokensBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenCounting

// HaikuBetaHeader Haiku 模型使用的 anthropic-beta header（不需要 claude-code beta）
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// APIKeyBetaHeader API-key 账号建议使用的 anthropic-beta header（不包含 oauth）
const APIKeyBetaHeader = BetaClaudeCode + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// APIKeyHaikuBetaHeader Haiku 模型在 API-key 账号下使用的 anthropic-beta header（不包含 oauth / claude-code）
const APIKeyHaikuBetaHeader = BetaInterleavedThinking

// DefaultHeaders 是 Claude Code 客户端默认请求头。
var DefaultHeaders = map[string]string{
	// Keep these in sync with recent Claude CLI traffic to reduce the chance
	// that Claude Code-scoped OAuth credentials are rejected as "non-CLI" usage.
	"User-Agent":                                "claude-cli/2.1.88 (external, cli)",
	"X-Stainless-Lang":                          "js",
	"X-Stainless-Package-Version":               "0.70.0",
	"X-Stainless-OS":                            "Linux",
	"X-Stainless-Arch":                          "arm64",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v24.13.0",
	"X-Stainless-Retry-Count":                   "0",
	"X-Stainless-Timeout":                       "600",
	"X-App":                                     "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
}

// Model 表示一个 Claude 模型
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels Claude Code 客户端支持的默认模型列表
var DefaultModels = []Model{
	{
		ID:          "claude-3-7-sonnet-20250219",
		Type:        "model",
		DisplayName: "Claude 3.7 Sonnet",
		CreatedAt:   "2025-02-19T00:00:00Z",
	},
	{
		ID:          "claude-3-7-sonnet",
		Type:        "model",
		DisplayName: "Claude 3.7 Sonnet (latest)",
		CreatedAt:   "2025-02-19T00:00:00Z",
	},
	{
		ID:          "claude-3-5-sonnet-20241022",
		Type:        "model",
		DisplayName: "Claude 3.5 Sonnet (20241022)",
		CreatedAt:   "2024-10-22T00:00:00Z",
	},
	{
		ID:          "claude-3-5-sonnet-20240620",
		Type:        "model",
		DisplayName: "Claude 3.5 Sonnet (20240620)",
		CreatedAt:   "2024-06-20T00:00:00Z",
	},
	{
		ID:          "claude-3-5-haiku-20241022",
		Type:        "model",
		DisplayName: "Claude 3.5 Haiku",
		CreatedAt:   "2024-10-22T00:00:00Z",
	},
	{
		ID:          "claude-3-opus-20240229",
		Type:        "model",
		DisplayName: "Claude 3 Opus",
		CreatedAt:   "2024-02-29T00:00:00Z",
	},
	{
		ID:          "claude-3-sonnet-20240229",
		Type:        "model",
		DisplayName: "Claude 3 Sonnet",
		CreatedAt:   "2024-02-29T00:00:00Z",
	},
	{
		ID:          "claude-3-haiku-20240307",
		Type:        "model",
		DisplayName: "Claude 3 Haiku",
		CreatedAt:   "2024-03-07T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-5-20251101",
		Type:        "model",
		DisplayName: "Claude Opus 4.5",
		CreatedAt:   "2025-11-01T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-6",
		Type:        "model",
		DisplayName: "Claude Opus 4.6",
		CreatedAt:   "2026-02-06T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-6",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.6",
		CreatedAt:   "2026-02-18T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-5-20250929",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.5",
		CreatedAt:   "2025-09-29T00:00:00Z",
	},
	{
		ID:          "claude-haiku-4-5-20251001",
		Type:        "model",
		DisplayName: "Claude Haiku 4.5",
		CreatedAt:   "2025-10-01T00:00:00Z",
	},
	{
		ID:          "gpt-4o",
		Type:        "model",
		DisplayName: "GPT-4o",
		CreatedAt:   "2024-05-13T00:00:00Z",
	},
	{
		ID:          "gpt-4o-mini",
		Type:        "model",
		DisplayName: "GPT-4o Mini",
		CreatedAt:   "2024-07-18T00:00:00Z",
	},
	{
		ID:          "gpt-4-turbo",
		Type:        "model",
		DisplayName: "GPT-4 Turbo",
		CreatedAt:   "2024-04-09T00:00:00Z",
	},
	{
		ID:          "o1",
		Type:        "model",
		DisplayName: "OpenAI o1",
		CreatedAt:   "2024-12-17T00:00:00Z",
	},
	{
		ID:          "o3-mini",
		Type:        "model",
		DisplayName: "OpenAI o3-mini",
		CreatedAt:   "2025-01-31T00:00:00Z",
	},
	{
		ID:          "deepseek-r1",
		Type:        "model",
		DisplayName: "DeepSeek R1",
		CreatedAt:   "2025-01-20T00:00:00Z",
	},
	{
		ID:          "deepseek-chat",
		Type:        "model",
		DisplayName: "DeepSeek V3 / Chat",
		CreatedAt:   "2024-12-26T00:00:00Z",
	},
	{
		ID:          "gemini-2.5-pro",
		Type:        "model",
		DisplayName: "Gemini 2.5 Pro",
		CreatedAt:   "2025-01-01T00:00:00Z",
	},
	{
		ID:          "gemini-2.5-flash",
		Type:        "model",
		DisplayName: "Gemini 2.5 Flash",
		CreatedAt:   "2025-01-01T00:00:00Z",
	},
	{
		ID:          "gemini-3-flash",
		Type:        "model",
		DisplayName: "Gemini 3 Flash",
		CreatedAt:   "2025-06-01T00:00:00Z",
	},
	{
		ID:          "gemini-3-pro-high",
		Type:        "model",
		DisplayName: "Gemini 3 Pro High",
		CreatedAt:   "2025-06-01T00:00:00Z",
	},
}

// DefaultModelIDs 返回默认模型的 ID 列表
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel 测试时使用的默认模型
const DefaultTestModel = "claude-sonnet-4-5-20250929"

// ModelIDOverrides Claude OAuth 请求需要的模型 ID 映射
var ModelIDOverrides = map[string]string{
	"claude-sonnet-4-5": "claude-sonnet-4-5-20250929",
	"claude-opus-4-5":   "claude-opus-4-5-20251101",
	"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
}

// ModelIDReverseOverrides 用于将上游模型 ID 还原为短名
var ModelIDReverseOverrides = map[string]string{
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	"claude-opus-4-5-20251101":   "claude-opus-4-5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
}

// NormalizeModelID 根据 Claude OAuth 规则映射模型
func NormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDOverrides[id]; ok {
		return mapped
	}
	return id
}

// DenormalizeModelID 将上游模型 ID 转换为短名
func DenormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		return mapped
	}
	return id
}
