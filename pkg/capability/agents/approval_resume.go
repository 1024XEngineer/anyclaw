package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
)

type ApprovalResumeState struct {
	Messages         []llm.Message `json:"messages,omitempty"`
	AssistantMessage llm.Message   `json:"assistant_message"`
	ToolMessages     []llm.Message `json:"tool_messages,omitempty"`
	Results          []string      `json:"results,omitempty"`
	PendingTool      ToolCall      `json:"pending_tool"`
}

type ApprovalPauseError struct {
	State ApprovalResumeState
	Cause error
}

func (e *ApprovalPauseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "approval required"
}

func (e *ApprovalPauseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (a *Agent) ResumeAfterApproval(ctx context.Context, resume ApprovalResumeState) (string, error) {
	if a == nil {
		return "", errors.New("agent is not initialized")
	}
	if strings.TrimSpace(resume.PendingTool.Name) == "" {
		return "", errors.New("approval resume missing pending tool")
	}

	a.resetToolActivities()

	messages := cloneLLMMessages(resume.Messages)
	assistantMessage := cloneLLMMessage(resume.AssistantMessage)
	if strings.TrimSpace(assistantMessage.Role) == "" {
		assistantMessage.Role = "assistant"
	}
	messages = append(messages, assistantMessage)
	if len(resume.ToolMessages) > 0 {
		messages = append(messages, cloneLLMMessages(resume.ToolMessages)...)
	}

	results := append([]string(nil), resume.Results...)
	pendingTool := cloneToolCall(resume.PendingTool)

	if result, err := a.executeTool(ctx, pendingTool); err != nil {
		results = append(results, fmt.Sprintf("[%s] Error: %v", pendingTool.Name, err))
		a.recordToolActivity(ToolActivity{ToolName: pendingTool.Name, Args: pendingTool.Args, Error: err.Error()})
		messages = append(messages, llm.Message{
			Role:       "tool",
			ToolCallID: pendingTool.ID,
			Name:       pendingTool.Name,
			Content:    fmt.Sprintf("error: %v", err),
		})
	} else {
		results = append(results, fmt.Sprintf("[%s] %s", pendingTool.Name, result))
		a.recordToolActivity(ToolActivity{ToolName: pendingTool.Name, Args: pendingTool.Args, Result: result})
		messages = append(messages, llm.Message{
			Role:       "tool",
			ToolCallID: pendingTool.ID,
			Name:       pendingTool.Name,
			Content:    result,
		})
	}

	messages = append(messages, llm.Message{Role: "user", Content: a.toolContinuationPrompt(results)})
	toolDefs := buildToolDefinitionsFromInfos(a.selectToolInfos(a.latestUserInput()))

	response, err := a.chatWithTools(ctx, messages, toolDefs)
	if err != nil {
		return "", err
	}

	a.appendHistoryMessage(ctx, "assistant", response)
	userInput := a.latestUserInput()
	a.recordConversation(userInput, response)

	if a.preferenceLearner != nil {
		if prefResponse, learned := a.preferenceLearner.Learn(userInput, response); learned {
			response = prefResponse + "\n\n" + response
		}
	}

	return response, nil
}

func cloneLLMMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		cloned = append(cloned, cloneLLMMessage(message))
	}
	return cloned
}

func cloneLLMMessage(message llm.Message) llm.Message {
	cloned := message
	if len(message.ToolCalls) > 0 {
		cloned.ToolCalls = make([]llm.ToolCall, len(message.ToolCalls))
		copy(cloned.ToolCalls, message.ToolCalls)
	}
	return cloned
}

func cloneToolCall(tc ToolCall) ToolCall {
	cloned := tc
	if tc.Args != nil {
		cloned.Args = make(map[string]any, len(tc.Args))
		for key, value := range tc.Args {
			cloned.Args[key] = value
		}
	}
	return cloned
}
