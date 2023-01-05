package pbft

import (
	"blockEmulator/broker"
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

// //本地消息池（模拟持久化层），只有确认提交成功后才会存入此池
// var localMessagePool = []Message{}

type node struct {
	//节点ID
	nodeID  string
	shardID string
	//节点监听地址
	addr          string
	CurChain      *chain.BlockChain
	CurGraphChain *chain.GraphBlockChain
}

type Pbft struct {
	//节点信息
	Node node
	//每笔请求自增序号
	sequenceID int
	//P分片的分片号
	PsharID string
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
	malicious_num int
	// 建立block、queue_length以及transaction的日记文件，也是只有主节点使用
	blocklog *csv.Writer
	txlog    *csv.Writer
	queuelog *csv.Writer

	// 以下为账户划分算法新增：
	mainNode string // 判断是否为pbft主节点
}

func NewPBFT(shardID int, nodeID int) *Pbft {
	config := params.ChainConfig{}
	config.ChainID = params.Config.ChainID
	config.Block_interval = params.Config.Block_interval
	config.MaxBlockSize = params.Config.MaxBlockSize
	config.MaxRelayBlockSize = params.Config.MaxRelayBlockSize
	config.MinRelayBlockSize = params.Config.MinRelayBlockSize
	config.Inject_speed = params.Config.Inject_speed
	config.Relay_interval = params.Config.Relay_interval
	config.Shard_num = params.Config.Shard_num
	p := new(Pbft)
	p.PsharID = fmt.Sprintf("S%d", config.Shard_num)
	p.Node.nodeID = fmt.Sprintf("N%d", nodeID)
	p.Node.shardID = fmt.Sprintf("S%d", shardID)
	//p.Node.addr = params.NodeTable[config.ShardID][config.NodeID]
	p.Node.addr = fmt.Sprintf("127.0.0.1:%d", 8201+shardID*100+nodeID)
	config.ShardID = fmt.Sprintf("S%d", shardID)
	config.NodeID = fmt.Sprintf("N%d", nodeID)
	//fmt.Println("config.NodeID ", config.NodeID)

	p.Node.CurChain, _ = chain.NewBlockChain(&config)
	fmt.Printf("Node S%dN%d and chain nodeid is %s\n", shardID, nodeID, p.Node.CurChain.ChainConfig.NodeID)
	p.sequenceID = p.Node.CurChain.CurrentBlock.Header.Number + 1
	p.messagePool = make(map[string]*Request)
	p.prePareConfirmCount = make(map[string]map[string]bool)
	p.commitConfirmCount = make(map[string]map[string]bool)
	p.isCommitBordcast = make(map[string]bool)
	p.isReply = make(map[string]bool)
	p.height2Digest = make(map[int]string)
	p.nodeTable = params.NodeTable[config.ShardID]
	p.Stop = make(chan int, 0)
	p.malicious_num = config.Malicious_num
	p.mainNode = "N0"
	return p
}

func (p *Pbft) InitPartitionMap(inputMap map[string]int) {
	p.Node.CurChain.InitPartitionMap(inputMap)
}

func NewLog(p *Pbft) {
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
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
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
	case gSendBlock:
		p.handleGraphBlock(content)

	// 以下为 账户迁移 相关代码
	case cHandleAccountTransfer:
		p.handle_AccountTransferMsg(content)
	//case cPartitionMsg:
	//	p.handle_PartitionMsg_FromMtoW(content)
	case cCTX2:
		p.handleCtx2(content)
	case cStop:
		p.Stop <- 1
	}
}

// 只有主节点可以调用
// 生成一个区块并发起共识
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

func (p *Pbft) propose() bool {
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

// 处理预准备消息
func (p *Pbft) handlePrePrepare(content []byte) {
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
	} else if !p.Node.CurChain.IsBlockValid(core.DecodeBlock(pp.RequestMessage.Message.Content)) {
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
func (p *Pbft) handlePrepare(content []byte) {
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
func (p *Pbft) handleCommit(content []byte) {
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
			block := core.DecodeBlock(encoded_block)
			p.Node.CurChain.AddBlock(block)
			fmt.Printf("%s%s : 编号为 %d 的区块已加入本地区块链！\n", p.Node.shardID, p.Node.nodeID, p.sequenceID)
			curBlock := p.Node.CurChain.CurrentBlock
			fmt.Printf("curBlock: \n")
			curBlock.PrintBlock()

			if p.Node.nodeID == "N0" {

				//todo 发送broker交易
				fmt.Println("已经上链的交易↓")
				for _, v := range block.Transactions {
					v.PrintTx()
				}
				ctx2 := broker.CTX1ToCTX2(block.Transactions)
				fmt.Println("ctx2交易如下↓")
				for _, v := range ctx2 {
					v.PrintTx()
				}
				sendBrokerCtx2Msg(ctx2)

				tx_total := len(block.Transactions)
				now := time.Now().Unix()
				relayCount := 0
				//已上链交易集
				commit_ids := []int{}
				//fmt.Println("block Transactions is ", block.Transactions)
				for _, v := range block.Transactions {
					// 若交易接收者属于本分片才加入已上链交易集
					//fmt.Println("Addr2Shard is ", utils.Addr2Shard(hex.EncodeToString(v.Recipient)), hex.EncodeToString(v.Recipient))
					if params.ShardTable[p.Node.shardID] == utils.Addr2Shard(hex.EncodeToString(v.Recipient)) {
						commit_ids = append(commit_ids, v.Id)
						s := fmt.Sprintf("%v %v %v %v %v", v.Id, block.Header.Number, v.RequestTime, now, now-v.RequestTime)
						p.txlog.Write(strings.Split(s, " "))
					}
					if utils.Addr2Shard(hex.EncodeToString(v.Sender)) != utils.Addr2Shard(hex.EncodeToString(v.Recipient)) {
						relayCount++
					}
				}
				p.txlog.Flush()
				s := fmt.Sprintf("%v %v %v %v %v %v", now, block.Header.Number, tx_total, tx_total-relayCount, tx_total-len(commit_ids), relayCount-tx_total+len(commit_ids))
				p.blocklog.Write(strings.Split(s, " "))
				p.blocklog.Flush()
				// 暂时不与客户端通讯了
				//主节点向客户端发送已确认上链的交易集
				fmt.Println("commit_ids is ", commit_ids)
				c, err := json.Marshal(commit_ids)
				if err != nil {
					log.Panic(err)
				}
				m := jointMessage(cReply, c)
				utils.TcpDial(m, params.ClientAddr)

				// 给P分片发送上链的区块
				p.SendBlocks2PShard(p.sequenceID)

				queue_len := len(p.Node.CurChain.Tx_pool.Queue)
				err = p.queuelog.Write([]string{fmt.Sprintf("%v", now), fmt.Sprintf("%v", queue_len)})
				if err != nil {
					log.Panic(err)
				}
				p.queuelog.Flush()
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

func (p *Pbft) SendBlocks2PShard(id int) {
	//return
	rb := RequestBlocks{
		StartID:  id,
		EndID:    id,
		ServerID: "N0",
		NodeID:   "N0",
	}

	fmt.Printf("节点%s%s准备向P分片的%s节点发送区块 ... \n", p.Node.shardID, p.Node.nodeID, rb.NodeID)
	blocks := make([]*core.Block, 0)
	if _, ok := p.height2Digest[id]; !ok {
		fmt.Printf("主节点发送给P分片时没有找到高度%d对应的区块摘要！\n", id)
	}
	if r, ok := p.messagePool[p.height2Digest[id]]; !ok {
		fmt.Printf("主节点发送给P分片时没有找到高度%d对应的区块！\n", id)
		log.Panic()
	} else {
		encoded_block := r.Message.Content
		block := core.DecodeBlock(encoded_block)
		blocks = append(blocks, block)
	}
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
	fmt.Printf("节点%s%s正在向P分片节点%s发送区块高度%d到%d的区块\n", p.Node.shardID, p.Node.nodeID, rb.NodeID, s.StartID, s.EndID, params.NodeTable[p.PsharID][rb.NodeID])
	message := jointMessage(cSendBlock, bc)
	go utils.TcpDial(message, params.NodeTable[p.PsharID][rb.NodeID])
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
	fmt.Printf("节点%s%s已接收到%s节点发来的 requestBlock ... \n", p.Node.shardID, p.Node.nodeID, rb.NodeID)
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
	fmt.Printf("节点%s%s已接收到%s发来的%d到%d的区块 \n", p.Node.shardID, p.Node.nodeID, sb.NodeID, sb.StartID, sb.EndID)
	for id := sb.StartID; id <= sb.EndID; id++ {
		p.Node.CurChain.AddBlock(sb.Blocks[id-sb.StartID])
		fmt.Printf("%s%s : 编号为 %d 的区块已加入本地区块链！\n", p.Node.shardID, p.Node.nodeID, id)
		curBlock := p.Node.CurChain.CurrentBlock
		fmt.Printf("curBlock: \n")
		curBlock.PrintBlock()
	}
	p.sequenceID = sb.EndID + 1
	p.requestLock.Unlock()
}
func (p *Pbft) handleGraphBlock(content []byte) {
	sb := new(SendBlocks)
	err := json.Unmarshal(content, sb)
	if err != nil {
		log.Panic(err)
	}
	graphblock := core.DecodeGraphBlock(sb.EncodeGraph)
	for _, graph := range graphblock.Graphs {
		fmt.Printf("Node%s%s get the GraphBlock\n", p.Node.shardID, p.Node.nodeID)
		graph.PrintCLPA()
	}
}

// 向除自己外的其他节点进行广播
func (p *Pbft) broadcast(cmd command, content []byte) {
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
func (p *Pbft) setPrePareConfirmMap(val, val2 string, b bool) {
	if _, ok := p.prePareConfirmCount[val]; !ok {
		p.prePareConfirmCount[val] = make(map[string]bool)
	}
	p.prePareConfirmCount[val][val2] = b
}

// 为多重映射开辟赋值
func (p *Pbft) setCommitConfirmMap(val, val2 string, b bool) {
	if _, ok := p.commitConfirmCount[val]; !ok {
		p.commitConfirmCount[val] = make(map[string]bool)
	}
	p.commitConfirmCount[val][val2] = b
}

// 返回一个十位数的随机数，作为msgid
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
			if k == p.Node.shardID || k == p.PsharID {
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

				fmt.Printf("%s%s正在向分片%v的主节点发送relay交易\n", p.Node.shardID, p.Node.nodeID, k)
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
	fmt.Printf("节点%s已接收到分片%v发来的relay交易 \n", p.Node.nodeID, relay.ShardID)
	p.Node.CurChain.Tx_pool.AddTxs(relay.Txs)
}

func (p *Pbft) handleCtx2(content []byte) {
	relay := new(Relay)
	err := json.Unmarshal(content, relay)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("节点%s已接收到分片%v发来的CTX2交易 \n", p.Node.nodeID, relay.ShardID)
	p.Node.CurChain.Tx_pool.AddTxs(relay.Txs)
}

func sendBrokerCtx2Msg(txs []*core.Transaction) {
	shard2Tx := make(map[string][]*core.Transaction)
	for _, v := range txs {
		sid := fmt.Sprintf("S%d", utils.Addr2Shard(hex.EncodeToString(v.Recipient)))
		shard2Tx[sid] = append(shard2Tx[sid], v)
	}
	for k, v := range shard2Tx {
		r := Relay{
			Txs:     v,
			ShardID: k,
		}
		bc, err := json.Marshal(r)
		if err != nil {
			log.Panic(err)
		}
		targetLeader := params.NodeTable[k]["N0"]
		fmt.Printf("正在向分片%v的主节点发送CTX2交易\n", k)
		message := jointMessage(cCTX2, bc)
		utils.TcpDial(message, targetLeader)
	}

}
