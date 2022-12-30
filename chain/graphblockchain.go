package chain

import (
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/partition"
	"blockEmulator/storage"
	"blockEmulator/trie"
	"fmt"
	"log"
	"sync"
	"time"
)

type GraphBlockChain struct {
	ChainConfig *params.ChainConfig // Chain configuration

	CurrentBlock *core.GraphBlock // Current head of the block chain

	Storage *storage.Storage

	StatusTrie *trie.Trie

	GraphTool []*partition.CLPAState

	graphToolLock sync.Mutex
}

func NewGraphBlockChain(chainConfig *params.ChainConfig) (*GraphBlockChain, error) {
	fmt.Printf("%v\n", chainConfig)
	bc := &GraphBlockChain{
		ChainConfig: chainConfig,
		Storage:     storage.NewStorage(chainConfig),
	}

	blockHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		if err.Error() == "newestBlockHash is not found" {
			genesisBlock := bc.NewGenesisGraphBlock()
			bc.AddGenesisGraphBlock(genesisBlock)
			return bc, nil
		}
		log.Panic()
	}
	block, err := bc.Storage.GetGraphBlock(blockHash)
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

func (bc *GraphBlockChain) AddGraph2Tool(graph *partition.CLPAState) {
	bc.graphToolLock.Lock()
	bc.GraphTool = bc.GraphTool[:0]
	bc.GraphTool = append(bc.GraphTool, graph)
	bc.graphToolLock.Unlock()
}
func (bc *GraphBlockChain) GenerateGraphBlock() *core.GraphBlock {
	graphs := bc.GraphTool
	blockHeader := &core.GraphBlockHeader{
		ParentHash: bc.CurrentBlock.Hash,
		Number:     bc.CurrentBlock.Header.Number + 1,
		Time:       uint64(time.Now().Unix()),
	}
	//fmt.Printf("GenerateBlock txs length is %d\n", len(txs))
	block := core.NewGraphBlock(blockHeader, graphs)

	mpt_tx := bc.buildTreeOfGraohs(graphs)
	block.Header.GraphHash = mpt_tx.Hash()

	mpt_state := bc.getUpdatedTreeOfState(graphs)
	block.Header.StateRoot = mpt_state.Hash()

	block.Hash = block.GetHash()

	return block
}
func (bc *GraphBlockChain) NewGenesisGraphBlock() *core.GraphBlock {
	graphblockHeader := &core.GraphBlockHeader{
		Number: 0,
		Time:   uint64(time.Date(2022, 05, 28, 17, 11, 0, 0, time.Local).Unix()),
	}

	graphs := make([]*partition.CLPAState, 0)
	graphblock := core.NewGraphBlock(graphblockHeader, graphs)

	stateTree := genesisGraphStateTree()
	bc.Storage.UpdateStateTree(stateTree)
	graphblock.Header.StateRoot = stateTree.Hash()

	mpt_tx := bc.buildTreeOfGraohs(graphs)
	graphblock.Header.GraphHash = mpt_tx.Hash()

	graphblock.Hash = graphblock.GetHash()

	return graphblock
}

func genesisGraphStateTree() *trie.Trie {
	trie := trie.NewTrie()
	//for i := 0; i < len(params.Init_addrs); i++ {
	//	address := params.Init_addrs[i]
	//	value := new(big.Int)
	//	value.SetString(params.Init_balance, 10)
	//	accountState := &core.AccountState{
	//		Balance: value,
	//	}
	//	hex_address, _ := hex.DecodeString(address)
	//	trie.Put(hex_address, accountState.Encode())
	//}
	return trie
}

func (bc *GraphBlockChain) buildTreeOfGraohs(graphs []*partition.CLPAState) *trie.Trie {
	trie := trie.NewTrie()
	for _, graph := range graphs {
		trie.Put(graph.GraphHash[:], graph.Encode())
	}
	return trie
}

func (bc *GraphBlockChain) AddGraphBlock(block *core.GraphBlock) {
	bc.Storage.AddGraphBlock(block)
	bc.StatusTrie = bc.getUpdatedTreeOfState(block.Graphs)
	bc.Storage.UpdateStateTree(bc.StatusTrie)
	bc.CurrentBlock = block
}

func (bc *GraphBlockChain) getUpdatedTreeOfState(graphs []*partition.CLPAState) *trie.Trie {

	stateTree := &trie.Trie{}
	trie.DeepCopy(stateTree, bc.StatusTrie)
	buff := make([]byte, 0)
	// 这个地方后面想办法再弄，最好是多个账户的签名
	for _, graph := range graphs {
		stateTree.Put(buff, graph.Encode())
	}
	return stateTree
}

func (bc *GraphBlockChain) AddGenesisGraphBlock(block *core.GraphBlock) {
	bc.Storage.AddGraphBlock(block)

	// 重新从数据库中获取最新内容
	newestBlockHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		log.Panic()
	}
	curBlock, err := bc.Storage.GetGraphBlock(newestBlockHash)
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

func (bc *GraphBlockChain) IsGraphBlockValid(block *core.GraphBlock) bool {
	// todo

	return true
}
