// Package gemini provides a client for the Google Gemini API
package gemini

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

const (
	DefaultModel          = "gemini-3-flash-preview"
	DefaultMaxURLs        = 20
	DefaultMaxContentSize = 34 * 1024 * 1024 // 34MB
)

// Client implements the GeminiClient interface
type Client struct {
	client         *genai.Client
	model          string
	maxURLs        int
	maxContentSize int64
	logger         *common.Logger
}

// ClientOption configures the client
type ClientOption func(*Client)

// WithModel sets the model to use
func WithModel(model string) ClientOption {
	return func(c *Client) {
		c.model = model
	}
}

// WithMaxURLs sets the maximum URLs for URL context
func WithMaxURLs(maxURLs int) ClientOption {
	return func(c *Client) {
		c.maxURLs = maxURLs
	}
}

// WithLogger sets the logger
func WithLogger(logger *common.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new Gemini client
func NewClient(ctx context.Context, apiKey string, opts ...ClientOption) (*Client, error) {
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	c := &Client{
		client:         genaiClient,
		model:          DefaultModel,
		maxURLs:        DefaultMaxURLs,
		maxContentSize: DefaultMaxContentSize,
		logger:         common.NewSilentLogger(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Close closes the client
func (c *Client) Close() error {
	// The genai client doesn't have a Close method
	return nil
}

// GenerateContent generates AI content from a prompt
func (c *Client) GenerateContent(ctx context.Context, prompt string) (string, error) {
	c.logger.Debug().Str("model", c.model).Msg("Generating content")

	contents := genai.Text(prompt)
	result, err := c.client.Models.GenerateContent(ctx, c.model, contents, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	return extractTextFromResponse(result)
}

// GenerateWithURLContext generates content using Gemini's URL context tool.
// If urls are provided, they are prepended to the prompt as reference URLs.
func (c *Client) GenerateWithURLContext(ctx context.Context, prompt string, urls ...string) (string, error) {
	c.logger.Debug().Str("model", c.model).Int("urls", len(urls)).Msg("Generating content with URL context")

	if len(urls) > 0 {
		var sb strings.Builder
		sb.WriteString("Reference URLs:\n")
		for _, u := range urls {
			sb.WriteString("- ")
			sb.WriteString(u)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		sb.WriteString(prompt)
		prompt = sb.String()
	}

	contents := genai.Text(prompt)
	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{{URLContext: &genai.URLContext{}}},
	}

	result, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("failed to generate content with URL context: %w", err)
	}

	return extractTextFromResponse(result)
}

// extractTextFromResponse extracts text from a generate content response
func extractTextFromResponse(result *genai.GenerateContentResponse) (string, error) {
	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	text := ""
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}

	return text, nil
}

// AnalyzeStock generates AI analysis for a stock
func (c *Client) AnalyzeStock(ctx context.Context, ticker string, data *models.StockData) (string, error) {
	prompt := buildStockAnalysisPrompt(ticker, data)
	return c.GenerateContent(ctx, prompt)
}

// buildStockAnalysisPrompt creates a prompt for stock analysis
func buildStockAnalysisPrompt(ticker string, data *models.StockData) string {
	prompt := fmt.Sprintf(`Analyze the following stock data for %s (%s) and provide:
1. A brief summary of the current price action
2. Key technical signals and their implications
3. Risk factors to consider
4. A compliance status classification

`, ticker, data.Name)

	if data.Price != nil {
		prompt += fmt.Sprintf(`
Price Data:
- Current Price: $%.2f
- Change: $%.2f (%.2f%%)
- Volume: %d (Avg: %d)
- 52-Week High: $%.2f
- 52-Week Low: $%.2f
`,
			data.Price.Current,
			data.Price.Change,
			data.Price.ChangePct,
			data.Price.Volume,
			data.Price.AvgVolume,
			data.Price.High52Week,
			data.Price.Low52Week,
		)
	}

	if data.Signals != nil {
		prompt += fmt.Sprintf(`
Technical Signals:
- Trend: %s
- RSI: %.2f (%s)
- SMA20: $%.2f (Distance: %.2f%%)
- SMA50: $%.2f (Distance: %.2f%%)
- SMA200: $%.2f (Distance: %.2f%%)
- Volume Signal: %s
`,
			data.Signals.Trend,
			data.Signals.Technical.RSI,
			data.Signals.Technical.RSISignal,
			data.Signals.Price.SMA20,
			data.Signals.Price.DistanceToSMA20,
			data.Signals.Price.SMA50,
			data.Signals.Price.DistanceToSMA50,
			data.Signals.Price.SMA200,
			data.Signals.Price.DistanceToSMA200,
			data.Signals.Technical.VolumeSignal,
		)

		if len(data.Signals.RiskFlags) > 0 {
			prompt += fmt.Sprintf("\nRisk Flags: %v\n", data.Signals.RiskFlags)
		}
	}

	if data.Fundamentals != nil {
		prompt += fmt.Sprintf(`
Fundamentals:
- Market Cap: $%.2fM
- P/E Ratio: %.2f
- P/B Ratio: %.2f
- Dividend Yield: %.2f%%
- Beta: %.2f
- Sector: %s
`,
			data.Fundamentals.MarketCap/1000000,
			data.Fundamentals.PE,
			data.Fundamentals.PB,
			data.Fundamentals.DividendYield*100,
			data.Fundamentals.Beta,
			data.Fundamentals.Sector,
		)
	}

	prompt += "\nProvide your analysis in a concise, actionable format."

	return prompt
}

// Ensure Client implements GeminiClient
var _ interfaces.GeminiClient = (*Client)(nil)
