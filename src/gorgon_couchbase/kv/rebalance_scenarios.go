package kv

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/jrpc"
	"github.com/couchbaselabs/gorgon/src/gorgon/rpcs"
)

type rebalanceGenerator struct {
	db           *database
	addNode      string
	removeNode   string
	mode         string
	apiNode      string   // Node to send the REST API requests
	nodes        []string // Set of nodes in the cluster in current rebalance scenario
	invokeTime   time.Time
	done         bool
	crashLater   bool   // Only set to true when a crash process is specified
	process      string // Only set when rebalance is followed by a process kill
	swapKillNode string // Only set in swap rebalance to specify the node to kill
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

func NewRebalanceGenerator(db *database, addNode, removeNode string, args ...string) gorgon.Generator {
	var mode string

	if addNode != "" && removeNode != "" {
		mode = "swap"
	} else if addNode != "" {
		mode = "rebalance-in"
	} else {
		mode = "rebalance-out"
	}

	crashLater := len(args) > 0 // Set to true when process is provided to function

	var processName string
	if len(args) > 0 {
		processName = args[0]
	}

	var swapKillNode string // node to kill in swap-rebalance
	if mode == "swap" && len(args) > 1 {
		swapKillNode = args[1]
	}

	return &rebalanceGenerator{
		db:           db,
		addNode:      addNode,
		removeNode:   removeNode,
		mode:         mode,
		process:      processName,
		crashLater:   crashLater,
		swapKillNode: swapKillNode,
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

func (rebalance *rebalanceGenerator) killProcess(node string) error {
	client, err := jrpc.Dial(fmt.Sprintf("%s:%d", node, rebalance.db.options.RpcPort), []byte(rebalance.db.options.RpcPassword))
	if err != nil {
		return err
	}
	defer client.Close()
	var reply string
	return client.Call("KillRpc.Pkill", &rpcs.KillInstruction{Process: rebalance.process, Signal: 9}, &reply)
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
	time.Sleep(3 * time.Second) // rebalance takes a while before starting
	if rebalance.crashLater {
		var targetNode string
		if rebalance.process == "beam.smp" {
			targetNode, err = rebalance.db.findOrchestrator(rebalance.apiNode)
			if err != nil {
				return getTime(), err
			}
			targetNode = strings.TrimPrefix(targetNode, "ns_1@")
		} else {
			switch rebalance.mode {
			case "rebalance-out":
				targetNode = rebalance.removeNode
			case "rebalance-in":
				targetNode = rebalance.addNode
			case "swap":
				targetNode = rebalance.swapKillNode
				if targetNode == "" { // if no swap kill node was specified
					targetNode = rebalance.addNode
				}
			default:
				return getTime(), errors.New("unexpected rebalance mode: " + rebalance.mode)
			}
		}
		if err := rebalance.killProcess(targetNode); err != nil {
			return getTime(), err
		}
		return getTime(), nil // if kill is successful, rebalance stops and rebalance-polling can be skipped
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
