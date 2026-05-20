package provider

import "context"

type ProviderType string

const (
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeOpenRouter ProviderType = "openrouter"
)

type TaskType string

const (
	TaskTypeTextToImage       TaskType = "text_to_image"
	TaskTypeImageEdit         TaskType = "image_edit"
	TaskTypeReferenceGenerate TaskType = "reference_generate"
)

type ResponseFormat string

const (
	ResponseFormatURL     ResponseFormat = "url"
	ResponseFormatB64JSON ResponseFormat = "b64_json"
)

type ImageInput struct {
	Filename string
	MIMEType string
	Data     []byte
}

type ImageRequest struct {
	Model            string
	TaskType         TaskType
	Prompt           string
	Size             string
	Quality          string
	OutputImageCount int
	ResponseFormat   ResponseFormat
	ReferenceImages  []ImageInput
	Mask             *ImageInput
	User             string
}

type ImageResult struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Format        string `json:"format,omitempty"`
}

type ImageResponse struct {
	Created           int64         `json:"created,omitempty"`
	Data              []ImageResult `json:"data"`
	ProviderRequestID string        `json:"-"`
}

type ImageProvider interface {
	Generate(ctx context.Context, req ImageRequest) (ImageResponse, error)
	Edit(ctx context.Context, req ImageRequest) (ImageResponse, error)
}
