package seedreamimage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/png"
)

func validateTransparency(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read transparent reference: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 36000000 {
		return fmt.Errorf("transparent reference dimensions outside model limits")
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode transparent reference: %w", err)
	}
	for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
			_, _, _, alpha := im.At(x, y).RGBA()
			if alpha < 65535 {
				return nil
			}
		}
	}
	return fmt.Errorf("transparent output requires reference pixels with alpha transparency")
}
