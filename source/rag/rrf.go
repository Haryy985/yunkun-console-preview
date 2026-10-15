package rag

import "sort"

// Ranked is one ranked list item used by reciprocal-rank fusion.
type Ranked[T any] struct {
	ID    int64
	Value T
	Rank  int
	Score float64
}

// FuseRRF merges ranked lists without comparing scores from different systems.
// This is intentionally score-agnostic: BM25/keyword and cosine scores have
// different scales, while rank is stable and easy to explain in retrieval logs.
func FuseRRF[T any](lists ...[]Ranked[T]) []Ranked[T] {
	const rrfK = 60.0
	type fused struct {
		item  Ranked[T]
		score float64
	}
	byID := make(map[int64]*fused)
	for _, list := range lists {
		for rank, item := range list {
			if item.ID == 0 {
				continue
			}
			position := rank + 1
			entry, ok := byID[item.ID]
			if !ok {
				item.Rank = position
				entry = &fused{item: item}
				byID[item.ID] = entry
			}
			entry.score += 1 / (rrfK + float64(position))
			if item.Score > entry.item.Score {
				entry.item = item
			}
		}
	}
	out := make([]Ranked[T], 0, len(byID))
	for _, entry := range byID {
		entry.item.Score = entry.score
		out = append(out, entry.item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}
