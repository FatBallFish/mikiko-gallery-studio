package imagetask

import (
	"context"
	"strings"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/imagetitle"
)

type TitleGenerator interface {
	Generate(context.Context, string) string
}

func (s *Service) SetTitleGenerator(generator TitleGenerator) { s.titles = generator }

// Titles are checkpointed with the upstream results, so artifact retries reuse them.
func (s *Service) nameImageResults(ctx context.Context, task domainimagetask.Task, results []provider.ImageResult) {
	missing := false
	for _, result := range results {
		if strings.TrimSpace(result.Name) == "" {
			missing = true
		}
	}
	if !missing {
		return
	}
	title := ""
	if s.titles != nil {
		titleCtx, cancel := context.WithTimeout(ctx, imagetitle.Timeout)
		title = imagetitle.Sanitize(s.titles.Generate(titleCtx, task.Prompt))
		cancel()
	}
	for i := range results {
		if strings.TrimSpace(results[i].Name) != "" {
			continue
		}
		results[i].Name = title
		if title == "" {
			results[i].Name = imagetitle.Fallback(s.nowUTC())
		}
	}
}
