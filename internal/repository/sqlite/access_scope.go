package sqlite

import (
	"context"
	"strings"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func (r *DomainRepository) ListByOrganizationScoped(ctx context.Context, organizationID int64, includeArchived bool, scope repository.WorkspaceAccessScope) ([]domain.Domain, error) {
	workspaceIDs := normalizedWorkspaceIDs(scope.WorkspaceIDs)
	if organizationID <= 0 || len(workspaceIDs) == 0 {
		return []domain.Domain{}, nil
	}

	query := domainSelect + ` WHERE organization_id = ? AND security_workspace_id IN (` + placeholders(len(workspaceIDs)) + `)`
	args := make([]any, 0, len(workspaceIDs)+1)
	args = append(args, organizationID)
	for _, workspaceID := range workspaceIDs {
		args = append(args, workspaceID)
	}
	if !includeArchived {
		query += ` AND status <> 'archived'`
	}
	query += ` ORDER BY sort_order, name, id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Domain, 0)
	for rows.Next() {
		item, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) ListByDomainScoped(ctx context.Context, domainID int64, includeArchived bool, scope repository.WorkspaceAccessScope) ([]domain.Project, error) {
	workspaceIDs := normalizedWorkspaceIDs(scope.WorkspaceIDs)
	if domainID <= 0 || len(workspaceIDs) == 0 {
		return []domain.Project{}, nil
	}

	query := projectSelect + ` WHERE s.domain_id = ? AND s.security_workspace_id IN (` + placeholders(len(workspaceIDs)) + `)`
	args := make([]any, 0, len(workspaceIDs)+1)
	args = append(args, domainID)
	for _, workspaceID := range workspaceIDs {
		args = append(args, workspaceID)
	}
	if !includeArchived {
		query += ` AND s.status <> 'archived'`
	}
	query += ` ORDER BY s.sort_order, s.name, s.id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Project, 0)
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func normalizedWorkspaceIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

var (
	_ repository.ScopedDomainReader  = (*DomainRepository)(nil)
	_ repository.ScopedProjectReader = (*ProjectRepository)(nil)
)
