package checkers

import (
	"math/rand"
	"testing"
)

func TestCache(t *testing.T) {
	cache := NewCache(15, func(v any) uint64 {
		return uint64(v.(int)) / 3
	}, func(a, b any) bool {
		return a.(int) == b.(int)
	})

	m := make(map[int]int)

	for i := 0; i < 1000; i++ {
		x := rand.Intn(50)
		if _, ok := m[x]; ok {
			if !cache.Contains(x) {
				t.Errorf("Expected cache to contain %d", x)
			}
			if cache.Insert(x) {
				t.Errorf("Expected cache to not insert %d", x)
			}
		} else {
			if cache.Contains(x) {
				t.Errorf("Expected cache to not contain %d", x)
			}
			if !cache.Insert(x) {
				t.Errorf("Expected cache to insert %d", x)
			}
		}
		m[x] = i
		if len(m) > 15 {
			oldest := -1
			for k := range m {
				if oldest == -1 || m[k] < m[oldest] {
					oldest = k
				}
			}
			delete(m, oldest)
		}
		if cache.Len() != len(m) {
			t.Errorf("Expected cache length to be %d, got %d", len(m), cache.Len())
		}
	}
}
