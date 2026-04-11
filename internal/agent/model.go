package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// Provider identifies the LLM backend.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
	ProviderGrok      Provider = "grok"
)

// grokBaseURL is the xAI OpenAI-compatible endpoint.
const grokBaseURL = "https://api.x.ai/v1"

// DetectProvider infers the provider from the model name when the caller has
// not set an explicit base URL or provider hint.
//
//   - "claude-*"  → Anthropic
//   - "gemini-*"  → Gemini
//   - "grok-*"    → Grok (xAI)
//   - everything else → OpenAI
func DetectProvider(modelName string) Provider {
	switch {
	case strings.HasPrefix(modelName, "claude"):
		return ProviderAnthropic
	case strings.HasPrefix(modelName, "gemini"):
		return ProviderGemini
	case strings.HasPrefix(modelName, "grok"):
		return ProviderGrok
	default:
		return ProviderOpenAI
	}
}

// NewChatModel creates a ToolCallingChatModel for the given model name.
//
// Provider selection:
//   - Pass a non-empty baseURL to force the OpenAI-compatible path (any provider).
//   - Otherwise the provider is inferred from the model name via DetectProvider.
//
// Required environment variables per provider (when apiKey is empty):
//
//	OpenAI/Grok  OPENAI_API_KEY / XAI_API_KEY
//	Anthropic    ANTHROPIC_API_KEY
//	Gemini       GOOGLE_API_KEY (or ADC credentials)
func NewChatModel(ctx context.Context, modelName, apiKey, baseURL string) (einomodel.ToolCallingChatModel, error) {
	// Explicit base URL → always use OpenAI-compatible path.
	if baseURL != "" {
		return newOpenAIModel(ctx, modelName, apiKey, baseURL)
	}

	switch DetectProvider(modelName) {
	case ProviderAnthropic:
		return newAnthropicModel(ctx, modelName, apiKey)
	case ProviderGemini:
		return newGeminiModel(ctx, modelName, apiKey)
	case ProviderGrok:
		return newOpenAIModel(ctx, modelName, apiKey, grokBaseURL)
	default:
		return newOpenAIModel(ctx, modelName, apiKey, "https://api.openai.com/v1")
	}
}

func newOpenAIModel(ctx context.Context, modelName, apiKey, baseURL string) (einomodel.ToolCallingChatModel, error) {
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("openai model: %w", err)
	}
	return m, nil
}

func newAnthropicModel(ctx context.Context, modelName, apiKey string) (einomodel.ToolCallingChatModel, error) {
	m, err := claude.NewChatModel(ctx, &claude.Config{
		Model:  modelName,
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic model: %w", err)
	}
	return m, nil
}

func newGeminiModel(ctx context.Context, modelName, apiKey string) (einomodel.ToolCallingChatModel, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}

	m, err := gemini.NewChatModel(ctx, &gemini.Config{
		Client: client,
		Model:  modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini model: %w", err)
	}
	return m, nil
}
