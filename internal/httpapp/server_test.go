package httpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
)

func newTestApp(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()

	dir := t.TempDir()
	cfg := config.Config{
		DBPath:        filepath.Join(dir, "docshub.db"),
		UploadDir:     filepath.Join(dir, "uploads"),
		SiteName:      "Docs Hub Test",
		AdminUser:     "admin",
		AdminPassword: "admin123",
		SessionSecret: "test-secret",
	}
	database, err := db.Open(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return httptest.NewServer(app.Routes()), client
}

func loginTestUser(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	res, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {"admin"},
		"password": {"admin123"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", res.StatusCode, http.StatusSeeOther)
	}
}

func saveArticle(t *testing.T, client *http.Client, baseURL string, form url.Values) string {
	t.Helper()
	res, err := client.PostForm(baseURL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("save status = %d, want %d; body: %s", res.StatusCode, http.StatusSeeOther, body)
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		t.Fatal("save did not return Location header")
	}
	return loc
}

type uploadedMedia struct {
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	Markdown string `json:"markdown"`
}

func uploadTestMedia(t *testing.T, client *http.Client, baseURL, filename, contentType string) uploadedMedia {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("media payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/uploads", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status = %d, want %d; body: %s", res.StatusCode, http.StatusOK, b)
	}
	var media uploadedMedia
	if err := json.NewDecoder(res.Body).Decode(&media); err != nil {
		t.Fatal(err)
	}
	return media
}

func TestAnonymousUsersOnlySeeLogin(t *testing.T) {
	ts, client := newTestApp(t)
	defer ts.Close()

	for _, path := range []string{"/", "/new", "/admin"} {
		res, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("%s status = %d, want %d", path, res.StatusCode, http.StatusSeeOther)
		}
		if got := res.Header.Get("Location"); got != "/login" {
			t.Fatalf("%s redirect = %q, want /login", path, got)
		}
	}

	res, err := client.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	html := string(body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	for _, forbidden := range []string{"sidepanel", "topbar", "mobile-navigation", "Поиск по статьям"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("anonymous login page should not render %q: %s", forbidden, html)
		}
	}
	for _, want := range []string{`name="username"`, `name="password"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("login page should contain %q: %s", want, html)
		}
	}
}

func TestSavedUnicodeArticleOpensFromRedirectLocation(t *testing.T) {
	ts, client := newTestApp(t)
	defer ts.Close()
	loginTestUser(t, client, ts.URL)

	loc := saveArticle(t, client, ts.URL, url.Values{
		"slug":       {""},
		"title":      {"Русская статья"},
		"visibility": {"authenticated"},
		"content":    {"# Привет"},
	})
	if !strings.Contains(loc, "%") {
		t.Fatalf("Location %q should be path-escaped for unicode slug", loc)
	}

	res, err := client.Get(ts.URL + loc)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("open saved article status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "Русская статья") {
		t.Fatalf("opened page does not contain saved title: %s", body)
	}
}

func TestSaveArticleAllocatesFallbackSlug(t *testing.T) {
	ts, client := newTestApp(t)
	defer ts.Close()
	loginTestUser(t, client, ts.URL)

	form := url.Values{
		"slug":       {""},
		"title":      {""},
		"visibility": {"authenticated"},
		"content":    {"empty title"},
	}
	first := saveArticle(t, client, ts.URL, form)
	second := saveArticle(t, client, ts.URL, form)

	if first != "/a/article" {
		t.Fatalf("first fallback Location = %q, want /a/article", first)
	}
	if second != "/a/article-2" {
		t.Fatalf("second fallback Location = %q, want /a/article-2", second)
	}
}

func TestUploadedMediaRendersInSavedArticle(t *testing.T) {
	ts, client := newTestApp(t)
	defer ts.Close()
	loginTestUser(t, client, ts.URL)

	media := []uploadedMedia{
		uploadTestMedia(t, client, ts.URL, "diagram.png", "image/png"),
		uploadTestMedia(t, client, ts.URL, "briefing.mp3", "audio/mpeg"),
		uploadTestMedia(t, client, ts.URL, "demo.mp4", "video/mp4"),
	}
	var snippets []string
	for _, item := range media {
		if item.URL == "" || item.Markdown == "" {
			t.Fatalf("upload returned incomplete payload: %+v", item)
		}
		snippets = append(snippets, item.Markdown)
	}

	loc := saveArticle(t, client, ts.URL, url.Values{
		"slug":       {"media-page"},
		"title":      {"Media Page"},
		"visibility": {"authenticated"},
		"content":    {strings.Join(snippets, "\n\n")},
	})
	res, err := client.Get(ts.URL + loc)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	html := string(body)
	for _, want := range []string{"<img", "<audio", "<video", "/uploads/"} {
		if !strings.Contains(html, want) {
			t.Fatalf("saved article html should contain %q: %s", want, html)
		}
	}

	for _, item := range media {
		res, err := client.Get(ts.URL + item.URL)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("media %s status = %d, want %d", item.URL, res.StatusCode, http.StatusOK)
		}
		res.Body.Close()
	}
}

func TestAdminCanCreateCategoryAndEditorShowsSelector(t *testing.T) {
	ts, client := newTestApp(t)
	defer ts.Close()
	loginTestUser(t, client, ts.URL)

	res, err := client.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("admin status = %d, want %d; body: %s", res.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), "Панель управления") {
		t.Fatalf("admin page should render management UI: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/admin/categories", url.Values{
		"name":       {"Процессы"},
		"slug":       {"processes"},
		"nav_order":  {"10"},
		"is_visible": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create category status = %d, want %d", res.StatusCode, http.StatusSeeOther)
	}

	res, err = client.Get(ts.URL + "/new")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new article status = %d, want %d; body: %s", res.StatusCode, http.StatusOK, html)
	}
	if !strings.Contains(html, `name="category_id"`) || !strings.Contains(html, "Процессы") {
		t.Fatalf("editor should include category selector: %s", html)
	}
}
