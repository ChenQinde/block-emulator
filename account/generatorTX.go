package account

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
)

// GenerateTransaction
//
//	@Description: 生成交易数据，具体包含如下 ： 交易（transaction.csv），预计交易结果（result_transaction.csv），账号信息
//	@param n 账号数量
//	@param m 交易数量
//	@param ShardID 分片id
//	@param NodeID 节点id
func GenerateTransaction(n, m int, ShardID, NodeID string) {
	Init(ShardID, NodeID)
	ClearAccount()
	var addrList []string
	addr2Bal := map[string]int{}
	for i := 0; i < n; i++ {
		account := AddAccount()
		addrList = append(addrList, account.Address)
		addr2Bal[account.Address] = 0
	}
	os.Remove("transaction.csv")
	csvFile1, err := os.Create("transaction.csv")

	if err != nil {
		log.Panic(err)
	}
	txlog := csv.NewWriter(csvFile1)
	txlog.Write([]string{"from", "to", "value"})
	txlog.Flush()

	rand.Seed(86)
	for i := 0; i < m; i++ {
		p1 := rand.Intn(n)
		p2 := rand.Intn(n)
		money := rand.Intn(100)
		addr2Bal[addrList[p1]] -= money
		addr2Bal[addrList[p2]] += money

		s := fmt.Sprintf("%v %v %v", addrList[p1], addrList[p2], money)
		txlog.Write(strings.Split(s, " "))
	}
	txlog.Flush()

	os.Remove("result_transaction.csv")
	csvFile2, err := os.Create("result_transaction.csv")
	relog := csv.NewWriter(csvFile2)
	relog.Write([]string{"address", "value"})
	relog.Flush()
	for k, v := range addr2Bal {
		s := fmt.Sprintf("%v %v", k, v)
		relog.Write(strings.Split(s, " "))
	}
	relog.Flush()

}
