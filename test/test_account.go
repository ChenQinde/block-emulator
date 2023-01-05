package test

import (
	"blockEmulator/account"
	"blockEmulator/core"
	"fmt"
)

func Test_account() {
	for i := 0; i < 10; i++ {
		address := core.GenerateAddress()
		fmt.Printf("%v\n", address)
	}
}

func TestAccountAddandGet() {
	account.Init("S0", "NO")
	account1 := account.AddAccount()
	fmt.Println("account1 address :", account1.Address)
	account2 := account.GetAccount(account1.Address)
	fmt.Println("account2 address :", account2.Address)
}

func TestAccountGetAll() {
	account.Init("S0", "N0")
	accounts := account.GetAllAccount()
	fmt.Println(len(accounts))
	account.AddAccount()
	accounts = account.GetAllAccount()
	fmt.Println(len(accounts))
}

func TestGetAccountState() {
	account.Init("S0", "N0")
	fmt.Println(account.GetAccountState("0x380dde9af3602e85f05cf0fe56c2556aa4f1fb06").Balance)
}

func TestGenerateTransaction() {
	account.GenerateTransaction(5, 100, "S0", "N0")
}
