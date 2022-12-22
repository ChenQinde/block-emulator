package account

import (
	"blockEmulator/core"
	"blockEmulator/trie"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"github.com/boltdb/bolt"
	"log"
	"os"
)

var (
	pathDB = "record/S0_N0_blockchain_db"
)

const (
	blocksBucket    = "blocks"
	accountsBucket  = "accounts"
	stateTreeBucket = "stateTree"
)

type Account struct {
	PublicKey  *[]byte
	PrivateKey *[]byte
	AddrByte   *[]byte
	Address    string
}

// Init
//
//	@Description: 初始化账户模块
//	@param ShardID 分片id
//	@param NodeID 节点id
func Init(ShardID, NodeID string) {
	pathDB = "./record/" + ShardID + "_" + NodeID + "_" + "blockchain_db"
}

// Encode
//
//	@Description: 编码，用于数据存储
//	@receiver a 账户
//	@return []byte 账户信息的字节形式
func (a *Account) Encode() []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(a)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// DecodeAccount
//
//	@Description: 解码，用于数据读取
//	@param toDecode 账户信息的字节形式
//	@return *Account 账户对象
func DecodeAccount(toDecode []byte) *Account {
	var account Account
	decoder := gob.NewDecoder(bytes.NewReader(toDecode))
	err := decoder.Decode(&account)
	if err != nil {
		log.Panic(err)
	}
	return &account
}

// AddAccount
//
//	@Description: 添加账户
//	@return *Account 账户对象
func AddAccount() *Account {
	priKey, pubKey, addr := NewKeyPairAndAddress()
	addrByte, _ := hex.DecodeString(addr[2:])
	account := &Account{
		Address:    addr,
		PrivateKey: &priKey,
		PublicKey:  &pubKey,
		AddrByte:   &addrByte,
	}
	db, err := bolt.Open(pathDB, os.ModePerm, nil)
	if err != nil {
		panic(err)
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		accountsBucket, _ := tx.CreateBucketIfNotExists([]byte(accountsBucket))
		err := accountsBucket.Put(*account.AddrByte, account.Encode())
		if err != nil {
			log.Panic()
		}
		return nil
	})
	err = db.Close()
	if err != nil {
		return nil
	}
	return account
}

// GetAccount
//
//	@Description: 获取账户
//	@param address 地址
//	@return *Account 账户对象
func GetAccount(address string) *Account {
	addrByte, _ := hex.DecodeString(address[2:])
	db, _ := bolt.Open(pathDB, os.ModePerm, nil)
	account := new(Account)
	_ = db.View(func(tx *bolt.Tx) error {
		blockBucket := tx.Bucket([]byte(accountsBucket))
		if blockBucket == nil {
			return nil
		}
		account = DecodeAccount(blockBucket.Get(addrByte))
		return nil
	})
	err := db.Close()
	if err != nil {
		return nil
	}
	return account
}

// GetAllAccount
//
//	@Description:获取所有账户
//	@return []*Account 账户对象数组
func GetAllAccount() []*Account {
	var accounts []*Account
	db, _ := bolt.Open(pathDB, os.ModePerm, nil)
	_ = db.View(func(tx *bolt.Tx) error {
		blockBucket := tx.Bucket([]byte(accountsBucket))
		c := blockBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			accounts = append(accounts, DecodeAccount(v))
		}
		return nil
	})
	err := db.Close()
	if err != nil {
		return nil
	}

	return accounts
}

// ClearAccount
//
//	@Description: 清空账号信息
func ClearAccount() {
	db, err := bolt.Open(pathDB, os.ModePerm, nil)
	if err != nil {
		panic(err)
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket([]byte(accountsBucket))
		if err != nil {
			return err
		}
		return nil
	})
	err = db.Close()
	if err != nil {
		return
	}
}

// GetAccountState
//
//	@Description: 获取账号状态
//	@param address 账号地址
//	@return *core.AccountState 账号状态对象
func GetAccountState(address string) *core.AccountState {
	db, err := bolt.Open(pathDB, os.ModePerm, nil)
	accountState := new(core.AccountState)
	if err != nil {
		panic(err)
	}
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(stateTreeBucket))
		stateTreeData := b.Get([]byte(stateTreeBucket))
		if stateTreeData == nil {
			return errors.New("stateTree is not found")
		}
		stateTree := trie.DecodeStateTree(stateTreeData)
		key, _ := hex.DecodeString(address[2:])
		accountStateD, _ := stateTree.Get(key)
		if accountStateD == nil {
			return nil
		}
		accountState = core.DecodeAccountState(accountStateD)
		return nil
	})
	err = db.Close()
	if err != nil {
		return nil
	}
	return accountState
}

// GetBlockChainGraph
//
//	@Description: 账户图划分相关数据
//	@return map[Account][]Account 账户交易邻接表
//	@return map[Account]bool 账户是否交易
func GetBlockChainGraph() (map[Account][]Account, map[Account]bool) {
	db, err := bolt.Open(pathDB, os.ModePerm, nil)
	if err != nil {
		panic(err)
	}
	addrAccountMap := map[string]Account{}
	AccountSet := map[Account]bool{}
	AccountEdgeSet := map[Account][]Account{}

	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			block := core.DecodeBlock(v)
			for _, v := range block.Transactions {
				senderAddr := hex.EncodeToString(v.Sender)
				RecipientAddr := hex.EncodeToString(v.Recipient)
				senderAccount, ok1 := addrAccountMap[senderAddr]
				recipientAccount, ok2 := addrAccountMap[RecipientAddr]

				if !ok1 {
					senderAccount = Account{Address: senderAddr}
					addrAccountMap[senderAddr] = senderAccount
				}
				if !ok2 {
					recipientAccount = Account{Address: RecipientAddr}
					addrAccountMap[RecipientAddr] = recipientAccount
				}
				AccountSet[senderAccount] = true
				AccountSet[recipientAccount] = true
				AccountEdgeSet[senderAccount] = append(AccountEdgeSet[senderAccount], recipientAccount)
				AccountEdgeSet[recipientAccount] = append(AccountEdgeSet[recipientAccount], senderAccount)
			}
		}
		return nil
	})
	err = db.Close()
	if err != nil {
		return nil, nil
	}
	return AccountEdgeSet, AccountSet
}

// NewKeyPairAndAddress
//
//	@Description: 初始化账户数据
//	@return []byte 公私钥对
//	@return []byte 公私钥对
//	@return string 地址
func NewKeyPairAndAddress() ([]byte, []byte, string) {
	priKeyBytesLen := 32
	curve := elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}
	d := private.D.Bytes()
	b := make([]byte, 0, priKeyBytesLen)
	priKey := paddedAppend(uint(priKeyBytesLen), b, d)
	pubKey := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)
	publicSHA256 := sha256.Sum256(pubKey)
	addr := "0x" + hex.EncodeToString(publicSHA256[:20])
	return priKey, pubKey, addr
}

// paddedAppend appends the src byte slice to dst, returning the new slice.
// If the length of the source is smaller than the passed size, leading zero
// bytes are appended to the dst slice before appending src.
func paddedAppend(size uint, dst, src []byte) []byte {
	for i := 0; i < int(size)-len(src); i++ {
		dst = append(dst, 0)
	}
	return append(dst, src...)
}
