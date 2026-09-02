package httpapp

import (
	"context"
	"database/sql"

	"github.com/homiakus/docshub-next/internal/application"
)

// resolveWorkspaceActor derives the request Organization from server-side ACL
// state. It never trusts a query parameter or header to assign tenant identity.
//
// Compatibility behavior is deliberately conservative:
//   - exactly one explicit organization membership -> use it;
//   - no explicit membership + exactly one Organization in the database ->
//     preserve the legacy single-tenant deployment;
//   - multiple memberships -> fail closed until an authorized organization
//     selection flow is persisted in the session.
func (s *Server) resolveWorkspaceActor(ctx context.Context, userID int64) application.WorkspaceActor {
	if s == nil || s.db == nil || userID <= 0 {
		return application.WorkspaceActor{}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT rb.organization_id
		FROM role_bindings rb
		WHERE (rb.subject_type = 'user' AND rb.subject_id = ?)
		   OR (rb.subject_type = 'group' AND rb.subject_id IN (
			SELECT gm.group_id FROM group_members gm WHERE gm.user_id = ?
		   ))
		ORDER BY rb.organization_id
		LIMIT 2
	`, userID, userID)
	if err != nil {
		return application.WorkspaceActor{}
	}
	defer rows.Close()

	organizationIDs := make([]int64, 0, 2)
	for rows.Next() {
		var organizationID int64
		if err := rows.Scan(&organizationID); err != nil || organizationID <= 0 {
			return application.WorkspaceActor{}
		}
		organizationIDs = append(organizationIDs, organizationID)
	}
	if err := rows.Err(); err != nil {
		return application.WorkspaceActor{}
	}
	if len(organizationIDs) == 1 {
		return application.WorkspaceActor{UserID: userID, OrganizationID: organizationIDs[0]}
	}
	if len(organizationIDs) > 1 {
		return application.WorkspaceActor{}
	}

	// Legacy single-Organization fallback is safe because there is no second
	// tenant to cross. As soon as another Organization exists, explicit
	// membership becomes mandatory.
	var organizationID int64
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*), coalesce(min(id), 0) FROM organizations`).Scan(&count, &organizationID); err != nil {
		if err == sql.ErrNoRows {
			return application.WorkspaceActor{}
		}
		return application.WorkspaceActor{}
	}
	if count != 1 || organizationID <= 0 {
		return application.WorkspaceActor{}
	}
	return application.WorkspaceActor{UserID: userID, OrganizationID: organizationID}
}
