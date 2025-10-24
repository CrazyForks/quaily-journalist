package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"quaily-journalist/internal/model"

	openai "github.com/sashabaranov/go-openai"
)

// Summarizer defines the AI summary interface used by builders and commands.
type Summarizer interface {
	// SummarizeItem creates a concise 1-2 sentence description for an item in the given language.
	SummarizeItem(ctx context.Context, title, content, language string) (string, error)
	// SummarizePost creates a short post-level summary for a set of items in the given language.
	SummarizePost(ctx context.Context, items []model.NewsItem, language string) (string, error)
}

// OpenAIClient implements Summarizer using OpenAI Chat Completions API.
type OpenAIClient struct {
	client *openai.Client
	model  string
}

type Config struct {
	APIKey  string
	Model   string
	BaseURL string // optional
}

func NewOpenAI(cfg Config) *OpenAIClient {
	var c *openai.Client
	if cfg.BaseURL != "" {
		cc := openai.DefaultConfig(cfg.APIKey)
		cc.BaseURL = cfg.BaseURL
		c = openai.NewClientWithConfig(cc)
	} else {
		c = openai.NewClient(cfg.APIKey)
	}
	model := cfg.Model
	if model == "" {
		panic("OpenAI model must be specified")
	}
	return &OpenAIClient{client: c, model: model}
}

func (o *OpenAIClient) SummarizeItem(ctx context.Context, title, content, language string) (string, error) {
	// set timeout to 120s for item-level summary
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	// Trim inputs to keep tokens reasonable
	content = strings.TrimSpace(content)
	if content == "" {
		content = title
	}
	if len([]rune(content)) > 1000 {
		content = string([]rune(content)[:1000])
	}

	sys := fmt.Sprintf("You are a concise newsletter editor. Write in %s. Return 1–3 sentences (30–180 words) summarizing the topic. Plain text, no links.", langOrDefault(language))
	user := fmt.Sprintf("Title: %s\nContent: %s", title, content)
	out, err := o.create(ctx, sys, user)
	if err != nil {
		slog.Error("openai: summarize item error", "err", err)
		return "", err
	}

	return strings.TrimSpace(out), nil
}

func (o *OpenAIClient) SummarizePost(ctx context.Context, items []model.NewsItem, language string) (string, error) {
	// set timeout to 300s for post-level summary
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	if len(items) == 0 {
		return "", nil
	}
	b := &strings.Builder{}
	for i, it := range items {
		if i >= 10 {
			break
		}
		fmt.Fprintf(b, "- %s (%s)\n", it.Title, it.NodeName)
	}
	sys := fmt.Sprintf("You are a succinct newsletter editor. Write in %s. Always produce 5 sentences at most (135–270 words total) that summarize the overall themes. If details are limited, infer from titles. Plain text only. Never return an empty output.", langOrDefault(language))
	user := fmt.Sprintf("Top items (title and node):\n%s\nTask: Write some sentences for summarizing today's highlights. Output the summarization only, plain text, no links.", b.String())
	out, err := o.create(ctx, sys, user)
	if err != nil {
		slog.Error("openai: summarize post error", "err", err)
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (o *OpenAIClient) create(ctx context.Context, system, user string) (string, error) {
	// Default timeout guard, if caller didn't set one
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 300*time.Second)
		defer cancel()
	}
	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.4,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Message.Content, nil
}

func langOrDefault(lang string) string {
	l := strings.TrimSpace(lang)
	if l == "" {
		return "English"
	}
	return l
}
