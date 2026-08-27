package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

type CommentRepository struct {
	db *db.DB
}

func NewCommentRepository(d *db.DB) *CommentRepository {
	return &CommentRepository{db: d}
}

func (r *CommentRepository) CreateComment(ctx context.Context, c *domain.Comment) error {
	now := time.Now().UTC().Format(time.RFC3339)
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = domain.CommentStatusOpen
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO comments(
			document_id, author_id, parent_id, base_revision_id,
			start_offset, end_offset, quote_exact, quote_prefix, quote_suffix,
			ast_node_kind, ast_path, heading_id, status, body, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		c.DocumentID, c.AuthorID, c.ParentID, c.BaseRevisionID,
		c.StartOffset, c.EndOffset, c.QuoteExact, c.QuotePrefix, c.QuoteSuffix,
		c.ASTNodeKind, c.ASTPath, c.HeadingID, string(c.Status), c.Body, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

func (r *CommentRepository) GetCommentsByDocument(ctx context.Context, docID int64) ([]domain.Comment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.document_id, c.author_id, coalesce(u.display_name, u.username, 'автор'),
		       c.parent_id, c.base_revision_id, c.start_offset, c.end_offset,
		       c.quote_exact, c.quote_prefix, c.quote_suffix,
		       c.ast_node_kind, c.ast_path, c.heading_id, c.status, c.body, c.created_at, c.updated_at
		FROM comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.document_id = ? AND c.deleted_at IS NULL
		ORDER BY c.id ASC
	`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []domain.Comment
	for rows.Next() {
		var c domain.Comment
		var parentID sql.NullInt64
		var status string
		if err := rows.Scan(
			&c.ID, &c.DocumentID, &c.AuthorID, &c.AuthorName,
			&parentID, &c.BaseRevisionID, &c.StartOffset, &c.EndOffset,
			&c.QuoteExact, &c.QuotePrefix, &c.QuoteSuffix,
			&c.ASTNodeKind, &c.ASTPath, &c.HeadingID, &status, &c.Body, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if parentID.Valid {
			pid := parentID.Int64
			c.ParentID = &pid
		}
		c.Status = domain.CommentStatus(status)
		all = append(all, c)
	}

	// Organize tree (roots and replies)
	commentMap := make(map[int64]*domain.Comment)
	var rootComments []domain.Comment

	for i := range all {
		commentMap[all[i].ID] = &all[i]
	}

	for i := range all {
		if all[i].ParentID == nil {
			rootComments = append(rootComments, all[i])
		} else {
			if parent, ok := commentMap[*all[i].ParentID]; ok {
				parent.Replies = append(parent.Replies, all[i])
			}
		}
	}

	// Fill replies in rootComments from updated map
	for i := range rootComments {
		if ptr, ok := commentMap[rootComments[i].ID]; ok {
			rootComments[i].Replies = ptr.Replies
		}
	}

	return rootComments, nil
}

func (r *CommentRepository) ResolveComment(ctx context.Context, commentID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, `
		UPDATE comments
		SET status = 'resolved', updated_at = ?
		WHERE (id = ? OR parent_id = ?) AND deleted_at IS NULL
	`, now, commentID, commentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *CommentRepository) DeleteComment(ctx context.Context, commentID int64, authorID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var res sql.Result
	var err error
	if authorID > 0 {
		res, err = r.db.ExecContext(ctx, `
			UPDATE comments
			SET deleted_at = ?, updated_at = ?
			WHERE (id = ? OR parent_id = ?) AND author_id = ? AND deleted_at IS NULL
		`, now, now, commentID, commentID, authorID)
	} else {
		// Admin deletion
		res, err = r.db.ExecContext(ctx, `
			UPDATE comments
			SET deleted_at = ?, updated_at = ?
			WHERE (id = ? OR parent_id = ?) AND deleted_at IS NULL
		`, now, now, commentID, commentID)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

var _ repository.CommentRepository = (*CommentRepository)(nil)
