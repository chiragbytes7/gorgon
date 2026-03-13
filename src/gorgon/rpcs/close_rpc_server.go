package rpcs

import (
	"github.com/couchbaselabs/gorgon/src/gorgon/jrpc"
)

type CloseRpcServerRpc struct{}

func (*CloseRpcServerRpc) Shutdown(args *string, reply *string) error {
	err := jrpc.CloseListener()
	if err != nil {
		return err
	}
	*reply = "ok"
	return nil
}
