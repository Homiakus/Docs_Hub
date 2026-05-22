package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	salt, hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if salt == "" || hash == "" {
		t.Fatal("empty salt/hash")
	}
	if !VerifyPassword("secret", salt, hash) {
		t.Fatal("valid password did not verify")
	}
	if VerifyPassword("wrong", salt, hash) {
		t.Fatal("wrong password verified")
	}
}

func TestLoadStoreDoesNotLogBootstrapPassword(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "super-secret-bootstrap")

	var logs bytes.Buffer
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
	}()

	if _, err := LoadStore(filepath.Join(dir, "storage.json")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "super-secret-bootstrap") {
		t.Fatalf("bootstrap password leaked to logs: %s", logs.String())
	}
}

func TestMarkdownEscapesXSS(t *testing.T) {
	out := string(RenderMarkdown(`# Title
<script>alert(1)</script>
[[Start Page|safe link]] #tag`))
	if strings.Contains(out, "<script>") {
		t.Fatalf("script tag was not escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("escaped script not found: %s", out)
	}
	if !strings.Contains(out, `/a/start-page`) {
		t.Fatalf("wiki link not rendered: %s", out)
	}
	if !strings.Contains(out, `?tag=tag`) {
		t.Fatalf("tag link not rendered: %s", out)
	}

	table := string(RenderMarkdown(`<table><tr><td colspan="2">A<script>alert(1)</script></td></tr></table>`))
	if strings.Contains(table, "<script>") {
		t.Fatalf("script inside html table was not removed: %s", table)
	}
	if !strings.Contains(table, `<td colspan="2">A</td>`) {
		t.Fatalf("safe table attributes were not preserved: %s", table)
	}
}

func TestMarkdownMediaEmbedsAndSizing(t *testing.T) {
	out := string(RenderMarkdown(`![Chart|50%](/files/1/chart.png)
[Clip|640x360](https://cdn.example.com/video.mp4)
https://youtu.be/dQw4w9WgXcQ
[YT|480](https://www.youtube.com/watch?v=abc_DEF-12)`))

	if !strings.Contains(out, `<img class="md-img"`) || !strings.Contains(out, `style="width:50%"`) {
		t.Fatalf("sized image was not rendered: %s", out)
	}
	if !strings.Contains(out, `<video class="md-media"`) || !strings.Contains(out, `style="width:640px;height:360px"`) {
		t.Fatalf("sized video link was not rendered: %s", out)
	}
	if !strings.Contains(out, `https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ`) {
		t.Fatalf("bare youtube link was not embedded: %s", out)
	}
	if !strings.Contains(out, `https://www.youtube-nocookie.com/embed/abc_DEF-12`) || !strings.Contains(out, `style="width:480px"`) {
		t.Fatalf("sized youtube link was not embedded: %s", out)
	}
}

func TestACLUserAndGroup(t *testing.T) {
	user := &User{ID: "u1", Role: RoleUser, Active: true}
	admin := &User{ID: "a1", Role: RoleAdmin, Active: true}
	article := &Article{ID: "p1", AllUsers: false, AllowedUserIDs: []string{"u2"}, AllowedGroupIDs: []string{"g1"}}
	groups := map[string]*Group{"g1": {ID: "g1", MemberIDs: []string{"u1"}}}

	if !CanRead(admin, article, groups) {
		t.Fatal("admin should read everything")
	}
	if !CanRead(user, article, groups) {
		t.Fatal("group member should read article")
	}
	groups["g1"].MemberIDs = []string{"u3"}
	if CanRead(user, article, groups) {
		t.Fatal("non-member should not read restricted article")
	}
}

func TestAdminFlowAndCSRF(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "pass")
	store, err := LoadStore(filepath.Join(dir, "storage.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

	loginForm := url.Values{"username": {"admin"}, "password": {"pass"}}
	resp, err := client.PostForm(server.URL+"/login", loginForm)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie not set")
	}

	req, _ := http.NewRequest("GET", server.URL+"/admin/users", nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(string(body))
	if len(csrf) != 2 {
		t.Fatalf("csrf token not found in admin page: %s", string(body))
	}

	previewForm := url.Values{"content": {"# Preview\n<script>alert(1)</script>"}}
	req, _ = http.NewRequest("POST", server.URL+"/admin/preview", strings.NewReader(previewForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("preview without csrf status = %d, want 403", resp.StatusCode)
	}
	_ = resp.Body.Close()

	previewForm.Set("csrf", csrf[1])
	req, _ = http.NewRequest("POST", server.URL+"/admin/preview", strings.NewReader(previewForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	previewBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", resp.StatusCode, string(previewBody))
	}
	if !strings.Contains(string(previewBody), "<h1>Preview</h1>") || strings.Contains(string(previewBody), "<script>") {
		t.Fatalf("preview did not render safe markdown: %s", string(previewBody))
	}

	draftForm := url.Values{"csrf": {csrf[1]}}
	req, _ = http.NewRequest("POST", server.URL+"/admin/draft", strings.NewReader(draftForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	draftBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("draft status=%d body=%s", resp.StatusCode, string(draftBody))
	}
	var draft map[string]string
	if err := json.Unmarshal(draftBody, &draft); err != nil {
		t.Fatal(err)
	}
	if draft["id"] == "" || draft["edit_url"] == "" {
		t.Fatalf("draft response incomplete: %s", string(draftBody))
	}

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	_ = writer.WriteField("csrf", csrf[1])
	_ = writer.WriteField("article_id", draft["id"])
	part, err := writer.CreateFormFile("file", "clip.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest("POST", server.URL+"/attachments/drop", &uploadBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dropBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("drop upload status=%d body=%s", resp.StatusCode, string(dropBody))
	}
	if !strings.Contains(string(dropBody), `"markdown"`) || !strings.Contains(string(dropBody), "clip.png") {
		t.Fatalf("drop upload did not return attachment metadata: %s", string(dropBody))
	}

	ribbonForm := url.Values{"csrf": {csrf[1]}, "article_ids": {"1"}, "order_1": {"1"}}
	req, _ = http.NewRequest("POST", server.URL+"/admin/ribbon/save", strings.NewReader(ribbonForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("ribbon save status = %d, want 303", resp.StatusCode)
	}
	_ = resp.Body.Close()
	store.mu.RLock()
	if len(store.db.RibbonArticleIDs) != 1 || store.db.RibbonArticleIDs[0] != "1" {
		t.Fatalf("ribbon articles not saved: %#v", store.db.RibbonArticleIDs)
	}
	store.mu.RUnlock()

	badForm := url.Values{"username": {"bob"}, "password": {"bobpass"}}
	req, _ = http.NewRequest("POST", server.URL+"/admin/users/create", strings.NewReader(badForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d, want 403", resp.StatusCode)
	}

	goodForm := url.Values{"csrf": {csrf[1]}, "username": {"bob"}, "password": {"bobpass"}, "role": {"user"}}
	req, _ = http.NewRequest("POST", server.URL+"/admin/users/create", strings.NewReader(goodForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create user status=%d body=%s", resp.StatusCode, string(b))
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	found := false
	for _, u := range store.db.Users {
		if u.Username == "bob" {
			found = true
		}
	}
	if !found {
		t.Fatal("created user not found in store")
	}

	_ = os.Remove(filepath.Join(dir, "storage.json"))
}

func TestPasswordResetRevokesUserSessions(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "pass")
	store, err := LoadStore(filepath.Join(dir, "storage.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)

	store.mu.Lock()
	store.db.Sessions["1"] = &Session{ID: "1", UserID: "1"}
	store.db.Sessions["2"] = &Session{ID: "2", UserID: "1"}
	store.db.Sessions["3"] = &Session{ID: "3", UserID: "other"}
	_ = store.saveLocked()
	store.mu.Unlock()

	form := url.Values{"id": {"1"}, "password": {"new-pass"}}
	req := httptest.NewRequest("POST", "/admin/users/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ctx := &RequestContext{User: &User{ID: "1", Role: RoleAdmin, Active: true}, Session: &Session{CSRFToken: "csrf"}}

	app.adminUserPassword(rr, req, ctx)
	if rr.Code != http.StatusOK {
		t.Fatalf("password reset status=%d body=%s", rr.Code, rr.Body.String())
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.db.Sessions["1"] != nil || store.db.Sessions["2"] != nil {
		t.Fatalf("target user sessions were not revoked: %#v", store.db.Sessions)
	}
	if store.db.Sessions["3"] == nil {
		t.Fatalf("unrelated user session was removed: %#v", store.db.Sessions)
	}
}

func TestArchivedArticleAttachmentNotServed(t *testing.T) {
	dir := t.TempDir()
	store := &Store{path: filepath.Join(dir, "storage.json"), db: Database{
		Groups:      map[string]*Group{},
		Articles:    map[string]*Article{"1": {ID: "1", AllUsers: true, Archived: true}},
		Attachments: map[string]*Attachment{"1": {ID: "1", ArticleID: "1", StoredName: "1.txt", OriginalName: "note.txt", MIME: "text/plain"}},
	}}
	app := NewApp(store)
	req := httptest.NewRequest("GET", "/files/1/note.txt", nil)
	rr := httptest.NewRecorder()
	ctx := &RequestContext{User: &User{ID: "u1", Role: RoleUser, Active: true}}

	app.attachmentServe(rr, req, ctx)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("archived attachment status=%d, want 404", rr.Code)
	}
}

func TestLoadStorePersistsMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "storage.json")
	if err := os.WriteFile(path, []byte(`{"users":{},"groups":{},"articles":{},"sessions":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStore(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var db Database
	if err := json.Unmarshal(b, &db); err != nil {
		t.Fatal(err)
	}
	if db.Version != CurrentDBVersion {
		t.Fatalf("version was not persisted: %d", db.Version)
	}
	if len(db.Migrations) == 0 {
		t.Fatalf("migration records were not persisted: %s", string(b))
	}
	if db.Attachments == nil || db.NextAttachmentID == 0 {
		t.Fatalf("normalized attachment fields were not persisted: %s", string(b))
	}
}

func TestBackupFailsWhenUploadsPathIsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "storage.json")
	if err := os.WriteFile(path, []byte(`{"version":4}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uploads"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{path: path, db: Database{}}
	app := NewApp(store)
	ctx := &RequestContext{User: &User{ID: "1", Role: RoleAdmin, Active: true}}
	req := httptest.NewRequest("POST", "/admin/backups/create", nil)

	if _, err := app.createBackup(req, ctx); err == nil {
		t.Fatal("backup succeeded with invalid uploads path")
	}
}

func TestFullTextSearchRespectsACL(t *testing.T) {
	user := &User{ID: "u1", Role: RoleUser, Active: true}
	articles := map[string]*Article{
		"1": {ID: "1", Title: "Visible Go Notes", Content: "enterprise search target", AllUsers: true, UpdatedAt: timeNowForTest()},
		"2": {ID: "2", Title: "Hidden Go Notes", Content: "enterprise search target", AllUsers: false, AllowedUserIDs: []string{"u2"}, UpdatedAt: timeNowForTest()},
	}
	res := SearchArticles(user, "enterprise", "", articles, map[string]*Group{})
	if len(res) != 1 || res[0].ID != "1" {
		t.Fatalf("search leaked or missed articles: %#v", res)
	}
}

func TestMarkdownImportParsing(t *testing.T) {
	content := "# Imported Title\n\nBody with #go and #go plus #внутренний-тег."
	if got := markdownImportTitle(content, "folder/fallback.md"); got != "Imported Title" {
		t.Fatalf("title=%q", got)
	}
	if got := markdownImportTitle("No heading", "folder/Fallback Name.md"); got != "Fallback Name" {
		t.Fatalf("fallback title=%q", got)
	}
	if got := markdownImportSlug("folder/Fallback Name.md"); got != "fallback-name" {
		t.Fatalf("slug=%q", got)
	}
	tags := markdownImportTags(content)
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "внутренний-тег" {
		t.Fatalf("tags=%#v", tags)
	}
	existing := map[string]bool{"fallback-name": true, "fallback-name-2": true}
	if got := nextAvailableSlug("fallback-name", existing); got != "fallback-name-3" {
		t.Fatalf("conflict slug=%q", got)
	}
}

func TestAdminMarkdownFolderImport(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "pass")
	store, err := LoadStore(filepath.Join(dir, "storage.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	cookie, csrf := loginAdminAndCSRF(t, client, server.URL, "/admin/import")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("csrf", csrf)
	writeMultipartFile(t, writer, "files", "vault/start.md", "# Replacement Start\n\n#tag")
	writeMultipartFile(t, writer, "files", "vault/nested/note.md", "# Nested Note\n\nText #docs #docs")
	writeMultipartFile(t, writer, "files", "vault/readme.txt", "not markdown")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", server.URL+"/admin/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	importBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import status=%d body=%s", resp.StatusCode, string(importBody))
	}
	if !strings.Contains(string(importBody), "Импортировано: 2") || !strings.Contains(string(importBody), "Пропущено: 1") {
		t.Fatalf("import summary missing: %s", string(importBody))
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	var start2, note *Article
	for _, a := range store.db.Articles {
		switch a.Slug {
		case "start-2":
			start2 = a
		case "note":
			note = a
		}
	}
	if start2 == nil || start2.Title != "Replacement Start" {
		t.Fatalf("conflicting import article missing: %#v", start2)
	}
	if note == nil || note.Title != "Nested Note" {
		t.Fatalf("nested import article missing: %#v", note)
	}
	for _, a := range []*Article{start2, note} {
		if a.AllUsers || len(a.AllowedUserIDs) != 0 || len(a.AllowedGroupIDs) != 0 || a.OwnerID != "1" {
			t.Fatalf("imported article is not private/admin-owned: %#v", a)
		}
	}
	if len(note.Tags) != 1 || note.Tags[0] != "docs" {
		t.Fatalf("tags not imported: %#v", note.Tags)
	}
	foundAudit := false
	for _, e := range store.db.Audit {
		if e.Action == "article.import" && e.Target == "articles:2" {
			foundAudit = true
		}
	}
	if !foundAudit {
		t.Fatalf("import audit event missing: %#v", store.db.Audit)
	}
}

func TestAdminMarkdownImportRequiresCSRF(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "pass")
	store, err := LoadStore(filepath.Join(dir, "storage.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	cookie, _ := loginAdminAndCSRF(t, client, server.URL, "/admin/import")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeMultipartFile(t, writer, "files", "vault/note.md", "# Note")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", server.URL+"/admin/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d, want 403", resp.StatusCode)
	}
}

func TestBackupCreatesZipWithStorage(t *testing.T) {
	old := PBKDF2Rounds
	PBKDF2Rounds = 1000
	defer func() { PBKDF2Rounds = old }()

	dir := t.TempDir()
	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "pass")
	store, err := LoadStore(filepath.Join(dir, "storage.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	ctx := &RequestContext{User: &User{ID: "1", Role: RoleAdmin, Active: true}}
	req := httptest.NewRequest("POST", "/admin/backups/create", nil)
	name, err := app.createBackup(req, ctx)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(dir, "backups", name))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	found := false
	for _, f := range zr.File {
		if f.Name == "storage.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("backup does not contain storage.json")
	}
}

func loginAdminAndCSRF(t *testing.T, client *http.Client, serverURL, page string) (*http.Cookie, string) {
	t.Helper()
	loginForm := url.Values{"username": {"admin"}, "password": {"pass"}}
	resp, err := client.PostForm(serverURL+"/login", loginForm)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie not set")
	}
	req, _ := http.NewRequest("GET", serverURL+page, nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(string(body))
	if len(csrf) != 2 {
		t.Fatalf("csrf token not found on %s: %s", page, string(body))
	}
	return cookie, csrf[1]
}

func writeMultipartFile(t *testing.T, writer *multipart.Writer, field, name, content string) {
	t.Helper()
	part, err := writer.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}

func timeNowForTest() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }
