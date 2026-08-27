package autotrace

import (
	"errors"
	"time"
)

var (
	ErrSourceTooLarge   = errors.New("autotrace: source exceeds maximum allowed size (64KB)")
	ErrTooManyNodes     = errors.New("autotrace: diagram exceeds maximum allowed nodes (256)")
	ErrTooManyEdges     = errors.New("autotrace: diagram exceeds maximum allowed edges (512)")
	ErrLabelTooLong     = errors.New("autotrace: node/edge label exceeds 128 characters")
	ErrExecutionTimeout = errors.New("autotrace: diagram layout/routing exceeded timeout budget")
)

const (
	MaxSourceSizeBytes = 64 * 1024
	MaxNodeCount       = 256
	MaxEdgeCount       = 512
	MaxLabelLength     = 128
	DefaultTimeout     = 5 * time.Second
)

func ValidateLimits(source string, nodeCount, edgeCount int) error {
	if len(source) > MaxSourceSizeBytes {
		return ErrSourceTooLarge
	}
	if nodeCount > MaxNodeCount {
		return ErrTooManyNodes
	}
	if edgeCount > MaxEdgeCount {
		return ErrTooManyEdges
	}
	return nil
}
