package autotrace

import (
	"fmt"
	"html"
	"strings"
)

type DiagnosticError struct {
	Line    int
	Column  int
	Message string
}

func (e *DiagnosticError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("autotrace [line %d, col %d]: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("autotrace: %s", e.Message)
}

func RenderErrorSVG(err error, width, height int) string {
	if width <= 0 {
		width = 600
	}
	if height <= 0 {
		height = 140
	}
	msg := html.EscapeString(err.Error())
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="100%%" height="%d" class="autotrace-error">
  <rect width="100%%" height="100%%" rx="8" fill="#fff1f0" stroke="#ff4d4f" stroke-width="1.5"/>
  <text x="24" y="45" font-family="-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif" font-size="14" font-weight="bold" fill="#cf1322">Ошибка в схеме AutoTrace</text>
  <text x="24" y="80" font-family="ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace" font-size="12" fill="#595959">%s</text>
</svg>`, width, height, height, msg)
}

func parseSourceCoordinates(source string, targetSubstr string) (int, int) {
	lines := strings.Split(source, "\n")
	for i, l := range lines {
		if idx := strings.Index(l, targetSubstr); idx >= 0 {
			return i + 1, idx + 1
		}
	}
	return 1, 1
}
