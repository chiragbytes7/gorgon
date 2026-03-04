package rpcs

import (
	"os"
	"os/exec"
	"time"
	"github.com/couchbaselabs/gorgon/src/gorgon/log"
)

type NetworkTraceRpc struct {}

// first arg for directory for captured network dump
func (*NetworkTraceRpc) CaptureTrace(arg *string, reply *string) error {
	log.Info("Network Trace Capture rpc invoked")
	dir := *arg
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Info("Directory doesn't exist, using default")
		dir = "/opt/cbcollects_and_captures"
	}
	timestamp := time.Now().Format("2006-01-02-150405")
	outputFile := dir + "/" + timestamp + "_full_capture.pcap"
	err := exec.Command("tshark", "-i", "any", "-s", "0", "-w", outputFile).Start()
	if err != nil {
		log.Error("tshark failed: %v", err)
		return err
	}
	
	*reply = "ok"
	return nil
}

func (*NetworkTraceRpc) StopCapture(arg *string, reply *string) error {
	err := exec.Command("pkill", "tshark").Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			log.Info("No tshark process found to kill")
			return nil
		}
		log.Error("Failed to stop tshark")
		return err 
	}
	*reply = "ok"
	log.Info("Successfully stopped tshark")
	return nil 
}


