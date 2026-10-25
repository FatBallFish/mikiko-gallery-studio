package seedreamimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

const Adapter = nativeimage.AdapterSeedream
const DefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Profile    *nativeimage.CapabilityProfile
}
type Client struct{ config Config }

func New(c Config) *Client {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 600 * time.Second}
	}
	return &Client{c}
}
func (c *Client) Generate(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	if r.Operation == "" {
		r.Operation = nativeimage.OperationGenerate
	}
	if r.Operation != nativeimage.OperationGenerate {
		return provider.ImageResponse{}, fmt.Errorf("Generate requires generate operation")
	}
	return c.run(ctx, r)
}
func (c *Client) Edit(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	if r.Operation == "" {
		r.Operation = nativeimage.OperationEdit
	}
	if r.Operation != nativeimage.OperationEdit && r.Operation != nativeimage.OperationLayerDecomposition {
		return provider.ImageResponse{}, fmt.Errorf("Edit requires edit or layer decomposition")
	}
	return c.run(ctx, r)
}
func (c *Client) run(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	p, ok := nativeimage.ProfileForModel(r.Model)
	if c.config.Profile != nil {
		if err := nativeimage.ValidateProfile(*c.config.Profile); err != nil {
			return provider.ImageResponse{}, err
		}
		if ok && c.config.Profile.Model != r.Model {
			return provider.ImageResponse{}, fmt.Errorf("profile model conflicts with request model")
		}
		p = *c.config.Profile
		ok = true
	}
	if !ok || p.Adapter != Adapter {
		return provider.ImageResponse{}, fmt.Errorf("Seedream endpoint ID requires an explicit model profile")
	}
	if r.Mask != nil || r.Quality != "" || r.OutputCompression != 0 || r.Moderation != "" || r.User != "" || r.SendResponseFormat {
		return provider.ImageResponse{}, fmt.Errorf("unsupported Seedream request field")
	}
	if r.ResponseFormat != "" && r.ResponseFormat != provider.ResponseFormatURL && r.ResponseFormat != provider.ResponseFormatB64JSON {
		return provider.ImageResponse{}, fmt.Errorf("unsupported response format")
	}
	if err := nativeimage.ValidateRequest(p, nativeimage.ValidationRequest{Operation: r.Operation, Prompt: r.Prompt, Size: r.Size, OutputFormat: r.OutputFormat, Background: r.Background, Count: r.OutputImageCount, ReferenceCount: len(r.ReferenceImages), Options: r.NativeOptions}); err != nil {
		return provider.ImageResponse{}, err
	}
	prompt := r.Prompt
	if r.NativeOptions != nil {
		for _, a := range r.NativeOptions.Annotations {
			prompt += fmt.Sprintf(" 图%d %s%s", a.ReferenceIndex, a.Label, a.PromptTag())
		}
	}
	payload := map[string]any{"model": r.Model}
	if prompt != "" {
		payload["prompt"] = prompt
	}
	if r.Size != "" {
		payload["size"] = r.Size
	}
	if r.OutputFormat != "" {
		payload["output_format"] = r.OutputFormat
	}
	if r.Background != "" {
		payload["background"] = r.Background
	}
	if r.ResponseFormat != "" {
		payload["response_format"] = r.ResponseFormat
	}
	if r.Operation == nativeimage.OperationLayerDecomposition {
		payload["layer_decomposition"] = true
	}
	images := []string{}
	for _, im := range r.ReferenceImages {
		allowedFormat := false
		for _, format := range p.InputFormats {
			if im.MIMEType == "image/"+format {
				allowedFormat = true
			}
		}
		if !allowedFormat {
			return provider.ImageResponse{}, fmt.Errorf("reference format excluded by model profile")
		}
		if len(im.Data) == 0 || !validMIME(im.MIMEType, r.Operation) {
			return provider.ImageResponse{}, fmt.Errorf("unsupported or empty Seedream reference image")
		}
		if err := nativeimage.ValidateReference(ctx, Adapter, r.Operation, im.MIMEType, im.Data); err != nil {
			return provider.ImageResponse{}, err
		}
		if r.Background == "transparent" && im.MIMEType == "image/jpeg" {
			return provider.ImageResponse{}, fmt.Errorf("transparent editing does not accept JPEG")
		}
		if r.Background == "transparent" {
			if err := validateTransparency(ctx, im.Data); err != nil {
				return provider.ImageResponse{}, err
			}
		}
		images = append(images, "data:"+im.MIMEType+";base64,"+base64.StdEncoding.EncodeToString(im.Data))
	}
	if len(images) == 1 {
		payload["image"] = images[0]
	} else if len(images) > 1 {
		payload["image"] = images
	}
	if o := r.NativeOptions; o != nil {
		if o.Watermark != nil {
			payload["watermark"] = *o.Watermark
		}
		if o.Seedream != nil && o.Seedream.OptimizePromptMode != "" {
			payload["optimize_prompt_options"] = map[string]string{"mode": o.Seedream.OptimizePromptMode}
		}
	}
	body, _ := json.Marshal(payload)
	endpoint, err := generationEndpoint(c.config.BaseURL)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return provider.ImageResponse{}, fmt.Errorf("build Seedream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image request canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, fmt.Errorf("image transport: %w", &transportError{upstream: provider.NewTransportError(provider.ProviderType(Adapter), err), cause: err})
	}
	defer resp.Body.Close()
	raw, err := nativeimage.ReadResponse(ctx, resp.Body, 64<<20)
	if err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image response canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), err)
	}
	var wire struct {
		Created int64           `json:"created"`
		Usage   json.RawMessage `json:"usage"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Data []struct {
			URL         string                   `json:"url"`
			B64JSON     string                   `json:"b64_json"`
			Size        string                   `json:"size"`
			Format      string                   `json:"output_format"`
			ZIndex      *int                     `json:"z_index"`
			Name        string                   `json:"name"`
			Description string                   `json:"description"`
			BoundingBox *nativeimage.BoundingBox `json:"bounding_box"`
			Error       *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image response canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), err)
	}
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || wire.Error != nil {
		e := &provider.UpstreamError{Provider: provider.ProviderType(Adapter), HTTPStatus: resp.StatusCode, RequestID: requestID}
		if wire.Error != nil {
			e.Code = wire.Error.Code
			e.Message = wire.Error.Message
		}
		provider.ClassifyUpstreamError(e)
		return provider.ImageResponse{}, e
	}
	invalid := func(s string) (provider.ImageResponse, error) {
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), fmt.Errorf("%s", s))
	}
	if len(wire.Data) == 0 {
		return invalid("empty image response")
	}
	if r.Operation != nativeimage.OperationLayerDecomposition && len(wire.Data) != 1 {
		return invalid("ordinary Seedream requests return one image")
	}
	if len(wire.Data) > 17 {
		return invalid("too many decomposition artifacts")
	}
	out := provider.ImageResponse{Created: wire.Created, Operation: r.Operation, Usage: nativeimage.MeteringUsage(wire.Usage), ProviderRequestID: requestID, ProviderMetadata: raw}
	partial := false
	seen := map[int]bool{}
	for _, im := range wire.Data {
		if im.Error != nil {
			partial = true
			continue
		}
		if (im.URL == "") == (im.B64JSON == "") {
			return invalid("artifact requires one URL or base64 payload")
		}
		w, h, err := nativeimage.ParseDimensions(im.Size)
		if err != nil {
			return invalid("invalid artifact dimensions")
		}
		if im.Format != "png" && im.Format != "jpeg" {
			return invalid("invalid artifact format")
		}
		result := provider.ImageResult{Name: im.Name, URL: im.URL, B64JSON: im.B64JSON, Format: im.Format, MimeType: "image/" + im.Format, Width: w, Height: h}
		if r.Operation == nativeimage.OperationLayerDecomposition {
			if im.ZIndex == nil || *im.ZIndex < 0 || *im.ZIndex > p.MaxLayers || seen[*im.ZIndex] {
				return invalid("invalid layer z_index")
			}
			z := *im.ZIndex
			seen[z] = true
			if z == 0 {
				if im.BoundingBox != nil {
					return invalid("base image must not have bounding box")
				}
			} else {
				if im.Format != "png" || im.BoundingBox == nil {
					return invalid("layer requires PNG and bounding box")
				}
				b := im.BoundingBox
				if b.Absolute[0] < 0 || b.Absolute[1] < 0 || b.Absolute[2] <= b.Absolute[0] || b.Absolute[3] <= b.Absolute[1] {
					return invalid("invalid absolute bounding box")
				}
				for _, n := range b.Normalized {
					if n < 0 || n > 1000 {
						return invalid("invalid normalized bounding box")
					}
				}
				if b.Normalized[0] >= b.Normalized[2] || b.Normalized[1] >= b.Normalized[3] {
					return invalid("invalid normalized bbox ordering")
				}
			}
			result.Layer = &nativeimage.LayerMetadata{ZIndex: z, Name: im.Name, Description: im.Description, BoundingBox: im.BoundingBox, IsBase: z == 0}
		}
		out.Data = append(out.Data, result)
	}
	if r.Operation == nativeimage.OperationLayerDecomposition {
		if !seen[0] && !partial {
			return invalid("missing base image")
		}
		var bw, bh int
		for _, im := range out.Data {
			if im.Layer.IsBase {
				bw, bh = im.Width, im.Height
			}
		}
		for _, im := range out.Data {
			if b := im.Layer.BoundingBox; b != nil && bw > 0 && (b.Absolute[2] > bw || b.Absolute[3] > bh) {
				return invalid("layer bbox outside base")
			}
		}
	}
	if r.Operation == nativeimage.OperationLayerDecomposition {
		sort.SliceStable(out.Data, func(i, j int) bool { return out.Data[i].Layer.ZIndex < out.Data[j].Layer.ZIndex })
	}
	if partial {
		return out, &provider.UpstreamError{Provider: provider.ProviderType(Adapter), HTTPStatus: resp.StatusCode, Code: "partial_artifact_error", Message: "provider returned artifact errors; inspect private checkpoint diagnostics", RequestID: requestID, Action: provider.UpstreamErrorActionPass, Family: provider.UpstreamErrorFamilyUnknown}
	}
	return out, nil
}

// generationEndpoint accepts an origin, an API base, or a full endpoint.
// Origins use Ark's /api/v3 API. Nonempty proxy prefixes are preserved and
// receive /images/generations unless they already end with that resource.
// Reject ambiguous paths rather than cleaning or stripping proxy segments.
func generationEndpoint(base string) (string, error) {
	invalid := func() (string, error) {
		return "", fmt.Errorf("invalid Seedream BaseURL: use an HTTP(S) origin, API base, or images/generations endpoint without credentials, query, fragment, or ambiguous path segments")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(base, "#") || u.Opaque != "" {
		return invalid()
	}
	// Encoded separators/dot segments can be interpreted differently by proxies.
	if u.EscapedPath() != u.Path || strings.ContainsAny(u.Path, "\\\\ \t\r\n") {
		return invalid()
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.Contains(path, "//") {
		return invalid()
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return invalid()
		}
	}
	switch {
	case path == "":
		path = "/api/v3/images/generations"
	case strings.HasSuffix(path, "/images/generations"):
		// A full endpoint is already configured; keep every proxy prefix.
	default:
		path += "/images/generations"
	}
	u.Path = path
	return u.String(), nil
}

func validMIME(s string, op nativeimage.Operation) bool {
	if s == "image/png" || s == "image/jpeg" {
		return true
	}
	if op == nativeimage.OperationLayerDecomposition {
		return false
	}
	switch s {
	case "image/webp", "image/bmp", "image/tiff", "image/gif", "image/heic", "image/heif":
		return true
	}
	return false
}

// Preserve both provider classification and the original transport cause.
type transportError struct {
	upstream *provider.UpstreamError
	cause    error
}

func (e *transportError) Error() string   { return e.upstream.Error() }
func (e *transportError) Unwrap() []error { return []error{e.upstream, e.cause} }
