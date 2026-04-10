package embedding

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

type OpenAIEmbedding struct {
	apiKey     string
	baseURL    string
	model      string
	dimensions int
	httpClient *http.Client
	logger     *zerolog.Logger
}

func NewOpenAIEmbedding(apiKey, baseURL, model string, dimensions int, logger *zerolog.Logger) *OpenAIEmbedding {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dimensions <= 0 {
		dimensions = 1536
	}
	return &OpenAIEmbedding{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

func (o *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	body := map[string]interface{}{
		"model": o.model,
		"input": text,
	}
	if o.dimensions > 0 {
		body["dimensions"] = o.dimensions
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/embeddings", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embedding: status=%d body=%s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding in response")
	}
	return result.Data[0].Embedding, nil
}

func (o *OpenAIEmbedding) Dim() int { return o.dimensions }
func (o *OpenAIEmbedding) Name() string { return "openai-" + o.model }
