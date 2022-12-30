package pbft

import (
	"blockEmulator/chain"
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/partition"
	"blockEmulator/utils"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type PShard_pbft struct {
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
	//确保保存图的读写唯一
	graphLock sync.Mutex
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
	malicious_num int
	// 图的存储
	Graph      partition.CLPAState
	Graph2Pack partition.CLPAState
	// 建立block、queue_length以及transaction的日记文件，也是只有主节点使用
	blocklog *csv.Writer
	txlog    *csv.Writer
	queuelog *csv.Writer
}

func NewPShard_pbft(shardID int, nodeID int) *PShard_pbft {
	config := params.ChainConfig{}
	config.ChainID = params.Config.ChainID
	config.Block_interval = params.Config.Block_interval
	config.MaxBlockSize = params.Config.MaxBlockSize
	config.MaxRelayBlockSize = params.Config.MaxRelayBlockSize
	config.MinRelayBlockSize = params.Config.MinRelayBlockSize
	config.Inject_speed = params.Config.Inject_speed
	config.Relay_interval = params.Config.Relay_interval
	p := new(PShard_pbft)
	p.Node.nodeID = fmt.Sprintf("N%d", nodeID)
	p.Node.shardID = fmt.Sprintf("S%d", shardID)
	//fmt.Printf("The address of graph node is 127.0.0.1:%d\n", 8201+shardID*100+nodeID)
	p.Node.addr = fmt.Sprintf("127.0.0.1:%d", 8201+shardID*100+nodeID)
	config.ShardID = fmt.Sprintf("S%d", shardID)
	config.NodeID = fmt.Sprintf("N%d", nodeID)
	//fmt.Println("config.NodeID ", config.NodeID)
	p.Graph.Init_CLPAState(0.1, 150, len(params.NodeTable))

	p.Node.CurGraphChain, _ = chain.NewGraphBlockChain(&config)
	//fmt.Printf("p.Node.CurGraphChain.CurrentBlock.Header.Number is %d\n", p.Node.CurGraphChain.CurrentBlock.Header.Number)
	p.sequenceID = p.Node.CurGraphChain.CurrentBlock.Header.Number + 1
	p.messagePool = make(map[string]*Request)
	p.prePareConfirmCount = make(map[string]map[string]bool)
	p.commitConfirmCount = make(map[string]map[string]bool)
	p.isCommitBordcast = make(map[string]bool)
	p.isReply = make(map[string]bool)
	p.height2Digest = make(map[int]string)
	p.nodeTable = params.NodeTable[config.ShardID]
	p.Stop = make(chan int, 0)
	p.malicious_num = config.Malicious_num
	return p
}

func NewPLog(p *PShard_pbft) {
	cond, err := PathExists("./log")
	if !cond {
		err := os.Mkdir("./log", os.ModePerm)
		if err != nil {
			fmt.Printf("创建目录异常 -> %v\n", err)
		} else {
			fmt.Println("创建成功!")
		}
	}
	csvFile, err := os.Create("./log/" + p.Node.shardID + "_block.csv")
	if err != nil {
		log.Panic(err)
	}
	// defer csvFile.Close()
	p.blocklog = csv.NewWriter(csvFile)
	p.blocklog.Write([]string{"timestamp", "blockHeight", "tx_total", "tx_normal", "tx_relay_first_half", "tx_relay_second_half"})
	p.blocklog.Flush()

	csvFile, err = os.Create("./log/" + p.Node.shardID + "_transaction.csv")
	if err != nil {
		log.Panic(err)
	}
	p.txlog = csv.NewWriter(csvFile)
	p.txlog.Write([]string{"txid", "blockHeight", "request_time", "commit_time", "delay"})
	p.txlog.Flush()

	csvFile, err = os.Create("./log/" + p.Node.shardID + "_queue_length.csv")
	if err != nil {
		log.Panic(err)
	}
	p.queuelog = csv.NewWriter(csvFile)
	p.queuelog.Write([]string{"timestamp", "queue_length"})
	p.queuelog.Flush()
}

func (p *PShard_pbft) handleRequest(data []byte) {
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
	case gPropose:
		p.handlegPropose()
	case cStop:
		p.Stop <- 1
	}
}

// 只有主节点可以调用
// 生成一个区块并发起共识
func (p *PShard_pbft) Propose() {
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

func (p *PShard_pbft) propose() bool {
	r := &Request{}
	r.Timestamp = time.Now().Unix()
	r.Message.ID = getRandom()

	block := p.Node.CurGraphChain.GenerateGraphBlock()
	encoded_block := block.Encode()

	r.Message.Content = encoded_block

	// //添加信息序号
	// p.sequenceIDAdd()
	//获取消息摘要
	digest := getDigest(r)
	fmt.Printf("%s%s: 已将request存入临时消息池\n", p.Node.shardID, p.Node.nodeID)
	//存入临时消息池
	p.messagePool[digest] = r

	//拼接成PrePrepare，准备发往follower节点
	pp := PrePrepare{r, digest, p.sequenceID}
	p.height2Digest[p.sequenceID] = digest
	tx_num, err := json.Marshal(pp)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("%s%s: 正在向其他节点进行进行PrePrepare广播 ...\n", p.Node.shardID, p.Node.nodeID)
	//进行PrePrepare广播
	p.broadcast(cPrePrepare, tx_num)
	fmt.Printf("%s%s: PrePrepare广播完成\n", p.Node.shardID, p.Node.nodeID)
	return false
}

func (p *PShard_pbft) handlegPropose() {
	p.graphLock.Lock()
	p.Graph2Pack = p.Graph
	err := p.Graph2Pack.Stable_Init_Partition()
	if err == nil {
		p.Graph2Pack.CLPA_Partition()
	}
	p.Node.CurGraphChain.AddGraph2Tool(&p.Graph2Pack)
	p.graphLock.Unlock()
	if p.Node.nodeID == "N0" {
		p.sequenceLock.Lock() //通过锁强制要求上一个区块commit完成新的区块才能被提出
	}
	//fmt.Printf("%s%s: prepapre3 to popose a graph block.\n", p.Node.shardID, p.Node.nodeID)
	p.propose()
}

// 处理预准备消息
func (p *PShard_pbft) handlePrePrepare(content []byte) {
	fmt.Printf("节点%s%s已接收到主节点发来的PrePrepare ...\n", p.Node.shardID, p.Node.nodeID)
	//	//使用json解析出PrePrepare结构体
	pp := new(PrePrepare)
	err := json.Unmarshal(content, pp)
	if err != nil {
		log.Panic(err)
	}

	if digest := getDigest(pp.RequestMessage); digest != pp.Digest {
		fmt.Printf("%s%s: 信息摘要对不上，拒绝进行prepare广播\n", p.Node.shardID, p.Node.nodeID)
	} else if p.sequenceID != pp.SequenceID {
		fmt.Printf("%s%s: 消息序号对不上，拒绝进行prepare广播\n", p.Node.shardID, p.Node.nodeID)
	} else if !p.Node.CurGraphChain.IsGraphBlockValid(core.DecodeGraphBlock(pp.RequestMessage.Message.Content)) {
		// todo
		fmt.Printf("%s%s: 区块不合法，拒绝进行prepare广播", p.Node.shardID, p.Node.nodeID)
	} else {
		// //序号赋值
		// p.sequenceID = pp.SequenceID
		//将信息存入临时消息池
		fmt.Printf("节点%s%s已将消息存入临时节点池\n", p.Node.shardID, p.Node.nodeID)
		p.messagePool[pp.Digest] = pp.RequestMessage
		//拼接成Prepare
		pre := Prepare{pp.Digest, pp.SequenceID, p.Node.nodeID}
		bPre, err := json.Marshal(pre)
		if err != nil {
			log.Panic(err)
		}
		//进行准备阶段的广播
		fmt.Printf("节点%s%s正在进行Prepare广播 ...\n", p.Node.shardID, p.Node.nodeID)
		p.broadcast(cPrepare, bPre)
		fmt.Printf("节点%s%sPrepare广播完成。\n", p.Node.shardID, p.Node.nodeID)
	}
}

// 处理准备消息
func (p *PShard_pbft) handlePrepare(content []byte) {
	//使用json解析出Prepare结构体
	pre := new(Prepare)
	err := json.Unmarshal(content, pre)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("节点%s%s已接收到%s节点发来的Prepare ... \n", p.Node.shardID, p.Node.nodeID, pre.NodeID)

	if _, ok := p.messagePool[pre.Digest]; !ok {
		fmt.Printf("%s%s当前临时消息池无此摘要，拒绝执行commit广播\n", p.Node.shardID, p.Node.nodeID)
	} else if p.sequenceID != pre.SequenceID {
		fmt.Printf("%s%s消息序号对不上，拒绝执行commit广播\n", p.Node.shardID, p.Node.nodeID)
	} else {
		p.setPrePareConfirmMap(pre.Digest, pre.NodeID, true)
		count := 0
		for range p.prePareConfirmCount[pre.Digest] {
			count++
		}
		//因为主节点不会发送Prepare，所以不包含自己
		specifiedCount := 0
		if p.Node.nodeID == "N0" {
			specifiedCount = 2 * p.malicious_num
		} else {
			specifiedCount = 2*p.malicious_num - 1
		}
		//如果节点至少收到了2f个prepare的消息（包括自己）,并且没有进行过commit广播，则进行commit广播
		p.lock.Lock()
		//获取消息源节点的公钥，用于数字签名验证
		if count >= specifiedCount && !p.isCommitBordcast[pre.Digest] {
			fmt.Printf("节点%s%s已收到至少2f个节点(包括本地节点)发来的Prepare信息 ...\n", p.Node.shardID, p.Node.nodeID)

			c := Commit{pre.Digest, pre.SequenceID, p.Node.nodeID}
			bc, err := json.Marshal(c)
			if err != nil {
				log.Panic(err)
			}
			//进行提交信息的广播
			fmt.Printf("节点%s%s正在进行commit广播\n", p.Node.shardID, p.Node.nodeID)
			p.broadcast(cCommit, bc)
			p.isCommitBordcast[pre.Digest] = true
			fmt.Printf("节点%s%scommit广播完成\n", p.Node.shardID, p.Node.nodeID)
		}
		p.lock.Unlock()
	}
}

// 处理提交确认消息
func (p *PShard_pbft) handleCommit(content []byte) {
	//使用json解析出Commit结构体
	c := new(Commit)
	err := json.Unmarshal(content, c)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("节点%s%s已接收到%s节点发来的Commit ... \n", p.Node.shardID, p.Node.nodeID, c.NodeID)

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
	require_cnt := p.malicious_num * 2
	if p.sequenceID != c.SequenceID {
		require_cnt += 1
	}
	if count >= p.malicious_num*2 && !p.isReply[c.Digest] {
		fmt.Printf("节点%s%s已收到至少2f + 1 个节点(包括本地节点)发来的Commit信息 ...\n", p.Node.shardID, p.Node.nodeID)
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
			block := core.DecodeGraphBlock(encoded_block)
			p.Node.CurGraphChain.AddGraphBlock(block)
			fmt.Printf("%s%s : 编号为 %d 的区块已加入本地区块链！\n", p.Node.shardID, p.Node.nodeID, p.sequenceID)
			curBlock := p.Node.CurGraphChain.CurrentBlock
			fmt.Printf("curBlock: \n")
			curBlock.PrintBlock()

			if p.Node.nodeID == "N0" {
				graph_total := len(block.Graphs)
				now := time.Now().Unix()
				p.txlog.Flush()
				s := fmt.Sprintf("%v %v %v", now, block.Header.Number, graph_total)
				p.blocklog.Write(strings.Split(s, " "))
				p.blocklog.Flush()
				//主节点向M分片发送已确认上链的图
				content := []int{}
				c, err := json.Marshal(content)
				if err != nil {
					log.Panic(err)
				}
				m := jointMessage(gGenerate, c)
				utils.TcpDial(m, params.ClientAddr)
				// 向M分片发送新生成的图划分区块。
				p.SendBlocks2PShard(p.sequenceID)

				//queue_len := len(p.Node.CurGraphChain.Tx_pool.Queue)
				//err = p.queuelog.Write([]string{fmt.Sprintf("%v", now), fmt.Sprintf("%v", queue_len)})
				//if err != nil {
				//	log.Panic(err)
				//}
				//p.queuelog.Flush()
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

func (p *PShard_pbft) SendBlocks2PShard(id int) {
	//return
	rb := RequestBlocks{
		StartID:  id,
		EndID:    id,
		ServerID: "N0",
		NodeID:   "N0",
	}

	blocks := make([]*core.GraphBlock, 0)
	if _, ok := p.height2Digest[id]; !ok {
		//fmt.Printf("主节点发送给M分片%s时没有找到高度%d对应的区块摘要！\n", id, k)
	}
	if r, ok := p.messagePool[p.height2Digest[id]]; !ok {
		//fmt.Printf("主节点发送给M分片%s时没有找到高度%d对应的区块！\n", id, k)
		log.Panic()
	} else {
		encoded_block := r.Message.Content
		block := core.DecodeGraphBlock(encoded_block)
		blocks = append(blocks, block)
	}
	s := SendBlocks{
		StartID:     rb.StartID,
		EndID:       rb.EndID,
		EncodeGraph: blocks[0].Encode(),
		NodeID:      rb.ServerID,
	}
	bc, err := json.Marshal(s)
	if err != nil {
		log.Panic(err)
	}
	for k, _ := range params.NodeTable {
		if k == p.Node.shardID {
			continue
		}
		fmt.Printf("节点%s%s准备向M分片%s发送区块 ... \n", p.Node.shardID, p.Node.nodeID, k)
		message := jointMessage(gSendBlock, bc)
		go utils.TcpDial(message, params.NodeTable[k][rb.NodeID])
	}
}
func (p *PShard_pbft) requestBlocks(startID, endID int) {
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
func (p *PShard_pbft) handleRequestBlock(content []byte) {
	rb := new(RequestBlocks)
	err := json.Unmarshal(content, rb)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("节点%s%s已接收到%s节点发来的 requestBlock ... \n", p.Node.shardID, p.Node.nodeID, rb.NodeID)
	blocks := make([]*core.GraphBlock, 0)
	for id := rb.StartID; id <= rb.EndID; id++ {
		if _, ok := p.height2Digest[id]; !ok {
			fmt.Printf("主节点没有找到高度%d对应的区块摘要！\n", id)
		}
		if r, ok := p.messagePool[p.height2Digest[id]]; !ok {
			fmt.Printf("主节点没有找到高度%d对应的区块！\n", id)
			log.Panic()
		} else {
			encoded_block := r.Message.Content
			block := core.DecodeGraphBlock(encoded_block)
			blocks = append(blocks, block)
		}
	}
	p.SendBlocks(rb, blocks)

}
func (p *PShard_pbft) SendBlocks(rb *RequestBlocks, blocks []*core.GraphBlock) {
	s := SendBlocks{
		StartID:     rb.StartID,
		EndID:       rb.EndID,
		GraphBlocks: blocks,
		NodeID:      rb.ServerID,
	}
	bc, err := json.Marshal(s)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("正在向节点%s发送区块高度%d到%d的区块\n", rb.NodeID, s.StartID, s.EndID)
	message := jointMessage(cSendBlock, bc)
	go utils.TcpDial(message, p.nodeTable[rb.NodeID])
}

func (p *PShard_pbft) handleSendBlock(content []byte) {
	sb := new(SendBlocks)
	err := json.Unmarshal(content, sb)
	if err != nil {
		log.Panic(err)
	}
	p.graphLock.Lock()
	for id := sb.StartID; id <= sb.EndID; id++ {
		block := sb.Blocks[id-sb.StartID]
		for _, tx := range block.Transactions {
			var sender, recipient partition.Vertex
			sender.ConstructVertex(hex.EncodeToString(tx.Sender))
			recipient.ConstructVertex(hex.EncodeToString(tx.Recipient))
			p.Graph.AddEdge(sender, recipient)
			//fmt.Printf("id is %d : %s %s %d\n", the_id, hex.EncodeToString(tx.Sender), hex.EncodeToString(tx.Recipient), tx.Value)
		}
	}
	p.graphLock.Unlock()

}

//func (p *PShard_pbft) handleRelay(content []byte) {
//	sb := new(SendBlocks)
//	err := json.Unmarshal(content, sb)
//	if err != nil {
//		log.Panic(err)
//	}
//	fmt.Printf("节点%s%s已接收到%s发来的%d到%d的区块 \n", p.Node.shardID, p.Node.nodeID, sb.NodeID, sb.StartID, sb.EndID)
//	for id := sb.StartID; id <= sb.EndID; id++ {
//		p.Node.CurChain.AddBlock(sb.Blocks[id-sb.StartID])
//		fmt.Printf("%s%s : 编号为 %d 的区块已加入本地区块链！\n", p.Node.shardID, p.Node.nodeID, id)
//		curBlock := p.Node.CurChain.CurrentBlock
//		fmt.Printf("curBlock: \n")
//		curBlock.PrintBlock()
//	}
//	p.sequenceID = sb.EndID + 1
//	p.requestLock.Unlock()
//}

// 向除自己外的其他节点进行广播
func (p *PShard_pbft) broadcast(cmd command, content []byte) {
	for i := range p.nodeTable {
		if i == p.Node.nodeID {
			continue
		}
		//fmt.Printf("%s : The i is %s", p.Node.shardID, i)
		message := jointMessage(cmd, content)
		go utils.TcpDial(message, p.nodeTable[i])
	}
}

// 为多重映射开辟赋值
func (p *PShard_pbft) setPrePareConfirmMap(val, val2 string, b bool) {
	if _, ok := p.prePareConfirmCount[val]; !ok {
		p.prePareConfirmCount[val] = make(map[string]bool)
	}
	p.prePareConfirmCount[val][val2] = b
}

// 为多重映射开辟赋值
func (p *PShard_pbft) setCommitConfirmMap(val, val2 string, b bool) {
	if _, ok := p.commitConfirmCount[val]; !ok {
		p.commitConfirmCount[val] = make(map[string]bool)
	}
	p.commitConfirmCount[val][val2] = b
}
