package chain

import (
	"blockEmulator/broker"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"time"

	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/storage"
	"blockEmulator/trie"
)

type BlockChain struct {
	ChainConfig *params.ChainConfig // Chain configuration

	CurrentBlock *core.Block // Current head of the block chain

	Storage *storage.Storage

	StatusTrie *trie.Trie

	Tx_pool *core.Tx_pool

	PartitionMap map[string]int // 划分表
}

func NewBlockChain(chainConfig *params.ChainConfig) (*BlockChain, error) {
	fmt.Printf("%v\n", chainConfig)
	bc := &BlockChain{
		ChainConfig: chainConfig,
		Storage:     storage.NewStorage(chainConfig),
		Tx_pool:     core.NewTxPool(),
	}

	bc.PartitionMap = make(map[string]int)

	blockHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		if err.Error() == "newestBlockHash is not found" {
			genesisBlock := bc.NewGenesisBlock()
			bc.AddGenesisBlock(genesisBlock)
			return bc, nil
		}
		log.Panic()
	}
	block, err := bc.Storage.GetBlock(blockHash)
	if err != nil {
		log.Panic()
	}
	bc.CurrentBlock = block
	stateTree, err := bc.Storage.GetStatusTree()
	if err != nil {
		log.Panic()
	}
	bc.StatusTrie = stateTree

	return bc, nil
}

func (bc *BlockChain) InitPartitionMap(inputMap map[string]int) {
	for addr := range inputMap {
		bc.PartitionMap[addr] = inputMap[addr]
	}
}

func (bc *BlockChain) AddBlock(block *core.Block) {
	bc.Storage.AddBlock(block)
	bc.StatusTrie = bc.getUpdatedTreeOfState(block.Transactions)
	bc.Storage.UpdateStateTree(bc.StatusTrie)

	// // 重新从数据库中获取最新内容
	// newestBlockHash, err := bc.Storage.GetNewestBlockHash()
	// if err != nil {
	// 	log.Panic()
	// }
	// curBlock, err := bc.Storage.GetBlock(newestBlockHash)
	// if err != nil {
	// 	log.Panic()
	// }
	bc.CurrentBlock = block
	// stateTree, err := bc.Storage.GetStatusTree()
	// if err != nil {
	// 	log.Panic()
	// }
	// bc.StatusTrie = stateTree

	// relay
	if bc.ChainConfig.NodeID == "N0" {
		//todo relay交易先注释掉
		//bc.genRelayTxs(block)
	}
}

func (bc *BlockChain) genRelayTxs(block *core.Block) {
	for _, tx := range block.Transactions {
		shardID := bc.PartitionMap[hex.EncodeToString(tx.Recipient)]
		if shardID != params.ShardTable[bc.ChainConfig.ShardID] {
			bc.Tx_pool.AddRelayTx(tx, params.ShardTableInt2Str[shardID])
		}
	}

}

func (bc *BlockChain) GenerateBlock() *core.Block {
	txs := bc.Tx_pool.FetchTxs2Pack()
	blockHeader := &core.BlockHeader{
		ParentHash: bc.CurrentBlock.Hash,
		Number:     bc.CurrentBlock.Header.Number + 1,
		Time:       uint64(time.Now().Unix()),
	}
	//fmt.Printf("GenerateBlock txs length is %d\n", len(txs))
	block := core.NewBlock(blockHeader, txs)

	mpt_tx := bc.buildTreeOfTxs(txs)
	block.Header.TxHash = mpt_tx.Hash()

	mpt_state := bc.getUpdatedTreeOfState(txs)
	block.Header.StateRoot = mpt_state.Hash()

	block.Hash = block.GetHash()

	return block
}

func (bc *BlockChain) buildTreeOfTxs(txs []*core.Transaction) *trie.Trie {
	trie := trie.NewTrie()
	for _, tx := range txs {
		trie.Put(tx.TxHash[:], tx.Encode())
	}
	return trie
}

func (bc *BlockChain) getUpdatedTreeOfState(txs []*core.Transaction) *trie.Trie {
	//stateTree := bc.preExecute(txs, bc)

	stateTree := &trie.Trie{}
	trie.DeepCopy(stateTree, bc.StatusTrie)

	for _, tx := range txs {
		// 确保发送地址属于此分片，即此交易不是其它分片发来的relay交易
		//if utils.Addr2Shard(hex.EncodeToString(tx.Sender)) == params.ShardTable[bc.ChainConfig.ShardID] {
		//fmt.Println("bc address is ", hex.EncodeToString(tx.Sender), " || ", bc.PartitionMap[hex.EncodeToString(tx.Sender)])
		//todo 发送方是broker
		if bc.PartitionMap[hex.EncodeToString(tx.Sender)] == params.ShardTable[bc.ChainConfig.ShardID] || broker.IsBroker(hex.EncodeToString(tx.Sender)) {
			decoded, success := stateTree.Get(tx.Sender)
			if !success {
				log.Panic()
			}
			account_state := core.DecodeAccountState(decoded)
			account_state.Deduct(tx.Value)
			stateTree.Put(tx.Sender, account_state.Encode())
		}

		// 接收地址不在此分片，不对该状态进行修改
		//if utils.Addr2Shard(hex.EncodeToString(tx.Recipient)) != params.ShardTable[bc.ChainConfig.ShardID] {
		//todo 状态更新，接收地址不在此分片，不对该状态进行修改，且不是broker
		if bc.PartitionMap[hex.EncodeToString(tx.Recipient)] != params.ShardTable[bc.ChainConfig.ShardID] && !broker.IsBroker(hex.EncodeToString(tx.Recipient)) {
			continue
		}
		decoded, success := stateTree.Get(tx.Recipient)
		if !success {
			log.Panic()
		}
		account_state := core.DecodeAccountState(decoded)
		account_state.Deposit(tx.Value)
		stateTree.Put(tx.Recipient, account_state.Encode())

	}
	return stateTree
}

func (bc *BlockChain) NewGenesisBlock() *core.Block {
	blockHeader := &core.BlockHeader{
		Number: 0,
		Time:   uint64(time.Date(2022, 05, 28, 17, 11, 0, 0, time.Local).Unix()),
	}

	txs := make([]*core.Transaction, 0)
	block := core.NewBlock(blockHeader, txs)

	stateTree := genesisStateTree()
	bc.Storage.UpdateStateTree(stateTree)
	block.Header.StateRoot = stateTree.Hash()

	mpt_tx := bc.buildTreeOfTxs(txs)
	block.Header.TxHash = mpt_tx.Hash()

	block.Hash = block.GetHash()

	return block
}

func (bc *BlockChain) AddGenesisBlock(block *core.Block) {
	bc.Storage.AddBlock(block)

	// 重新从数据库中获取最新内容
	newestBlockHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		log.Panic()
	}
	curBlock, err := bc.Storage.GetBlock(newestBlockHash)
	if err != nil {
		log.Panic()
	}
	bc.CurrentBlock = curBlock
	stateTree, err := bc.Storage.GetStatusTree()
	if err != nil {
		log.Panic()
	}
	bc.StatusTrie = stateTree
}

// 创世区块中初始化几个账户
func genesisStateTree() *trie.Trie {
	trie := trie.NewTrie()
	for i := 0; i < len(params.Init_addrs); i++ {
		address := params.Init_addrs[i]
		value := new(big.Int)
		value.SetString(params.Init_balance, 10)
		accountState := &core.AccountState{
			Balance: value,
		}
		hex_address, _ := hex.DecodeString(address)
		trie.Put(hex_address, accountState.Encode())
	}
	return trie
}

func GenerateTxs(bc *BlockChain) {
	tx_cnt := 2
	txs := make([]*core.Transaction, tx_cnt)
	for i := 0; i < tx_cnt; i++ {
		sender := core.GenerateAddress()
		receiver := core.GenerateAddress()
		txs[i] = &core.Transaction{
			Sender:    sender,
			Recipient: receiver,
			Value:     big.NewInt(int64(i + 1)),
		}
	}
	bc.Tx_pool.AddTxs(txs)
	bc.preExecute(txs, bc)
}

func (bc *BlockChain) preExecute(txs []*core.Transaction, chain *BlockChain) *trie.Trie {
	// stateTree, err := chain.Storage.GetStatusTree()
	// if err != nil {
	// 	log.Panic()
	// }
	stateTree := &trie.Trie{}
	trie.DeepCopy(stateTree, bc.StatusTrie)

	for _, tx := range txs {
		sender := tx.Sender
		//if utils.Addr2Shard(hex.EncodeToString(sender)) == params.ShardTable[bc.ChainConfig.ShardID] { // 发送地址在此分片，此交易不是其它分片发送过来的relay交易
		if bc.PartitionMap[hex.EncodeToString(sender)] == params.ShardTable[bc.ChainConfig.ShardID] { // 发送地址在此分片，此交易不是其它分片发送过来的relay交易
			if _, ok := stateTree.Get(sender); !ok {
				value := new(big.Int)
				value.SetString(params.Init_balance, 10)
				accountState := &core.AccountState{
					Balance: value,
				}
				stateTree.Put(sender, accountState.Encode())
			}
		}

		receiver := tx.Recipient
		//if utils.Addr2Shard(hex.EncodeToString(receiver)) != params.ShardTable[bc.ChainConfig.ShardID] { // 接收地址不在此分片
		if bc.PartitionMap[hex.EncodeToString(receiver)] != params.ShardTable[bc.ChainConfig.ShardID] { // 接收地址不在此分片
			continue
		}
		if _, ok := stateTree.Get(receiver); !ok {
			value := new(big.Int)
			value.SetString(params.Init_balance, 10)
			accountState := &core.AccountState{
				Balance: value,
			}
			stateTree.Put(receiver, accountState.Encode())
		}
	}
	// chain.Storage.UpdateStateTree(stateTree)
	return stateTree

}

func (bc *BlockChain) IsBlockValid(block *core.Block) bool {
	// todo

	return true
}
