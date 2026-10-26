package nativeimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// ValidateReference checks actual encoded headers before base64 allocation.
func ValidateReference(ctx context.Context, adapter string, op Operation, mime string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	maxBytes := 10 << 20
	if adapter == AdapterSeedream {
		maxBytes = 30 << 20
	}
	if len(data) == 0 || len(data) > maxBytes {
		return fmt.Errorf("reference image exceeds byte limit or is empty")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if mime == "image/heic" || mime == "image/heif" {
		cfg, err = heifConfig(data)
		format = mime[len("image/"):]
	}
	if err != nil {
		return fmt.Errorf("invalid reference image header: %w", err)
	}
	if "image/"+format != mime {
		return fmt.Errorf("reference MIME does not match encoded format")
	}
	w, h := int64(cfg.Width), int64(cfg.Height)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid reference dimensions")
	}
	if adapter == AdapterQwen {
		if w > 8000 || h > 8000 || w > 10*h || h > 10*w {
			return fmt.Errorf("Qwen reference dimensions outside model limits")
		}
	} else {
		if w > 36000000 || h > 36000000 || w*h > 36000000 || w > 16*h || h > 16*w {
			return fmt.Errorf("Seedream reference dimensions outside model limits")
		}
		if op == OperationLayerDecomposition {
			if (format != "png" && format != "jpeg") || w*h < 262144 {
				return fmt.Errorf("decomposition requires PNG/JPEG with at least 262144 pixels")
			}
		} else if w <= 14 || h <= 14 || w*h < 196 {
			return fmt.Errorf("Seedream reference dimensions too small")
		}
	}
	if format != "heic" && format != "heif" {
		if _, _, err := image.Decode(&contextReader{ctx: ctx, r: bytes.NewReader(data)}); err != nil {
			return fmt.Errorf("invalid encoded reference image: %w", err)
		}
	}
	return ctx.Err()
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// HEIF carries dimensions in ispe properties. Parse box boundaries rather than
// searching arbitrary compressed bytes. No full pixel decoder is required here.
func heifConfig(data []byte) (image.Config, error) {
	if len(data) < 16 || string(data[4:8]) != "ftyp" {
		return image.Config{}, fmt.Errorf("invalid HEIF container")
	}
	brand := string(data[8:12])
	if brand != "heic" && brand != "heix" && brand != "hevc" && brand != "hevx" && brand != "mif1" && brand != "msf1" {
		return image.Config{}, fmt.Errorf("unsupported HEIF brand")
	}
	var result image.Config
	var walk func([]byte, int) error
	walk = func(b []byte, depth int) error {
		if depth > 8 {
			return fmt.Errorf("HEIF nesting too deep")
		}
		for len(b) > 0 {
			if len(b) < 8 {
				return fmt.Errorf("truncated HEIF box")
			}
			n := uint64(binary.BigEndian.Uint32(b))
			header := 8
			if n == 1 {
				if len(b) < 16 {
					return fmt.Errorf("truncated HEIF box")
				}
				n = binary.BigEndian.Uint64(b[8:])
				header = 16
			}
			if n == 0 {
				n = uint64(len(b))
			}
			if n < uint64(header) || n > uint64(len(b)) {
				return fmt.Errorf("invalid HEIF box length")
			}
			kind := string(b[4:8])
			body := b[header:int(n)]
			switch kind {
			case "meta":
				if len(body) < 4 {
					return fmt.Errorf("invalid HEIF meta")
				}
				if err := walk(body[4:], depth+1); err != nil {
					return err
				}
			case "iprp", "ipco":
				if err := walk(body, depth+1); err != nil {
					return err
				}
			case "ispe":
				if len(body) != 12 {
					return fmt.Errorf("invalid HEIF dimensions")
				}
				w, h := int(binary.BigEndian.Uint32(body[4:])), int(binary.BigEndian.Uint32(body[8:]))
				if w > result.Width {
					result.Width = w
				}
				if h > result.Height {
					result.Height = h
				}
			}
			b = b[int(n):]
		}
		return nil
	}
	if err := walk(data, 0); err != nil {
		return image.Config{}, err
	}
	if result.Width == 0 || result.Height == 0 {
		return image.Config{}, fmt.Errorf("HEIF dimensions unavailable")
	}
	return result, nil
}

// ReadResponse detects overflow instead of accepting a truncated JSON prefix.
func ReadResponse(ctx context.Context, body io.Reader, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(body, limit+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("provider response exceeds byte limit")
	}
	return b, nil
}
