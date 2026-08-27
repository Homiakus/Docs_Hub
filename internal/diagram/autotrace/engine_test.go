package autotrace

import (
	"context"
	"strings"
	"testing"
)

func TestAutoTraceEngineRender(t *testing.T) {
	engine := NewEngine()
	ctx := context.Background()

	validDSL := `
version: 1
flow: left-to-right
nodes:
  - id: controller
    title: Controller
    outputs: [modbus]
  - id: selector
    title: Selector
    inputs: [modbus]
    outputs: [motor]
  - id: motor
    title: Motor
    inputs: [motor]
edges:
  - from: controller.modbus
    to: selector.modbus
    label: RS-485
  - from: selector.motor
    to: motor.motor
    label: STEP/DIR
`

	res, err := engine.Render(ctx, validDSL, RenderOptions{})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if res.NodesNum != 3 {
		t.Errorf("expected 3 nodes, got %d", res.NodesNum)
	}
	if res.EdgesNum != 2 {
		t.Errorf("expected 2 edges, got %d", res.EdgesNum)
	}
	if !strings.Contains(res.SVG, "<svg") || !strings.Contains(res.SVG, "Controller") || !strings.Contains(res.SVG, "RS-485") {
		t.Errorf("svg missing expected elements: %s", res.SVG)
	}

	// Test invalid YAML error rendering
	invalidDSL := `
nodes:
  - id: 123
`
	errRes, _ := engine.Render(ctx, invalidDSL, RenderOptions{})
	if !strings.Contains(errRes.SVG, "<svg") {
		t.Errorf("expected error SVG on invalid input")
	}
}

func TestLimitsValidation(t *testing.T) {
	if err := ValidateLimits("short source", 10, 20); err != nil {
		t.Errorf("unexpected error on valid limits: %v", err)
	}

	largeSource := strings.Repeat("a", MaxSourceSizeBytes+1)
	if err := ValidateLimits(largeSource, 1, 1); err != ErrSourceTooLarge {
		t.Errorf("expected ErrSourceTooLarge, got %v", err)
	}

	if err := ValidateLimits("source", MaxNodeCount+1, 1); err != ErrTooManyNodes {
		t.Errorf("expected ErrTooManyNodes, got %v", err)
	}

	if err := ValidateLimits("source", 1, MaxEdgeCount+1); err != ErrTooManyEdges {
		t.Errorf("expected ErrTooManyEdges, got %v", err)
	}
}
