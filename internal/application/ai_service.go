package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/homiakus/docshub-next/internal/domain"
)

type KnowledgeChunk struct {
	DocumentID int64   `json:"document_id"`
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	Heading    string  `json:"heading"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

type AIService struct {
	docService *DocumentService
}

func NewAIService(docService *DocumentService) *AIService {
	return &AIService{docService: docService}
}

func (s *AIService) SearchKnowledgeContext(ctx context.Context, u *domain.User, query string) ([]KnowledgeChunk, error) {
	articles, err := s.docService.ListArticles(ctx, u, query)
	if err != nil {
		return nil, fmt.Errorf("ai search error: %w", err)
	}

	var chunks []KnowledgeChunk
	for _, a := range articles {
		lines := strings.Split(a.Content, "\n\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			chunks = append(chunks, KnowledgeChunk{
				DocumentID: a.ID,
				Slug:       a.Slug,
				Title:      a.Title,
				Heading:    fmt.Sprintf("Section %d", i+1),
				Content:    line,
				Score:      0.95,
			})
		}
	}
	return chunks, nil
}
