package cmd

import (
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/checkers"
	"github.com/couchbaselabs/gorgon/src/gorgon/generators"
)

func CheckSeqnuentialConsistency(m gorgon.Model, history [][]gorgon.Operation, timeout time.Duration) (checkers.CheckResult, []int) {
	valuesRead := make(map[gorgon.KeyValueInt]bool)
	for _, ops := range history {
		for _, op := range ops {
			reads, _ := m.Values(op.Input, op.Output)
			for _, r := range reads {
				valuesRead[r] = true
			}
		}
	}
	for _, ops := range history {
		for i := range ops {
			if _, ok := ops[i].Output.(error); !ok {
				continue
			}
			if set, ok := ops[i].Input.(*generators.SetInstruction); ok {
				if valuesRead[gorgon.KeyValueInt{Key: set.Key, Value: set.Value}] {
					ops[i].Output = nil
				}
			}
		}
	}
	step := m.Step
	m.Step = func(state gorgon.State, input gorgon.Instruction, output gorgon.Output) []gorgon.State {
		states := step(state, input, output)
		if len(states) == 0 {
			return nil
		}
		return states[:1]
	}
	return checkers.CheckSeqnuentialConsistency(m, history, timeout)
}
