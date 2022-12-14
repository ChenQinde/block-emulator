package shard

import (
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/pbft"
	"blockEmulator/utils"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"time"
)

type Node struct {
	P *pbft.Pbft //代表分片内的那部分
}

var (
	txs []*core.Transaction
)

func NewNode() *Node {
	node := new(Node)
	node.P = pbft.NewPBFT()

	go node.P.TcpListen() //启动节点
	block := node.P.Node.CurChain.CurrentBlock
	fmt.Printf("current block: \n")
	block.PrintBlock()

	config := params.Config
	if config.NodeID == "N0" {
		pbft.NewLog(config.ShardID)
		txs = LoadTxsWithShard(config.Path, params.ShardTable[config.ShardID])
		go InjectTxs2Shard(node.P.Node.CurChain.Tx_pool)
		go node.P.Propose()
		go node.P.TryRelay()
	}

	return node
}

func LoadTxsWithShard(path string, sid int) []*core.Transaction {
	txs := make([]*core.Transaction, 0)
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
		if utils.Addr2Shard(row[0]) == sid { // 发送者地址属于本分片
			sender, _ := hex.DecodeString(row[0][2:])
			recipient, _ := hex.DecodeString(row[1][2:])
			value := new(big.Int)
			value.SetString(row[2], 10)
			txs = append(txs, &core.Transaction{
				Sender:    sender,
				Recipient: recipient,
				Value:     value,
				Id:        txid,
			})
		}
		txid += 1
	}
	fmt.Printf("%d\n", len(txs))
	txs[0].PrintTx()
	return txs
}

func InjectTxs2Shard(pool *core.Tx_pool) {
	cnt := 0
	inject_speed := params.Config.Inject_speed
	for {
		time.Sleep(1000 * time.Millisecond)
		upperBound := utils.Min(cnt+inject_speed, len(txs))
		for i := cnt; i < upperBound; i++ {
			txs[i].RequestTime = time.Now().Unix()
			// pool.AddTx(txs[i])
		}
		pool.AddTxs(txs[cnt:upperBound])
		cnt = upperBound
		if cnt == len(txs) {
			break
		}
	}
}
