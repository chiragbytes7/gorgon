package kv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"errors"
	"github.com/couchbase/gocb/v2"
	"github.com/couchbaselabs/gorgon/src/gorgon"
	"github.com/couchbaselabs/gorgon/src/gorgon/log"
	"github.com/couchbaselabs/gorgon/src/gorgon/nemeses"
	"github.com/couchbaselabs/gorgon/src/gorgon/rpcs"
	"github.com/couchbaselabs/gorgon/src/gorgon/workloads"
	"github.com/couchbaselabs/gorgon/src/gorgon/jrpc"
)

// Flags initialized in main.go
type DatabaseConfig struct {
	User          *string
	Pass          *string
	Port          *int
	Replicas      *int
	Durability    *string
	Timeout       *time.Duration
	ClientOverRpc *bool
	StorageEngine *string
	Vbuckets      *int
}

func NewDatabase(config DatabaseConfig) gorgon.Database {
	return &database{config: config}
}

type database struct {
	config     DatabaseConfig
	options    *gorgon.Options
	durability gocb.DurabilityLevel
}

func (*database) Name() string {
	return "couchbase"
}

func (db *database) SetOptions(opt *gorgon.Options) error {
	db.options = opt
	if durability := *db.config.Durability; len(durability) != 0 {
		db.durability = parseDurabilityLevel(durability)
		if db.durability == gocb.DurabilityLevelUnknown {
			return fmt.Errorf("kv: invalid durability level %q", durability)
		}
	}
	if n := *db.config.Replicas; n < 0 || n > 3 {
		return fmt.Errorf("kv: invalid number of replicas %d", n)
	}
	return nil
}

func (db *database) SetUp() error {
	opt := db.options
	user := *db.config.User
	pass := *db.config.Pass
	replicas := *db.config.Replicas
	storageEngine := *db.config.StorageEngine
	vbuckets := *db.config.Vbuckets

	if opt.NetworkTraceCapture {
		err := db.NetworkTraceCapture()
		if err != nil {
			return err 
		}
	}

	for _, node := range opt.Nodes {
		if err := db.httpPost(node, "controller/hardResetNode", nil); err != nil {
			return err
		}
		if err := db.httpPost(node, "nodes/self/controller/settings", map[string]string{
			"path": "/tmp/lazyfs.mnt",
		}); err != nil {
			return err
		}
		if err := db.httpPost(node, "node/controller/rename", map[string]string{
			"hostname": node}); err != nil {
			return err
		}
	}
	if err := db.httpPost(opt.Nodes[0], "clusterInit", map[string]string{
		"hostname": opt.Nodes[0],
		"username": user,
		"password": pass,
		"services": "kv",
		"port":     "SAME"}); err != nil {
		return err
	}
	for i, node := range opt.Nodes {
		if i == 0 {
			continue
		}
		if err := db.httpPost(node, "node/controller/doJoinCluster", map[string]string{
			"hostname": opt.Nodes[0],
			"user":     user,
			"password": pass,
			"services": "kv"}); err != nil {
			return err
		}
	}
	if err := db.rebalance(db.options.Nodes, nil); err != nil {
		return err
	}
	// Wait for rebalance to complete
	for {
		time.Sleep(time.Second)
		bytes, err := db.httpGet(opt.Nodes[0], "pools/default/rebalanceProgress")
		if err != nil {
			return err
		}
		obj := make(map[string]interface{})
		if err := json.Unmarshal(bytes, &obj); err != nil {
			return fmt.Errorf("kv: cannot parse rebalance progress: %v", err)
		}
		status, ok := obj["status"].(string)
		if !ok {
			return fmt.Errorf("kv: cannot find rebalance status in %s", string(bytes))
		}
		if status == "none" {
			log.Info("Rebalance completed")
			break
		}
		log.Info("Rebalance in progress: %s", string(bytes))
	}
	if err := db.httpPost(opt.Nodes[0], "settings/autoFailover", map[string]string{
		"enabled":                            "true",
		"timeout":                            "15",
		"failoverPreserveDurabilityMajority": "true"}); err != nil {
		return err
	}
	if err := db.httpPost(opt.Nodes[0], "pools/default/buckets", map[string]string{
		"name":           "default",
		"ramQuota":       "1024",
		"storageBackend": storageEngine,
		"evictionPolicy": "fullEviction",
		"replicaNumber":  strconv.Itoa(replicas),
		"numVBuckets":    strconv.Itoa(vbuckets),
		"flushEnabled":   "1"}); err != nil {
		return err
	}
	time.Sleep(5 * time.Second) // Wait for bucket creation

	return nil
}

// We will have to write the logic for cb collects here
func (db *database) TearDown() error {
	if db.options.NetworkTraceCapture {
		if err := db.StopNetworkCapture(); err != nil {
			log.Error("Error stopping network capture: %v", err)
		}
	}
	if db.options.CbcollectLogging {
		return db.CbCollectLogging()
	}
	return nil
}

func (db *database) rebalance(known, ejected []string) error {
	return db.httpPost(db.options.Nodes[0], "controller/rebalance", map[string]string{
		"knownNodes":   formatOtpNodes(known),
		"ejectedNodes": formatOtpNodes(ejected)})
}

func formatOtpNodes(nodes []string) string {
	var builder strings.Builder
	for i, node := range nodes {
		if i == 0 {
			builder.WriteString("ns_1@")
		} else {
			builder.WriteString(",ns_1@")
		}
		builder.WriteString(node)
	}
	return builder.String()
}

func (db *database) httpGet(node, endpoint string) ([]byte, error) {
	uri := fmt.Sprintf("http://%s:%s@%s:8091/%s", *db.config.User, *db.config.Pass, node, endpoint)
	log.Info("HTTP GET %s %s", node, endpoint)
	resp, err := http.Get(uri)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET returned %d: %s", resp.StatusCode, string(bytes))
	}
	return bytes, nil
}

func (db *database) httpPost(node, endpoint string, form map[string]string) error {
	values := make(url.Values, len(form))
	for k, v := range form {
		values.Set(k, v)
	}
	uri := fmt.Sprintf("http://%s:%s@%s:8091/%s", *db.config.User, *db.config.Pass, node, endpoint)
	log.Info("HTTP POST %s %s %s", node, endpoint, values.Encode())
	resp, err := http.PostForm(uri, values)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusAccepted {
			return nil
		}
		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("HTTP POST returned %d: %s", resp.StatusCode, string(bytes))
	}
	return nil
}

func (db *database) NewClient(id int) (gorgon.Client, error) {
	nodes := db.options.Nodes

	// If the a proxy mode is enabled,
	// This method returns the ClientOverRpc object which is the proxy object
	if *db.config.ClientOverRpc {
		return rpcs.NewClientOverRpc(id, nodes[id%len(nodes)], db.options), nil
	}

	// Otherwise return the normal client object that connects directly to the couchbase cluster
	uri := fmt.Sprintf("couchbase://%s:%d", strings.Join(nodes, ","), *db.config.Port)
	return NewClient(id, uri, *db.config.User, *db.config.Pass), nil
}

func (db *database) ClientConfig() string {
	config := ClientConfig{
		Durability: *db.config.Durability,
		Timeout:    *db.config.Timeout}
	configJson, err := json.Marshal(config)
	if err != nil {
		panic(err)
	}
	return string(configJson)
}

func (db *database) CbCollectLogging() error {
	// Flag to track if atleast one node successfully created a client 
	anySuccess := false
	var err error
	// Iterate over all nodes to create rpc clients 
	for _, node := range db.options.Nodes { 
		client, dialErr := jrpc.Dial(
			fmt.Sprintf("%s:%d", node, db.options.RpcPort),
			[]byte(db.options.RpcPassword),
		)
		if dialErr != nil {
			log.Error("Client for cbcollect_info logging rpc on (%s) could not be created", node)
			continue
		}
		anySuccess = true 
		// Passing the directory location and the required arguments to the rpc call 
		outputPath := db.options.LogDirectory
		var reply string
		err = client.Call("CbcollectRpc.CbCollectLogs", &outputPath, &reply)
		if err != nil {
			log.Error("cbcollect_info command on (%s) could not be invoked", node)
		}
		client.Close()
	}
	// This block runs when the logging did not succeed on any of the worker nodes 
	if !anySuccess && err == nil {
		err = errors.New("Failed to initiate logging on any node as rpc clients could not be created")
	}
	return err
}


func (db *database) NetworkTraceCapture() error {
	// Flag to track if atleast one node successfully created a client
	anySuccess := false
	var err error
	// Iterate over all nodes to create rpc clients
	for _, node := range db.options.Nodes { 
		client, dialErr := jrpc.Dial(
			fmt.Sprintf("%s:%d", node, db.options.RpcPort),
			[]byte(db.options.RpcPassword),
		)
		if dialErr != nil {
			log.Error("Client for network trace capture rpc on (%s) could not be created", node)
			continue
		}
		anySuccess = true 
		// Passing the directory location and the required arguments to the rpc call 
		outputPath := db.options.LogDirectory
		var reply string
		err = client.Call("NetworkTraceRpc.CaptureTrace", &outputPath, &reply)
		if err != nil {
			log.Error("network trace command on (%s) could not be invoked", node)
		}
		client.Close()
	}
	// This block runs when the logging did not succeed on any of the worker nodes
	if !anySuccess && err == nil {
		err = errors.New("Failed to initiate logging on any node as rpc clients could not be created")
	}
	return err
}

func (db *database) StopNetworkCapture() error {
	var err error  
	anySuccess := false
	for _, node := range db.options.Nodes {
		client, dialErr := jrpc.Dial(
			fmt.Sprintf("%s:%d", node, db.options.RpcPort),
			[]byte(db.options.RpcPassword),
		)
		if dialErr != nil {
			log.Error("Client for network trace capture stop rpc on (%s) could not be created", node)
			continue
		}
		anySuccess = true 
		emptyStr := ""
		var reply string 
		err = client.Call("NetworkTraceRpc.StopCapture", &emptyStr, &reply)
		if err != nil {
			log.Error("network trace stop command on (%s) could not be invoked", node)
		}
		client.Close()
	}
	if !anySuccess && err == nil {
		err = errors.New("Failed to terminate tshark on any node as rpc clients could not be created")
	} 
	return err
}

func (db *database) Workloads() []gorgon.Workload {
	return []gorgon.Workload{
		// Basic workload with getset instruction
		workloads.GetSetWorkload(),
		// Workload with Kill nemesis to kill memcached process
		workloads.GetSetWorkload().Add(nemeses.NewKillNemesis("memcached")).Add(NewSetAfterKillGenerator()),
		// Partition the cluster, but don't block the web UI port
		workloads.GetSetWorkload().Add(nemeses.NewNetworkPartitionNemesis(8091)).Add(NewPartitionAwareGetSetGenerator()),
		// Workload with LazyFS nemesis
		workloads.GetSetWorkload().Add(nemeses.NewLazyFsNemesis("echo \"lazyfs::clear-cache\" > /tmp/faults.fifo")),
		// Workload to failover (hard or graceful) and recover (full or delta)
		workloads.GetSetWorkload().Add(NewFailoverAndRecoveryNemesis(db, "Graceful", "Full")),
		workloads.GetSetWorkload().Add(NewFailoverAndRecoveryNemesis(db, "Hard", "Full")),
	}
}
