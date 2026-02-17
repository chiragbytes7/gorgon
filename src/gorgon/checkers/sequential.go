package checkers

import (
	"sort"
	"sync/atomic"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/log"
)

type CheckResult string

const (
	Unknown CheckResult = "Unknown" // timed out
	Ok      CheckResult = "Ok"
	Illegal CheckResult = "Illegal"
)

func CheckSeqnuentialConsistency(m gorgon.Model, history [][]gorgon.Operation, timeout time.Duration) (result CheckResult, info []int) {
	if timeout <= 0 {
		return checkSeqnuentialConsistency(m, history, nil)
	}
	stop := &atomic.Bool{}
	resultChan := make(chan bool)
	timeoutChan := time.After(timeout)
	go func() {
		result, info = checkSeqnuentialConsistency(m, history, stop)
		resultChan <- true
	}()
	for {
		select {
		case <-resultChan:
			return result, info
		case <-timeoutChan:
			stop.Store(true)
		}
	}
}

func checkSeqnuentialConsistency(m gorgon.Model, history [][]gorgon.Operation, stop *atomic.Bool) (CheckResult, []int) {
	// Stack frame for DFS exploration; heap allocated to avoid recursion depth limits
	type stackFrame struct {
		progress []int // Tracks how many operations each thread has consumed
		state    gorgon.State
		threads  []int
		seqLen   int  // Length of the current valid sequence
		checked  int  // How many threads we've examined in this frame
		mutable  bool // Whether we're exploring state mutations or no-op batching
	}

	// Cached state to prune duplicate search branches and improve performance
	type cacheItem struct {
		progress []int
		state    gorgon.State
	}

	// Memoize visited (state, progress) pairs to avoid exploring the same configuration multiple times
	cache := NewCache(
		8_000_000, // Large cache to accommodate realistic histories
		func(a any) uint64 { // Combine state hash with progress for collision-resistant cache key
			item := a.(*cacheItem)
			h := m.Hash(item.state)
			for _, p := range item.progress {
				h = h*0x100000001b3 + uint64(p)
			}
			return h
		},
		func(a, b any) bool { // Two cache entries are identical if both state and progress match
			ia := a.(*cacheItem)
			ib := b.(*cacheItem)
			if len(ia.progress) != len(ib.progress) {
				return false
			}
			for i := range ia.progress {
				if ia.progress[i] != ib.progress[i] {
					return false
				}
			}
			return m.Equal(ia.state, ib.state)
		})

	// Track total operations to detect when we've found a complete sequence
	numOps := 0
	for _, ops := range history {
		numOps += len(ops)
	}

	// Pre-compute read-write constraints to enable efficient pruning of invalid branches
	constraints := findReadWriteConstraints(history, m.Values)

	// Initialize DFS with empty progress and initial model state
	stack := []stackFrame{{
		progress: make([]int, len(history)),
		state:    m.Init()[0]}}
	stack[0].threads = sortedThreads(history, stack[0].progress, -1)
	var result []int
	var sequence []int

loop:
	for {
		if stop != nil && stop.Load() {
			cache.Clear()
			return Unknown, result
		}
		frame := &stack[len(stack)-1]

		// All threads exhausted in this frame; backtrack to try alternative paths
		if frame.checked >= len(history) {
			frame = nil
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				break
			}

			// Restore sequence depth to parent frame's position
			sequence = sequence[:stack[len(stack)-1].seqLen]
			continue
		}

		var nextProgress []int
		var nextState gorgon.State
		lastThread := -1

		// Try adding the next state-mutating operation from each thread
		if frame.mutable {
			thread := frame.threads[frame.checked]
			frame.checked++

			idx := frame.progress[thread]
			// Skip threads that have no remaining operations
			if idx >= len(history[thread]) {
				continue
			}
			op := history[thread][idx]
			nextStates := m.Step(frame.state, op.Input, op.Output)
			// Deterministic transitions only
			if len(nextStates) != 1 {
				continue
			}
			if m.Equal(frame.state, nextStates[0]) {
				continue
			}
			nextProgress = make([]int, len(frame.progress))
			copy(nextProgress, frame.progress)
			nextProgress[thread]++
			if m.ValuesOvewritten != nil {
				changed := m.ValuesOvewritten(frame.state, op.Input, op.Output)
				for _, c := range changed {
					for t, constr := range constraints[c] {
						if constr.lastRead >= nextProgress[t] {
							continue loop
						}
					}
				}
			}
			if m.Values != nil {
				_, writes := m.Values(op.Input, op.Output)
				for _, w := range writes {
					for t, constr := range constraints[w] {
						if constr.lastRead > nextProgress[t] && constr.prevWrite >= nextProgress[t] {
							continue loop
						}
					}
				}
			}
			lastThread = thread
			nextState = nextStates[0]
			sequence = append(sequence, thread)
		} else {
			// Batch all non-mutating operations (e.g. reads)
			frame.mutable = true
			nextProgress = make([]int, len(frame.progress))
			copy(nextProgress, frame.progress)
			for thread := range history {
				for {
					idx := nextProgress[thread]
					if idx >= len(history[thread]) {
						break
					}
					op := history[thread][idx]
					nextStates := m.Step(frame.state, op.Input, op.Output)
					if len(nextStates) != 1 {
						break
					}
					if !m.Equal(frame.state, nextStates[0]) {
						break
					}
					lastThread = thread
					nextProgress[thread]++
					sequence = append(sequence, thread)
				}
			}
			if lastThread < 0 {
				continue
			}
			nextState = frame.state // No state change when processing no-ops
		}

		// Found a valid sequence covering all operations
		if len(sequence) == numOps {
			cache.Clear()
			return Ok, sequence
		}

		item := &cacheItem{nextProgress, nextState}
		prevLen := cache.Len()

		if cache.Insert(item) {
			if prevLen != cache.Len() && cache.Len()%1_000_000 == 0 {
				log.Info("Sequential consistency cache size: %d", cache.Len())
			}
			if len(sequence) > len(result) {
				result = make([]int, len(sequence))
				copy(result, sequence)
			}
			frame = nil
			stack = append(stack, stackFrame{
				progress: nextProgress,
				state:    nextState,
				threads:  sortedThreads(history, nextProgress, lastThread),
				seqLen:   len(sequence)})
		} else {
			// State already visited; skip this branch to avoid redundant work
			sequence = sequence[:frame.seqLen]
		}
	}

	// Exhausted all branches without finding a valid sequential ordering
	cache.Clear()
	return Illegal, result
}

// Sort threads by their next operation's call time; prioritizes operations that happened earlier in real time
func sortedThreads(history [][]gorgon.Operation, progress []int, index int) []int {
	threads := make([]int, len(history))
	// Rotate starting position to explore different interleavings
	for i := range threads {
		index++
		if index >= len(threads) {
			index = 0
		}
		threads[i] = index
	}

	// Sort by call order: operations that happened earlier in real time are tried first
	sort.Slice(threads, func(i, j int) bool {
		i = threads[i]
		j = threads[j]
		pi := progress[i]
		pj := progress[j]
		if pi >= len(history[i]) {
			return false
		}
		if pj >= len(history[j]) {
			return true
		}
		return history[i][pi].Call < history[j][pj].Call
	})
	return threads
}

type rwConstraint struct {
	lastRead  int
	prevWrite int
}

// Build constraint map: for each (key, value), track the last read and previous write in each thread
// This enables early pruning of branches where reads would violate serialization order
func findReadWriteConstraints(
	history [][]gorgon.Operation,
	values func(input gorgon.Instruction, output gorgon.Output) ([]gorgon.KeyValueInt, []gorgon.KeyValueInt),
) map[gorgon.KeyValueInt][]rwConstraint {

	constraints := make(map[gorgon.KeyValueInt][]rwConstraint)
	if values == nil {
		return constraints
	}

	for t, ops := range history {
		lastWrites := make(map[string]int)
		for i, op := range ops {
			reads, writes := values(op.Input, op.Output)
			for _, r := range reads {
				if _, ok := constraints[r]; !ok {
					v := make([]rwConstraint, len(history))
					for j := range v {
						v[j] = rwConstraint{-1, -1}
					}
					constraints[r] = v
				}
				prevWrite := -1
				if w, ok := lastWrites[r.Key]; ok {
					prevWrite = w
				}
				constraints[r][t] = rwConstraint{i, prevWrite}
			}
			for _, w := range writes {
				lastWrites[w.Key] = i
			}
		}
	}
	return constraints
}
