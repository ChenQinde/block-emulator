package main

import (
	"blockEmulator/params"
	"blockEmulator/pbft"
	"fmt"
	flag "github.com/spf13/pflag"
	"log"
)

// GO111MODULE=on go run main.go

var (
	shard_num     int
	node_num      int
	shardID       string
	malicious_num int
	nodeID        string
	testFile      string
	isClient      bool
)

func main() {
	//test.Test_account()
	// test.Test_blockChain()
	// test.Test_pbft()
	// test.Test_node()
	// test.Test_random()
	//test.Test_shard()
	// fmt.Println(len("a1e4380a3b1f749673e270229993ee55f35663b4"))
	build()
}

func build() {
	flag.IntVarP(&shard_num, "shard_num", "S", 1, "indicate that how many shards are deployed")
	flag.IntVarP(&node_num, "node_num", "N", 1, "indicate that how many nodes of each shard are deployed")
	//flag.StringVarP(&shardID, "shardID", "s", "", "id of the shard to which this node belongs, for example, S0")
	flag.IntVarP(&malicious_num, "malicious_num", "f", 1, "indicate the maximum of malicious nodes in one shard")
	//flag.StringVarP(&nodeID, "nodeID", "n", "", "id of this node, for example, N0")
	flag.StringVarP(&testFile, "testFile", "t", "", "path of the input test file")
	flag.BoolVarP(&isClient, "client", "c", false, "whether this node is a client")

	flag.Parse()

	if isClient {
		if testFile == "" {
			log.Panic("参数不正确！")
		}
		pbft.RunClient(testFile)
		return
	}
	// 修改全局变量 Config，之后其他地方会调用
	config := params.Config
	//config.NodeID = nodeID
	//config.ShardID = "S0"
	config.Malicious_num = int((node_num - 1) / 3)
	config.Shard_num = int(shard_num)
	config.Path = testFile
	config.MaxRelayBlockSize = 10

	newShards(shard_num, node_num)
	fmt.Println("开始读取交易数据！")
	N0startReadTX(shard_num)

	//fmt.Println("等待关闭节点！")
	closeNodeChan(shard_num)
	fmt.Println("============================================================================\n")
	//
	//if _, ok := params.NodeTable[shardID][nodeID]; ok {
	//	node = shard.NewNode()
	//} else {
	//	log.Fatal("无此节点编号！")
	//}
	//
	//<-node.P.Stop
	//fmt.Printf("节点收到终止节点消息，停止运行")
}
