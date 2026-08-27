$ErrorActionPreference = 'Stop'
$inputText = [Console]::In.ReadToEnd()
try { $call = $inputText | ConvertFrom-Json } catch { exit 0 }
$command = [string]$call.tool_input.command
if ([string]::IsNullOrWhiteSpace($command)) { exit 0 }

$patterns = @(
  '(?i)(^|[;&|]\s*)rm\s+-[^\r\n]*r[^\r\n]*f[^\r\n]*(/|~|\\)',
  '(?i)Remove-Item\b[^\r\n]*-(Recurse|r)\b[^\r\n]*-(Force|fo)\b',
  '(?i)git\s+push\b[^\r\n]*--force(?:-with-lease)?\b',
  '(?i)git\s+reset\s+--hard\b',
  '(?i)git\s+clean\b[^\r\n]*-[^\r\n]*[xX]',
  '(?i)(curl|wget)\b[^\r\n]*\|\s*(sh|bash|zsh|pwsh|powershell)\b',
  '(?i)\b(format|diskpart|bcdedit)\b'
)
foreach ($pattern in $patterns) {
  if ($command -match $pattern) {
    @{
      hookSpecificOutput = @{
        hookEventName = 'PreToolUse'
        permissionDecision = 'deny'
        permissionDecisionReason = 'Claude Code Studio guard blocked a destructive shell operation. Run it manually only after reviewing the exact command.'
      }
    } | ConvertTo-Json -Depth 6 -Compress
    exit 0
  }
}
exit 0
