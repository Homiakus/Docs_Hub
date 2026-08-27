package autotrace

import (
	"fmt"
	"html"
	"strings"
)

func RenderSVG(d *Diagram, opts RenderOptions) string {
	w := int(d.Bounds.Width)
	h := int(d.Bounds.Height)
	if w <= 0 {
		w = 600
	}
	if h <= 0 {
		h = 300
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" class="autotrace-diagram" style="max-width:100%%;height:auto;display:block;background:var(--bg-canvas,#0f141c);border:1px solid var(--border-subtle,rgba(255,255,255,0.08));border-radius:8px;">`, w, h))
	sb.WriteString(`
  <defs>
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1.5 L 8 5 L 0 8.5 z" fill="#3b82f6" />
    </marker>
    <filter id="shadow" x="-5%" y="-5%" width="110%" height="115%">
      <feDropShadow dx="0" dy="2" stdDeviation="3" flood-color="#000" flood-opacity="0.3"/>
    </filter>
  </defs>
`)

	// 1. Render edges/connections
	for _, edge := range d.Edges {
		if len(edge.Waypoints) < 2 {
			continue
		}
		var pathData strings.Builder
		pathData.WriteString(fmt.Sprintf("M %.1f %.1f", edge.Waypoints[0].X, edge.Waypoints[0].Y))
		for _, pt := range edge.Waypoints[1:] {
			pathData.WriteString(fmt.Sprintf(" L %.1f %.1f", pt.X, pt.Y))
		}

		sb.WriteString(fmt.Sprintf(`  <path d="%s" fill="none" stroke="#3b82f6" stroke-width="2" stroke-linejoin="round" marker-end="url(#arrow)"/>`+"\n", pathData.String()))

		// Edge label if present
		if edge.Label != "" && len(edge.Waypoints) >= 2 {
			midIdx := len(edge.Waypoints) / 2
			midPt := edge.Waypoints[midIdx]
			labelEscaped := html.EscapeString(edge.Label)
			sb.WriteString(fmt.Sprintf(`  <text x="%.1f" y="%.1f" font-family="ui-sans-serif,system-ui,sans-serif" font-size="11" fill="#94a3b8" text-anchor="middle" dy="-6">%s</text>`+"\n", midPt.X, midPt.Y, labelEscaped))
		}
	}

	// 2. Render nodes
	for _, node := range d.Nodes {
		rx := node.Rect.X
		ry := node.Rect.Y
		rw := node.Rect.Width
		rh := node.Rect.Height
		titleEscaped := html.EscapeString(node.Title)

		sb.WriteString(fmt.Sprintf(`  <g class="autotrace-node" id="node-%s">`+"\n", html.EscapeString(node.ID)))
		sb.WriteString(fmt.Sprintf(`    <rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6" fill="#1e293b" stroke="#334155" stroke-width="1.5" filter="url(#shadow)"/>`+"\n", rx, ry, rw, rh))
		sb.WriteString(fmt.Sprintf(`    <text x="%.1f" y="%.1f" font-family="ui-sans-serif,system-ui,sans-serif" font-size="13" font-weight="600" fill="#f8fafc" text-anchor="middle">%s</text>`+"\n", rx+rw/2, ry+rh/2+4, titleEscaped))

		// Render ports
		for _, port := range node.Ports {
			fillColor := "#10b981"
			if port.Kind == PortInput {
				fillColor = "#6366f1"
			}
			sb.WriteString(fmt.Sprintf(`    <circle cx="%.1f" cy="%.1f" r="3.5" fill="%s" stroke="#0f172a" stroke-width="1.5"/>`+"\n", port.Point.X, port.Point.Y, fillColor))
		}

		sb.WriteString("  </g>\n")
	}

	sb.WriteString("</svg>")
	return sb.String()
}
