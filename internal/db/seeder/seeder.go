package seeder

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"

	"github.com/homiakus/docshub-next/internal/db"
)

func SeedDemo(ctx context.Context, database *db.DB) error {
	bt := "```"
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Seed Users
	users := []struct {
		username string
		name     string
		email    string
		role     string
	}{
		{"admin", "System Administrator", "admin@docshub.local", "admin"},
		{"editor", "Senior Tech Writer", "editor@docshub.local", "editor"},
		{"reader", "Junior Engineer", "reader@docshub.local", "reader"},
		{"reviewer", "Quality Auditor", "reviewer@docshub.local", "editor"},
	}

	userIds := make(map[string]int64)
	for _, u := range users {
		var existingID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, u.username).Scan(&existingID)
		if err == sql.ErrNoRows {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO users(username, display_name, email, password_hash, role, is_active, created_at, updated_at)
				 VALUES(?, ?, ?, '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', ?, 1, datetime('now'), datetime('now'))`,
				u.username, u.name, u.email, u.role)
			if err != nil {
				return fmt.Errorf("seed user %s: %w", u.username, err)
			}
			id, _ := res.LastInsertId()
			userIds[u.username] = id
		} else if err == nil {
			userIds[u.username] = existingID
		}
	}

	adminID := userIds["admin"]
	editorID := userIds["editor"]
	reviewerID := userIds["reviewer"]

	// 2. Seed Spaces
	spacesData := []struct {
		name string
		slug string
		desc string
	}{
		{"General Workspace", "general", "Company-wide documentation and knowledge base"},
		{"Engineering & Architecture", "engineering", "Technical specs, architectural decision records, and API guidelines"},
		{"Quality Assurance", "quality-assurance", "Testing procedures, quality metrics, audit reports, and compliance docs"},
		{"Human Resources", "human-resources", "Employee handbooks, onboarding materials, and company policies"},
		{"Product Management", "product-management", "Product roadmaps, feature specs, and user research synthesis"},
	}

	spaceIDs := make([]int64, 0, len(spacesData))
	for _, s := range spacesData {
		var sID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM spaces WHERE slug=?`, s.slug).Scan(&sID)
		if err == sql.ErrNoRows {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO spaces(organization_id, name, slug, description, default_visibility, created_at, updated_at)
				 VALUES(1, ?, ?, ?, 'space_members', datetime('now'), datetime('now'))`,
				s.name, s.slug, s.desc)
			if err != nil {
				return fmt.Errorf("seed space %s: %w", s.slug, err)
			}
			sID, _ = res.LastInsertId()
		}
		spaceIDs = append(spaceIDs, sID)
	}

	// 3. Seed Featured Long Document
	longDocSlug := "enterprise-architecture-guidelines"
	var longDocID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE slug=?`, longDocSlug).Scan(&longDocID)
	if err == sql.ErrNoRows {
		longDocContent := `# Руководство по архитектуре Docs_Hub

## 1. Общие принципы

Настоящий документ определяет базовые технические регламенты и шаблоны проектирования базы знаний Docs_Hub.

> [!NOTE]
> Все компоненты интерфейса должны строго следовать гайдлайнам WCAG 2.2 AA и системным CSS-токенам.

### 1.1 Производительность и изоляция
1. Server-Side Rendering на Go HTML templates.
2. Изолированные клиентские модули без тяжелых фреймворков.
3. Оптимистическое блокирование версий при редактировании.

` + bt + `go
type DocumentService struct {
    repo Repository
}

func (s *DocumentService) UpdateDocument(ctx context.Context, doc *Article) error {
    return s.repo.Save(ctx, doc)
}
` + bt + `

## 2. Схемы взаимодействия (Mermaid)

` + bt + `mermaid
graph TD
    A[Пользователь] -->|HTTP GET /search| B[Go Web Handler]
    B -->|FTS Query| C[(SQLite Database)]
    B -->|Render Template| D[HTML Document]
` + bt + `

## 3. Таблица метрик доступности

| Компонент | Требование | Метрика | Статус |
|---|---|---|---|
| Контрастность | 4.5:1 Normal, 3:1 Large | Contrast Ratio | ✅ Pass |
| Клавиатурная навигация | Focus Ring 2px | Tab Navigation | ✅ Pass |
| Поддержка Screen Reader | ARIA Landmark / Labels | Lighthouse 100 | ✅ Pass |
`
		res, err := tx.ExecContext(ctx,
			`INSERT INTO articles(organization_id, space_id, stable_key, slug, title, status, classification, language, content, rendered_html, owner_id, visibility, created_at, updated_at)
			 VALUES(1, ?, 'stable-long-doc', ?, 'Руководство по архитектуре Docs_Hub', 'published', 'internal', 'ru', ?, '', ?, 'authenticated', datetime('now', '-10 days'), datetime('now'))`,
			spaceIDs[1], longDocSlug, longDocContent, editorID)
		if err != nil {
			return fmt.Errorf("seed long doc: %w", err)
		}
		longDocID, _ = res.LastInsertId()
	}

	// 4. Seed Document with 20 Revisions
	multiRevSlug := "policy-versioning-sop"
	var multiRevID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE slug=?`, multiRevSlug).Scan(&multiRevID)
	if err == sql.ErrNoRows {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO articles(organization_id, space_id, stable_key, slug, title, status, classification, language, lock_version, content, rendered_html, owner_id, visibility, created_at, updated_at)
			 VALUES(1, ?, 'stable-multi-rev', ?, 'Стандартная операционная процедура версионирования', 'published', 'internal', 'ru', 20, 'Итоговое содержание процедуры версионирования ред. 20', '', ?, 'authenticated', datetime('now', '-30 days'), datetime('now'))`,
			spaceIDs[2], multiRevSlug, adminID)
		if err == nil {
			multiRevID, _ = res.LastInsertId()
			for i := 1; i <= 20; i++ {
				author := editorID
				if i%2 == 0 {
					author = adminID
				}
				_, _ = tx.ExecContext(ctx,
					`INSERT INTO document_revisions(document_id, revision_no, source_format, content, rendered_html, change_summary, created_by, created_at)
					 VALUES(?, ?, 'markdown', ?, '', ?, ?, datetime('now', ?))`,
					multiRevID, i, fmt.Sprintf("Редакция регламента версионирования №%d.", i), fmt.Sprintf("Обновление пункта %d", i), author, fmt.Sprintf("-%d days", 30-i))
				_, _ = tx.ExecContext(ctx,
					`INSERT INTO article_versions(article_id, version_no, title, content, author_id, created_at)
					 VALUES(?, ?, ?, ?, ?, datetime('now', ?))`,
					multiRevID, i, "Стандартная операционная процедура версионирования", fmt.Sprintf("Текст версии №%d", i), author, fmt.Sprintf("-%d days", 30-i))
			}
		}
	}

	// 5. Seed 100 Documents across spaces
	statuses := []string{"published", "draft", "in_review", "approved", "archived"}
	classifications := []string{"public", "internal", "confidential"}
	categories := []string{"Архитектура", "Процессы", "Инструкции", "Стандарты", "Отчеты"}

	rnd := rand.New(rand.NewSource(1337))

	for i := 1; i <= 98; i++ {
		slug := fmt.Sprintf("doc-seed-%d", i)
		var existingID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE slug=?`, slug).Scan(&existingID)
		if err == sql.ErrNoRows {
			spaceID := spaceIDs[rnd.Intn(len(spaceIDs))]
			status := statuses[rnd.Intn(len(statuses))]
			classification := classifications[rnd.Intn(len(classifications))]
			category := categories[rnd.Intn(len(categories))]
			owner := editorID
			if i%3 == 0 {
				owner = adminID
			} else if i%5 == 0 {
				owner = reviewerID
			}

			content := fmt.Sprintf("# Документ базы знаний №%d\n\nКатегория: %s.\nСтатус: %s.\n\nДанный документ сгенерирован автоматически в рамках демонстрационного тестового набора `seed-demo`.\n\n## Раздел 1. Описание\n\nПример содержимого статьи со ссылкой на [[enterprise-architecture-guidelines|Руководство по архитектуре]].\n", i, category, status)

			if i%4 == 0 {
				// Include table in some docs
				content += "\n### Таблица спецификаций\n\n| Параметр | Значение |\n|---|---|\n| Требование | WCAG 2.2 AA |\n| Время отклика | < 2.5s LCP |\n"
			}
				content += "\n### Диаграмма процесса\n\n" + bt + "mermaid\ngraph LR\n    A[Черновик] --> B[Проверка]\n    B --> C[Публикация]\n" + bt + "\n"

			res, err := tx.ExecContext(ctx,
				`INSERT INTO articles(organization_id, space_id, stable_key, slug, title, status, classification, language, content, rendered_html, owner_id, visibility, created_at, updated_at)
				 VALUES(1, ?, ?, ?, ?, ?, ?, 'ru', ?, '', ?, 'authenticated', datetime('now', ?), datetime('now', ?))`,
				spaceID, fmt.Sprintf("key-seed-%d", i), slug, fmt.Sprintf("Документ базы знаний #%d - %s", i, category), status, classification, content, owner, fmt.Sprintf("-%d days", rnd.Intn(60)+1), fmt.Sprintf("-%d hours", rnd.Intn(24)))
			if err == nil {
				docID, _ := res.LastInsertId()
				// Add initial revision
				_, _ = tx.ExecContext(ctx,
					`INSERT INTO document_revisions(document_id, revision_no, source_format, content, rendered_html, change_summary, created_by, created_at)
					 VALUES(?, 1, 'markdown', ?, '', 'Начальное создание документа', owner, datetime('now', '-1 day'))`,
					docID, content)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit seed tx: %w", err)
	}

	// 6. Rebuild FTS index
	_, _ = database.ExecContext(ctx, `DELETE FROM article_fts`)
	_, _ = database.ExecContext(ctx,
		`INSERT INTO article_fts(rowid, title, slug, content)
		 SELECT id, title, slug, content FROM articles WHERE deleted_at IS NULL`)

	return nil
}

func CleanDemo(ctx context.Context, database *db.DB) error {
	tables := []string{
		"article_fts", "links", "article_tags", "article_files", "document_revisions",
		"document_reviews", "document_permissions", "space_members", "articles", "spaces",
	}
	for _, t := range tables {
		_, _ = database.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", t))
	}
	return nil
}
