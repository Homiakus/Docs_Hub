package httpapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

type createCommentRequest struct {
	ParentID       *int64 `json:"parent_id,omitempty"`
	BaseRevisionID int64  `json:"base_revision_id"`
	StartOffset    int    `json:"start_offset"`
	EndOffset      int    `json:"end_offset"`
	QuoteExact     string `json:"quote_exact"`
	QuotePrefix    string `json:"quote_prefix"`
	QuoteSuffix    string `json:"quote_suffix"`
	ASTNodeKind    string `json:"ast_node_kind"`
	ASTPath        string `json:"ast_path"`
	HeadingID      string `json:"heading_id"`
	Body           string `json:"body"`
}

func (s *Server) apiGetComments(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	docID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || docID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "Некорректный ID документа")
		return
	}

	u := userFrom(r.Context())
	article, err := s.dbArticleByID(r.Context(), docID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "Документ не найден")
		return
	}

	if !s.canViewArticle(r.Context(), u, *article) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Нет доступа к комментариям этого документа")
		return
	}

	if s.commentRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service_unavailable", "Сервис комментариев недоступен")
		return
	}

	comments, err := s.commentRepo.GetCommentsByDocument(r.Context(), docID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось загрузить комментарии")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"comments": comments})
}

func (s *Server) apiCreateComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	docID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || docID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "Некорректный ID документа")
		return
	}

	u := userFrom(r.Context())
	if u == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
		return
	}

	article, err := s.dbArticleByID(r.Context(), docID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "Документ не найден")
		return
	}

	if !s.canViewArticle(r.Context(), u, *article) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Нет прав для добавления комментариев к этому документу")
		return
	}

	var req createCommentRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Некорректное тело запроса")
		return
	}

	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		writeJSONError(w, http.StatusBadRequest, "empty_body", "Текст комментария не может быть пустым")
		return
	}

	comment := domain.Comment{
		DocumentID:     docID,
		AuthorID:       u.ID,
		AuthorName:     u.DisplayName,
		ParentID:       req.ParentID,
		BaseRevisionID: req.BaseRevisionID,
		StartOffset:    req.StartOffset,
		EndOffset:      req.EndOffset,
		QuoteExact:     req.QuoteExact,
		QuotePrefix:    req.QuotePrefix,
		QuoteSuffix:    req.QuoteSuffix,
		ASTNodeKind:    req.ASTNodeKind,
		ASTPath:        req.ASTPath,
		HeadingID:      req.HeadingID,
		Status:         domain.CommentStatusOpen,
		Body:           req.Body,
	}

	if err := s.commentRepo.CreateComment(r.Context(), &comment); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось сохранить комментарий")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"comment": comment})
}

func (s *Server) apiResolveComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || commentID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "Некорректный ID комментария")
		return
	}

	if s.commentRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service_unavailable", "Сервис комментариев недоступен")
		return
	}

	if err := s.commentRepo.ResolveComment(r.Context(), commentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "not_found", "Комментарий не найден")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось обновить статус")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "resolved"})
}

func (s *Server) apiDeleteComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || commentID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "Некорректный ID комментария")
		return
	}

	u := userFrom(r.Context())
	if u == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
		return
	}

	if s.commentRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service_unavailable", "Сервис комментариев недоступен")
		return
	}

	authorID := u.ID
	if u.Role == "admin" {
		authorID = 0 // Admin can delete any comment
	}

	if err := s.commentRepo.DeleteComment(r.Context(), commentID, authorID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "not_found", "Комментарий не найден")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось удалить комментарий")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "deleted"})
}

func (s *Server) dbArticleByID(ctx context.Context, id int64) (*Article, error) {
	var a Article
	err := s.db.QueryRowContext(ctx, `
		SELECT id, slug, title, status, content, visibility, updated_at, coalesce(owner_id, 0), space_id
		FROM articles WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(&a.ID, &a.Slug, &a.Title, &a.Status, &a.Content, &a.Visibility, &a.UpdatedAt, &a.OwnerID, &a.SpaceID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
