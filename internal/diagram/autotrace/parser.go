package autotrace

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func Parse(source string) (*Diagram, error) {
	if len(source) > MaxSourceSizeBytes {
		return nil, ErrSourceTooLarge
	}

	d := &Diagram{
		Version: 1,
		Flow:    FlowLeftToRight,
		Options: DiagramOptions{
			GridSize:    20,
			NodeSpacing: 60,
			RankSpacing: 140,
		},
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	lineNum := 0
	section := ""
	var currentNode *DiagramNode
	var currentEdge *DiagramEdge

	flushNode := func() {
		if currentNode != nil {
			if currentNode.ID == "" {
				currentNode.ID = fmt.Sprintf("node_%d", len(d.Nodes)+1)
			}
			if currentNode.Title == "" {
				currentNode.Title = currentNode.ID
			}
			if len(currentNode.Title) > MaxLabelLength {
				currentNode.Title = currentNode.Title[:MaxLabelLength]
			}
			d.Nodes = append(d.Nodes, *currentNode)
			currentNode = nil
		}
	}

	flushEdge := func() {
		if currentEdge != nil {
			if currentEdge.FromNode != "" && currentEdge.ToNode != "" {
				if len(currentEdge.Label) > MaxLabelLength {
					currentEdge.Label = currentEdge.Label[:MaxLabelLength]
				}
				d.Edges = append(d.Edges, *currentEdge)
			}
			currentEdge = nil
		}
	}

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check section headers
		if trimmed == "nodes:" {
			flushNode()
			flushEdge()
			section = "nodes"
			continue
		} else if trimmed == "edges:" {
			flushNode()
			flushEdge()
			section = "edges"
			continue
		}

		if section == "" {
			if strings.HasPrefix(trimmed, "version:") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
				if v, err := strconv.Atoi(val); err == nil {
					d.Version = v
				}
			} else if strings.HasPrefix(trimmed, "flow:") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "flow:"))
				switch strings.ToLower(val) {
				case "top-to-bottom", "tb", "vertical":
					d.Flow = FlowTopToBottom
				case "right-to-left", "rl":
					d.Flow = FlowRightToLeft
				case "bottom-to-top", "bt":
					d.Flow = FlowBottomToTop
				default:
					d.Flow = FlowLeftToRight
				}
			}
			continue
		}

		if section == "nodes" {
			if strings.HasPrefix(trimmed, "- ") {
				flushNode()
				currentNode = &DiagramNode{}
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			}
			if currentNode == nil {
				continue
			}

			if strings.HasPrefix(trimmed, "id:") {
				currentNode.ID = cleanScalar(strings.TrimPrefix(trimmed, "id:"))
			} else if strings.HasPrefix(trimmed, "title:") {
				currentNode.Title = cleanScalar(strings.TrimPrefix(trimmed, "title:"))
			} else if strings.HasPrefix(trimmed, "subtitle:") {
				currentNode.Subtitle = cleanScalar(strings.TrimPrefix(trimmed, "subtitle:"))
			} else if strings.HasPrefix(trimmed, "inputs:") {
				currentNode.Inputs = parseList(strings.TrimPrefix(trimmed, "inputs:"))
			} else if strings.HasPrefix(trimmed, "outputs:") {
				currentNode.Outputs = parseList(strings.TrimPrefix(trimmed, "outputs:"))
			}
		}

		if section == "edges" {
			if strings.HasPrefix(trimmed, "- ") {
				flushEdge()
				currentEdge = &DiagramEdge{}
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			}
			if currentEdge == nil {
				continue
			}

			if strings.HasPrefix(trimmed, "from:") {
				target := cleanScalar(strings.TrimPrefix(trimmed, "from:"))
				currentEdge.FromNode, currentEdge.FromPort = splitNodePort(target)
			} else if strings.HasPrefix(trimmed, "to:") {
				target := cleanScalar(strings.TrimPrefix(trimmed, "to:"))
				currentEdge.ToNode, currentEdge.ToPort = splitNodePort(target)
			} else if strings.HasPrefix(trimmed, "label:") {
				currentEdge.Label = cleanScalar(strings.TrimPrefix(trimmed, "label:"))
			}
		}
	}

	flushNode()
	flushEdge()

	if err := ValidateLimits(source, len(d.Nodes), len(d.Edges)); err != nil {
		return nil, err
	}

	return d, nil
}

func cleanScalar(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

func parseList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		cleaned := cleanScalar(p)
		if cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func splitNodePort(s string) (string, string) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 {
		return cleanScalar(parts[0]), cleanScalar(parts[1])
	}
	return cleanScalar(s), "default"
}
