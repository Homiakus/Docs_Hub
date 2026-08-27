package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/homiakus/docshub-next/internal/auth"
	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
)

type Bot struct {
	token       string
	adminChatID string
	apiURL      string
	db          *db.DB
	cfg         config.Config
	log         *slog.Logger
	client      *http.Client
	startTime   time.Time
	mu          sync.Mutex
	stopCh      chan struct{}
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
	Date      int64  `json:"date"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type ReplyKeyboardMarkup struct {
	Keyboard       [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard bool               `json:"resize_keyboard"`
	IsPersistent   bool               `json:"is_persistent,omitempty"`
}

type KeyboardButton struct {
	Text string `json:"text"`
}

type sendPayload struct {
	ChatID      any    `json:"chat_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type editPayload struct {
	ChatID      any                   `json:"chat_id"`
	MessageID   int                   `json:"message_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type answerCallbackPayload struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

func NewBot(cfg config.Config, database *db.DB, logger *slog.Logger) *Bot {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bot{
		token:       strings.TrimSpace(cfg.TelegramBotToken),
		adminChatID: strings.TrimSpace(cfg.TelegramChatID),
		apiURL:      "https://api.telegram.org",
		db:          database,
		cfg:         cfg,
		log:         logger,
		client:      &http.Client{Timeout: 35 * time.Second},
		startTime:   time.Now(),
		stopCh:      make(chan struct{}),
	}
}

// SetBaseURL overrides the Telegram API base URL (useful for testing).
func (b *Bot) SetBaseURL(u string) {
	b.apiURL = strings.TrimRight(u, "/")
}

// Start launches the Telegram worker (Long-Polling or Webhook registration).
func (b *Bot) Start(ctx context.Context) {
	if b.token == "" {
		b.log.Debug("telegram bot: token not configured, skipping")
		return
	}

	if b.cfg.TelegramWebhookURL != "" {
		b.log.Info("telegram bot: configuring webhook mode", "url", b.cfg.TelegramWebhookURL)
		if err := b.registerWebhook(ctx); err != nil {
			b.log.Error("telegram bot: register webhook failed", "err", err)
		}
		return
	}

	b.log.Info("telegram bot started in long-polling mode", "adminChat", b.adminChatID)
	go b.pollLoop(ctx)
}

func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.stopCh:
	default:
		close(b.stopCh)
	}
}

func (b *Bot) registerWebhook(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/bot%s/setWebhook", b.apiURL, b.token)
	data := url.Values{}
	data.Set("url", b.cfg.TelegramWebhookURL)
	if b.cfg.TelegramWebhookSecret != "" {
		data.Set("secret_token", b.cfg.TelegramWebhookSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("setWebhook status %d", resp.StatusCode)
	}
	return nil
}

// HandleWebhook processes incoming updates received via HTTP webhook.
func (b *Bot) HandleWebhook(ctx context.Context, secretHeader string, body []byte) error {
	if b.cfg.TelegramWebhookSecret != "" && secretHeader != b.cfg.TelegramWebhookSecret {
		return errors.New("invalid webhook secret token")
	}

	var update Update
	if err := json.Unmarshal(body, &update); err != nil {
		return fmt.Errorf("decode update: %w", err)
	}

	if update.Message != nil && update.Message.Text != "" {
		b.handleMessage(ctx, update.Message)
	} else if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		b.handleCallback(ctx, update.CallbackQuery)
	}
	return nil
}

func (b *Bot) pollLoop(ctx context.Context) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		default:
		}

		updates, err := b.fetchUpdates(ctx, offset)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message != nil && u.Message.Text != "" {
				b.handleMessage(ctx, u.Message)
			} else if u.CallbackQuery != nil && u.CallbackQuery.Message != nil {
				b.handleCallback(ctx, u.CallbackQuery)
			}
		}
	}
}

func (b *Bot) fetchUpdates(ctx context.Context, offset int) ([]Update, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=25", b.apiURL, b.token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram API status %d", resp.StatusCode)
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (b *Bot) getBaseURL() string {
	addr := strings.TrimSpace(b.cfg.Addr)
	if addr == "" {
		addr = "localhost:8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = "localhost:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	if strings.HasPrefix(addr, "127.0.0.1:") {
		addr = "localhost:" + strings.TrimPrefix(addr, "127.0.0.1:")
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		if b.cfg.TLS.Enabled {
			addr = "https://" + addr
		} else {
			addr = "http://" + addr
		}
	}
	return strings.TrimRight(addr, "/")
}

func isPublicURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "0.0.0.0" || lower == "::1" || lower == "0" {
		return false
	}
	if strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".lan") || strings.HasSuffix(lower, ".example") || strings.HasSuffix(lower, ".test") {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false
		}
	}
	return strings.Contains(host, ".")
}

func (b *Bot) SendMessage(ctx context.Context, chatID any, text string) error {
	return b.SendMessageWithMarkup(ctx, chatID, text, nil)
}

func (b *Bot) SendMessageWithMarkup(ctx context.Context, chatID any, text string, markup any) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", b.apiURL, b.token)
	payload := sendPayload{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		b.log.Error("telegram sendMessage request error", "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		b.log.Error("telegram sendMessage API error", "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("telegram sendMessage error HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *Bot) EditMessageText(ctx context.Context, chatID any, messageID int, text string, markup *InlineKeyboardMarkup) error {
	endpoint := fmt.Sprintf("%s/bot%s/editMessageText", b.apiURL, b.token)
	payload := editPayload{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		b.log.Error("telegram editMessageText request error", "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		b.log.Error("telegram editMessageText API error", "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("telegram editMessageText error HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *Bot) AnswerCallback(ctx context.Context, callbackID string, text string, showAlert bool) error {
	if callbackID == "" {
		return nil
	}
	endpoint := fmt.Sprintf("%s/bot%s/answerCallbackQuery", b.apiURL, b.token)
	payload := answerCallbackPayload{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// SendNotification sends an administrative broadcast to configured admin chat.
func (b *Bot) SendNotification(ctx context.Context, text string) error {
	if b.adminChatID == "" {
		return nil
	}
	return b.SendMessage(ctx, b.adminChatID, text)
}

func (b *Bot) isAuthorized(chatID int64, username string) bool {
	if b.adminChatID == "" {
		return true
	}
	chatIDStr := strconv.FormatInt(chatID, 10)
	return chatIDStr == b.adminChatID || (username != "" && username == strings.TrimPrefix(b.adminChatID, "@"))
}

func (b *Bot) defaultReplyKeyboard() *ReplyKeyboardMarkup {
	return &ReplyKeyboardMarkup{
		Keyboard: [][]KeyboardButton{
			{
				{Text: "🚀 Войти в 1 клик"},
				{Text: "📊 Состояние сервера"},
			},
			{
				{Text: "👥 Пользователи"},
				{Text: "📈 Статистика"},
			},
			{
				{Text: "📚 База знаний"},
				{Text: "ℹ️ Помощь"},
			},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

func renderGauge(percent float64, width int) string {
	if width < 5 {
		width = 10
	}
	filled := int(math.Round(percent / 100 * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func (b *Bot) handleCallback(ctx context.Context, cb *CallbackQuery) {
	if !b.isAuthorized(cb.Message.Chat.ID, cb.From.Username) {
		_ = b.SendMessage(ctx, cb.Message.Chat.ID, "⛔ <b>Доступ ограничен</b>")
		return
	}

	data := cb.Data
	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID

	// Instant toast feedback
	_ = b.AnswerCallback(ctx, cb.ID, "", false)

	switch {
	case data == "cb_mylogin":
		b.cmdMyLogin(ctx, chatID, cb.From.Username)
	case data == "cb_status":
		b.renderStatusCard(ctx, chatID, msgID, true)
	case data == "cb_stats":
		b.renderStatsCard(ctx, chatID, msgID, true)
	case data == "cb_users":
		b.renderUsersHub(ctx, chatID, msgID, true)
	case data == "cb_recent":
		b.cmdRecent(ctx, chatID)
	case data == "cb_spaces":
		b.cmdSpaces(ctx, chatID)
	case data == "cb_help":
		b.renderHelpCard(ctx, chatID, msgID, true)
	case strings.HasPrefix(data, "user_view:"):
		username := strings.TrimPrefix(data, "user_view:")
		b.renderUserCard(ctx, chatID, msgID, username)
	case strings.HasPrefix(data, "user_role:"):
		parts := strings.Split(strings.TrimPrefix(data, "user_role:"), ":")
		if len(parts) == 2 {
			b.cmdSetRole(ctx, chatID, parts)
			b.renderUserCard(ctx, chatID, msgID, parts[0])
		}
	case strings.HasPrefix(data, "user_reset:"):
		username := strings.TrimPrefix(data, "user_reset:")
		b.cmdResetPassword(ctx, chatID, []string{username})
	case strings.HasPrefix(data, "user_toggle:"):
		parts := strings.Split(strings.TrimPrefix(data, "user_toggle:"), ":")
		if len(parts) == 2 {
			isActive := parts[1] == "1"
			b.cmdToggleActive(ctx, chatID, []string{parts[0]}, isActive)
			b.renderUserCard(ctx, chatID, msgID, parts[0])
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *Message) {
	if !b.isAuthorized(msg.Chat.ID, msg.Chat.Username) {
		_ = b.SendMessage(ctx, msg.Chat.ID, "⛔ <b>Доступ ограничен</b>\nЭтот бот предназначен исключительно для администрирования базы знаний Docs_Hub.")
		return
	}

	text := strings.TrimSpace(msg.Text)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	if idx := strings.Index(cmd, "@"); idx != -1 {
		cmd = cmd[:idx]
	}

	// Handle natural bottom reply keyboard commands
	switch text {
	case "🚀 Войти в 1 клик":
		b.cmdMyLogin(ctx, msg.Chat.ID, msg.Chat.Username)
		return
	case "📊 Состояние сервера":
		b.cmdStatus(ctx, msg.Chat.ID)
		return
	case "👥 Пользователи":
		b.cmdListUsers(ctx, msg.Chat.ID)
		return
	case "📈 Статистика":
		b.cmdStats(ctx, msg.Chat.ID)
		return
	case "📚 База знаний":
		b.cmdRecent(ctx, msg.Chat.ID)
		return
	case "ℹ️ Помощь":
		b.cmdHelp(ctx, msg.Chat.ID)
		return
	}

	switch cmd {
	case "/start":
		b.cmdStart(ctx, msg.Chat.ID)
	case "/help":
		b.cmdHelp(ctx, msg.Chat.ID)
	case "/login", "/my_login", "/me", "/auth":
		userParam := ""
		if len(parts) > 1 {
			userParam = parts[1]
		} else if msg.Chat.Username != "" {
			userParam = msg.Chat.Username
		}
		b.cmdMyLogin(ctx, msg.Chat.ID, userParam)
	case "/status", "/health":
		b.cmdStatus(ctx, msg.Chat.ID)
	case "/stats":
		b.cmdStats(ctx, msg.Chat.ID)
	case "/users":
		b.cmdListUsers(ctx, msg.Chat.ID)
	case "/add_user", "/invite":
		b.cmdAddUser(ctx, msg.Chat.ID, parts[1:])
	case "/magic_link", "/otp":
		b.cmdMagicLink(ctx, msg.Chat.ID, parts[1:])
	case "/search", "/find":
		b.cmdSearch(ctx, msg.Chat.ID, parts[1:])
	case "/recent":
		b.cmdRecent(ctx, msg.Chat.ID)
	case "/spaces":
		b.cmdSpaces(ctx, msg.Chat.ID)
	case "/set_role":
		b.cmdSetRole(ctx, msg.Chat.ID, parts[1:])
	case "/reset_password":
		b.cmdResetPassword(ctx, msg.Chat.ID, parts[1:])
	case "/block_user":
		b.cmdToggleActive(ctx, msg.Chat.ID, parts[1:], false)
	case "/unblock_user":
		b.cmdToggleActive(ctx, msg.Chat.ID, parts[1:], true)
	default:
		_ = b.SendMessage(ctx, msg.Chat.ID, "Неизвестная команда. Введите <b>/help</b> или воспользуйтесь кнопками ниже.")
	}
}

func (b *Bot) cmdStart(ctx context.Context, chatID int64) {
	welcome := `⚡ <b>Docs_Hub · Control Plane & ChatOps</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

Добро пожаловать в командный центр корпоративной базы знаний.
Используйте кнопки меню внизу для быстрого доступа или инлайн-действия:`

	_ = b.SendMessageWithMarkup(ctx, chatID, welcome, b.defaultReplyKeyboard())
	b.renderHelpCard(ctx, chatID, 0, false)
}

func (b *Bot) renderHelpCard(ctx context.Context, chatID int64, msgID int, isEdit bool) {
	helpText := `⚡ <b>Docs_Hub · Справка по командам</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

<b>🔑 Быстрый доступ:</b>
• <code>/login</code> или <code>/me</code> — <b>Мгновенный вход в систему</b> (в 1 клик)

<b>👥 Управление доступом & Onboarding:</b>
• <code>/add_user &lt;login&gt; [role]</code> — Создать аккаунт со ссылкой
• <code>/invite &lt;@handle&gt; [role]</code> — Подключить профиль Telegram
• <code>/magic_link &lt;login&gt;</code> — Сгенерировать OTP вход
• <code>/users</code> — Интерактивное управление командой
• <code>/set_role &lt;login&gt; &lt;admin|editor|reader&gt;</code> — Назначить роль

<b>📚 Навигация и поиск:</b>
• <code>/search &lt;запрос&gt;</code> — Мгновенный поиск по документам
• <code>/recent</code> — Последние обновлённые статьи
• <code>/spaces</code> — Список пространств

<b>📊 Мониторинг:</b>
• <code>/status</code> — Живая телеметрия (RAM, Uptime, Горутины)
• <code>/stats</code> — Сводная статистика контента`

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🚀 Войти в 1 клик", CallbackData: "cb_mylogin"},
				{Text: "📊 Состояние", CallbackData: "cb_status"},
			},
			{
				{Text: "👥 Команда", CallbackData: "cb_users"},
				{Text: "📈 Статистика", CallbackData: "cb_stats"},
			},
			{
				{Text: "📚 Статьи", CallbackData: "cb_recent"},
				{Text: "📁 Пространства", CallbackData: "cb_spaces"},
			},
		},
	}

	if isEdit && msgID > 0 {
		_ = b.EditMessageText(ctx, chatID, msgID, helpText, markup)
	} else {
		_ = b.SendMessageWithMarkup(ctx, chatID, helpText, markup)
	}
}

func (b *Bot) cmdHelp(ctx context.Context, chatID int64) {
	b.renderHelpCard(ctx, chatID, 0, false)
}

func (b *Bot) renderStatusCard(ctx context.Context, chatID int64, msgID int, isEdit bool) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(b.startTime).Round(time.Second)

	dbStatus := "🟢 WAL подключена"
	var userCount, docCount int
	if b.db != nil {
		_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE is_active=1`).Scan(&userCount)
		_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&docCount)
	} else {
		dbStatus = "🔴 Отключена"
	}

	allocMB := float64(mem.Alloc) / 1024 / 1024
	sysMB := float64(mem.Sys) / 1024 / 1024
	goroutines := runtime.NumGoroutine()

	// Visual Gauges
	memPercent := math.Min(100, (allocMB/128)*100)
	gaugeMem := renderGauge(memPercent, 10)

	msg := fmt.Sprintf(`📊 <b>Docs_Hub · Живая телеметрия сервера</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

🟢 <b>Статус сервиса:</b> <code>ONLINE</code>
⏱ <b>Аптайм:</b> <code>%s</code>
🗄 <b>База данных:</b> %s

💾 <b>Оперативная память:</b>
<code>[%s]</code> %.1f MB / Sys %.1f MB

⚡ <b>Нагрузка и горутины:</b>
• Активных горутин: <code>%d</code>
• Версия ядра: <code>v0.4.0-alpha.2</code>
• Адрес сервера: <code>%s</code>

👥 <b>Активные пользователи:</b> <code>%d</code>
📚 <b>Документов в индексе:</b> <code>%d</code>
━━━━━━━━━━━━━━━━━━━━━━━━━━
<i>Данные обновлены: %s</i>`,
		uptime,
		dbStatus,
		gaugeMem,
		allocMB,
		sysMB,
		goroutines,
		html.EscapeString(b.getBaseURL()),
		userCount,
		docCount,
		time.Now().Format("15:04:05"),
	)

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Обновить метрики", CallbackData: "cb_status"},
				{Text: "📈 Статистика", CallbackData: "cb_stats"},
			},
			{
				{Text: "🚀 Войти в Docs_Hub", CallbackData: "cb_mylogin"},
				{Text: "ℹ️ Меню", CallbackData: "cb_help"},
			},
		},
	}

	if isEdit && msgID > 0 {
		_ = b.EditMessageText(ctx, chatID, msgID, msg, markup)
	} else {
		_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
	}
}

func (b *Bot) cmdStatus(ctx context.Context, chatID int64) {
	b.renderStatusCard(ctx, chatID, 0, false)
}

func (b *Bot) renderStatsCard(ctx context.Context, chatID int64, msgID int, isEdit bool) {
	if b.db == nil {
		_ = b.SendMessage(ctx, chatID, "База данных недоступна.")
		return
	}

	var usersTotal, usersActive, usersAdmin, usersEditor, usersReader int
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&usersTotal)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE is_active=1`).Scan(&usersActive)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='admin'`).Scan(&usersAdmin)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='editor'`).Scan(&usersEditor)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='reader'`).Scan(&usersReader)

	var docsTotal, docsPublished, docsDrafts, spacesTotal, commentsTotal int
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&docsTotal)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE status='published' AND deleted_at IS NULL`).Scan(&docsPublished)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE status='draft' AND deleted_at IS NULL`).Scan(&docsDrafts)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM spaces`).Scan(&spacesTotal)
	_ = b.db.QueryRowContext(ctx, `SELECT count(*) FROM comments WHERE status='open'`).Scan(&commentsTotal)

	publishedRatio := 0.0
	if docsTotal > 0 {
		publishedRatio = float64(docsPublished) / float64(docsTotal) * 100
	}
	gaugePub := renderGauge(publishedRatio, 10)

	msg := fmt.Sprintf(`📈 <b>Docs_Hub · Сводная аналитика контента</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

👥 <b>Пользователи (%d всего / %d активных):</b>
• 🛡 Администраторы: <code>%d</code>
• ✍️ Редакторы: <code>%d</code>
• 📖 Читатели: <code>%d</code>

📚 <b>Статьи и публикации (%d всего):</b>
<code>[%s]</code> %.0f%% опубликовано
• 🟢 Опубликовано: <code>%d</code>
• 🟡 Черновиков в работе: <code>%d</code>

📁 <b>Организация знаний:</b>
• Пространств (Spaces): <code>%d</code>
• Открытых веток обсуждений: <code>%d</code>`,
		usersTotal, usersActive, usersAdmin, usersEditor, usersReader,
		docsTotal, gaugePub, publishedRatio, docsPublished, docsDrafts,
		spacesTotal, commentsTotal,
	)

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "👥 Список пользователей", CallbackData: "cb_users"},
				{Text: "📁 Пространства", CallbackData: "cb_spaces"},
			},
			{
				{Text: "🔄 Обновить", CallbackData: "cb_stats"},
				{Text: "ℹ️ Меню", CallbackData: "cb_help"},
			},
		},
	}

	if isEdit && msgID > 0 {
		_ = b.EditMessageText(ctx, chatID, msgID, msg, markup)
	} else {
		_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
	}
}

func (b *Bot) cmdStats(ctx context.Context, chatID int64) {
	b.renderStatsCard(ctx, chatID, 0, false)
}

func (b *Bot) renderUsersHub(ctx context.Context, chatID int64, msgID int, isEdit bool) {
	if b.db == nil {
		_ = b.SendMessage(ctx, chatID, "База данных недоступна.")
		return
	}

	rows, err := b.db.QueryContext(ctx, `SELECT id, username, coalesce(display_name,''), role, is_active, created_at FROM users ORDER BY id DESC LIMIT 8`)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка получения пользователей: %v", err))
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("👥 <b>Docs_Hub · Центр управления пользователями</b>\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	var keyboard [][]InlineKeyboardButton
	count := 0

	for rows.Next() {
		var id int64
		var username, name, role, createdAt string
		var active int
		if err := rows.Scan(&id, &username, &name, &role, &active, &createdAt); err == nil {
			count++
			statusIcon := "🟢"
			if active == 0 {
				statusIcon = "🔴"
			}
			if len(createdAt) > 10 {
				createdAt = createdAt[:10]
			}
			sb.WriteString(fmt.Sprintf("%s <code>%-12s</code> [%s] %s\n", statusIcon, html.EscapeString(username), html.EscapeString(role), html.EscapeString(name)))

			// Add interactive button for each user
			btnText := fmt.Sprintf("%s %s (%s)", statusIcon, username, role)
			keyboard = append(keyboard, []InlineKeyboardButton{
				{Text: btnText, CallbackData: "user_view:" + username},
			})
		}
	}

	if count == 0 {
		sb.WriteString("Пользователи не найдены.\n")
	} else {
		sb.WriteString("\n<i>💡 Нажмите на пользователя для смены роли, сброса пароля или блокировки:</i>")
	}

	keyboard = append(keyboard, []InlineKeyboardButton{
		{Text: "➕ Добавить пользователя", CallbackData: "cb_help"},
		{Text: "◀️ Меню", CallbackData: "cb_help"},
	})

	markup := &InlineKeyboardMarkup{InlineKeyboard: keyboard}

	if isEdit && msgID > 0 {
		_ = b.EditMessageText(ctx, chatID, msgID, sb.String(), markup)
	} else {
		_ = b.SendMessageWithMarkup(ctx, chatID, sb.String(), markup)
	}
}

func (b *Bot) cmdListUsers(ctx context.Context, chatID int64) {
	b.renderUsersHub(ctx, chatID, 0, false)
}

func (b *Bot) renderUserCard(ctx context.Context, chatID int64, msgID int, username string) {
	var id int64
	var displayName, role, createdAt, updatedAt string
	var isActive int

	err := b.db.QueryRowContext(ctx,
		`SELECT id, coalesce(display_name, ''), role, is_active, created_at, updated_at FROM users WHERE LOWER(username)=LOWER(?)`,
		username,
	).Scan(&id, &displayName, &role, &isActive, &createdAt, &updatedAt)
	if err != nil {
		_ = b.AnswerCallback(ctx, "", "Пользователь не найден", true)
		return
	}

	statusText := "🟢 Активен"
	toggleAction := "user_toggle:" + username + ":0"
	toggleBtnText := "🔴 Заблокировать доступ"
	if isActive == 0 {
		statusText = "🔴 Заблокирован"
		toggleAction = "user_toggle:" + username + ":1"
		toggleBtnText = "🟢 Разблокировать"
	}

	if len(createdAt) > 16 {
		createdAt = createdAt[:16]
	}

	cardText := fmt.Sprintf(`👤 <b>Карточка пользователя: %s</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

• <b>Логин:</b> <code>%s</code>
• <b>Имя:</b> %s
• <b>Статус:</b> %s
• <b>Текущая роль:</b> <b>%s</b>
• <b>Создан:</b> <code>%s</code>
━━━━━━━━━━━━━━━━━━━━━━━━━━
<i>Выберите действие для мгновенного управления:</i>`,
		html.EscapeString(username),
		html.EscapeString(username),
		html.EscapeString(displayName),
		statusText,
		html.EscapeString(role),
		html.EscapeString(createdAt),
	)

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🛡 Сделать Admin", CallbackData: "user_role:" + username + ":admin"},
				{Text: "✍️ Сделать Editor", CallbackData: "user_role:" + username + ":editor"},
				{Text: "📖 Reader", CallbackData: "user_role:" + username + ":reader"},
			},
			{
				{Text: "🔑 Сбросить пароль & OTP", CallbackData: "user_reset:" + username},
			},
			{
				{Text: toggleBtnText, CallbackData: toggleAction},
			},
			{
				{Text: "◀️ К списку команды", CallbackData: "cb_users"},
			},
		},
	}

	if msgID > 0 {
		_ = b.EditMessageText(ctx, chatID, msgID, cardText, markup)
	} else {
		_ = b.SendMessageWithMarkup(ctx, chatID, cardText, markup)
	}
}

// CreateMagicLink generates an ephemeral, single-use authentication URL for a user.
func (b *Bot) CreateMagicLink(ctx context.Context, username string) (string, error) {
	var userID int64
	var isActive int
	err := b.db.QueryRowContext(ctx, `SELECT id, is_active FROM users WHERE LOWER(username)=LOWER(?)`, username).Scan(&userID, &isActive)
	if err != nil {
		return "", err
	}
	if isActive == 0 {
		return "", errors.New("пользователь заблокирован")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	rawToken := hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute).Format(time.RFC3339)
	createdAt := now.Format(time.RFC3339)

	_, err = b.db.ExecContext(ctx,
		`INSERT INTO auth_tokens(token_hash, user_id, expires_at, created_at) VALUES(?, ?, ?, ?)`,
		tokenHash, userID, expiresAt, createdAt,
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/auth/magic?token=%s", b.getBaseURL(), rawToken), nil
}

func (b *Bot) cmdMyLogin(ctx context.Context, chatID int64, username string) {
	targetUser := strings.TrimPrefix(username, "@")
	targetUser = strings.TrimSpace(targetUser)

	adminUser := b.cfg.AdminUser
	if adminUser == "" {
		adminUser = "admin"
	}

	selectedUser := adminUser
	if targetUser != "" {
		var exists int
		_ = b.db.QueryRowContext(ctx, `SELECT 1 FROM users WHERE LOWER(username)=LOWER(?)`, targetUser).Scan(&exists)
		if exists == 1 {
			selectedUser = targetUser
		}
	}

	magicURL, err := b.CreateMagicLink(ctx, selectedUser)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Ошибка создания ссылки входа: %v", html.EscapeString(err.Error())))
		return
	}

	var keyboard [][]InlineKeyboardButton
	if isPublicURL(magicURL) {
		keyboard = append(keyboard, []InlineKeyboardButton{
			{Text: "🚀 Войти в Docs_Hub сразу", URL: magicURL},
		})
	}
	keyboard = append(keyboard, []InlineKeyboardButton{
		{Text: "📊 Состояние", CallbackData: "cb_status"},
		{Text: "ℹ️ Меню", CallbackData: "cb_help"},
	})

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}

	msg := fmt.Sprintf(`🔑 <b>Docs_Hub · Мгновенная авторизация</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

👤 <b>Учётная запись:</b> <code>%s</code>
⏱ <b>Срок действия:</b> 10 минут (однократный вход)

🌐 <b>Ссылка для мгновенного входа:</b>
<code>%s</code>

<i>👉 Нажмите на ссылку выше или скопируйте её в браузер для входа без пароля.</i>`,
		html.EscapeString(selectedUser), html.EscapeString(magicURL),
	)

	_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
}

func (b *Bot) cmdMagicLink(ctx context.Context, chatID int64, args []string) {
	if len(args) == 0 {
		_ = b.SendMessage(ctx, chatID, "⚠️ Использование: <code>/magic_link &lt;login&gt;</code>")
		return
	}

	username := strings.TrimPrefix(args[0], "@")
	magicURL, err := b.CreateMagicLink(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Пользователь <code>%s</code> не найден.", html.EscapeString(username)))
			return
		}
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка создания ссылки: %v", html.EscapeString(err.Error())))
		return
	}

	var keyboard [][]InlineKeyboardButton
	if isPublicURL(magicURL) {
		keyboard = append(keyboard, []InlineKeyboardButton{
			{Text: "🚀 Открыть ссылку входа", URL: magicURL},
		})
	}
	keyboard = append(keyboard, []InlineKeyboardButton{
		{Text: "👥 Пользователи", CallbackData: "cb_users"},
		{Text: "◀️ Меню", CallbackData: "cb_help"},
	})

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}

	msg := fmt.Sprintf(`🔗 <b>Одноразовая ссылка для входа (Magic Link)</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

👤 <b>Пользователь:</b> <code>%s</code>
⏱ <b>Срок действия:</b> 10 минут (однократный вход)

🌐 <b>Ссылка для входа:</b>
<code>%s</code>

<i>Перешлите эту ссылку пользователю для мгновенного подключения.</i>`,
		html.EscapeString(username), html.EscapeString(magicURL),
	)

	_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
}

func (b *Bot) cmdSearch(ctx context.Context, chatID int64, args []string) {
	if len(args) == 0 {
		_ = b.SendMessage(ctx, chatID, "⚠️ Использование: <code>/search &lt;запрос&gt;</code>\nПример: <code>/search архитектура</code>")
		return
	}

	query := strings.Join(args, " ")
	likeQuery := "%" + query + "%"

	rows, err := b.db.QueryContext(ctx,
		`SELECT id, slug, title, status, coalesce(language, 'ru'), created_at 
		 FROM articles 
		 WHERE deleted_at IS NULL AND (title LIKE ? OR content LIKE ?) 
		 ORDER BY updated_at DESC LIMIT 5`,
		likeQuery, likeQuery,
	)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка поиска: %v", html.EscapeString(err.Error())))
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 <b>Результаты поиска по запросу «%s»:</b>\n━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n", html.EscapeString(query)))

	count := 0
	for rows.Next() {
		var id int64
		var slug, title, status, lang, createdAt string
		if err := rows.Scan(&id, &slug, &title, &status, &lang, &createdAt); err == nil {
			count++
			link := fmt.Sprintf("%s/a/%s", b.getBaseURL(), url.PathEscape(slug))
			sb.WriteString(fmt.Sprintf("%d. 📄 <b><a href=\"%s\">%s</a></b> [%s]\n   <code>%s</code>\n\n", count, link, html.EscapeString(title), html.EscapeString(status), link))
		}
	}

	if count == 0 {
		sb.WriteString("Ничего не найдено по вашему запросу.\n")
	}

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📚 Последние статьи", CallbackData: "cb_recent"},
				{Text: "📁 Пространства", CallbackData: "cb_spaces"},
			},
		},
	}

	_ = b.SendMessageWithMarkup(ctx, chatID, sb.String(), markup)
}

func (b *Bot) cmdRecent(ctx context.Context, chatID int64) {
	rows, err := b.db.QueryContext(ctx,
		`SELECT id, slug, title, status, updated_at 
		 FROM articles 
		 WHERE deleted_at IS NULL 
		 ORDER BY updated_at DESC LIMIT 5`,
	)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка: %v", html.EscapeString(err.Error())))
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("📚 <b>Docs_Hub · Недавно обновлённые статьи</b>\n━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	count := 0
	for rows.Next() {
		var id int64
		var slug, title, status, updatedAt string
		if err := rows.Scan(&id, &slug, &title, &status, &updatedAt); err == nil {
			count++
			if len(updatedAt) > 16 {
				updatedAt = updatedAt[:16]
			}
			link := fmt.Sprintf("%s/a/%s", b.getBaseURL(), url.PathEscape(slug))
			sb.WriteString(fmt.Sprintf("%d. <b>%s</b> [%s]\n   📅 %s\n   <code>%s</code>\n\n", count, html.EscapeString(title), html.EscapeString(status), updatedAt, link))
		}
	}

	if count == 0 {
		sb.WriteString("Статьи не найдены.")
	}

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📁 Все пространства", CallbackData: "cb_spaces"},
				{Text: "ℹ️ Меню", CallbackData: "cb_help"},
			},
		},
	}

	_ = b.SendMessageWithMarkup(ctx, chatID, sb.String(), markup)
}

func (b *Bot) cmdSpaces(ctx context.Context, chatID int64) {
	rows, err := b.db.QueryContext(ctx,
		`SELECT s.name, s.slug, count(a.id) 
		 FROM spaces s 
		 LEFT JOIN articles a ON a.space_id = s.id AND a.deleted_at IS NULL 
		 GROUP BY s.id ORDER BY s.name ASC`,
	)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка: %v", html.EscapeString(err.Error())))
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("📁 <b>Docs_Hub · Пространства базы знаний</b>\n━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	count := 0
	for rows.Next() {
		var name, slug string
		var docCount int
		if err := rows.Scan(&name, &slug, &docCount); err == nil {
			count++
			sb.WriteString(fmt.Sprintf("• 📁 <b>%s</b> (<code>%s</code>) — <b>%d</b> документов\n", html.EscapeString(name), html.EscapeString(slug), docCount))
		}
	}

	if count == 0 {
		sb.WriteString("Пространства не найдены.")
	}

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📚 Недавние статьи", CallbackData: "cb_recent"},
				{Text: "ℹ️ Меню", CallbackData: "cb_help"},
			},
		},
	}

	_ = b.SendMessageWithMarkup(ctx, chatID, sb.String(), markup)
}

func (b *Bot) cmdAddUser(ctx context.Context, chatID int64, args []string) {
	if len(args) == 0 {
		_ = b.SendMessage(ctx, chatID, "⚠️ Использование: <code>/add_user &lt;login&gt; [admin|editor|reader]</code>\nПример: <code>/add_user ivan editor</code> или <code>/invite @ivan_dev reader</code>")
		return
	}

	rawInput := strings.TrimPrefix(args[0], "@")
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		_ = b.SendMessage(ctx, chatID, "⚠️ Укажите корректный логин или @telegram_handle.")
		return
	}

	role := "reader"
	if len(args) > 1 {
		r := strings.ToLower(args[1])
		if r == "admin" || r == "editor" || r == "reader" {
			role = r
		}
	}

	password := GenerateSecurePassword(14)
	hash, err := auth.HashPassword(password)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка хеширования: %v", html.EscapeString(err.Error())))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	displayName := rawInput

	res, err := b.db.ExecContext(ctx,
		`INSERT INTO users(username, display_name, password_hash, role, is_active, created_at, updated_at)
		 VALUES(?, ?, ?, ?, 1, ?, ?)`,
		rawInput, displayName, hash, role, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "already exists") {
			_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Пользователь <code>%s</code> уже существует! Используйте <code>/reset_password %s</code> для сброса пароля.", html.EscapeString(rawInput), html.EscapeString(rawInput)))
			return
		}
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка создания пользователя: %v", html.EscapeString(err.Error())))
		return
	}

	newID, _ := res.LastInsertId()
	magicLink, _ := b.CreateMagicLink(ctx, rawInput)

	var keyboard [][]InlineKeyboardButton
	if isPublicURL(magicLink) {
		keyboard = append(keyboard, []InlineKeyboardButton{
			{Text: "🚀 Войти по Magic Link", URL: magicLink},
		})
	}
	keyboard = append(keyboard, []InlineKeyboardButton{
		{Text: "👥 К списку команды", CallbackData: "cb_users"},
	})
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}

	msg := fmt.Sprintf(`✅ <b>Пользователь успешно создан!</b> (ID: %d)
━━━━━━━━━━━━━━━━━━━━━━━━━━

👤 <b>Логин:</b> <code>%s</code>
🔑 <b>Пароль:</b> <code>%s</code>
🛡 <b>Роль:</b> <code>%s</code>
🌐 <b>Вход в систему:</b> %s/login

🔗 <b>Ссылка для быстрого входа (10 мин):</b>
<code>%s</code>
━━━━━━━━━━━━━━━━━━━━━━━━━━
<i>Отправьте эту карточку пользователю для первого входа.</i>`,
		newID, html.EscapeString(rawInput), html.EscapeString(password), html.EscapeString(role), html.EscapeString(b.getBaseURL()), html.EscapeString(magicLink),
	)

	_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
}

func (b *Bot) cmdSetRole(ctx context.Context, chatID int64, args []string) {
	if len(args) < 2 {
		_ = b.SendMessage(ctx, chatID, "⚠️ Использование: <code>/set_role &lt;login&gt; &lt;admin|editor|reader&gt;</code>")
		return
	}

	username := strings.TrimPrefix(args[0], "@")
	role := strings.ToLower(args[1])
	if role != "admin" && role != "editor" && role != "reader" {
		_ = b.SendMessage(ctx, chatID, "⚠️ Допустимые роли: <code>admin</code>, <code>editor</code>, <code>reader</code>")
		return
	}

	res, err := b.db.ExecContext(ctx, `UPDATE users SET role=?, updated_at=? WHERE LOWER(username)=LOWER(?)`, role, time.Now().UTC().Format(time.RFC3339), username)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка обновления роли: %v", html.EscapeString(err.Error())))
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Пользователь <code>%s</code> не найден.", html.EscapeString(username)))
		return
	}

	_ = b.SendMessage(ctx, chatID, fmt.Sprintf("✅ Роль пользователя <code>%s</code> изменена на <b>%s</b>.", html.EscapeString(username), html.EscapeString(role)))
}

func (b *Bot) cmdResetPassword(ctx context.Context, chatID int64, args []string) {
	if len(args) == 0 {
		_ = b.SendMessage(ctx, chatID, "⚠️ Использование: <code>/reset_password &lt;login&gt;</code>")
		return
	}

	username := strings.TrimPrefix(args[0], "@")
	password := GenerateSecurePassword(14)
	hash, err := auth.HashPassword(password)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка хеширования: %v", html.EscapeString(err.Error())))
		return
	}

	var userID int64
	err = b.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(username)=LOWER(?)`, username).Scan(&userID)
	if err == sql.ErrNoRows {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Пользователь <code>%s</code> не найден.", html.EscapeString(username)))
		return
	} else if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка базы данных: %v", html.EscapeString(err.Error())))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = b.db.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, now, userID)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка смены пароля: %v", html.EscapeString(err.Error())))
		return
	}

	// Revoke sessions
	_, _ = b.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	magicLink, _ := b.CreateMagicLink(ctx, username)

	var keyboard [][]InlineKeyboardButton
	if isPublicURL(magicLink) {
		keyboard = append(keyboard, []InlineKeyboardButton{
			{Text: "🚀 Войти по новому OTP", URL: magicLink},
		})
	}
	keyboard = append(keyboard, []InlineKeyboardButton{
		{Text: "👥 К списку команды", CallbackData: "cb_users"},
	})
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}

	msg := fmt.Sprintf(`🔑 <b>Пароль пользователя успешно сброшен!</b>
━━━━━━━━━━━━━━━━━━━━━━━━━━

👤 <b>Логин:</b> <code>%s</code>
🔑 <b>Новый пароль:</b> <code>%s</code>
🌐 <b>Вход:</b> %s/login

🔗 <b>Быстрый вход (10 мин):</b>
<code>%s</code>
━━━━━━━━━━━━━━━━━━━━━━━━━━
<i>Активные сессии пользователя были принудительно завершены.</i>`,
		html.EscapeString(username), html.EscapeString(password), html.EscapeString(b.getBaseURL()), html.EscapeString(magicLink),
	)

	_ = b.SendMessageWithMarkup(ctx, chatID, msg, markup)
}

func (b *Bot) cmdToggleActive(ctx context.Context, chatID int64, args []string, active bool) {
	if len(args) == 0 {
		action := "/block_user"
		if active {
			action = "/unblock_user"
		}
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Использование: <code>%s &lt;login&gt;</code>", action))
		return
	}

	username := strings.TrimPrefix(args[0], "@")
	activeInt := 0
	statusWord := "заблокирован 🔴"
	if active {
		activeInt = 1
		statusWord = "разблокирован 🟢"
	}

	var userID int64
	err := b.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(username)=LOWER(?)`, username).Scan(&userID)
	if err == sql.ErrNoRows {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Пользователь <code>%s</code> не найден.", html.EscapeString(username)))
		return
	}

	_, err = b.db.ExecContext(ctx, `UPDATE users SET is_active=?, updated_at=? WHERE id=?`, activeInt, time.Now().UTC().Format(time.RFC3339), userID)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Ошибка: %v", html.EscapeString(err.Error())))
		return
	}

	if !active {
		_, _ = b.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	}

	_ = b.SendMessage(ctx, chatID, fmt.Sprintf("✅ Пользователь <code>%s</code> успешно %s.", html.EscapeString(username), statusWord))
}

// GenerateSecurePassword generates a friendly, high-entropy password.
func GenerateSecurePassword(length int) string {
	if length < 10 {
		length = 12
	}
	const chars = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%&*"
	res := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			buf := make([]byte, 1)
			_, _ = rand.Read(buf)
			res[i] = chars[int(buf[0])%len(chars)]
		} else {
			res[i] = chars[num.Int64()]
		}
	}
	return string(res)
}
