package rpcs

import (
	"github.com/couchbaselabs/gorgon/src/gorgon/log"
	"os/exec"
	"strings"
)

type OutOfStorageInstruction struct{}

func (instruction *OutOfStorageInstruction) String() string {
	return "OutOfStorageInstruction"
}

func (instruction *OutOfStorageInstruction) ForSelf() bool {
	return true
}

type OutOfStorageRpc struct{}

// the instruction does not need which node, it just needs

func (*OutOfStorageRpc) SendSignal(args *OutOfStorageInstruction, reply *string) error {
	log.Info("invoked the SendSignal rpc call")
	cmd := exec.Command("pgrep", "memcached")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	pid := strings.TrimSpace(string(output)) // Kill sends the sigusr1 signal to this pid
	log.Info("the pid of memcached is : %v", pid)
	err = exec.Command("kill", "-SIGUSR1", pid).Run()
	if err != nil {
		return err
	}

	*reply = "ok"
	return nil
}
