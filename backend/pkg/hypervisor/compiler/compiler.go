package compiler

import (
	"fmt"
	"log/slog"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ProjectSnapshot struct {
	WorkspaceID string    `json:"workspace_id"`
	GitBranch   string    `json:"git_branch"`
	GitCommit   string    `json:"git_commit"`
	DirtyFiles  []string  `json:"dirty_files"`
	OpenFiles   []string  `json:"open_files"`
	LastCommand string    `json:"last_command"`
	SnapshotRef string    `json:"snapshot_ref"`
}

type ExecutionContext struct {
	SystemPrompt   string           `json:"system_prompt"`
	Summary        string           `json:"summary"`
	PinnedFacts    []string         `json:"pinned_facts"`
	RecentMessages []Message        `json:"recent_messages"`
	TerminalState  string           `json:"terminal_state"`
	ToolState      string           `json:"tool_state"`
	ProjectState   *ProjectSnapshot `json:"project_state,omitempty"`
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
	if ctx.ProjectState != nil {
		prompt += fmt.Sprintf("Project State:\nWorkspace: %s\nBranch: %s\nCommit: %s\n", ctx.ProjectState.WorkspaceID, ctx.ProjectState.GitBranch, ctx.ProjectState.GitCommit)
		if len(ctx.ProjectState.DirtyFiles) > 0 {
			prompt += "Dirty Files:\n"
			for _, file := range ctx.ProjectState.DirtyFiles {
				prompt += fmt.Sprintf("- %s\n", file)
			}
		}
		if len(ctx.ProjectState.OpenFiles) > 0 {
			prompt += "Open Files:\n"
			for _, file := range ctx.ProjectState.OpenFiles {
				prompt += fmt.Sprintf("- %s\n", file)
			}
		}
		if ctx.ProjectState.LastCommand != "" {
			prompt += fmt.Sprintf("Last Command:\n%s\n", ctx.ProjectState.LastCommand)
		}
		prompt += "\n"
	}

	if len(ctx.RecentMessages) > 0 {
		prompt += "Recent Messages:\n"
		for _, msg := range ctx.RecentMessages {
			prompt += fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content)
		}
		prompt += "\n"
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
