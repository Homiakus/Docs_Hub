package telegram

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/homiakus/docshub-next/internal/auth"
	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
)

type mockTelegramServer struct {
	server       *httptest.Server
	sentMessages []sendPayload
	mu           sync.Mutex
}

func setupTestBot(t *testing.T) (*Bot, *db.DB, *mockTelegramServer, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bot_test.db")
	database, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}

	mock := &mockTelegramServer{}
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			body, _ := io.ReadAll(r.Body)
			var payload sendPayload
			_ = json.Unmarshal(body, &payload)
			mock.mu.Lock()
			mock.sentMessages = append(mock.sentMessages, payload)
			mock.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
			return
		}
		if strings.Contains(r.URL.Path, "editMessageText") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
			return
		}
		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		if strings.Contains(r.URL.Path, "getUpdates") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "setWebhook") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
	}))

	cfg := config.Config{
		TelegramBotToken:      "test-bot-token",
		TelegramChatID:        "12345",
		TelegramWebhookSecret: "secret-test-token",
		Addr:                  ":8080",
		AdminUser:             "admin",
	}

	bot := NewBot(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bot.SetBaseURL(mock.server.URL)

	cleanup := func() {
		bot.Stop()
		mock.server.Close()
		_ = database.Close()
	}

	return bot, database, mock, cleanup
}

func TestBotCommandsFull(t *testing.T) {
	bot, database, mock, cleanup := setupTestBot(t)
	defer cleanup()

	ctx := context.Background()

	// 1. /start & /help
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "/start"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "/help"})

	// 2. /add_user with role
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 12345},
		Text: "/add_user alice editor",
	})

	var role, hash string
	var isActive int
	err := database.QueryRowContext(ctx, `SELECT role, password_hash, is_active FROM users WHERE username='alice'`).Scan(&role, &hash, &isActive)
	if err != nil {
		t.Fatalf("user alice not found in DB: %v", err)
	}
	if role != "editor" {
		t.Fatalf("alice role = %q, want editor", role)
	}
	if isActive != 1 {
		t.Fatalf("alice isActive = %d, want 1", isActive)
	}

	// 3. /my_login & /login
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 12345, Username: "alice"},
		Text: "/login",
	})

	// 4. /magic_link
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 12345},
		Text: "/magic_link alice",
	})

	var tokenCount int
	err = database.QueryRowContext(ctx, `SELECT count(*) FROM auth_tokens WHERE user_id=(SELECT id FROM users WHERE username='alice')`).Scan(&tokenCount)
	if err != nil || tokenCount == 0 {
		t.Fatalf("magic link token was not inserted into auth_tokens table: %v, count: %d", err, tokenCount)
	}

	// 5. Natural reply keyboard actions
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "🚀 Войти в 1 клик"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "📊 Состояние сервера"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "👥 Пользователи"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "📈 Статистика"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "📚 База знаний"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "ℹ️ Помощь"})

	// 6. Interactive callbacks and in-place navigation
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-1",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "cb_status",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-2",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "cb_stats",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-3",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "cb_users",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-4",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "user_view:alice",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-5",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "user_role:alice:admin",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-6",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "user_reset:alice",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-7",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "user_toggle:alice:0",
	})
	bot.handleCallback(ctx, &CallbackQuery{
		ID:      "cb-8",
		Message: &Message{Chat: Chat{ID: 12345}, MessageID: 10},
		Data:    "user_toggle:alice:1",
	})

	// 7. /search, /recent, /spaces
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "/search architecture"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "/recent"})
	bot.handleMessage(ctx, &Message{Chat: Chat{ID: 12345}, Text: "/spaces"})

	// 8. Webhook update
	webhookUpdate := Update{
		UpdateID: 100,
		Message: &Message{
			Chat: Chat{ID: 12345},
			Text: "/status",
		},
	}
	rawUpdate, _ := json.Marshal(webhookUpdate)
	err = bot.HandleWebhook(ctx, "secret-test-token", rawUpdate)
	if err != nil {
		t.Fatalf("valid HandleWebhook failed: %v", err)
	}

	mock.mu.Lock()
	msgCount := len(mock.sentMessages)
	mock.mu.Unlock()

	if msgCount < 5 {
		t.Fatalf("expected at least 5 sent messages, got %d", msgCount)
	}
}

func TestRenderGauge(t *testing.T) {
	g0 := renderGauge(0, 10)
	if g0 != "░░░░░░░░░░" {
		t.Fatalf("renderGauge(0) = %q", g0)
	}
	g50 := renderGauge(50, 10)
	if g50 != "█████░░░░░" {
		t.Fatalf("renderGauge(50) = %q", g50)
	}
	g100 := renderGauge(100, 10)
	if g100 != "██████████" {
		t.Fatalf("renderGauge(100) = %q", g100)
	}
}

func TestBotHTMLEscaping(t *testing.T) {
	bot, _, mock, cleanup := setupTestBot(t)
	defer cleanup()

	ctx := context.Background()

	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 12345},
		Text: "/add_user <hacker>&test editor",
	})

	mock.mu.Lock()
	lastPayload := mock.sentMessages[len(mock.sentMessages)-1]
	mock.mu.Unlock()

	if strings.Contains(lastPayload.Text, "<hacker>") {
		t.Fatalf("raw unescaped HTML tag found in message: %q", lastPayload.Text)
	}
	if !strings.Contains(lastPayload.Text, "&lt;hacker&gt;&amp;test") {
		t.Fatalf("expected properly escaped HTML in message, got: %q", lastPayload.Text)
	}
}

func TestBotUnauthorizedChatRejected(t *testing.T) {
	bot, _, _, cleanup := setupTestBot(t)
	defer cleanup()

	ctx := context.Background()

	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 99999},
		Text: "/add_user hacker admin",
	})

	var count int
	_ = bot.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE username='hacker'`).Scan(&count)
	if count != 0 {
		t.Fatal("unauthorized chat was able to create a user")
	}
}

func TestBotPollingLoopStartAndStop(t *testing.T) {
	bot, _, _, cleanup := setupTestBot(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	bot.Stop()
}

func TestGenerateSecurePassword(t *testing.T) {
	p := GenerateSecurePassword(16)
	if len(p) != 16 {
		t.Fatalf("password length = %d, want 16", len(p))
	}
	hash, err := auth.HashPassword(p)
	if err != nil {
		t.Fatalf("HashPassword failed for generated password: %v", err)
	}
	if !auth.VerifyPassword(hash, p) {
		t.Fatal("VerifyPassword failed for generated password")
	}
}

func TestIsPublicURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"", false},
		{"http://localhost:8080/auth/magic", false},
		{"http://127.0.0.1:8080/auth/magic", false},
		{"http://0.0.0.0:9876/auth/magic", false},
		{"http://192.168.1.100:8080/auth/magic", false},
		{"http://10.0.0.5:8080/auth/magic", false},
		{"http://172.20.0.2:8080/auth/magic", false},
		{"http://docs.local/auth/magic", false},
		{"http://docs.internal/auth/magic", false},
		{"https://docs.mycompany.com/auth/magic", true},
		{"http://wiki.corp.net:8443/auth/magic", true},
		{"ftp://docs.com", false},
	}

	for _, tt := range tests {
		got := isPublicURL(tt.url)
		if got != tt.want {
			t.Errorf("isPublicURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestGetBaseURL(t *testing.T) {
	b := &Bot{cfg: config.Config{Addr: ":9876"}}
	if got := b.getBaseURL(); got != "http://localhost:9876" {
		t.Errorf("getBaseURL(:9876) = %q, want http://localhost:9876", got)
	}

	b0 := &Bot{cfg: config.Config{Addr: "0.0.0.0:9876"}}
	if got := b0.getBaseURL(); got != "http://localhost:9876" {
		t.Errorf("getBaseURL(0.0.0.0:9876) = %q, want http://localhost:9876", got)
	}
}

