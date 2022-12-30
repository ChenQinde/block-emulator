package core

import (
	"blockEmulator/partition"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

// var (
// 	EmptyRootHash = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
// )

// Header represents a block header in the Ethereum blockchain.
type GraphBlockHeader struct {
	ParentHash []byte `json:"parentHash"       gencodec:"required"`
	StateRoot  []byte `json:"stateRoot"        gencodec:"required"`
	GraphHash  []byte `json:"GraphsRoot"       gencodec:"required"`
	Number     int    `json:"number"           gencodec:"required"`
	Time       uint64 `json:"timestamp"        gencodec:"required"`
}

func (bh *GraphBlockHeader) Encode() []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(bh)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

func (bh *GraphBlockHeader) Hash() []byte {
	hash := sha256.Sum256(bh.Encode())
	return hash[:]
}

func DecodeGraphBlockHeader(to_decode []byte) *GraphBlockHeader {
	var graphblockHeader GraphBlockHeader

	decoder := gob.NewDecoder(bytes.NewReader(to_decode))
	err := decoder.Decode(&graphblockHeader)
	if err != nil {
		log.Panic(err)
	}

	return &graphblockHeader
}

func (bh *GraphBlockHeader) PrintBlockHeader() {
	vals := []interface{}{
		hex.EncodeToString(bh.ParentHash),
		hex.EncodeToString(bh.StateRoot),
		hex.EncodeToString(bh.GraphHash),
		bh.Number,
		bh.Time,
	}
	fmt.Printf("%v\n", vals)
}

type GraphBody struct {
	Graphs []*partition.CLPAState
}

// 区块结构
type GraphBlock struct {
	Header *GraphBlockHeader
	Graphs []*partition.CLPAState
	Hash   []byte
}

// core/types/graphblock.go
func NewGraphBlock(graphblockHeader *GraphBlockHeader, graphs []*partition.CLPAState) *GraphBlock {
	b := &GraphBlock{
		Header: graphblockHeader,
		Graphs: graphs,
	}
	return b
}

func (b *GraphBlock) PrintBlock() {
	fmt.Printf("graphblockHeader: \n")
	b.Header.PrintBlockHeader()
	fmt.Printf("graphs: \n")
	for _, graph := range b.Graphs {
		graph.PrintCLPA()
	}
	fmt.Printf("graphblockHash: \n")
	fmt.Printf("%v\n", hex.EncodeToString(b.Hash))
}

// special
func (b *GraphBlock) GetHash() []byte {
	return b.Header.Hash()
}

func (b *GraphBlock) Encode() []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(b)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

func DecodeGraphBlock(to_decode []byte) *GraphBlock {
	var graphblock GraphBlock

	decoder := gob.NewDecoder(bytes.NewReader(to_decode))
	err := decoder.Decode(&graphblock)
	if err != nil {
		log.Panic(err)
	}

	return &graphblock
}
