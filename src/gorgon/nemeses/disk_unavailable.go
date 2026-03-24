package nemeses

import (
	"fmt"
	"net/rpc"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/jrpc"
	"github.com/couchbaselabs/gorgon/src/gorgon/rpcs"
	"github.com/couchbaselabs/gorgon/src/gorgon/splitmix"
)

type unavailableDisk struct {
	node           string
	invokeTime     time.Time
	recoverTime    time.Time
	injected       bool
	recovered      bool
	client         *rpc.Client
	processCrashed bool
	crashTime      time.Time
}

func NewDiskUnavailableNemesis() gorgon.Generator {
	return &unavailableDisk{}
}

func (*unavailableDisk) Name() string {
	return "DiskUnavailable"
}

func (nemesis *unavailableDisk) SetUp(opt *gorgon.Options) error {
	nemesis.node = opt.Nodes[splitmix.Rand.Intn(len(opt.Nodes))]
	client, err := jrpc.Dial(fmt.Sprintf("%s:%d", nemesis.node, opt.RpcPort), []byte(opt.RpcPassword))
	if err != nil {
		return err
	}
	nemesis.client = client
	nemesis.invokeTime = time.Now().Add(30 * time.Second)
	nemesis.recoverTime = nemesis.invokeTime.Add(10 * time.Second)
	nemesis.crashTime = nemesis.recoverTime
	return nil
}

func (nemesis *unavailableDisk) Next(client int) (gorgon.Instruction, error) {
	if client >= 0 {
		return nil, nil
	}
	if !nemesis.processCrashed && time.Until(nemesis.crashTime) <= 0 {
		nemesis.processCrashed = true
		return &rpcs.KillInstruction{Process: "memcached", Signal: 9}, nil
	}
	if !nemesis.injected && time.Until(nemesis.invokeTime) <= 0 {
		nemesis.injected = true
		return &rpcs.OutOfStorageInstruction{}, nil
	}
	if nemesis.injected && !nemesis.recovered && time.Until(nemesis.recoverTime) <= 0 {
		nemesis.recovered = true
		return &rpcs.OutOfStorageInstruction{}, nil
	}
	return nil, nil
}

func (nemesis *unavailableDisk) Invoke(instruction gorgon.Instruction, getTime func() int64) (int64, gorgon.Output) {
	if instr, ok := instruction.(*rpcs.OutOfStorageInstruction); ok {
		var reply string
		err := nemesis.client.Call("OutOfStorageRpc.SendSignal", instr, &reply)
		if err != nil {
			return getTime(), err
		}
		return getTime(), nil
	}
	if instr, ok := instruction.(*rpcs.KillInstruction); ok {
		var reply string
		err := nemesis.client.Call("KillRpc.Pkill", instr, &reply)
		return getTime(), err
	}
	return -1, gorgon.ErrUnsupportedInstruction
}

func (nemesis *unavailableDisk) OnCall(client int, instruction gorgon.Instruction) error {
	return nil
}

func (nemesis *unavailableDisk) OnReturn(client int, instruction gorgon.Instruction, output gorgon.Output) error {
	return nil
}

func (nemesis *unavailableDisk) TearDown() error {
	if nemesis.client == nil {
		return nil
	}
	return nemesis.client.Close()
}
