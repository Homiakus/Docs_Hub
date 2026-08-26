package searchquality

import "math"

// MRRAtK computes mean reciprocal rank for one expected relevant set per
// query. A query contributes 0 when no relevant document appears in the first
// k results.
func MRRAtK(results [][]string, relevant []map[string]struct{}, k int) float64 {
	if len(results) == 0 || len(results) != len(relevant) || k <= 0 {
		return 0
	}
	var total float64
	for i, ranked := range results {
		limit := min(k, len(ranked))
		for rank := 0; rank < limit; rank++ {
			if _, ok := relevant[i][ranked[rank]]; ok {
				total += 1 / float64(rank+1)
				break
			}
	}
	return total / float64(len(results))
}

// RecallAtK computes macro-average recall across queries.
func RecallAtK(results [][]string, relevant []map[string]struct{}, k int) float64 {
	if len(results) == 0 || len(results) != len(relevant) || k <= 0 {
		return 0
	}
	var total float64
	for i, ranked := range results {
		if len(relevant[i]) == 0 {
			continue
		}
		seen := make(map[string]struct{}, min(k, len(ranked)))
		hits := 0
		for rank := 0; rank < min(k, len(ranked)); rank++ {
			id := ranked[rank]
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			if _, ok := relevant[i][id]; ok {
				hits++
			}
		}
		total += float64(hits) / float64(len(relevant[i]))
	}
	return total / float64(len(results))
}

// NDCGAtK computes macro-average normalized discounted cumulative gain using
// integer relevance grades. Missing documents have grade 0.
func NDCGAtK(results [][]string, grades []map[string]int, k int) float64 {
	if len(results) == 0 || len(results) != len(grades) || k <= 0 {
		return 0
	}
	var total float64
	for i, ranked := range results {
		dcg := 0.0
		for rank := 0; rank < min(k, len(ranked)); rank++ {
			grade := grades[i][ranked[rank]]
			if grade > 0 {
				dcg += (math.Pow(2, float64(grade)) - 1) / math.Log2(float64(rank)+2)
			}
		}
		ideal := make([]int, 0, len(grades[i]))
		for _, grade := range grades[i] {
			if grade > 0 {
				ideal = append(ideal, grade)
			}
		}
		for a := 0; a < len(ideal); a++ {
			for b := a + 1; b < len(ideal); b++ {
				if ideal[b] > ideal[a] {
					ideal[a], ideal[b] = ideal[b], ideal[a]
				}
			}
		}
		idcg := 0.0
		for rank := 0; rank < min(k, len(ideal)); rank++ {
			idcg += (math.Pow(2, float64(ideal[rank])) - 1) / math.Log2(float64(rank)+2)
		}
		if idcg > 0 {
			total += dcg / idcg
		}
	}
	return total / float64(len(results))
}
