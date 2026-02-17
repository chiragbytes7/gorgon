package rpcs

import (
	"os/exec"

	"github.com/couchbaselabs/gorgon/src/gorgon/log"
)

// RPC handler for iptables operations (required by rpc.Register to expose methods)
type IpTablesRpc struct{}

// Execute iptables command on worker node for network partitioning
func (*IpTablesRpc) IpTables(arg *[]string, reply *string) error {
	err := exec.Command("iptables", (*arg)...).Run()
	log.Info("IpTables(%v) returned %v", *arg, err)
	if err != nil {
		return err
	}
	*reply = "ok"
	return nil
}
