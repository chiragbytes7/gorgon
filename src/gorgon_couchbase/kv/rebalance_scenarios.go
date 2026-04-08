package kv

import (
	"errors"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
)

type rebalanceGenerator struct {
	db         *database
	addNode    string
	removeNode string
	mode       string
	apiNode    string   // Node to send the REST api requests
	nodes      []string // Set of nodes in the cluster in current rebalance scenario
	invokeTime time.Time
	done       bool
}

type RebalanceInInstruction struct {
	addNode string
}

func (instr *RebalanceInInstruction) String() string {
	return "RebalanceIn(" + instr.addNode + ")"
}

func (instr *RebalanceInInstruction) ForSelf() bool {
	return true
}

type RebalanceOutInstruction struct {
	removeNode string
}

func (instr *RebalanceOutInstruction) String() string {
	return "RebalanceOut(" + instr.removeNode + ")"
}

func (instr *RebalanceOutInstruction) ForSelf() bool {
	return true
}

type SwapRebalanceInstruction struct {
	addNode    string
	removeNode string
}

func (instr *SwapRebalanceInstruction) String() string {
	return "SwapRebalance(" + instr.addNode + ", " + instr.removeNode + ")"
}

func (instr *SwapRebalanceInstruction) ForSelf() bool {
	return true
}

func NewRebalanceGenerator(db *database, addNode, removeNode string) gorgon.Generator {
	var mode string

	if addNode != "" && removeNode != "" {
		mode = "swap"
	} else if addNode != "" {
		mode = "rebalance-in"
	} else {
		mode = "rebalance-out"
	}

	return &rebalanceGenerator{
		db:         db,
		addNode:    addNode,
		removeNode: removeNode,
		mode:       mode,
	}
}

func (rebalance *rebalanceGenerator) SetUp(opt *gorgon.Options) error {
	rebalance.invokeTime = time.Now().Add(30 * time.Second)
	rebalance.nodes = make([]string, len(rebalance.db.options.Nodes))
	copy(rebalance.nodes, rebalance.db.options.Nodes)
	// If rebalance-in configuration
	if rebalance.mode == "rebalance-in" {
		rebalance.apiNode = rebalance.db.options.Nodes[0]
		return nil
	}
	// if rebalance-out or swap rebalance
	for _, node := range rebalance.db.options.Nodes {
		if node != rebalance.removeNode {
			rebalance.apiNode = node
			return nil
		}
	}
	return errors.New("apiNode not found")
}

func (rebalance *rebalanceGenerator) Next(client int) (gorgon.Instruction, error) {
	if client >= 0 {
		return nil, nil
	}
	if rebalance.done || time.Until(rebalance.invokeTime) > 0 {
		return nil, nil
	}
	rebalance.done = true
	switch rebalance.mode {
	case "swap":
		return &SwapRebalanceInstruction{
			addNode:    rebalance.addNode,
			removeNode: rebalance.removeNode,
		}, nil
	case "rebalance-in":
		return &RebalanceInInstruction{
			addNode: rebalance.addNode,
		}, nil
	case "rebalance-out":
		return &RebalanceOutInstruction{
			removeNode: rebalance.removeNode,
		}, nil
	default:
		return nil, errors.New("Rebalance operation not of the supported type")
	}
}

func (rebalance *rebalanceGenerator) Invoke(instruction gorgon.Instruction, getTime func() int64) (int64, gorgon.Output) {
	var ejected []string
	var err error

	switch instr := instruction.(type) {
	case *RebalanceInInstruction:
		err = rebalance.db.requestAddNode(rebalance.apiNode, instr.addNode)
		if err != nil {
			return getTime(), err
		}
		rebalance.nodes = append(rebalance.nodes, instr.addNode)
		ejected = []string{}
	case *RebalanceOutInstruction:
		ejected = []string{instr.removeNode}
	case *SwapRebalanceInstruction:
		err = rebalance.db.requestAddNode(rebalance.apiNode, instr.addNode)
		if err != nil {
			return getTime(), err
		}
		rebalance.nodes = append(rebalance.nodes, instr.addNode)
		ejected = []string{instr.removeNode}
	default:
		return -1, gorgon.ErrUnsupportedInstruction
	}
	err = rebalance.db.rebalance(rebalance.apiNode, rebalance.nodes, ejected)
	if err != nil {
		return getTime(), err
	}
	return getTime(), rebalance.db.waitForRebalance(rebalance.apiNode)
}

func (rebalance *rebalanceGenerator) Name() string {
	switch rebalance.mode {
	case "swap":
		return "SwapRebalance"
	case "rebalance-in":
		return "RebalanceIn"
	case "rebalance-out":
		return "RebalanceOut"
	default:
		return "unknown-" + rebalance.mode
	}
}

func (rebalance *rebalanceGenerator) OnCall(client int, instruction gorgon.Instruction) error {
	return nil
}

func (rebalance *rebalanceGenerator) OnReturn(client int, instruction gorgon.Instruction, output gorgon.Output) error {
	return nil
}

func (rebalance *rebalanceGenerator) TearDown() error {
	return nil
}
