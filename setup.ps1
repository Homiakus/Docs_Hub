# Docs_Hub Interactive Setup Script (Windows PowerShell)
# Encoding: UTF-8

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

function Show-Header {
    Clear-Host
    Write-Host "================================================================" -ForegroundColor Cyan
    Write-Host "                DOCS_HUB INTERACTIVE SETUP & RUN               " -ForegroundColor Yellow
    Write-Host "================================================================" -ForegroundColor Cyan
    Write-Host ""
}

function Generate-Secret([int]$length = 32) {
    $bytes = New-Object byte[] $length
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    $rng.GetBytes($bytes)
    return [System.BitConverter]::ToString($bytes).Replace("-", "").ToLower()
}

function Load-Env {
    $envFile = Join-Path $PSScriptRoot ".env"
    $config = @{}
    if (Test-Path $envFile) {
        Get-Content $envFile | ForEach-Object {
            $line = $_.Trim()
            if ($line -and -not $line.StartsWith("#")) {
                $idx = $line.IndexOf("=")
                if ($idx -gt 0) {
                    $key = $line.Substring(0, $idx).Trim()
                    $val = $line.Substring($idx + 1).Trim()
                    $config[$key] = $val
                }
            }
        }
    }
    return $config
}

function Save-Env($config) {
    $envFile = Join-Path $PSScriptRoot ".env"
    $sb = [System.Text.StringBuilder]::new()
    [void]$sb.AppendLine("# Docs Hub Next — Configuration")
    [void]$sb.AppendLine("# Auto-generated / configured by setup script")
    [void]$sb.AppendLine("")
    [void]$sb.AppendLine("# Admin Credentials")
    [void]$sb.AppendLine("ADMIN_USER=$($config['ADMIN_USER'])")
    [void]$sb.AppendLine("ADMIN_PASSWORD=$($config['ADMIN_PASSWORD'])")
    [void]$sb.AppendLine("SESSION_SECRET=$($config['SESSION_SECRET'])")
    [void]$sb.AppendLine("")
    [void]$sb.AppendLine("# Server & Storage")
    [void]$sb.AppendLine("ADDR=$($config['ADDR'])")
    [void]$sb.AppendLine("HOST_PORT=$($config['HOST_PORT'])")
    [void]$sb.AppendLine("BIND_ADDR=$($config['BIND_ADDR'])")
    [void]$sb.AppendLine("DATA_DIR=$($config['DATA_DIR'])")
    [void]$sb.AppendLine("UPLOAD_DIR=$($config['UPLOAD_DIR'])")
    [void]$sb.AppendLine("DB_PATH=$($config['DB_PATH'])")
    [void]$sb.AppendLine("SITE_NAME=$($config['SITE_NAME'])")
    [void]$sb.AppendLine("")
    [void]$sb.AppendLine("# Telegram Bot Notifications")
    [void]$sb.AppendLine("TELEGRAM_BOT_TOKEN=$($config['TELEGRAM_BOT_TOKEN'])")
    [void]$sb.AppendLine("TELEGRAM_CHAT_ID=$($config['TELEGRAM_CHAT_ID'])")
    [void]$sb.AppendLine("TELEGRAM_NOTIFICATIONS_ENABLED=$($config['TELEGRAM_NOTIFICATIONS_ENABLED'])")
    [void]$sb.AppendLine("")
    [void]$sb.AppendLine("# Security")
    [void]$sb.AppendLine("COOKIE_SECURE=$($config['COOKIE_SECURE'])")
    [void]$sb.AppendLine("LOG_LEVEL=$($config['LOG_LEVEL'])")
    [void]$sb.AppendLine("RATE_LIMIT_ENABLED=$($config['RATE_LIMIT_ENABLED'])")
    [void]$sb.AppendLine("RATE_LIMIT_RPM=$($config['RATE_LIMIT_RPM'])")
    [void]$sb.AppendLine("RATE_LIMIT_BURST=$($config['RATE_LIMIT_BURST'])")

    [System.IO.File]::WriteAllText($envFile, $sb.ToString(), [System.Text.Encoding]::UTF8)
    Write-Host " -> Файл .env успешно сохранён!" -ForegroundColor Green
}

function Configure-All {
    Show-Header
    Write-Host "[1] Пошаговая настройка конфигурации .env" -ForegroundColor Yellow
    Write-Host "--------------------------------------------------------"

    $cfg = Load-Env
    if (-not $cfg['ADMIN_USER']) { $cfg['ADMIN_USER'] = "admin" }
    if (-not $cfg['ADDR']) { $cfg['ADDR'] = ":8080" }
    if (-not $cfg['HOST_PORT']) { $cfg['HOST_PORT'] = "8080" }
    if (-not $cfg['BIND_ADDR']) { $cfg['BIND_ADDR'] = "0.0.0.0" }
    if (-not $cfg['DATA_DIR']) { $cfg['DATA_DIR'] = "./data" }
    if (-not $cfg['UPLOAD_DIR']) { $cfg['UPLOAD_DIR'] = "./data/uploads" }
    if (-not $cfg['DB_PATH']) { $cfg['DB_PATH'] = "./data/docshub.db" }
    if (-not $cfg['SITE_NAME']) { $cfg['SITE_NAME'] = "Docs Hub Next" }
    if (-not $cfg['COOKIE_SECURE']) { $cfg['COOKIE_SECURE'] = "0" }
    if (-not $cfg['LOG_LEVEL']) { $cfg['LOG_LEVEL'] = "info" }
    if (-not $cfg['RATE_LIMIT_ENABLED']) { $cfg['RATE_LIMIT_ENABLED'] = "true" }
    if (-not $cfg['RATE_LIMIT_RPM']) { $cfg['RATE_LIMIT_RPM'] = "60" }
    if (-not $cfg['RATE_LIMIT_BURST']) { $cfg['RATE_LIMIT_BURST'] = "10" }

    # 1. Admin username
    $inputUser = Read-Host "Имя пользователя администратора (по умолчанию: $($cfg['ADMIN_USER']))"
    if ($inputUser) { $cfg['ADMIN_USER'] = $inputUser }

    # 2. Admin password
    $curPass = if ($cfg['ADMIN_PASSWORD']) { "(уже задан)" } else { "(не задан)" }
    $inputPass = Read-Host "Пароль администратора (мин. 8 симв.) $curPass"
    if ($inputPass) {
        $cfg['ADMIN_PASSWORD'] = $inputPass
    } elseif (-not $cfg['ADMIN_PASSWORD']) {
        $cfg['ADMIN_PASSWORD'] = Generate-Secret 12
        Write-Host " -> Сгенерирован надёжный пароль: $($cfg['ADMIN_PASSWORD'])" -ForegroundColor Green
    }

    # 3. Session Secret
    if (-not $cfg['SESSION_SECRET']) {
        $cfg['SESSION_SECRET'] = Generate-Secret 32
        Write-Host " -> Сгенерирован 32-байтный SESSION_SECRET." -ForegroundColor Green
    }

    # 4. Port
    $inputPort = Read-Host "Порт для Docs_Hub (по умолчанию: $($cfg['HOST_PORT']))"
    if ($inputPort) {
        $cfg['HOST_PORT'] = $inputPort
        $cfg['ADDR'] = ":$inputPort"
    }

    # 5. Telegram Bot
    Write-Host "`n--- Настройка Telegram бота для уведомлений (опционально) ---" -ForegroundColor Cyan
    $curToken = if ($cfg['TELEGRAM_BOT_TOKEN']) { "[$($cfg['TELEGRAM_BOT_TOKEN'].Substring(0, [Math]::Min(10, $cfg['TELEGRAM_BOT_TOKEN'].Length)))...]" } else { "(пусто)" }
    $inputToken = Read-Host "Telegram Bot Token (из @BotFather) $curToken"
    if ($inputToken) { $cfg['TELEGRAM_BOT_TOKEN'] = $inputToken }

    $curChat = if ($cfg['TELEGRAM_CHAT_ID']) { "[$($cfg['TELEGRAM_CHAT_ID'])]" } else { "(пусто)" }
    $inputChat = Read-Host "Telegram Chat ID / User ID (из @userinfobot) $curChat"
    if ($inputChat) { $cfg['TELEGRAM_CHAT_ID'] = $inputChat }

    if ($cfg['TELEGRAM_BOT_TOKEN'] -and $cfg['TELEGRAM_CHAT_ID']) {
        $cfg['TELEGRAM_NOTIFICATIONS_ENABLED'] = "true"
        Write-Host " -> Telegram уведомления включены." -ForegroundColor Green
    } else {
        $cfg['TELEGRAM_NOTIFICATIONS_ENABLED'] = "false"
    }

    Save-Env $cfg
    Write-Host "`nНажмите Enter для возврата в меню..."
    [void][System.Console]::ReadLine()
}

function Test-Telegram {
    Show-Header
    Write-Host "[2] Проверка отправки сообщения через Telegram Bot" -ForegroundColor Yellow
    Write-Host "--------------------------------------------------------"

    $cfg = Load-Env
    $token = $cfg['TELEGRAM_BOT_TOKEN']
    $chatId = $cfg['TELEGRAM_CHAT_ID']

    if (-not $token -or -not $chatId) {
        Write-Host "ОШИБКА: TELEGRAM_BOT_TOKEN или TELEGRAM_CHAT_ID не заполнены в .env!" -ForegroundColor Red
        Write-Host "Сначала выполните пункт 1 для настройки." -ForegroundColor Yellow
        Write-Host "`nНажмите Enter..."
        [void][System.Console]::ReadLine()
        return
    }

    Write-Host "Отправка тестового сообщения ботом в чат $chatId..." -ForegroundColor Cyan
    try {
        $msg = "🚀 <b>Docs_Hub</b>: Тестовое уведомление успешно доставлено!`nСистема готова к работе."
        $url = "https://api.telegram.org/bot$token/sendMessage"
        $body = @{
            chat_id = $chatId
            text = $msg
            parse_mode = "HTML"
        }
        $response = Invoke-RestMethod -Uri $url -Method Post -Body $body -ErrorAction Stop
        if ($response.ok) {
            Write-Host " -> УСПЕХ! Сообщение отправлено в Telegram (Message ID: $($response.result.message_id))" -ForegroundColor Green
        } else {
            Write-Host " -> Ответ Telegram API: $($response | ConvertTo-Json)" -ForegroundColor Red
        }
    } catch {
        Write-Host " -> Ошибка отправки: $($_.Exception.Message)" -ForegroundColor Red
    }

    Write-Host "`nНажмите Enter..."
    [void][System.Console]::ReadLine()
}

function Run-Local {
    Show-Header
    Write-Host "[3] Запуск Docs_Hub локально" -ForegroundColor Yellow
    Write-Host "--------------------------------------------------------"

    $cfg = Load-Env
    if (-not $cfg['ADMIN_PASSWORD'] -or -not $cfg['SESSION_SECRET']) {
        Write-Host "Внимание: .env не настроен. Запуск генерации..." -ForegroundColor Yellow
        Configure-All
        $cfg = Load-Env
    }

    # Set environment variables in current process
    foreach ($k in $cfg.Keys) {
        [System.Environment]::SetEnvironmentVariable($k, $cfg[$k], "Process")
    }

    Write-Host "Запуск Docs_Hub на http://localhost:$($cfg['HOST_PORT'])..." -ForegroundColor Green
    Write-Host "Логин админа: $($cfg['ADMIN_USER'])" -ForegroundColor Cyan
    Write-Host "Для остановки нажмите Ctrl+C" -ForegroundColor DarkGray
    Write-Host ""

    go run ./cmd/docshub
}

function Run-Docker {
    Show-Header
    Write-Host "[4] Запуск Docs_Hub в Docker Compose" -ForegroundColor Yellow
    Write-Host "--------------------------------------------------------"

    if (-not (Get-Command "docker" -ErrorAction SilentlyContinue)) {
        Write-Host "ОШИБКА: Docker не найден в системе!" -ForegroundColor Red
        Write-Host "`nНажмите Enter..."
        [void][System.Console]::ReadLine()
        return
    }

    Write-Host "Запуск: docker compose up -d --build..." -ForegroundColor Cyan
    docker compose up -d --build
    Write-Host "`nКонтейнеры запущены. Проверка:" -ForegroundColor Green
    docker compose ps
    Write-Host "`nНажмите Enter..."
    [void][System.Console]::ReadLine()
}

function Check-Health {
    Show-Header
    Write-Host "[5] Проверка состояния сервера (/healthz и /readyz)" -ForegroundColor Yellow
    Write-Host "--------------------------------------------------------"

    $cfg = Load-Env
    $port = if ($cfg['HOST_PORT']) { $cfg['HOST_PORT'] } else { "8080" }
    $baseUrl = "http://localhost:$port"

    Write-Host "Опрос $baseUrl/healthz..." -ForegroundColor Cyan
    try {
        $res = Invoke-RestMethod -Uri "$baseUrl/healthz" -Method Get -TimeoutSec 3
        Write-Host " -> /healthz: $($res | ConvertTo-Json -Compress)" -ForegroundColor Green
    } catch {
        Write-Host " -> /healthz недоступен ($($_.Exception.Message))" -ForegroundColor Red
    }

    Write-Host "Опрос $baseUrl/readyz..." -ForegroundColor Cyan
    try {
        $res = Invoke-RestMethod -Uri "$baseUrl/readyz" -Method Get -TimeoutSec 3
        Write-Host " -> /readyz: $($res | ConvertTo-Json -Compress)" -ForegroundColor Green
    } catch {
        Write-Host " -> /readyz недоступен ($($_.Exception.Message))" -ForegroundColor Red
    }

    Write-Host "`nНажмите Enter..."
    [void][System.Console]::ReadLine()
}

# Main Loop
do {
    Show-Header
    Write-Host "1. Пошаговая настройка .env (Admin, Пароль, Telegram Бот, Порт)" -ForegroundColor White
    Write-Host "2. Проверить отправку тестового сообщения в Telegram" -ForegroundColor White
    Write-Host "3. Запустить Docs_Hub локально (go run ./cmd/docshub)" -ForegroundColor White
    Write-Host "4. Запустить Docs_Hub в Docker Compose" -ForegroundColor White
    Write-Host "5. Проверить статус работающего сервера (/healthz, /readyz)" -ForegroundColor White
    Write-Host "6. Сгенерировать новые случайные ключи безопасности" -ForegroundColor White
    Write-Host "0. Выход" -ForegroundColor DarkGray
    Write-Host ""
    $choice = Read-Host "Выберите пункт меню [0-6]"

    switch ($choice) {
        "1" { Configure-All }
        "2" { Test-Telegram }
        "3" { Run-Local }
        "4" { Run-Docker }
        "5" { Check-Health }
        "6" {
            $cfg = Load-Env
            $cfg['ADMIN_PASSWORD'] = Generate-Secret 14
            $cfg['SESSION_SECRET'] = Generate-Secret 32
            Save-Env $cfg
            Write-Host "`nСгенерированы новые ключи:`nПароль: $($cfg['ADMIN_PASSWORD'])`nSession Secret: $($cfg['SESSION_SECRET'])" -ForegroundColor Green
            Write-Host "`nНажмите Enter..."
            [void][System.Console]::ReadLine()
        }
        "0" { break }
    }
} while ($true)
