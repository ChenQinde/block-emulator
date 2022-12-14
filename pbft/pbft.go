package pbft

import (
	"blockEmulator/chain"
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/utils"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	blocklog, txlog *csv.Writer
	queuelog        *csv.Writer
)

// //本地消息池（模拟持久化层），只有确认提交成功后才会存入此池
// var localMessagePool = []Message{}

type node struct {
	//节点ID
	nodeID string
	//节点监听地址
	addr     string
	CurChain *chain.BlockChain
}

type Pbft struct {
	//节点信息
	Node node
	//每笔请求自增序号
	sequenceID int
	//锁
	lock sync.Mutex
	//确保消息不重复发送的锁
	sequenceLock sync.Mutex
	//确保落后节点对同一区块最多只向主节点请求一次
	requestLock sync.Mutex
	//临时消息池，消息摘要对应消息本体
	messagePool map[string]*Request
	//存放收到的prepare数量(至少需要收到并确认2f个)，根据摘要来对应
	prePareConfirmCount map[string]map[string]bool
	//存放收到的commit数量（至少需要收到并确认2f+1个），根据摘要来对应
	commitConfirmCount map[string]map[string]bool
	//该笔消息是否已进行Commit广播
	isCommitBordcast map[string]bool
	//该笔消息是否已对客户端进行Reply
	isReply map[string]bool
	//区块高度到区块摘要的映射,目前只给主节点使用
	height2Digest map[int]string
	nodeTable     map[string]string
	Stop          chan int
}

func NewPBFT() *Pbft {
	config := params.Config
	p := new(Pbft)
	p.Node.nodeID = config.NodeID
	p.Node.addr = params.NodeTable[config.ShardID][config.NodeID]

	p.Node.CurChain, _ = chain.NewBlockChain(config)
	p.sequenceID = p.Node.CurChain.CurrentBlock.Header.Number + 1
	p.messagePool = make(map[string]*Request)
	p.prePareConfirmCount = make(map[string]map[string]bool)
	p.commitConfirmCount = make(map[string]map[string]bool)
	p.isCommitBordcast = make(map[string]bool)
	p.isReply = make(map[string]bool)
	p.height2Digest = make(map[int]string)
	p.nodeTable = params.NodeTable[config.ShardID]
	p.Stop = make(chan int, 0)
	return p
}

func NewLog(shardID string) {
	csvFile, err := os.Create("./log/" + shardID + "_block.csv")
	if err != nil {
		log.Panic(err)
	}
	// defer csvFile.Close()
	blocklog = csv.NewWriter(csvFile)
	blocklog.Write([]string{"timestamp", "blockHeight", "tx_total", "tx_normal", "tx_relay_first_half", "tx_relay_second_half"})
	blocklog.Flush()

	csvFile, err = os.Create("./log/" + shardID + "_transaction.csv")
	if err != nil {
		log.Panic(err)
	}
	txlog = csv.NewWriter(csvFile)
	txlog.Write([]string{"txid", "blockHeight", "request_time", "commit_time", "delay"})
	txlog.Flush()

	csvFile, err = os.Create("./log/" + shardID + "_queue_length.csv")
	if err != nil {
		log.Panic(err)
	}
	queuelog = csv.NewWriter(csvFile)
	queuelog.Write([]string{"timestamp", "queue_length"})
	queuelog.Flush()
}

func (p *Pbft) handleRequest(data []byte) {
	//切割消息，根据消息命令调用不同的功能
	cmd, content := splitMessage(data)
	switch command(cmd) {
	case cPrePrepare:
		p.handlePrePrepare(content)
	case cPrepare:
		p.handlePrepare(content)
	case cCommit:
		p.handleCommit(content)
	case cRequestBlock:
		p.handleRequestBlock(content)
	case cSendBlock:
		p.handleSendBlock(content)
	case cRelay:
		p.handleRelay(content)
	case cStop:
		p.Stop <- 1
	}
}

//只有主节点可以调用
//生成一个区块并发起共识
func (p *Pbft) Propose() {
	config := params.Config
	for {
		time.Sleep(time.Duration(config.Block_interval) * time.Millisecond)
		// chain.GenerateTxs(p.Node.CurChain)
		if p.Node.nodeID == "N0" {
			p.sequenceLock.Lock() //通过锁强制要求上一个区块commit完成新的区块才能被提出
		}
		p.propose()
	}
}

func (p *Pbft) propose() {
	r := &Request{}
	r.Timestamp = time.Now().Unix()
	r.Message.ID = getRandom()

	block := p.Node.CurChain.GenerateBlock()
	encoded_block := block.Encode()

	r.Message.Content = encoded_block

	// //添加信息序号
	// p.sequenceIDAdd()
	//获取消息摘要
	digest := getDigest(r)
	fmt.Println("已将request存入临时消息池")
	//存入临时消息池
	p.messagePool[digest] = r

	//拼接成PrePrepare，准备发往follower节点
	pp := PrePrepare{r, digest, p.sequenceID}
	p.height2Digest[p.sequenceID] = digest
	b, err := json.Marshal(pp)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println("正在向其他节点进行进行PrePrepare广播 ...")
	//进行PrePrepare广播
	p.broadcast(cPrePrepare, b)
	fmt.Println("PrePrepare广播完成")
}

//处理预准备消息
func (p *Pbft) handlePrePrepare(content []byte) {
	fmt.Println("本节点已接收到主节点发来的PrePrepare ...")
	//	//使用json解析出PrePrepare结构体
	pp := new(PrePrepare)
	err := json.Unmarshal(content, pp)
	if err != nil {
		log.Panic(err)
	}

	if digest := getDigest(pp.RequestMessage); digest != pp.Digest {
		fmt.Println("信息摘要对不上，拒绝进行prepare广播")
	} else if p.sequenceID != pp.SequenceID {
		fmt.Println("消息序号对不上，拒绝进行prepare广播")
	} else if !p.Node.CurChain.IsBlockValid(core.DecodeBlock(pp.RequestMessage.Message.Content)) {
		// todo
		fmt.Println("区块不合法，拒绝进行prepare广播")
	} else {
		// //序号赋值
		// p.sequenceID = pp.SequenceID
		//将信息存入临时消息池
		fmt.Println("已将消息存入临时节点池")
		p.messagePool[pp.Digest] = pp.RequestMessage
		//拼接成Prepare
		pre := Prepare{pp.Digest, pp.SequenceID, p.Node.nodeID}
		bPre, err := json.Marshal(pre)
		if err != nil {
			log.Panic(err)
		}
		//进行准备阶段的广播
		fmt.Println("正在进行Prepare广播 ...")
		p.broadcast(cPrepare, bPre)
		fmt.Println("Prepare广播完成")
	}
}

//处理准备消息
func (p *Pbft) handlePrepare(content []byte) {
	//使用json解析出Prepare结构体
	pre := new(Prepare)
	err := json.Unmarshal(content, pre)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("本节点已接收到%s节点发来的Prepare ... \n", pre.NodeID)

	if _, ok := p.messagePool[pre.Digest]; !ok {
		fmt.Println("当前临时消息池无此摘要，拒绝执行commit广播")
	} else if p.sequenceID != pre.SequenceID {
		fmt.Println("消息序号对不上，拒绝执行commit广播")
	} else {
		p.setPrePareConfirmMap(pre.Digest, pre.NodeID, true)
		count := 0
		for range p.prePareConfirmCount[pre.Digest] {
			count++
		}
		//因为主节点不会发送Prepare，所以不包含自己
		specifiedCount := 0
		if p.Node.nodeID == "N0" {
			specifiedCount = 2 * malicious_num
		} else {
			specifiedCount = 2*malicious_num - 1
		}
		//如果节点至少收到了2f个prepare的消息（包括自己）,并且没有进行过commit广播，则进行commit广播
		p.lock.Lock()
		//获取消息源节点的公钥，用于数字签名验证
		if count >= specifiedCount && !p.isCommitBordcast[pre.Digest] {
			fmt.Println("本节点已收到至少2f个节点(包括本地节点)发来的Prepare信息 ...")

			c := Commit{pre.Digest, pre.SequenceID, p.Node.nodeID}
			bc, err := json.Marshal(c)
			if err != nil {
				log.Panic(err)
			}
			//进行提交信息的广播
			fmt.Println("正在进行commit广播")
			p.broadcast(cCommit, bc)
			p.isCommitBordcast[pre.Digest] = true
			fmt.Println("commit广播完成")
		}
		p.lock.Unlock()
	}
}

//处理提交确认消息
func (p *Pbft) handleCommit(content []byte) {
	//使用json解析出Commit结构体
	c := new(Commit)
	err := json.Unmarshal(content, c)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("本节点已接收到%s节点发来的Commit ... \n", c.NodeID)

	// if _, ok := p.prePareConfirmCount[c.Digest]; !ok {
	// 	fmt.Println("当前prepare池无此摘要，拒绝将信息持久化到本地消息池")
	// } else if p.sequenceID != c.SequenceID {
	// if p.sequenceID != c.SequenceID {
	// 	fmt.Println("消息序号对不上，拒绝将信息持久化到本地消息池")
	// } else {
	p.setCommitConfirmMap(c.Digest, c.NodeID, true)
	count := 0
	for range p.commitConfirmCount[c.Digest] {
		count++
	}
	//如果节点至少收到了2f+1个commit消息（包括自己）,并且节点没有回复过,并且已进行过commit广播，则提交信息至本地消息池，并reply成功标志至客户端！
	p.lock.Lock()
	require_cnt := malicious_num * 2
	if p.sequenceID != c.SequenceID {
		require_cnt += 1
	}
	if count >= malicious_num*2 && !p.isReply[c.Digest] {
		fmt.Println("本节点已收到至少2f + 1 个节点(包括本地节点)发来的Commit信息 ...")
		//将消息信息，提交到本地消息池中！
		if _, ok := p.messagePool[c.Digest]; !ok {
			// 1. 如果本地消息池里没有这个消息，说明节点落后于其他节点，向主节点请求缺失的区块
			p.isReply[c.Digest] = true
			p.requestLock.Lock()
			p.requestBlocks(p.sequenceID, c.SequenceID)
		} else {
			// 2.
			r := p.messagePool[c.Digest]
			encoded_block := r.Message.Content
			block := core.DecodeBlock(encoded_block)
			p.Node.CurChain.AddBlock(block)
			fmt.Printf("编号为 %d 的区块已加入本地区块链！", p.sequenceID)
			curBlock := p.Node.CurChain.CurrentBlock
			fmt.Printf("curBlock: \n")
			curBlock.PrintBlock()

			if p.Node.nodeID == "N0" {
				tx_total := len(block.Transactions)
				now := time.Now().Unix()
				relayCount := 0
				//已上链交易集
				commit_ids := []int{}
				for _, v := range block.Transactions {
					//若交易接收者属于本分片才加入已上链交易集
					if params.ShardTable[params.Config.ShardID] == utils.Addr2Shard(hex.EncodeToString(v.Recipient)) {
						commit_ids = append(commit_ids, v.Id)
						s := fmt.Sprintf("%v %v %v %v %v", v.Id, block.Header.Number, v.RequestTime, now, now-v.RequestTime)
						txlog.Write(strings.Split(s, " "))
					}
					if utils.Addr2Shard(hex.EncodeToString(v.Sender)) != utils.Addr2Shard(hex.EncodeToString(v.Recipient)) {
						relayCount++
					}
				}
				txlog.Flush()
				s := fmt.Sprintf("%v %v %v %v %v %v", now, block.Header.Number, tx_total, tx_total-relayCount, tx_total-len(commit_ids), relayCount-tx_total+len(commit_ids))
				blocklog.Write(strings.Split(s, " "))
				blocklog.Flush()
				//主节点向客户端发送已确认上链的交易集
				c, err := json.Marshal(commit_ids)
				if err != nil {
					log.Panic(err)
				}
				m := jointMessage(cReply, c)
				utils.TcpDial(m, params.ClientAddr)

				queue_len := len(p.Node.CurChain.Tx_pool.Queue)
				err = queuelog.Write([]string{fmt.Sprintf("%v", now), fmt.Sprintf("%v", queue_len)})
				if err != nil {
					log.Panic(err)
				}
				queuelog.Flush()
			}
			p.isReply[c.Digest] = true

			p.sequenceID += 1
			if p.Node.nodeID == "N0" {
				p.sequenceLock.Unlock()
			}
		}

	}
	p.lock.Unlock()
	// }
}

func (p *Pbft) requestBlocks(startID, endID int) {
	r := RequestBlocks{
		StartID:  startID,
		EndID:    endID,
		ServerID: "N0",
		NodeID:   p.Node.nodeID,
	}
	bc, err := json.Marshal(r)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("正在请求区块高度%d到%d的区块\n", startID, endID)
	message := jointMessage(cRequestBlock, bc)
	go utils.TcpDial(message, p.nodeTable[r.ServerID])
}

// 目前只有主节点会接收和处理这个请求，假设主节点拥有完整的全部区块
func (p *Pbft) handleRequestBlock(content []byte) {
	rb := new(RequestBlocks)
	err := json.Unmarshal(content, rb)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("本节点已接收到%s节点发来的 requestBlock ... \n", rb.NodeID)
	blocks := make([]*core.Block, 0)
	for id := rb.StartID; id <= rb.EndID; id++ {
		if _, ok := p.height2Digest[id]; !ok {
			fmt.Printf("主节点没有找到高度%d对应的区块摘要！\n", id)
		}
		if r, ok := p.messagePool[p.height2Digest[id]]; !ok {
			fmt.Printf("主节点没有找到高度%d对应的区块！\n", id)
			log.Panic()
		} else {
			encoded_block := r.Message.Content
			block := core.DecodeBlock(encoded_block)
			blocks = append(blocks, block)
		}
	}
	p.SendBlocks(rb, blocks)

}
func (p *Pbft) SendBlocks(rb *RequestBlocks, blocks []*core.Block) {
	s := SendBlocks{
		StartID: rb.StartID,
		EndID:   rb.EndID,
		Blocks:  blocks,
		NodeID:  rb.ServerID,
	}
	bc, err := json.Marshal(s)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("正在向节点%s发送区块高度%d到%d的区块\n", rb.NodeID, s.StartID, s.EndID)
	message := jointMessage(cSendBlock, bc)
	go utils.TcpDial(message, p.nodeTable[rb.NodeID])
}

func (p *Pbft) handleSendBlock(content []byte) {
	sb := new(SendBlocks)
	err := json.Unmarshal(content, sb)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("本节点已接收到%s发来的%d到%d的区块 \n", sb.NodeID, sb.StartID, sb.EndID)
	for id := sb.StartID; id <= sb.EndID; id++ {
		p.Node.CurChain.AddBlock(sb.Blocks[id-sb.StartID])
		fmt.Printf("编号为 %d 的区块已加入本地区块链！", id)
		curBlock := p.Node.CurChain.CurrentBlock
		fmt.Printf("curBlock: \n")
		curBlock.PrintBlock()
	}
	p.sequenceID = sb.EndID + 1
	p.requestLock.Unlock()
}

//向除自己外的其他节点进行广播
func (p *Pbft) broadcast(cmd command, content []byte) {
	for i := range p.nodeTable {
		if i == p.Node.nodeID {
			continue
		}
		message := jointMessage(cmd, content)
		go utils.TcpDial(message, p.nodeTable[i])
	}
}

//为多重映射开辟赋值
func (p *Pbft) setPrePareConfirmMap(val, val2 string, b bool) {
	if _, ok := p.prePareConfirmCount[val]; !ok {
		p.prePareConfirmCount[val] = make(map[string]bool)
	}
	p.prePareConfirmCount[val][val2] = b
}

//为多重映射开辟赋值
func (p *Pbft) setCommitConfirmMap(val, val2 string, b bool) {
	if _, ok := p.commitConfirmCount[val]; !ok {
		p.commitConfirmCount[val] = make(map[string]bool)
	}
	p.commitConfirmCount[val][val2] = b
}

//返回一个十位数的随机数，作为msgid
func getRandom() int {
	x := big.NewInt(10000000000)
	for {
		result, err := rand.Int(rand.Reader, x)
		if err != nil {
			log.Panic(err)
		}
		if result.Int64() > 1000000000 {
			return int(result.Int64())
		}
	}
}

// relay
func (p *Pbft) TryRelay() {
	config := params.Config
	for {
		time.Sleep(time.Duration(config.Relay_interval) * time.Millisecond)
		for k, v := range params.NodeTable {
			if k == config.ShardID {
				continue
			}
			if txs, isEnough := p.Node.CurChain.Tx_pool.FetchRelayTxs(k); isEnough {
				target_leader := v["N0"]
				r := Relay{
					Txs:     txs,
					ShardID: k,
				}
				bc, err := json.Marshal(r)
				if err != nil {
					log.Panic(err)
				}

				fmt.Printf("正在向分片%v的主节点发送relay交易\n", k)
				message := jointMessage(cRelay, bc)
				go utils.TcpDial(message, target_leader)
			} else {
				fmt.Printf("发向分片%v的relay交易数量不足，暂不发送！\n", k)
			}
		}
	}
}

func (p *Pbft) handleRelay(content []byte) {
	relay := new(Relay)
	err := json.Unmarshal(content, relay)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("本节点已接收到分片%v发来的relay交易 \n", relay.ShardID)
	p.Node.CurChain.Tx_pool.AddTxs(relay.Txs)
}
