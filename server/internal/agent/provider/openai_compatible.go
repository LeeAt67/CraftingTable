// Package provider 提供 Agent 模型供应商适配器。
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrProviderUnavailable 表示模型服务请求失败。
	ErrProviderUnavailable = errors.New("agent provider unavailable")
	// ErrInvalidProviderResponse 表示模型服务没有返回可读取的流。
	ErrInvalidProviderResponse = errors.New("invalid agent provider response")
)

// Message 是发送给 OpenAI 兼容模型的消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamProvider 打开一次模型流。
type StreamProvider interface {
	Stream(ctx context.Context, messages []Message) (io.ReadCloser, error)
}

// OpenAICompatibleConfig 配置 OpenAI 兼容模型服务。
type OpenAICompatibleConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
}

// Configured 判断连接模型服务所需字段是否完整。
func (c OpenAICompatibleConfig) Configured() bool {
	return strings.TrimSpace(c.BaseURL) != "" &&
		strings.TrimSpace(c.APIKey) != "" &&
		strings.TrimSpace(c.Model) != ""
}

// OpenAICompatibleProvider 使用服务端密钥连接 OpenAI 兼容接口。
type OpenAICompatibleProvider struct {
	config OpenAICompatibleConfig
	client *http.Client
}

// NewOpenAICompatible 创建 OpenAI 兼容流式模型适配器。
func NewOpenAICompatible(config OpenAICompatibleConfig) *OpenAICompatibleProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &OpenAICompatibleProvider{
		config: config,
		client: &http.Client{Transport: transport},
	}
}

// Stream 发起一次流式 chat/completions 请求。
func (p *OpenAICompatibleProvider) Stream(
	ctx context.Context,
	messages []Message,
) (io.ReadCloser, error) {
	requestMessages := messages
	if prompt := strings.TrimSpace(p.config.SystemPrompt); prompt != "" {
		requestMessages = make([]Message, 0, len(messages)+1)
		requestMessages = append(requestMessages, Message{
			Role:    "system",
			Content: prompt,
		})
		requestMessages = append(requestMessages, messages...)
	}

	body, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{
		Model:    p.config.Model,
		Messages: requestMessages,
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: encode request: %v", ErrProviderUnavailable, err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrProviderUnavailable, err)
	}
	request.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", "CraftingTable/0.1")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request: %v", ErrProviderUnavailable, err)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, fmt.Errorf(
			"%w: status %d",
			ErrProviderUnavailable,
			response.StatusCode,
		)
	}
	if response.Body == nil {
		return nil, ErrInvalidProviderResponse
	}
	return response.Body, nil
}
