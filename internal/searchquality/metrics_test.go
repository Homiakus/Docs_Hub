package searchquality

import (
	"math"
	"testing"
)

func TestRelevanceMetrics(t *testing.T) {
	results := [][]string{
		{"a", "b", "c"},
		{"x", "y", "z"},
	}
	relevant := []map[string]struct{}{
		{"b": {}, "c": {}},
		{"x": {}},
	}
	grades := []map[string]int{
		{"b": 3, "c": 1},
		{"x": 2},
	}

	if got, want := MRRAtK(results, relevant, 3), 0.75; math.Abs(got-want) > 1e-9 {
		t.Fatalf("MRR@3=%f want %f", got, want)
	}
	if got, want := RecallAtK(results, relevant, 2), 0.75; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Recall@2=%f want %f", got, want)
	}
	if got := NDCGAtK(results, grades, 3); got <= 0 || got > 1 {
		t.Fatalf("nDCG@3=%f outside (0,1]", got)
	}
}

func TestMetricsRejectMismatchedOrEmptyInputs(t *testing.T) {
	if got := MRRAtK(nil, nil, 10); got != 0 { t.Fatalf("empty MRR=%f", got) }
	if got := RecallAtK([][]string{{"a"}}, nil, 10); got != 0 { t.Fatalf("mismatch recall=%f", got) }
	if got := NDCGAtK([][]string{{"a"}}, []map[string]int{{"a": 1}}, 0); got != 0 { t.Fatalf("k=0 ndcg=%f", got) }
}

func TestRecallIgnoresDuplicateResultIDs(t *testing.T) {
	results := [][]string{{"a", "a", "b"}}
	relevant := []map[string]struct{}{{"a": {}, "b": {}}}
	if got, want := RecallAtK(results, relevant, 3), 1.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Recall@3=%f want %f", got, want)
	}
}
