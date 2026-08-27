package autotrace

import (
	"context"
)

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) RouteDiagram(ctx context.Context, d *Diagram) error {
	nodeMap := make(map[string]*DiagramNode)
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}

	for i := range d.Edges {
		edge := &d.Edges[i]
		fromNode, ok1 := nodeMap[edge.FromNode]
		toNode, ok2 := nodeMap[edge.ToNode]

		if !ok1 || !ok2 {
			continue
		}

		start := findPortPoint(fromNode, edge.FromPort, PortOutput)
		end := findPortPoint(toNode, edge.ToPort, PortInput)

		// Compute orthogonal route with 3-segment or 5-segment step
		edge.Waypoints = routeOrthogonal(start, end)
	}

	return nil
}

func findPortPoint(n *DiagramNode, portID string, kind PortKind) Point {
	for _, p := range n.Ports {
		if p.ID == portID || p.Name == portID {
			return p.Point
		}
	}
	// Fallback to center-edge
	if kind == PortOutput {
		return Point{X: n.Rect.X + n.Rect.Width, Y: n.Rect.Y + n.Rect.Height/2}
	}
	return Point{X: n.Rect.X, Y: n.Rect.Y + n.Rect.Height/2}
}

func routeOrthogonal(start, end Point) []Point {
	var points []Point
	points = append(points, start)

	if start.X < end.X {
		midX := (start.X + end.X) / 2
		points = append(points, Point{X: midX, Y: start.Y})
		points = append(points, Point{X: midX, Y: end.Y})
	} else {
		// Backwards or same column loop
		offset := 30.0
		points = append(points, Point{X: start.X + offset, Y: start.Y})
		midY := (start.Y + end.Y) / 2
		points = append(points, Point{X: start.X + offset, Y: midY})
		points = append(points, Point{X: end.X - offset, Y: midY})
		points = append(points, Point{X: end.X - offset, Y: end.Y})
	}

	points = append(points, end)
	return points
}

func (e *Engine) Render(ctx context.Context, source string, opts RenderOptions) (RenderedDiagram, error) {
	diag, err := Parse(source)
	if err != nil {
		return RenderedDiagram{
			SVG:    RenderErrorSVG(err, 600, 140),
			Width:  600,
			Height: 140,
		}, nil
	}

	if err := ComputeLayout(diag); err != nil {
		return RenderedDiagram{
			SVG:    RenderErrorSVG(err, 600, 140),
			Width:  600,
			Height: 140,
		}, nil
	}

	if err := e.RouteDiagram(ctx, diag); err != nil {
		return RenderedDiagram{
			SVG:    RenderErrorSVG(err, 600, 140),
			Width:  600,
			Height: 140,
		}, nil
	}

	svg := RenderSVG(diag, opts)
	return RenderedDiagram{
		SVG:      svg,
		Width:    int(diag.Bounds.Width),
		Height:   int(diag.Bounds.Height),
		NodesNum: len(diag.Nodes),
		EdgesNum: len(diag.Edges),
	}, nil
}
