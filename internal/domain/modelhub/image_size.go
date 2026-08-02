package modelhub

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	imageSizeMultiple          = 16
	imageMaxEdge               = 3840
	imageMaxAspectRatio        = 3.0
	imageMinPixels             = 655360
	imageMaxPixels             = 8294400
	imageMaxRatioError         = 0.01
	imageDefaultBaseResolution = "1k"
)

var imageTierPixelBudget = map[string]int{
	"1k": 1572864,
	"2k": 4194304,
	"4k": imageMaxPixels,
}

var commonImageSizePresets = map[string]map[string]string{
	"1k": {
		"1:1":  "1024x1024",
		"3:2":  "1536x1024",
		"2:3":  "1024x1536",
		"16:9": "1280x720",
		"9:16": "720x1280",
		"4:3":  "1024x768",
		"3:4":  "768x1024",
		"21:9": "1280x544",
	},
	"2k": {
		"1:1":  "2048x2048",
		"3:2":  "2160x1440",
		"2:3":  "1440x2160",
		"16:9": "2560x1440",
		"9:16": "1440x2560",
		"4:3":  "2048x1536",
		"3:4":  "1536x2048",
		"21:9": "2560x1088",
	},
	"4k": {
		"1:1":  "2880x2880",
		"3:2":  "3456x2304",
		"2:3":  "2304x3456",
		"16:9": "3840x2160",
		"9:16": "2160x3840",
		"4:3":  "3200x2400",
		"3:4":  "2400x3200",
		"21:9": "3840x1600",
	},
}

func CalculateImageSize(baseResolution, aspectRatio string) (string, error) {
	resolution := normalizeSizeBaseResolution(baseResolution)
	ratioW, ratioH, ok := parseRatio(aspectRatio)
	if !ok {
		return "", fmt.Errorf("invalid aspect ratio")
	}
	if maxFloat(float64(ratioW)/float64(ratioH), float64(ratioH)/float64(ratioW)) > imageMaxAspectRatio {
		return "", fmt.Errorf("aspect ratio exceeds model limit")
	}

	rawRatioKey := fmt.Sprintf("%d:%d", ratioW, ratioH)
	if preset := commonImageSizePresets[resolution][rawRatioKey]; preset != "" {
		return preset, nil
	}
	if preset := commonImageSizePresets[resolution][simplifiedRatioKey(ratioW, ratioH)]; preset != "" {
		return preset, nil
	}

	targetRatio := float64(ratioW) / float64(ratioH)
	pixelBudget := imageTierPixelBudget[resolution]
	bestWidth, bestHeight, bestPixels := 0, 0, 0
	for width := imageSizeMultiple; width <= imageMaxEdge; width += imageSizeMultiple {
		idealHeight := float64(width) / targetRatio
		candidates := []int{
			int(math.Floor(idealHeight/float64(imageSizeMultiple))) * imageSizeMultiple,
			int(math.Ceil(idealHeight/float64(imageSizeMultiple))) * imageSizeMultiple,
		}
		for _, height := range candidates {
			if height < imageSizeMultiple || height > imageMaxEdge {
				continue
			}
			pixels := width * height
			if pixels > pixelBudget || pixels < imageMinPixels {
				continue
			}
			if maxFloat(float64(width)/float64(height), float64(height)/float64(width)) > imageMaxAspectRatio {
				continue
			}
			actualRatio := float64(width) / float64(height)
			ratioError := math.Abs(actualRatio-targetRatio) / targetRatio
			if ratioError > imageMaxRatioError {
				continue
			}
			if pixels > bestPixels {
				bestPixels = pixels
				bestWidth = width
				bestHeight = height
			}
		}
	}
	if bestPixels == 0 {
		return "", fmt.Errorf("no legal image size for ratio")
	}
	return fmt.Sprintf("%dx%d", bestWidth, bestHeight), nil
}

func NormalizeCustomImageSize(width, height int) (string, error) {
	if width <= 0 || height <= 0 {
		return "", fmt.Errorf("image width and height must be positive")
	}
	if IsLegalCustomImageSize(width, height) {
		return fmt.Sprintf("%dx%d", width, height), nil
	}

	targetRatio := float64(width) / float64(height)
	if targetRatio > imageMaxAspectRatio {
		targetRatio = imageMaxAspectRatio
	} else if targetRatio < 1/imageMaxAspectRatio {
		targetRatio = 1 / imageMaxAspectRatio
	}
	targetPixels := math.Min(imageMaxPixels, math.Max(imageMinPixels, float64(width)*float64(height)))
	targetWidth := math.Sqrt(targetPixels * targetRatio)
	targetHeight := math.Sqrt(targetPixels / targetRatio)
	if longest := math.Max(targetWidth, targetHeight); longest > imageMaxEdge {
		scale := imageMaxEdge / longest
		targetWidth *= scale
		targetHeight *= scale
		targetPixels = targetWidth * targetHeight
	}

	bestWidth, bestHeight := 0, 0
	bestScore, bestDistance := math.Inf(1), math.Inf(1)
	for candidateWidth := imageSizeMultiple; candidateWidth <= imageMaxEdge; candidateWidth += imageSizeMultiple {
		idealHeight := float64(candidateWidth) / targetRatio
		candidateHeights := []int{
			int(math.Floor(idealHeight/float64(imageSizeMultiple))) * imageSizeMultiple,
			int(math.Ceil(idealHeight/float64(imageSizeMultiple))) * imageSizeMultiple,
		}
		for _, candidateHeight := range candidateHeights {
			if !IsLegalCustomImageSize(candidateWidth, candidateHeight) {
				continue
			}
			actualRatio := float64(candidateWidth) / float64(candidateHeight)
			pixels := float64(candidateWidth * candidateHeight)
			ratioError := math.Abs(math.Log(actualRatio / targetRatio))
			pixelError := math.Abs(math.Log(pixels / targetPixels))
			score := ratioError*4 + pixelError
			distance := math.Abs(float64(candidateWidth)-targetWidth)/targetWidth + math.Abs(float64(candidateHeight)-targetHeight)/targetHeight
			if score < bestScore-1e-12 || (math.Abs(score-bestScore) <= 1e-12 && (distance < bestDistance-1e-12 || (math.Abs(distance-bestDistance) <= 1e-12 && candidateWidth*candidateHeight > bestWidth*bestHeight))) {
				bestWidth, bestHeight = candidateWidth, candidateHeight
				bestScore, bestDistance = score, distance
			}
		}
	}
	if bestWidth == 0 || bestHeight == 0 {
		return "", fmt.Errorf("no legal custom image size")
	}
	return fmt.Sprintf("%dx%d", bestWidth, bestHeight), nil
}

func IsLegalCustomImageSize(width, height int) bool {
	if width <= 0 || height <= 0 || width%imageSizeMultiple != 0 || height%imageSizeMultiple != 0 {
		return false
	}
	if width > imageMaxEdge || height > imageMaxEdge {
		return false
	}
	pixels := width * height
	if pixels < imageMinPixels || pixels > imageMaxPixels {
		return false
	}
	return maxFloat(float64(width)/float64(height), float64(height)/float64(width)) <= imageMaxAspectRatio
}

func ParseImageSize(size string) (int, int, bool) {
	parts := strings.FieldsFunc(strings.TrimSpace(size), func(r rune) bool {
		return r == 'x' || r == 'X' || r == '×'
	})
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func normalizeSizeBaseResolution(baseResolution string) string {
	normalized := strings.ToLower(strings.TrimSpace(baseResolution))
	if _, ok := imageTierPixelBudget[normalized]; ok {
		return normalized
	}
	return imageDefaultBaseResolution
}

func parseRatio(value string) (int, int, bool) {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == ':' || r == 'x' || r == 'X' || r == '×'
	})
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func simplifiedRatioKey(width, height int) string {
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
