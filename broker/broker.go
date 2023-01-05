package broker

import (
	"blockEmulator/core"
	"blockEmulator/utils"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"os"
	"sort"
	"time"
)

var (
	allTxs     []*core.Transaction //所有交易集合
	brokerTxs  []*core.Transaction //原始broker交易，即跨非broker参与的分片交易
	ctx2       []*core.Transaction //生成的 broker->Receiver 的交易
	brokers    []string            //broker的地址集合，注以 0x 开头
	txToBroker map[int]string      // broker交易ID到broker的映射
	addrTXsCnt map[string]int      // 地址到该地址参与交易的数目的映射
)

// IsBrokerTx
//
//	@Description: 判断交易是否为broker交易
//	@param txID 交易的id
//	@return bool 是或否
func IsBrokerTx(txID int) bool {
	for _, v := range brokerTxs {
		if v.Id == txID {
			return true
		}
	}
	return false
}

// IsBroker
//
//	@Description: 判断用户是否为broker用户
//	@param addr 用户地址，需要把开头0x去除
//	@return bool 是或否
func IsBroker(addr string) bool {
	for _, v := range brokers {
		if v[2:] == addr {
			return true
		}
	}
	return false
}

// getBrokerTx
//
//	@Description: 获取单个broker交易
//	@param txId  交易id
//	@return *core.Transaction 交易对象
func getBrokerTx(txId int) *core.Transaction {
	for _, v := range brokerTxs {
		if v.Id == txId {
			return v
		}
	}
	return nil
}

// SetBrokers
//
//	@Description: 设置broker账户
//	@param brokerNum 数量
//	@return []string broker账户地址合集，以0x开头
func SetBrokers(brokerNum int) []string {
	ms := NewMapSorter(addrTXsCnt)
	sort.Sort(ms)
	for i := 0; i < brokerNum; i++ {
		brokers = append(brokers, ms[i].Key)
		fmt.Printf("broker:%v\n", ms[i].Key)
	}
	fmt.Printf("broker的数量为:%v\n", len(brokers))
	return brokers
}

// GetAllTx
//
//	@Description: 获得所有交易地址
//	@param path 文件地址
func GetAllTx(path string) {
	addrTXsCnt = make(map[string]int, 0)
	allTxs = make([]*core.Transaction, 0)
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
		if err != nil && err != io.EOF {
			log.Panic()
		}
		if err == io.EOF {
			break
		}
		addrTXsCnt[row[0]] += 1
		addrTXsCnt[row[1]] += 1
		sender, _ := hex.DecodeString(row[0][2:])
		recipient, _ := hex.DecodeString(row[1][2:])
		value := new(big.Int)
		value.SetString(row[2], 10)
		allTxs = append(allTxs, &core.Transaction{
			Sender:    sender,
			Recipient: recipient,
			Value:     value,
			Id:        txid,
		})
		txid += 1
	}
	fmt.Print("broker55号交易为：")
	allTxs[55].PrintTx()
	fmt.Printf("Txs的数量是：%d\n", len(allTxs))
}

// GetBrokerTxs
//
//	@Description: 获得broker交易，即跨分片交易
//	@return []*core.Transaction
func GetBrokerTxs() []*core.Transaction {
	for _, v := range allTxs {
		//
		//fmt.Printf("Sender:%v Recipient:%v\n", hex.EncodeToString(v.Sender), hex.EncodeToString(v.Recipient))
		//fmt.Printf("sid:%v rid:%v\n", utils.Addr2Shard(hex.EncodeToString(v.Sender)), utils.Addr2Shard(hex.EncodeToString(v.Recipient)))
		//跳过broker参与的交易
		if IsBroker(hex.EncodeToString(v.Sender)) || IsBroker(hex.EncodeToString(v.Recipient)) {
			continue
		}

		//todo broker设置随机数,仅仅再一定的概率下进行替换
		//rand.Seed(time.Now().UnixNano())
		//if (rand.Intn(100) % 2) != 0 {
		//	continue
		//}

		// 选取发送者和接收者不在相同分片的交易，并且按照一定的概率进行转换
		if utils.Addr2Shard(hex.EncodeToString(v.Sender)) != utils.Addr2Shard(hex.EncodeToString(v.Recipient)) {
			brokerTxs = append(brokerTxs, v)
		}
	}
	fmt.Printf("brokerTxs的数量是：%d\n", len(brokerTxs))
	return brokerTxs
}

// AllocationTx
//
//	@Description: 给broker交易分配broker
func AllocationTx() {
	rand.Seed(time.Now().Unix())
	txToBroker = make(map[int]string)
	for _, v := range brokerTxs {
		txToBroker[v.Id] = brokers[rand.Intn(len(brokers))]
	}
	fmt.Printf("txToBroker的数量是：%d\n", len(txToBroker))
}

// BrokerTx2CTX1
//
//	@Description: 将原始交易转化为ctx1， 即a->c 转化为 a->b,b为broker
//	@param txs 原始交易
//	@return []*core.Transaction ctx1
func BrokerTx2CTX1(txs []*core.Transaction) []*core.Transaction {
	for k, v := range txs {
		if IsBrokerTx(v.Id) {
			fmt.Println("原始交易↓")
			txs[k].PrintTx()
			recipient, _ := hex.DecodeString(txToBroker[v.Id][2:])
			txs[k].Recipient = recipient
			txs[k].PrintTx()
			fmt.Println("更新交易↑")
		}
	}
	return txs
}

// CTX1ToCTX2
//
//	@Description: 根据ctx1生成为ctx2,即a->b 转化为 b->c, b 为 broker
//	@param txs
//	@return []*core.Transaction
func CTX1ToCTX2(txs []*core.Transaction) []*core.Transaction {
	ctx2 = make([]*core.Transaction, 0)
	for _, v := range txs {
		if IsBrokerTx(v.Id) {
			brokerTx := getBrokerTx(v.Id)
			id := len(allTxs)
			newTx := &core.Transaction{
				Sender:    v.Recipient,
				Recipient: brokerTx.Recipient,
				Value:     v.Value,
				Id:        id,
			}
			allTxs = append(allTxs, newTx)
			ctx2 = append(ctx2, newTx)
		}
	}
	return ctx2
}
