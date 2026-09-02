package repository

import (
	"context"

	"github.com/homiakus/docshub-next/internal/domain"
)

type CommentRepository interface {
	CreateComment(ctx context.Context, c *domain.Comment) error
	GetCommentByID(ctx context.Context, commentID int64) (*domain.Comment, error)
	GetCommentsByDocument(ctx context.Context, docID int64) ([]domain.Comment, error)
	ResolveComment(ctx context.Context, commentID int64) error
	DeleteComment(ctx context.Context, commentID int64, authorID int64) error
}
