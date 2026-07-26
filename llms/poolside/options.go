package poolside

import (
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms/openai"
	"net/http"
)

type Option func(*options)
type internalOption func(*Client) error
type options struct {
	token, model, embeddingModel, baseURL, organization, apiVersion string
	embeddingDimensions                                             int
	apiType                                                         openai.APIType
	httpClient                                                      *http.Client
	responseFormat                                                  *ResponseFormat
	callbackHandler                                                 callbacks.Handler
}

// WithToken passes the OpenAI API token to the client. If not set, the token
// is read from the OPENAI_API_KEY environment variable.
func WithToken(token string) Option {
	return func(opts *options) {
		opts.token = token
	}
}

// WithModel passes the OpenAI model to the client. If not set, the model
// is read from the OPENAI_MODEL environment variable.
// Required when ApiType is Azure.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithEmbeddingModel passes the OpenAI model to the client. Required when ApiType is Azure.
func WithEmbeddingModel(embeddingModel string) Option {
	return func(opts *options) {
		opts.embeddingModel = embeddingModel
	}
}

// WithEmbeddingDimensions passes the OpenAI embeddings dimensions to the client.
// Requires a compatible model, test-embedding-3 or later.
// For more info, please check openai doc
// https://platform.openai.com/docs/api-reference/embeddings/create#embeddings-create-dimensions
func WithEmbeddingDimensions(dimensions int) Option {
	return func(opts *options) {
		opts.embeddingDimensions = dimensions
	}
}

// WithBaseURL passes the OpenAI base url to the client. If not set, the base url
// is read from the OPENAI_BASE_URL environment variable. If still not set in ENV
// VAR OPENAI_BASE_URL, then the default value is https://api.openai.com/v1 is used.
func WithBaseURL(baseURL string) Option {
	return func(opts *options) {
		opts.baseURL = baseURL
	}
}

// WithOrganization passes the OpenAI organization to the client. If not set, the
// organization is read from the OPENAI_ORGANIZATION.
func WithOrganization(organization string) Option {
	return func(opts *options) {
		opts.organization = organization
	}
}

// WithAPIType passes the api type to the client. If not set, the default value
// is APITypeOpenAI.
func WithAPIType(apiType openai.APIType) Option {
	return func(opts *options) {
		opts.apiType = apiType
	}
}

// WithAPIVersion passes the api version to the client. If not set, the default value
// is DefaultAPIVersion.
func WithAPIVersion(apiVersion string) Option {
	return func(opts *options) {
		opts.apiVersion = apiVersion
	}
}

// WithHTTPClient allows setting a custom HTTP client. If not set, the default value
// is http.DefaultClient.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		opts.httpClient = client
	}
}

// WithCallback allows setting a custom Callback Handler.
func WithCallback(callbackHandler callbacks.Handler) Option {
	return func(opts *options) {
		opts.callbackHandler = callbackHandler
	}
}

// WithResponseFormat allows setting a custom response format.
func WithResponseFormat(responseFormat *ResponseFormat) Option {
	return func(opts *options) {
		opts.responseFormat = responseFormat
	}
}

// WithEmbeddingDimensions allows to setup specific dimensions for embedding's vector
func withEmbeddingDimensions(dimensions int) internalOption {
	return func(c *Client) error {
		c.EmbeddingDimensions = dimensions
		return nil
	}
}
