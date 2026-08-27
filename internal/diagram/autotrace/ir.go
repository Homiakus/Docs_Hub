package autotrace

type FlowDirection string

const (
	FlowLeftToRight FlowDirection = "left-to-right"
	FlowRightToLeft FlowDirection = "right-to-left"
	FlowTopToBottom FlowDirection = "top-to-bottom"
	FlowBottomToTop FlowDirection = "bottom-to-top"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type PortKind string

const (
	PortInput  PortKind = "input"
	PortOutput PortKind = "output"
)

type Port struct {
	ID    string   `json:"id"`
	Kind  PortKind `json:"kind"`
	Name  string   `json:"name"`
	Point Point    `json:"point"`
}

type DiagramNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Inputs      []string `json:"inputs,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
	Rect        Rect     `json:"rect"`
	Ports       []Port   `json:"ports,omitempty"`
	FillColor   string   `json:"fill_color,omitempty"`
	StrokeColor string   `json:"stroke_color,omitempty"`
}

type DiagramEdge struct {
	FromNode  string  `json:"from_node"`
	FromPort  string  `json:"from_port"`
	ToNode    string  `json:"to_node"`
	ToPort    string  `json:"to_port"`
	Label     string  `json:"label,omitempty"`
	Waypoints []Point `json:"waypoints,omitempty"`
	Color     string  `json:"color,omitempty"`
}

type DiagramOptions struct {
	Theme       string  `json:"theme,omitempty"`
	GridSize    float64 `json:"grid_size,omitempty"`
	NodeSpacing float64 `json:"node_spacing,omitempty"`
	RankSpacing float64 `json:"rank_spacing,omitempty"`
}

type Diagram struct {
	Version int            `json:"version"`
	Flow    FlowDirection  `json:"flow"`
	Nodes   []DiagramNode  `json:"nodes"`
	Edges   []DiagramEdge  `json:"edges"`
	Options DiagramOptions `json:"options"`
	Bounds  Rect           `json:"bounds"`
}

type RenderOptions struct {
	Theme      string
	DarkMode   bool
	Responsive bool
	MaxWidth   int
}

type RenderedDiagram struct {
	SVG      string `json:"svg"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	NodesNum int    `json:"nodes_num"`
	EdgesNum int    `json:"edges_num"`
}
