package checkers_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/checkers"
	"github.com/couchbaselabs/gorgon/src/gorgon/generators"
	"github.com/couchbaselabs/gorgon/src/gorgon/workloads"
)

func TestSequential(t *testing.T) {
	keys := []string{"a", "b", "c", "d"}
	history := make([][]gorgon.Operation, 8)
	state := make(map[string]int)
	for i := 0; i < 1600; i++ {
		var op gorgon.Operation
		t := rand.Intn(len(history))
		if rand.Intn(2) == 0 {
			key := keys[rand.Intn(len(keys))]
			state[key] = i + 1
			op = gorgon.Operation{Input: &generators.SetInstruction{Key: key, Value: i + 1}}
		} else {
			key := keys[rand.Intn(len(keys))]
			if i, ok := state[key]; ok {
				op = gorgon.Operation{Input: &generators.GetInstruction{Key: key}, Output: i}
			} else {
				op = gorgon.Operation{Input: &generators.GetInstruction{Key: key}}
			}
		}
		history[t] = append(history[t], op)
	}
	m := workloads.GetSetModel()
	result, seq := checkers.CheckSeqnuentialConsistency(m, history, time.Minute)
	if result != checkers.Ok {
		t.Errorf("Expected history to be sequentially consistent, got inconsistent with sequence %v", seq)
	}
}
