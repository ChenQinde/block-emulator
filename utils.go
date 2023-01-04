package main

import (
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/pbft"
	"blockEmulator/shard"
	"blockEmulator/utils"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"math/rand"
	"os"
	"time"
)

var shards []*shard.Shard
var pshard *shard.PShard

// var addrTable map[common.Address]int      // 账户地址 映射到 shardID
// var string2Addr map[string]common.Address // 交易中的 string 地址 映射 账户地址
var Shard2Txs [][]*core.Transaction // shard 对应的交易合集
var PartitionMap map[string]int     // 账户属于哪个分片
var Shard2Account map[int][]string  // 每个分片的账户

func generateTxs(txNum int, path string) {

	txCsvFile, err := os.Create(path)
	if err != nil {
		log.Panic(err)
	}
	// defer csvFile.Close()
	txsFile := csv.NewWriter(txCsvFile)
	txsFile.Write([]string{"from", "to", "value"})
	account_record := make([]string, 0)
	for i := 0; i < txNum; i++ {
		from := fmt.Sprintf("0x%08d", i)
		tmp := time.Now().UnixNano() + int64(i)
		tmp1 := time.Now().UnixNano() + int64(i*2)
		to := fmt.Sprintf("0x%08v", rand.New(rand.NewSource(tmp)).Int31n(100000000))
		if i < txNum/2 {
			account_record = append(account_record, to)
		}
		if i >= txNum/2 {
			to = account_record[i-txNum/2]
		}
		value := fmt.Sprintf("%03v", rand.New(rand.NewSource(tmp1)).Int31n(1000))
		txsFile.Write([]string{from, to, value})
		txsFile.Flush()
	}
}

func LoadTxsFromFIle(path string, shardNum int) {
	Shard2Txs = make([][]*core.Transaction, shardNum+1)
	Shard2Account = make(map[int][]string)
	for i := 0; i < shardNum; i++ {
		Shard2Account[i] = make([]string, 0)
	}
	//addrTable = make(map[common.Address]int)
	//string2Addr = make(map[string]common.Address)
	PartitionMap = make(map[string]int)
	file, err := os.Open(path)
	if err != nil {
		log.Panic()
	}
	defer file.Close()
	r := csv.NewReader(file)
	_, err = r.Read()
	if err != nil {
		log.Panic()
	}
	txid := 0
	for {
		row, err := r.Read()
		// fmt.Printf("%v %v %v\n", row[0][2:], row[1][2:], row[2])
		if err != nil && err != io.EOF {
			log.Panic()
		}
		if err == io.EOF {
			break
		}
		//sid := utils.Addr2Shard(row[0][2:])
		sender, _ := hex.DecodeString(row[0][2:])
		sid := utils.Addr2Shard(hex.EncodeToString(sender))
		recipient, _ := hex.DecodeString(row[1][2:])
		value := new(big.Int)
		value.SetString(row[2], 10)
		//sid := utils.Addr2Shard(row[3][2:])
		//sender, _ := hex.DecodeString(row[3][2:])
		//recipient, _ := hex.DecodeString(row[4][2:])
		//value := new(big.Int)
		//value.SetString(row[6], 10)
		//value.SetString("1", 10)
		Shard2Txs[sid] = append(Shard2Txs[sid], &core.Transaction{
			Sender:    sender,
			Recipient: recipient,
			Value:     value,
			Id:        txid,
		})
		//senderAddr := common.HexToAddress(row[0][2:])
		//recipientAddr := common.HexToAddress(row[1][2:])
		//senderAddr := common.HexToAddress(row[3][2:])
		//recipientAddr := common.HexToAddress(row[4][2:])
		//addrTable[senderAddr] = utils.Addr2Shard(hex.EncodeToString(sender))
		//addrTable[recipientAddr] = utils.Addr2Shard(hex.EncodeToString(recipient))
		//string2Addr[hex.EncodeToString(sender)] = senderAddr
		//string2Addr[hex.EncodeToString(recipient)] = recipientAddr
		PartitionMap[hex.EncodeToString(sender)] = utils.Addr2Shard(hex.EncodeToString(sender))
		PartitionMap[hex.EncodeToString(recipient)] = utils.Addr2Shard(hex.EncodeToString(recipient))
		// hex 有问题这个才是正确的
		//PartitionMap[row[0][2:]] = addrTable[senderAddr]
		//PartitionMap[row[1][2:]] = addrTable[recipientAddr]

		txid += 1
	}
	for addr, _ := range PartitionMap {
		Shard2Account[PartitionMap[addr]] = append(Shard2Account[PartitionMap[addr]], addr)
	}
	fmt.Printf("txs length is %d\n", txid)
}

func newShards(shardNum int, nodeNum int) {
	fmt.Printf("the size is %d %d %d\n", len(PartitionMap), len(Shard2Account[0]), len(Shard2Account[1]))
	AddShardNodeToConfig(shardNum, nodeNum)
	for shardID := 0; shardID < shardNum; shardID++ {
		params.ShardTable[fmt.Sprintf("S%d", shardID)] = shardID
		params.ShardTableInt2Str[shardID] = fmt.Sprintf("S%d", shardID)
		initAccount(shardID)
		sigshard := shard.NewShard(shardID, nodeNum)
		sigshard.InitAccountMap(PartitionMap)
		shards = append(shards, sigshard)
	}
	buildPshard(shardNum, nodeNum)
}

func initAccount(shardID int) {
	//for addr, _ := range addrTable {
	//	if addrTable[addr] == shardID {
	//		params.Init_addrs = append(params.Init_addrs, addr.String())
	//	}
	//fmt.Println("address is ", addr.String(), " sid is ", addrTable[addr])
	//}

	//fmt.Println("address num is is ", len(Shard2Account[shardID]), " sid is ", shardID)
	params.Init_addrs = make([]string, 0)
	for _, addr := range Shard2Account[shardID] {
		params.Init_addrs = append(params.Init_addrs, addr)
		//fmt.Println("address is ", addr, " sid is ", shardID)
	}
}

func buildPshard(shardNum int, nodeNum int) {
	params.ShardTable[fmt.Sprintf("S%d", shardNum)] = shardNum
	params.ShardTableInt2Str[shardNum] = fmt.Sprintf("S%d", shardNum)
	pshard = shard.NewPShard(shardNum, nodeNum)
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
	for shardID := 0; shardID <= shardNum; shardID++ {
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
		pbft.NewLog(node.P)
		fmt.Printf("The path is %s\n", config.Path)
		//txs := shard.LoadTxsWithShard(config.Path, params.ShardTable[fmt.Sprintf("S%d", shardID)])
		go shard.InjectTxs2Shard(node.P.Node.CurChain.Tx_pool, Shard2Txs[shardID])
		go node.P.Propose()
		go node.P.TryRelay()
	}
	node := pshard.Nodes[0]
	pbft.NewPLog(node.P)
	//go node.P.Propose()
}

func PrintAccount(shardNum int) {
	for shardID := 0; shardID < shardNum; shardID++ {
		node := shards[shardID].Nodes[0]
		fmt.Printf("The addres num of the shard %d is %d\n", shardID, len(Shard2Account[shardID]))
		for _, account := range Shard2Account[shardID] {
			addr, _ := hex.DecodeString(account)
			decoded, success := node.P.Node.CurChain.StatusTrie.Get(addr)
			if !success {
				log.Panic()
			}
			account_state := core.DecodeAccountState(decoded)
			fmt.Printf("Acount address is %s : %d\n", hex.EncodeToString(addr), account_state.Balance)
		}
	}
}
