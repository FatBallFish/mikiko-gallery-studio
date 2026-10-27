package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const previewSourceLimit = 16 << 20
const previewImageLimit = 18 << 10

func withImagePreview(ctx context.Context, cfg Config, p openplatform.Principal, operation string, arguments json.RawMessage, value any, result *mcp.CallToolResult) *mcp.CallToolResult {
	var in struct {
		Purpose string `json:"purpose"`
	}
	if operation != "get_image_result_access" || json.Unmarshal(arguments, &in) != nil || strings.ToLower(strings.TrimSpace(in.Purpose)) != "preview" || result.IsError {
		return result
	}
	unavailable := func() *mcp.CallToolResult {
		note := &mcp.TextContent{Text: "Inline thumbnail unavailable. The access result is preserved; use its preview URL with a compatible client or request purpose=download to save and open the original. Do not regenerate the image."}
		result.Content = append(result.Content, note)
		if !fitsResult(result) {
			// Keep structured metadata and continuation; omit only duplicate JSON text.
			result.Content = []mcp.Content{note}
		}
		return result
	}
	if cfg.Preview == nil {
		return unavailable()
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return unavailable()
	}
	var access struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &access) != nil || access.URL == "" {
		return unavailable()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stream, err := cfg.Preview(ctx, p, access.URL)
	if err != nil {
		return unavailable()
	}
	if stream.Reader == nil {
		return unavailable()
	}
	var closeOnce sync.Once
	closeReader := func() { closeOnce.Do(func() { _ = stream.Reader.Close() }) }
	defer closeReader()
	stopClose := context.AfterFunc(ctx, closeReader)
	defer stopClose()
	if stream.SizeBytes > previewSourceLimit {
		return unavailable()
	}
	data, err := thumbnailJPEG(ctx, stream.Reader)
	if err != nil {
		return unavailable()
	}
	result.Content = append(result.Content, &mcp.ImageContent{Data: data, MIMEType: "image/jpeg"})
	if !fitsResult(result) {
		result.Content = result.Content[:len(result.Content)-1]
		return unavailable()
	}
	return result
}

func thumbnailJPEG(ctx context.Context, reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, previewSourceLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > previewSourceLimit {
		return nil, errors.New("preview source too large")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 32_000_000/cfg.Height {
		return nil, errors.New("preview dimensions exceed pixel limit")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	if bounds.Dx() != cfg.Width || bounds.Dy() != cfg.Height || bounds.Dx() <= 0 || bounds.Dy() <= 0 || bounds.Dx() > 32_000_000/bounds.Dy() {
		return nil, errors.New("decoded preview dimensions mismatch")
	}
	for _, edge := range []int{256, 192, 128, 96} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		w, h := cfg.Width, cfg.Height
		if w > edge || h > edge {
			if w >= h {
				h = max(1, h*edge/w)
				w = edge
			} else {
				w = max(1, w*edge/h)
				h = edge
			}
		}
		target := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(target, target.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		xdraw.ApproxBiLinear.Scale(target, target.Bounds(), src, src.Bounds(), draw.Over, nil)
		for _, quality := range []int{65, 45, 30} {
			var output bytes.Buffer
			if err := jpeg.Encode(&output, target, &jpeg.Options{Quality: quality}); err != nil {
				return nil, err
			}
			if output.Len() <= previewImageLimit {
				return output.Bytes(), ctx.Err()
			}
		}
	}
	return nil, errors.New("thumbnail exceeds response budget")
}
