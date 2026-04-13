package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// GLMClient provides a client for Zhipu GLM API (OpenAI-compatible).
type GLMClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *zerolog.Logger
}

// NewGLMClient creates a new GLM client.
func NewGLMClient(apiKey, baseURL, model string, logger *zerolog.Logger) *GLMClient {
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if model == "" {
		model = "glm-4-flash"
	}
	return &GLMClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// ChatMessage represents a chat message.
type ChatMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"`
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a choice in the response.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage represents token usage.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletion sends a chat completion request to GLM.
func (c *GLMClient) ChatCompletion(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("GLM API key not configured")
	}

	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GLM API error: status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// SummarizeDreamPatterns uses GLM to summarize dream patterns.
func (c *GLMClient) SummarizeDreamPatterns(ctx context.Context, patterns []string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("GLM API key not configured")
	}

	systemPrompt := `你是一个记忆分析专家。请分析以下记忆模式，并提供简洁的总结。
重点关注：
1. 重复出现的主题
2. 趋势变化
3. 异常或孤立点
4. 潜在冲突

请用中文回答，保持简洁实用。`

	userContent := "发现的模式：\n" + joinStrings(patterns, "\n- ")

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	return c.ChatCompletion(ctx, messages, 0.3, 500)
}

// EnhanceReviewActionItems uses GLM to enhance review action items.
func (c *GLMClient) EnhanceReviewActionItems(ctx context.Context, findings string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("GLM API key not configured")
	}

	systemPrompt := `你是一个记忆管理顾问。基于以下审查发现，生成具体的行动建议。
建议应该：
1. 具体可执行
2. 按优先级排序
3. 简洁明确

请用中文回答，每条建议一行。`

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: findings},
	}

	return c.ChatCompletion(ctx, messages, 0.4, 300)
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
