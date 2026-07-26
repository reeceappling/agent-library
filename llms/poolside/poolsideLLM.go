package poolside

import (
	"context"
	"errors"
	"fmt"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/httputil"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	tokenEnvVarName        = "POOLSIDE_API_KEY"
	modelEnvVarName        = "POOLSIDE_MODEL_NAME"
	organizationEnvVarName = "POOLSIDE_ORGANIZATION_NAME"
	baseProtocol           = "https://"
	baseDomain             = "divers.poolsi.de"
	defaultBaseURL         = baseProtocol + baseDomain + "/openai/v1"
	defaultChatModel       = ModelMalibu2025_1021 //"Malibu-v2.20251021" // TODO; FIX
	defaultModel           = ModelMalibu2025_1021
)

var (
	ErrMissingToken  = errors.New("missing the Poolside API key, set it in the POOLSIDE_API_KEY environment variable")
	errEmptyResponse = errors.New("empty response")
)

var _ llms.Model = &Model{}

type Model struct {
	client           *Client
	model            string
	CallbacksHandler callbacks.Handler
}

func New(opts ...Option) (*Model, error) {
	opt, c, err := newClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Model{
		client:           c,
		CallbacksHandler: opt.callbackHandler,
		model:            c.Model, // Store the model for reasoning detection
	}, err
}

// Call is a simplified interface for a text-only Model, generating a single
// string response from a single string prompt.
//
// Deprecated: this method is retained for backwards compatibility. Use the
// more general [GenerateContent] instead. You can also use
// the [GenerateFromSinglePrompt] function which provides a similar capability
// to Call and is built on top of the new interface.
func (m *Model) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := m.GenerateContent(ctx, []llms.MessageContent{{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(prompt)},
	}})
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty response")
	}
	return resp.Choices[0].Content, nil
}

type Client struct {
	token        string
	Model        string
	baseURL      string
	organization string
	apiType      openai.APIType
	httpClient   *http.Client

	EmbeddingModel      string
	EmbeddingDimensions int
	// required when APIType is APITypeAzure or APITypeAzureAD
	apiVersion string

	ResponseFormat *ResponseFormat
}

// newClient creates an instance of the internal client.
func newClient(optsIn ...Option) (*options, *Client, error) {
	opts := &options{
		token:        os.Getenv(tokenEnvVarName),
		model:        os.Getenv(modelEnvVarName),
		baseURL:      defaultBaseURL,
		organization: os.Getenv(organizationEnvVarName),
		apiType:      openai.APITypeOpenAI, // TODO: default ok?
		httpClient:   httputil.DefaultClient,
	}

	for _, opt := range optsIn {
		opt(opts)
	}
	//// set of options needed for Azure client
	//if IsAzure(APIType(options.apiType)) && options.apiVersion == "" {
	//	options.apiVersion = DefaultAPIVersion
	//	if options.model == "" {
	//		return options, nil, ErrMissingAzureModel
	//	}
	//	if options.embeddingModel == "" {
	//		return options, nil, ErrMissingAzureEmbeddingModel
	//	}
	//}

	if len(opts.token) == 0 {
		return opts, nil, ErrMissingToken
	}

	var clientOptions []internalOption
	if opts.embeddingDimensions != 0 {
		clientOptions = append(clientOptions, withEmbeddingDimensions(opts.embeddingDimensions))
	}
	cli, err := newClientInternal(opts.token, opts.model, opts.baseURL, opts.organization,
		openai.APIType(opts.apiType), opts.apiVersion, opts.httpClient, opts.embeddingModel,
		opts.responseFormat, clientOptions...,
	)
	return opts, cli, err
}

// New returns a new OpenAI client.
func newClientInternal(token string, model string, baseURL string, organization string,
	apiType openai.APIType, apiVersion string, httpClient *http.Client, embeddingModel string,
	responseFormat *ResponseFormat,
	opts ...internalOption,
) (*Client, error) {
	c := &Client{
		token:          token,
		Model:          model,
		EmbeddingModel: embeddingModel,
		baseURL:        strings.TrimSuffix(baseURL, "/"),
		organization:   organization,
		apiType:        apiType,
		apiVersion:     apiVersion,
		httpClient:     httpClient,
		ResponseFormat: responseFormat,
	}
	if c.baseURL == "" {
		c.baseURL = defaultBaseURL
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// GenerateContent asks the model to generate content from a sequence of
// messages. It's the most general interface for multi-modal LLMs that support
// chat-like interactions.
func (m *Model) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// TODO: DO THIS!
	if m.CallbacksHandler != nil {
		m.CallbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	}
	opts := llms.CallOptions{}

	for _, opt := range options {
		if opt != nil {
			opt(&opts)
		}

	}

	// Determine the effective model for this request (don't mutate m.model to avoid races)
	effectiveModel := opts.Model
	if effectiveModel == "" {
		effectiveModel = m.model
	}

	// Get capabilities for this model
	modelCaps := getModelCapabilities(effectiveModel)

	// For models that don't support system messages, we need to merge them into user messages
	var systemContent string
	if !modelCaps.SupportsSystem {
		for _, mc := range messages {
			if mc.Role == llms.ChatMessageTypeSystem {
				// Extract system message content
				for _, part := range mc.Parts {
					if textPart, ok := part.(llms.TextContent); ok {
						if systemContent != "" {
							systemContent += "\n\n"
						}
						systemContent += textPart.Text
					}
				}
			}
		}
	}

	chatMsgs := make([]*ChatMessage, 0, len(messages))
	for _, mc := range messages {
		// Skip system messages for models that don't support them
		if mc.Role == llms.ChatMessageTypeSystem && !modelCaps.SupportsSystem {
			continue
		}

		msg := &ChatMessage{MultiContent: mc.Parts}
		switch mc.Role {
		case llms.ChatMessageTypeSystem:
			msg.Role = openai.RoleSystem
		case llms.ChatMessageTypeAI:
			msg.Role = openai.RoleAssistant
		case llms.ChatMessageTypeHuman:
			msg.Role = openai.RoleUser
			// For models without system support, prepend system content to first user message
			if systemContent != "" && !modelCaps.SupportsSystem {
				// Prepend system content to the user message
				newParts := []llms.ContentPart{}
				if systemContent != "" {
					newParts = append(newParts, llms.TextContent{Text: systemContent + "\n\n"})
				}
				newParts = append(newParts, mc.Parts...)
				msg.MultiContent = newParts
				systemContent = "" // Clear after using
			}
		case llms.ChatMessageTypeGeneric:
			msg.Role = openai.RoleUser
		case llms.ChatMessageTypeFunction:
			msg.Role = openai.RoleFunction
			// Extract name and content from ToolCallResponse for function messages
			if len(mc.Parts) == 1 {
				if p, ok := mc.Parts[0].(llms.ToolCallResponse); ok {
					msg.Name = p.Name
					msg.Content = p.Content
				}
			}
		case llms.ChatMessageTypeTool:
			msg.Role = openai.RoleTool
			// Here we extract tool calls from the message and populate the ToolCalls field.

			// parse mc.Parts (which should have one entry of type ToolCallResponse) and populate msg.Content and msg.ToolCallID
			if len(mc.Parts) != 1 {
				return nil, fmt.Errorf("expected exactly one part for role %v, got %v", mc.Role, len(mc.Parts))
			}
			switch p := mc.Parts[0].(type) {
			case llms.ToolCallResponse:
				msg.ToolCallID = p.ToolCallID
				msg.Content = p.Content
			default:
				return nil, fmt.Errorf("expected part of type ToolCallResponse for role %v, got %T", mc.Role, mc.Parts[0])
			}

		default:
			return nil, fmt.Errorf("role %v not supported", mc.Role)
		}

		// Here we extract tool calls from the message and populate the ToolCalls field.
		newParts, toolCalls := ExtractToolParts(msg)
		msg.MultiContent = newParts
		msg.ToolCalls = toolCallsFromToolCalls(toolCalls)

		chatMsgs = append(chatMsgs, msg)
	}
	// Check if we should use the legacy max_tokens field
	useLegacyMaxTokens := false
	if opts.Metadata != nil {
		if v, ok := opts.Metadata["openai:use_legacy_max_tokens"].(bool); ok {
			useLegacyMaxTokens = v
		}
	}

	// Extract reasoning effort for thinking models
	// Note: OpenAI o1/o3 models have built-in reasoning and don't support reasoning_effort parameter
	// This is kept for future models that might support it (like GPT-5)
	var reasoningEffort string
	// Commented out for now since current o1 models don't support this parameter
	/*
		if opts.Metadata != nil {
			if config, ok := opts.Metadata["thinking_config"].(*llms.ThinkingConfig); ok {
				// Map thinking mode to reasoning effort
				switch config.Mode {
				case llms.ThinkingModeLow:
					reasoningEffort = "low"
				case llms.ThinkingModeMedium:
					reasoningEffort = "medium"
				case llms.ThinkingModeHigh:
					reasoningEffort = "high"
				}

				// Handle streaming for thinking
				if config.StreamThinking && opts.StreamingReasoningFunc == nil && opts.StreamingFunc != nil {
					// Set up default reasoning streaming if requested but not provided
					// Wrap the single-param streaming func into a reasoning func
					opts.StreamingReasoningFunc = func(ctx context.Context, reasoningChunk []byte, chunk []byte) error {
						// For default behavior, we might want to stream both or just the main content
						// Here we'll just stream the main content chunk
						if len(chunk) > 0 {
							return opts.StreamingFunc(ctx, chunk)
						}
						return nil
					}
				}
			}
		}
	*/

	// Filter out internal metadata that shouldn't be sent to API
	apiMetadata := make(map[string]any)
	if opts.Metadata != nil {
		for k, v := range opts.Metadata {
			// Skip internal metadata keys
			if k == "thinking_config" || strings.HasPrefix(k, "openai:") {
				continue
			}
			apiMetadata[k] = v
		}
	}
	// Only include metadata if there are actual values to send
	if len(apiMetadata) == 0 {
		apiMetadata = nil
	}

	req := &ChatRequest{
		Model:                  ModelId(opts.Model),
		StopWords:              opts.StopWords,
		Messages:               chatMsgs,
		StreamingFunc:          opts.StreamingFunc,
		StreamingReasoningFunc: opts.StreamingReasoningFunc,
		Temperature:            opts.Temperature,
		N:                      opts.N,
		FrequencyPenalty:       opts.FrequencyPenalty,
		PresencePenalty:        opts.PresencePenalty,
		ReasoningEffort:        reasoningEffort,

		// Token handling: check metadata flag for legacy behavior
		// By default use max_completion_tokens (modern field)
		// If WithLegacyMaxTokensField() is used, use max_tokens instead
		MaxCompletionTokens: func() int {
			if useLegacyMaxTokens {
				return 0 // Don't set max_completion_tokens
			}
			return opts.MaxTokens
		}(),
		MaxTokens: func() int {
			if useLegacyMaxTokens {
				return opts.MaxTokens // Set the legacy field
			}
			return 0 // Don't set max_tokens
		}(),

		ToolChoice:           opts.ToolChoice,
		FunctionCallBehavior: FunctionCallBehavior(opts.FunctionCallBehavior),
		Seed:                 opts.Seed,
		Metadata:             apiMetadata,
	}
	if opts.JSONMode {
		req.ResponseFormat = ResponseFormatJSON
	}

	// since req.Functions is deprecated, we need to use the new Tools API.
	for _, fn := range opts.Functions {
		req.Tools = append(req.Tools, Tool{
			Type: "function",
			Function: FunctionDefinition{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  fn.Parameters,
				Strict:      fn.Strict,
			},
		})
	}
	// if opts.Tools is not empty, append them to req.Tools
	for _, tool := range opts.Tools {
		t, err := toolFromTool(tool)
		if err != nil {
			return nil, fmt.Errorf("failed to convert llms tool to openai tool: %w", err)
		}
		req.Tools = append(req.Tools, t)
	}

	// if m.client.ResponseFormat is set, use it for the request
	if m.client.ResponseFormat != nil {
		req.ResponseFormat = m.client.ResponseFormat
	}

	result, err := m.client.CreateChat(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, errEmptyResponse
	}

	choices := make([]*llms.ContentChoice, len(result.Choices))
	for i, c := range result.Choices {
		choices[i] = &llms.ContentChoice{
			Content:          c.Message.Content,
			ReasoningContent: c.Message.ReasoningContent,
			StopReason:       fmt.Sprint(c.FinishReason),
			GenerationInfo: map[string]any{
				"CompletionTokens":  result.Usage.CompletionTokens,
				"PromptTokens":      result.Usage.PromptTokens,
				"TotalTokens":       result.Usage.TotalTokens,
				"ReasoningTokens":   result.Usage.CompletionTokensDetails.ReasoningTokens,
				"PromptAudioTokens": result.Usage.PromptTokensDetails.AudioTokens,
				// Standardized fields for cross-provider compatibility
				"ThinkingContent":                    c.Message.ReasoningContent,                           // Standardized field
				"ThinkingTokens":                     result.Usage.CompletionTokensDetails.ReasoningTokens, // Standardized field
				"PromptCachedTokens":                 result.Usage.PromptTokensDetails.CachedTokens,
				"CompletionAudioTokens":              result.Usage.CompletionTokensDetails.AudioTokens,
				"CompletionReasoningTokens":          result.Usage.CompletionTokensDetails.ReasoningTokens,
				"CompletionAcceptedPredictionTokens": result.Usage.CompletionTokensDetails.AcceptedPredictionTokens,
				"CompletionRejectedPredictionTokens": result.Usage.CompletionTokensDetails.RejectedPredictionTokens,
			},
		}

		// Legacy function call handling
		if c.FinishReason == "function_call" {
			choices[i].FuncCall = &llms.FunctionCall{
				Name:      c.Message.FunctionCall.Name,
				Arguments: c.Message.FunctionCall.Arguments,
			}
		}
		for _, tool := range c.Message.ToolCalls {
			choices[i].ToolCalls = append(choices[i].ToolCalls, llms.ToolCall{
				ID:   tool.ID,
				Type: string(tool.Type),
				FunctionCall: &llms.FunctionCall{
					Name:      tool.Function.Name,
					Arguments: tool.Function.Arguments,
				},
			})
		}
		// populate legacy single-function call field for backwards compatibility
		if len(choices[i].ToolCalls) > 0 {
			choices[i].FuncCall = choices[i].ToolCalls[0].FunctionCall
		}
	}
	response := &llms.ContentResponse{Choices: choices}
	if m.CallbacksHandler != nil {
		m.CallbacksHandler.HandleLLMGenerateContentEnd(ctx, response)
	}
	return response, nil
}

type Usage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
		AudioTokens  int `json:"audio_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens          int `json:"reasoning_tokens"`
		AudioTokens              int `json:"audio_tokens"`
		AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
		RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
	} `json:"completion_tokens_details"`
}

// ModelCapability defines what a model supports
type ModelCapability struct {
	Pattern          string // Regex pattern to match model names
	SupportsSystem   bool   // If true, supports system messages
	SupportsThinking bool   // If true, supports reasoning/thinking
	SupportsCaching  bool   // If true, supports prompt caching
	// Add more capabilities as needed
}

// modelCapabilities defines capabilities for different model patterns
var modelCapabilities = []ModelCapability{
	// Initial model
	{
		Pattern:          `(?i)^` + string(defaultModel) + `$`,
		SupportsSystem:   false, // TODO: validate
		SupportsThinking: false, // TODO: validate
		SupportsCaching:  false, // TODO: validate
	},

	//// OpenAI reasoning models (o1, o3 series) - no system message support
	//{
	//	Pattern:          `(?i)^o[13](-mini|-preview)?$`, // Matches o1, o1-mini, o1-preview, o3, o3-mini // TODO: FIX
	//	SupportsSystem:   false,                          // O1 models don't support system messages
	//	SupportsThinking: true,
	//	SupportsCaching:  false,
	//},
	//// GPT-4 models
	//{
	//	Pattern:          `(?i)^gpt-4`, // Matches gpt-4, gpt-4-turbo, etc. // TODO: FIX
	//	SupportsSystem:   true,
	//	SupportsThinking: false,
	//	SupportsCaching:  false, // OpenAI caching coming soon
	//},
	//// GPT-3.5 models
	//{
	//	Pattern:          `(?i)^gpt-3\.5`, // TODO: FIX
	//	SupportsSystem:   true,
	//	SupportsThinking: false,
	//	SupportsCaching:  false,
	//},
	//// Future models can be added here
}

// getModelCapabilities returns the capabilities for a given model
func getModelCapabilities(model string) ModelCapability {
	for _, cap := range modelCapabilities {
		if matched, _ := regexp.MatchString(cap.Pattern, model); matched {
			return cap
		}
	}
	// Default capabilities - assume standard model
	return ModelCapability{
		SupportsSystem:   true,
		SupportsThinking: false,
		SupportsCaching:  false,
	}
}

// ChatUsage is the usage of a chat completion request.
type ChatUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
		AudioTokens  int `json:"audio_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens          int `json:"reasoning_tokens"`
		AudioTokens              int `json:"audio_tokens"`
		AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
		RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
	} `json:"completion_tokens_details"`
}

// toolFromTool converts an llms.Tool to a Tool.
func toolFromTool(t llms.Tool) (Tool, error) {
	tool := Tool{
		Type: ToolType(t.Type),
	}
	switch t.Type {
	case string(ToolTypeFunction):
		tool.Function = FunctionDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
			Strict:      t.Function.Strict,
		}
	default:
		panic(fmt.Errorf("tool type %v not supported", t.Type))
		return Tool{}, fmt.Errorf("tool type %v not supported", t.Type)
	}
	return tool, nil
}

// toolCallsFromToolCalls converts a slice of llms.ToolCall to a slice of ToolCall.
func toolCallsFromToolCalls(tcs []llms.ToolCall) []ToolCall {
	toolCalls := make([]ToolCall, len(tcs))
	for i, tc := range tcs {
		toolCalls[i] = toolCallFromToolCall(tc)
	}
	return toolCalls
}

// toolCallFromToolCall converts an llms.ToolCall to a ToolCall.
func toolCallFromToolCall(tc llms.ToolCall) ToolCall {
	return ToolCall{
		ID:   tc.ID,
		Type: ToolType(tc.Type),
		Function: ToolFunction{
			Name:      tc.FunctionCall.Name,
			Arguments: tc.FunctionCall.Arguments,
		},
	}
}

// CreateChat creates chat request.
func (c *Client) CreateChat(ctx context.Context, r *ChatRequest) (*ChatCompletionResponse, error) {
	if r.Model == "" {
		if c.Model == "" {
			r.Model = defaultChatModel
		} else {
			r.Model = ModelId(c.Model)
		}
	}
	resp, err := c.createChat(ctx, r)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errEmptyResponse
	}
	return resp, nil
}

// ChatCompletionResponse represents a response structure for chat completion API.
type ChatCompletionResponse struct {
	ID                  string                 `json:"id"`
	Object              string                 `json:"object"`
	Created             int64                  `json:"created"`
	Model               string                 `json:"model"`
	Choices             []ChatCompletionChoice `json:"choices"`
	Usage               Usage                  `json:"usage"`
	SystemFingerprint   string                 `json:"system_fingerprint"`
	PromptFilterResults []PromptFilterResult   `json:"prompt_filter_results,omitempty"`
	ServiceTier         ServiceTier            `json:"service_tier,omitempty"`

	httpHeader
}

type httpHeader http.Header

func (h *httpHeader) SetHeader(header http.Header) {
	*h = httpHeader(header)
}

func (h *httpHeader) Header() http.Header {
	return http.Header(*h)
}

func (h *httpHeader) GetRateLimitHeaders() RateLimitHeaders {
	return newRateLimitHeaders(h.Header())
}

// RateLimitHeaders struct represents Openai rate limits headers.
type RateLimitHeaders struct {
	LimitRequests     int       `json:"x-ratelimit-limit-requests"`
	LimitTokens       int       `json:"x-ratelimit-limit-tokens"`
	RemainingRequests int       `json:"x-ratelimit-remaining-requests"`
	RemainingTokens   int       `json:"x-ratelimit-remaining-tokens"`
	ResetRequests     ResetTime `json:"x-ratelimit-reset-requests"`
	ResetTokens       ResetTime `json:"x-ratelimit-reset-tokens"`
}
type ResetTime string

func (r ResetTime) String() string {
	return string(r)
}

func (r ResetTime) Time() time.Time {
	d, _ := time.ParseDuration(string(r))
	return time.Now().Add(d)
}

func newRateLimitHeaders(h http.Header) RateLimitHeaders {
	limitReq, _ := strconv.Atoi(h.Get("x-ratelimit-limit-requests"))
	limitTokens, _ := strconv.Atoi(h.Get("x-ratelimit-limit-tokens"))
	remainingReq, _ := strconv.Atoi(h.Get("x-ratelimit-remaining-requests"))
	remainingTokens, _ := strconv.Atoi(h.Get("x-ratelimit-remaining-tokens"))
	return RateLimitHeaders{
		LimitRequests:     limitReq,
		LimitTokens:       limitTokens,
		RemainingRequests: remainingReq,
		RemainingTokens:   remainingTokens,
		ResetRequests:     ResetTime(h.Get("x-ratelimit-reset-requests")),
		ResetTokens:       ResetTime(h.Get("x-ratelimit-reset-tokens")),
	}
}

type ServiceTier string

const (
	ServiceTierAuto     ServiceTier = "auto"
	ServiceTierDefault  ServiceTier = "default"
	ServiceTierFlex     ServiceTier = "flex"
	ServiceTierPriority ServiceTier = "priority"
)

type PromptFilterResult struct {
	Index                int                  `json:"index"`
	ContentFilterResults ContentFilterResults `json:"content_filter_results,omitempty"`
}

type ContentFilterResults struct {
	Hate      Hate      `json:"hate,omitempty"`
	SelfHarm  SelfHarm  `json:"self_harm,omitempty"`
	Sexual    Sexual    `json:"sexual,omitempty"`
	Violence  Violence  `json:"violence,omitempty"`
	JailBreak JailBreak `json:"jailbreak,omitempty"`
	Profanity Profanity `json:"profanity,omitempty"`
}

type Hate struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type SelfHarm struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Sexual struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Violence struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}

type JailBreak struct {
	Filtered bool `json:"filtered"`
	Detected bool `json:"detected"`
}

type Profanity struct {
	Filtered bool `json:"filtered"`
	Detected bool `json:"detected"`
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiType == openai.APITypeOpenAI || c.apiType == openai.APITypeAzureAD {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else {
		req.Header.Set("api-key", c.token)
	}
	if c.organization != "" {
		req.Header.Set("OpenAI-Organization", c.organization)
	}
}

func (c *Client) buildURL(suffix string, model ModelId) string {
	//if IsAzure(c.apiType) { // TODO: ok?
	//	return c.buildAzureURL(suffix, model)
	//}

	// open ai implement:
	return fmt.Sprintf("%s%s", c.baseURL, suffix)
}

func (c *Client) buildAzureURL(suffix string, model string) string {
	baseURL := c.baseURL
	baseURL = strings.TrimRight(baseURL, "/")

	// azure example url:
	// /openai/deployments/{model}/chat/completions?api-version={api_version}
	return fmt.Sprintf("%s/openai/deployments/%s%s?api-version=%s",
		baseURL, model, suffix, c.apiVersion,
	)
}
