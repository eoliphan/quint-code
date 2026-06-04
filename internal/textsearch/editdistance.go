package textsearch

// BoundedEditDistance returns the Levenshtein distance between a and b, but
// stops and returns maxDist+1 as soon as the distance is known to exceed
// maxDist. The early exit makes a fuzzy fallback cheap even when scanned over
// tens of thousands of symbol names. Pure DP, O(min(len a, len b)) memory.
//
// Callers compare case-folded inputs (pass lower-cased names); this function
// does not fold case itself. Ported from codegraph boundedEditDistance — the
// typo-tolerant tier the symbol store currently lacks (it only does substring).
func BoundedEditDistance(a, b string, maxDist int) int {
	if a == b {
		return 0
	}
	al, bl := len(a), len(b)
	if abs(al-bl) > maxDist {
		return maxDist + 1
	}
	if al == 0 {
		return bl
	}
	if bl == 0 {
		return al
	}

	prev := make([]int, bl+1)
	cur := make([]int, bl+1)
	for j := 0; j <= bl; j++ {
		prev[j] = j
	}

	for i := 1; i <= al; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= bl; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if cur[j] < rowMin {
				rowMin = cur[j]
			}
		}
		if rowMin > maxDist {
			return maxDist + 1
		}
		prev, cur = cur, prev
	}
	return prev[bl]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min3(a, b, c int) int {
	return min(a, min(b, c))
}
