package workloads

import (
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/generators"
)

func GetSetWorkload() gorgon.Workload {
	keys := []string{"key0", "key1", "key2", "key3", "key4", "key5", "key6", "key7"}

	// Use staggered timing to simulate realistic concurrent access patterns and expose race conditions
	return gorgon.Workload{
		Model:      GetSetModel(),
		Generators: []gorgon.Generator{generators.Stagger(generators.NewGetSetGenerator(keys), time.Millisecond)},
	}
}

func GetSetModel() gorgon.Model {
	return gorgon.Model{
		Init: func() []gorgon.State { return []gorgon.State{gorgon.IntMap{}} },
		Hash: func(state gorgon.State) uint64 {
			return state.(gorgon.IntMap).Hash()
		},
		Equal: func(s1, s2 gorgon.State) bool {
			return s1.(gorgon.IntMap).Equals(s2.(gorgon.IntMap))
		},
		DescribeState: func(state gorgon.State) string {
			return state.(gorgon.IntMap).String()
		},
		DescribeOperation: DescribeOperation,
		Partition:         PartitionByKey,
		Step: func(state gorgon.State, input gorgon.Instruction, output gorgon.Output) []gorgon.State {
			stateMap := state.(gorgon.IntMap)
			switch instr := input.(type) {
			case *generators.GetInstruction:
				// Errors are no-ops from the model's perspective
				if _, ok := output.(error); ok {
					return []gorgon.State{state}
				}
				// Get must return the current stored value
				if val, ok := stateMap.Get(instr.Key); ok {
					if i, ok := output.(int); ok && val == i {
						return []gorgon.State{state}
					}
					return nil // Return value doesn't match state
				}
				// Get on non-existent key must return nil
				if output == nil {
					return []gorgon.State{state}
				}
				return nil // Got a value for a non-existent key
			case *generators.SetInstruction:
				stateMap = stateMap.Put(instr.Key, instr.Value)
				if output != nil {
					// Set may succeed or fail (error is non-deterministic)
					if _, ok := output.(error); ok {
						return []gorgon.State{state, stateMap}
					}
					return nil
				}
				return []gorgon.State{stateMap}
			}
			return nil
		},
		Values: func(input gorgon.Instruction, output gorgon.Output) (reads []gorgon.KeyValueInt, writes []gorgon.KeyValueInt) {
			switch instr := input.(type) {
			case *generators.GetInstruction:
				// Get reads the key's value
				if i, ok := output.(int); ok {
					return []gorgon.KeyValueInt{{Key: instr.Key, Value: i}}, nil
				}
			case *generators.SetInstruction:
				// Set writes the key's value (track only successful writes)
				if _, ok := output.(error); !ok {
					return nil, []gorgon.KeyValueInt{{Key: instr.Key, Value: instr.Value}}
				}
			}
			return nil, nil
		},
		// Return values that were overwritten by this operation; used for read-write constraint checking
		ValuesOvewritten: func(state gorgon.State, input gorgon.Instruction, output gorgon.Output) []gorgon.KeyValueInt {
			if _, ok := output.(error); ok {
				return nil
			}
			// Successful Set overwrites the previous value (if any)
			if instr, ok := input.(*generators.SetInstruction); ok {
				if i, ok := state.(gorgon.IntMap).Get(instr.Key); ok {
					return []gorgon.KeyValueInt{{Key: instr.Key, Value: i}}
				}
			}
			return nil
		},
	}
}
