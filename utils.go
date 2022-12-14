package main

import (
	"blockEmulator/params"
	"blockEmulator/pbft"
	"blockEmulator/shard"
	"encoding/json"
	"fmt"
	"io/ioutil"
)

var shards []*shard.Shard

func newShards(shardNum int, node_num int) {
	AddShardNodeToConfig(shardNum, node_num)
	for shardID := 0; shardID < shardNum; shardID++ {
		sigshard := shard.NewShard(shardID, node_num)
		shards = append(shards, sigshard)
	}
}

// 废弃，因为在新增pbft节点的时候就必须声明表格了。
//
//	func AddShardNodeToConfig(sigshard *shard.Shard) {
//		nodetable := make(map[string]string)
//		for _, node := range sigshard.Nodes {
//			nodetable[fmt.Sprintf("N%d", node.NodeID)] = fmt.Sprintf("127.0.0.1:%d", 8201+sigshard.ShardID*100+node.NodeID)
//		}
//		params.NodeTable[fmt.Sprintf("S%d", sigshard.ShardID)] = nodetable
//	}
func AddShardNodeToConfig(shardNum int, node_num int) {
	for shardID := 0; shardID < shardNum; shardID++ {
		nodetable := make(map[string]string)
		for nodeID := 0; nodeID < node_num; nodeID++ {
			nodetable[fmt.Sprintf("N%d", nodeID)] = fmt.Sprintf("127.0.0.1:%d", 8201+shardID*100+nodeID)
		}
		params.NodeTable[fmt.Sprintf("S%d", shardID)] = nodetable
	}
	data, _ := json.Marshal(params.NodeTable)

	// 节点数据写入文件
	err := ioutil.WriteFile("params/nodeTable.json", data, 0644)
	if err != nil {
		panic(err)
	}
}

func closeNodeChan(shardNum int) {
	for shardID := 0; shardID < shardNum; shardID++ {
		for _, node := range shards[shardID].Nodes {
			<-node.P.Stop
			fmt.Printf("S%dN%d节点收到终止节点消息，停止运行\n", shardID, node.NodeID)
		}
	}
}
func N0startReadTX(shardNum int) {
	for shardID := 0; shardID < shardNum; shardID++ {
		config := params.Config
		node := shards[shardID].Nodes[0]
		pbft.NewLog(fmt.Sprintf("S%d", shardID))
		fmt.Printf("The path is %s\n", config.Path)
		txs := shard.LoadTxsWithShard(config.Path, params.ShardTable[fmt.Sprintf("S%d", shardID)])
		go shard.InjectTxs2Shard(node.P.Node.CurChain.Tx_pool, txs)
		go node.P.Propose()
		go node.P.TryRelay()
	}
}
