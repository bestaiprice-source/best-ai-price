package compiler

import (
	"fmt"
	"log/slog"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ExecutionContext struct {
	SystemPrompt   string    `json:"system_prompt"`
	Summary        string    `json:"summary"`
	PinnedFacts    []string  `json:"pinned_facts"`
	RecentMessages []Message `json:"recent_messages"`
	TerminalState  string    `json:"terminal_state"`
	ToolState      string    `json:"tool_state"`
}

type Compiler struct{}

func NewCompiler() *Compiler {
	return &Compiler{}
}

func (c *Compiler) Compile(ctx ExecutionContext) (string, error) {
	var prompt string
	if ctx.SystemPrompt != "" {
		prompt += fmt.Sprintf("System Prompt:\n%s\n\n", ctx.SystemPrompt)
	}
	if ctx.Summary != "" {
		prompt += fmt.Sprintf("Summary:\n%s\n\n", ctx.Summary)
	}
	if len(ctx.PinnedFacts) > 0 {
		prompt += "Pinned Facts:\n"
		for _, fact := range ctx.PinnedFacts {
			prompt += fmt.Sprintf("- %s\n", fact)
		}
		prompt += "\n"
	}
	if ctx.TerminalState != "" {
		prompt += fmt.Sprintf("Terminal State:\n%s\n\n", ctx.TerminalState)
	}
	if ctx.ToolState != "" {
		prompt += fmt.Sprintf("Tool State:\n%s\n\n", ctx.ToolState)
	}

	slog.Debug("[Compiler] Compiled execution context",
		"compiled_length", len(prompt),
		"has_system_prompt", ctx.SystemPrompt != "",
		"has_summary", ctx.Summary != "",
		"pinned_facts_count", len(ctx.PinnedFacts),
		"recent_messages_count", len(ctx.RecentMessages),
	)

	return prompt, nil
}
