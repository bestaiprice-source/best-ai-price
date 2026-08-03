package compiler_test

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/compiler"
	"github.com/stretchr/testify/require"
)

func TestCompilerCompile(t *testing.T) {
	c := compiler.NewCompiler()
	ctx := compiler.ExecutionContext{
		SystemPrompt:  "System instruction test",
		Summary:       "Conversation summary test",
		PinnedFacts:   []string{"Fact 1", "Fact 2"},
		TerminalState: "Terminal ok",
		ToolState:     "Tool active",
	}

	result, err := c.Compile(ctx)
	require.NoError(t, err)
	require.True(t, strings.Contains(result, "System Prompt:\nSystem instruction test"))
	require.True(t, strings.Contains(result, "Summary:\nConversation summary test"))
	require.True(t, strings.Contains(result, "Pinned Facts:\n- Fact 1\n- Fact 2"))
	require.True(t, strings.Contains(result, "Terminal State:\nTerminal ok"))
	require.True(t, strings.Contains(result, "Tool State:\nTool active"))
}
