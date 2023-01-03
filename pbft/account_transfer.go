package pbft

import (
	"blockEmulator/account"
	"blockEmulator/params"
	"blockEmulator/utils"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/boltdb/bolt"
)

// 主分片发向 W 分片的信息，告知 W 分片划分情况
type PartitionMsg_FromMtoW struct {
	PartitionMap    map[string]int    // 最新的划分表
	AcountsInserted []account.Account // 新加入的账户
}

// 账户迁移信息，W 分片发向 W 分片，为了实现不同分片之间的 账户数据 信息交换
type AccountTransferMsg struct {
	FromShardID, ToShardID int               // 迁移的账户来自哪个Shard，发送至哪个Shard
	Transfer_AccountsList  []account.Account // 需要从 FromShardID 迁移至 ToShardID 的账户
}

// 消息编码 针对 划分消息
func (pmfw *PartitionMsg_FromMtoW) MsgEncode() ([]byte, error) {
	content, err := json.Marshal(pmfw)
	if err != nil {
		return content, err
	}
	return jointMessage(cPartitionMsg, content), nil
}

// 消息编码 针对 账户迁移
func (atmsg *AccountTransferMsg) MsgEncode() ([]byte, error) {
	content, err := json.Marshal(atmsg)
	if err != nil {
		return content, err
	}
	return jointMessage(cHandleAccountTransfer, content), nil
}

// 向除自己外的其他节点进行广播(本分片) （烨彤的代码）
func (p *Pbft) broadcastInShard(msg []byte) {
	for i := range p.nodeTable {
		if i == p.Node.nodeID {
			continue
		}
		go utils.TcpDial(msg, p.nodeTable[i])
	}
}

// 向所有主节点进行广播 （烨彤的代码）
func (p *Pbft) broadcastToMain(msg []byte) {
	for i, node := range params.NodeTable {
		if i == params.Config.ShardID {
			continue
		}
		fmt.Printf("==========正在向节点%s发送消息======\n", node["N0"])
		go utils.TcpDial(msg, node["N0"])
	}
}

// 在一个本地数据库中插入一个 account
func (p *Pbft) insertAccount_forLocal(ac account.Account) error {
	dbpath := "./record/" + p.Node.shardID + "_" + p.Node.nodeID + "_" + "blockchain_db"
	db, err := bolt.Open(dbpath, 0600, nil)
	defer db.Close()
	if err != nil {
		return err
	}
	addrByte, _ := hex.DecodeString(ac.Address[2:])
	db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("mbucket"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		err = b.Put([]byte(addrByte), []byte(ac.Encode()))
		return nil
	})
	return nil
}

// 从本地获取 账户数据，package account 中有 GetAccount 方法了
func (p *Pbft) getAccount_fromLocal(addr string) account.Account {
	return *account.GetAccount(addr)
}

// 发送 AccountTransferMsg，一般来说是 Worker 分片调用
func (p *Pbft) send_AccountTransferMsg(desShard int, atmsg AccountTransferMsg) error {
	msg, err := atmsg.MsgEncode()
	if err != nil {
		return err
	}
	p.broadcastToMain(msg)
	return nil
}

// 发送 PartitionMsg_FromMtoW，一般来说是 Main 分片调用
func (p *Pbft) send_PartitionMsg_FromMtoW(pmfw PartitionMsg_FromMtoW) error {
	msg, err := pmfw.MsgEncode()
	if err != nil {
		return err
	}
	p.broadcastToMain(msg)
	return nil
}

// 获取 AccountTransferMsg
func (p *Pbft) generate_AccountTransferMsg(oldMap, newMap map[string]int) []AccountTransferMsg {
	shardNum := p.Node.CurChain.ChainConfig.Shard_num
	atmsg := make([]AccountTransferMsg, shardNum)
	shardID, _ := strconv.Atoi(p.Node.shardID[1:])
	// 数组中第 i 个 message 指的是发送给第 i 个分片的账户信息
	for i := 0; i < shardNum; i++ {
		atmsg[i].FromShardID = shardID
		atmsg[i].ToShardID = i
	}
	for addr, sID := range oldMap {
		if sID != shardID { // 原先不属于 shardID 这个分片，跳过
			continue
		}
		if nsID := newMap[addr]; nsID != sID { // 原先属于 shardID 这个分片，然后在 newMap 中迁移到了另一个分片 nsID
			// 插入到发送给 nsID 的消息中
			atmsg[nsID].Transfer_AccountsList = append(atmsg[nsID].Transfer_AccountsList, *account.GetAccount(addr))
		}
	}
	return atmsg
}

// 处理 AccountTransferMsg
func (p *Pbft) handle_AccountTransferMsg(rawMsg []byte) {
	atmsg := new(AccountTransferMsg)
	shardID, _ := strconv.Atoi(p.Node.shardID[1:])
	err := json.Unmarshal(rawMsg, atmsg)
	if err != nil {
		log.Panic(err)
		return
	}
	if atmsg.ToShardID != shardID {
		return
	}

	// 如果自己为 该分片 主节点，将消息发向本分片的其他节点
	if p.mainNode == p.Node.nodeID {
		Msg2shard, err := atmsg.MsgEncode()
		if err != nil {
			log.Panic(err)
			return
		}
		p.broadcastInShard(Msg2shard)
	}

	// 加入本地的数据库中
	for _, ac := range atmsg.Transfer_AccountsList {
		p.insertAccount_forLocal(ac)
	}
}

// 处理 Main 分片发来的 账户迁移信息
func (p *Pbft) handle_PartitionMsg_FromMtoW(rawMsg []byte) {
	pmsg := new(PartitionMsg_FromMtoW)
	shardID, _ := strconv.Atoi(p.Node.shardID[1:])
	err := json.Unmarshal(rawMsg, pmsg)
	if err != nil {
		log.Panic(err)
		return
	}
	// 首先处理要发送给其他分片的信息
	oldMap := p.Node.CurChain.PartitionMap
	atmsgList := p.generate_AccountTransferMsg(oldMap, pmsg.PartitionMap)
	for desShard, atmsg := range atmsgList {
		if desShard != shardID {
			p.send_AccountTransferMsg(desShard, atmsg)
		}
	}
	// 然后处理归属于本分片的新账户
	for _, ac := range pmsg.AcountsInserted {
		if pmsg.PartitionMap[ac.Address] == shardID {
			// 加入本地的数据库中
			p.insertAccount_forLocal(ac)
		}
	}
	// 更新分片 map
	p.Node.CurChain.PartitionMap = pmsg.PartitionMap
}
