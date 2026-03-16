package nemeses

import (
	"fmt"
	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/jrpc"
	"github.com/couchbaselabs/gorgon/src/gorgon/log"
	"github.com/couchbaselabs/gorgon/src/gorgon/rpcs"
	"math"
	"math/rand"
	"net/rpc"
	"time"
)

type outOfStorageNemesis struct {
	faultyNodes []string
	clients     []*rpc.Client
	faultTime   time.Time // time in seconds after start to run the nemesis.
	fixTime     time.Time
	toggled     bool
	done        bool
}

func NewOutOfStorageNemesis() *outOfStorageNemesis {
	return &outOfStorageNemesis{}
}

func (nemesis *outOfStorageNemesis) SetUp(opt *gorgon.Options) error {
	num := len(opt.Nodes)
	// Faulting majority of the nodes via the formula floor(N / 2) + 1
	faulty_node_count := math.Floor((float64(num) / 2)) + 1

	// Copy of the opt.Nodes slice to avoid corrupting it
	nodes_copy := make([]string, len(opt.Nodes))
	copy(nodes_copy, opt.Nodes)

	// Partial Fisher yates algorithm to create a random faultyNodes slice
	k := int(faulty_node_count)
	for i := 0; i < k; i++ {
		j := i + rand.Intn(num-i)
		nodes_copy[i], nodes_copy[j] = nodes_copy[j], nodes_copy[i]
	}
	nemesis.faultyNodes = nodes_copy[:int(faulty_node_count)]

	// Defer func to cleanup in case of a setup-failure
	defer func() {
		if len(nemesis.clients) != len(nemesis.faultyNodes) {
			for _, client := range nemesis.clients {
				client.Close()
			}
		}
	}()

	for _, node := range nemesis.faultyNodes {
		client, err := jrpc.Dial(fmt.Sprintf("%s:%d", node, opt.RpcPort), []byte(opt.RpcPassword))
		if err != nil {
			return err
		}
		nemesis.clients = append(nemesis.clients, client)
	}
	log.Info("OutOfStorage faulting nodes: %v", nemesis.faultyNodes)
	nemesis.faultTime = time.Now().Add(20 * time.Second)
	nemesis.fixTime = nemesis.faultTime.Add(20 * time.Second)
	return nil
}

func (nemesis *outOfStorageNemesis) Next(client int) (gorgon.Instruction, error) {
	if client > 0 {
		return nil, nil
	}
	if nemesis.done == true {
		return nil, nil
	}
	if !nemesis.toggled && time.Until(nemesis.faultTime) <= 0 {
		nemesis.toggled = true
		return &rpcs.OutOfStorageInstruction{}, nil
	}
	if nemesis.toggled && time.Until(nemesis.fixTime) <= 0 {
		nemesis.toggled = false
		nemesis.done = true
		return &rpcs.OutOfStorageInstruction{}, nil
	}
	return nil, nil
}

func (nemesis *outOfStorageNemesis) Invoke(instruction gorgon.Instruction, getTime func() int64) (int64, gorgon.Output) {
	if instr, ok := instruction.(*rpcs.OutOfStorageInstruction); ok {
		var firstError error
		for _, client := range nemesis.clients {
			var reply string
			err := client.Call("OutOfStorageRpc.SendSignal", instr, &reply)
			if err != nil && firstError == nil {
				firstError = err
			}
		}
		if firstError != nil {
			return getTime(), firstError
		}
		return getTime(), nil
	}
	return -1, gorgon.ErrUnsupportedInstruction
}

func (nemesis *outOfStorageNemesis) Name() string {
	if len(nemesis.faultyNodes) == 0 {
		return "OutOfStorage"
	}
	return fmt.Sprintf("OutOfStorage(%d nodes)", len(nemesis.faultyNodes))
}

func (*outOfStorageNemesis) OnCall(client int, instruction gorgon.Instruction) error {
	return nil
}

func (*outOfStorageNemesis) OnReturn(client int, instruction gorgon.Instruction, output gorgon.Output) error {
	return nil
}

func (nemesis *outOfStorageNemesis) TearDown() error {
	if nemesis.clients == nil {
		return nil
	}
	var firstErr error
	for _, client := range nemesis.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
