package application

import (
	"context"
	"errors"

	"github.com/homiakus/docshub-next/internal/domain"
)

var (
	ErrLockConflict  = errors.New("optimistic lock conflict: document was modified by another user")
	ErrInvalidStatus = errors.New("workflow error: invalid transition")
)

type WorkflowRepository interface {
	GetArticleLockVersion(ctx context.Context, id int64) (int, string, error)
	UpdateWorkflowStatus(ctx context.Context, id int64, expectedLockVersion int, newStatus domain.WorkflowStatus) (int, error)
	CreateRevision(ctx context.Context, rev domain.DocumentRevision) (int64, error)
	CreateReview(ctx context.Context, revID int64, reviewerID int64, comment string) (int64, error)
}

type WorkflowService struct {
	repo WorkflowRepository
}

func NewWorkflowService(repo WorkflowRepository) *WorkflowService {
	return &WorkflowService{repo: repo}
}

func (s *WorkflowService) SubmitForReview(ctx context.Context, docID int64, lockVersion int) (int, error) {
	return s.repo.UpdateWorkflowStatus(ctx, docID, lockVersion, domain.StatusInReview)
}

func (s *WorkflowService) Approve(ctx context.Context, docID int64, lockVersion int) (int, error) {
	return s.repo.UpdateWorkflowStatus(ctx, docID, lockVersion, domain.StatusApproved)
}

func (s *WorkflowService) Publish(ctx context.Context, docID int64, lockVersion int) (int, error) {
	return s.repo.UpdateWorkflowStatus(ctx, docID, lockVersion, domain.StatusPublished)
}

func (s *WorkflowService) Archive(ctx context.Context, docID int64, lockVersion int) (int, error) {
	return s.repo.UpdateWorkflowStatus(ctx, docID, lockVersion, domain.StatusArchived)
}

func (s *WorkflowService) Reject(ctx context.Context, docID int64, lockVersion int) (int, error) {
	return s.repo.UpdateWorkflowStatus(ctx, docID, lockVersion, domain.StatusDraft)
}
