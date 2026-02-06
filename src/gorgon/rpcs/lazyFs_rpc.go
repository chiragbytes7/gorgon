package rpcs

//this file will have code to run the rpc on the worker node
//the function that the client stub calls, and any other objects that it might need to use

//Lets design it in a way, keep the insturction here

import (
	"fmt"
	"os/exec"

	"github.com/couchbaselabs/gorgon/src/gorgon/log"
)

type LazyfsInstruction struct {
	Fault string
	Node  string
}

// Returns a string form of the instruction
func (instruction *LazyfsInstruction) String() string {
	return fmt.Sprintf("Fault called on the LazyFs is (%s)", instruction.Fault)
}

func (instruction *LazyfsInstruction) ForSelf() bool {
	return true
}

//We need to create a struct wiz a rpc object, basically the server stub
//This object will be registered to the global rpc server and its method, which is the
//the rpc call itself, will be able to be called

// This object and its methods need to be exported
// Registration of this object and its methods is done in the main.go file
// Only exportable methods are rpc runnable
type LazyFsRpc struct{}

func (lazyfs_rpc *LazyFsRpc) InjectFault(arg *LazyfsInstruction, reply *string) error {
	//Apply the function here on the worker node
	err := exec.Command("sh", "-c", arg.Fault).Run()
	log.Info("LazyFs fault injection %s , on the node %s returned %v", arg.Fault, arg.Node, err)
	if err != nil {
		return err
	}
	return nil
}
