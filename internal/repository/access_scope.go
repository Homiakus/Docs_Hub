package repository

import (
	"context"

	"github.com/homiakus/docshub-next/internal/domain"
)

// WorkspaceAccessScope is a host-side, request-scoped projection of security
// workspace IDs authorized for one permission. Repositories consume it inside
// SQL predicates; callers must never fetch a broad result and post-filter it.
// An empty scope is intentionally fail-closed and matches zero rows.
type WorkspaceAccessScope struct {
	WorkspaceIDs []string
}

// ScopedDomainReader restricts Domain catalog reads at query time, before
// ordering, pagination, aggregation, or any future ranking is applied.
type ScopedDomainReader interface {
	ListByOrganizationScoped(ctx context.Context, organizationID int64, includeArchived bool, scope WorkspaceAccessScope) ([]domain.Domain, error)
}

// ScopedProjectReader restricts Project catalog reads at query time. The
// physical table is still `spaces` during M1/M2 migration, but callers only see
// the Project product model.
type ScopedProjectReader interface {
	ListByDomainScoped(ctx context.Context, domainID int64, includeArchived bool, scope WorkspaceAccessScope) ([]domain.Project, error)
}
