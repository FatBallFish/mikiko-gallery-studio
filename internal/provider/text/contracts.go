package text

import (
	"context"
	"fmt"
)

const (
	APIStyleChatCompletions = "chat_completions"
	APIStyleResponses       = "responses"
)

type OptimizeRequest struct {
	Model           string
	SystemPrompt    string
	Prompt          string
	MaxOutputTokens int
}

type OptimizeResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
	RequestID    string
}

type Optimizer interface {
	Optimize(ctx context.Context, req OptimizeRequest) (OptimizeResponse, error)
}

type Error struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return "text provider request failed"
	}
	if e.Code != "" {
		return fmt.Sprintf("text provider request failed: %s", e.Code)
	}
	return "text provider request failed"
}
