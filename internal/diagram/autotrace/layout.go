package autotrace

import (
	"math"
)

const (
	DefaultNodeWidth  = 160.0
	DefaultNodeHeight = 70.0
	PortRadius        = 4.0
	Padding           = 40.0
)

func ComputeLayout(d *Diagram) error {
	if len(d.Nodes) == 0 {
		d.Bounds = Rect{X: 0, Y: 0, Width: 400, Height: 200}
		return nil
	}

	nodeMap := make(map[string]*DiagramNode)
	for i := range d.Nodes {
		n := &d.Nodes[i]
		if n.Rect.Width <= 0 {
			n.Rect.Width = DefaultNodeWidth
		}
		// Adjust height for number of ports
		maxPorts := math.Max(float64(len(n.Inputs)), float64(len(n.Outputs)))
		if maxPorts > 2 {
			n.Rect.Height = DefaultNodeHeight + (maxPorts-2)*20.0
		} else {
			n.Rect.Height = DefaultNodeHeight
		}
		nodeMap[n.ID] = n
	}

	// Calculate in-degree for topological ranking
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for _, n := range d.Nodes {
		inDegree[n.ID] = 0
		adj[n.ID] = nil
	}
	for _, e := range d.Edges {
		if _, exists := nodeMap[e.ToNode]; exists {
			inDegree[e.ToNode]++
		}
		if _, exists := nodeMap[e.FromNode]; exists {
			adj[e.FromNode] = append(adj[e.FromNode], e.ToNode)
		}
	}

	// Assign rank / layers
	ranks := make(map[string]int)
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			ranks[id] = 0
			queue = append(queue, id)
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		currRank := ranks[curr]
		for _, neighbor := range adj[curr] {
			if ranks[neighbor] < currRank+1 {
				ranks[neighbor] = currRank + 1
				queue = append(queue, neighbor)
			}
		}
	}

	// Group by rank
	rankGroups := make(map[int][]*DiagramNode)
	maxRank := 0
	for i := range d.Nodes {
		n := &d.Nodes[i]
		r := ranks[n.ID]
		rankGroups[r] = append(rankGroups[r], n)
		if r > maxRank {
			maxRank = r
		}
	}

	rankSpacing := d.Options.RankSpacing
	if rankSpacing <= 0 {
		rankSpacing = 160.0
	}
	nodeSpacing := d.Options.NodeSpacing
	if nodeSpacing <= 0 {
		nodeSpacing = 40.0
	}

	// Compute X and Y
	maxX := 0.0
	maxY := 0.0

	for r := 0; r <= maxRank; r++ {
		nodes := rankGroups[r]
		totalHeight := 0.0
		for _, n := range nodes {
			totalHeight += n.Rect.Height
		}
		totalHeight += float64(len(nodes)-1) * nodeSpacing

		currentY := Padding
		xPos := Padding + float64(r)*(DefaultNodeWidth+rankSpacing)

		for _, n := range nodes {
			n.Rect.X = xPos
			n.Rect.Y = currentY
			currentY += n.Rect.Height + nodeSpacing

			if n.Rect.X+n.Rect.Width > maxX {
				maxX = n.Rect.X + n.Rect.Width
			}
			if n.Rect.Y+n.Rect.Height > maxY {
				maxY = n.Rect.Y + n.Rect.Height
			}

			// Generate Port geometry
			n.Ports = nil
			// Inputs on Left
			inStep := n.Rect.Height / float64(len(n.Inputs)+1)
			for idx, inName := range n.Inputs {
				n.Ports = append(n.Ports, Port{
					ID:    inName,
					Kind:  PortInput,
					Name:  inName,
					Point: Point{X: n.Rect.X, Y: n.Rect.Y + float64(idx+1)*inStep},
				})
			}
			// Outputs on Right
			outStep := n.Rect.Height / float64(len(n.Outputs)+1)
			for idx, outName := range n.Outputs {
				n.Ports = append(n.Ports, Port{
					ID:    outName,
					Kind:  PortOutput,
					Name:  outName,
					Point: Point{X: n.Rect.X + n.Rect.Width, Y: n.Rect.Y + float64(idx+1)*outStep},
				})
			}
		}
	}

	d.Bounds = Rect{
		X:      0,
		Y:      0,
		Width:  maxX + Padding,
		Height: maxY + Padding,
	}

	return nil
}
