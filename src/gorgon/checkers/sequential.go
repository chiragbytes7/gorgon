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
	type stackFrame struct {
		progress []int
		state    gorgon.State
		threads  []int
		seqLen   int
		checked  int
		mutable  bool
	}
	type cacheItem struct {
		progress []int
		state    gorgon.State
	}
	cache := NewCache(
		8_000_000,
		func(a any) uint64 {
			item := a.(*cacheItem)
			h := m.Hash(item.state)
			for _, p := range item.progress {
				h = h*0x100000001b3 + uint64(p)
			}
			return h
		},
		func(a, b any) bool {
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
	numOps := 0
	for _, ops := range history {
		numOps += len(ops)
	}
	constraints := findReadWriteConstraints(history, m.Values)
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
		if frame.checked >= len(history) {
			frame = nil
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				break
			}
			sequence = sequence[:stack[len(stack)-1].seqLen]
			continue
		}
		var nextProgress []int
		var nextState gorgon.State
		lastThread := -1
		if frame.mutable {
			thread := frame.threads[frame.checked]
			frame.checked++
			idx := frame.progress[thread]
			if idx >= len(history[thread]) {
				continue
			}
			op := history[thread][idx]
			nextStates := m.Step(frame.state, op.Input, op.Output)
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
			nextState = frame.state
		}
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
			sequence = sequence[:frame.seqLen]
		}
	}
	cache.Clear()
	return Illegal, result
}

func sortedThreads(history [][]gorgon.Operation, progress []int, index int) []int {
	threads := make([]int, len(history))
	for i := range threads {
		index++
		if index >= len(threads) {
			index = 0
		}
		threads[i] = index
	}
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
