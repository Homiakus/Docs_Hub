package application

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/files"
	"github.com/homiakus/docshub-next/internal/repository"
)

type AttachmentService struct {
	filesRepo repository.FileRepository
	storage   files.ObjectStorage
}

func NewAttachmentService(repo repository.FileRepository, storage files.ObjectStorage) *AttachmentService {
	return &AttachmentService{filesRepo: repo, storage: storage}
}

func (s *AttachmentService) UploadAttachment(ctx context.Context, u *domain.User, filename, mimeType string, r io.Reader, size int64, sha string, storageKey string) (*domain.FileObject, error) {
	if size <= 0 {
		return nil, errors.New("empty file")
	}
	if err := s.storage.Put(ctx, storageKey, r, size, mimeType); err != nil {
		return nil, fmt.Errorf("storage put error: %w", err)
	}
	return s.filesRepo.SaveFile(ctx, sha, storageKey, filename, mimeType, size, u)
}

func (s *AttachmentService) OpenAttachment(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	return s.storage.Open(ctx, key)
}
