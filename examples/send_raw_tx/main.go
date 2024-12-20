package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/blocto/solana-go-sdk/program/system"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/qg5/go-solana-tpu/literpc"
	"github.com/qg5/go-solana-tpu/tpu"
)

func main() {

	tx, err := createExampleTransaction("")
	if err != nil {
		log.Fatalf("failed to create tx: %v", err)
	}

	txSerialized, err := tx.Serialize()
	if err != nil {
		log.Fatalf("failed to serialize tx: %v", err)
	}

	tpuClient, err := tpu.New(literpc.DevnetRPCURL, nil)
	if err != nil {
		log.Fatalf("failed to initialize tpu client: %v", err)
	}

	// You may call tpuClient.Update() if you want to retrieve the latest values from the RPC

	signature := base58.Encode(tx.Signatures[0])

	if err := tpuClient.SendRawTransaction(txSerialized); err != nil {
		if errors.Is(err, tpu.ErrMaxRetries) {
			// Sometimes this error can happen even if the transaction was successful
			// Perhaps call 'getsignaturestatuses' (https://solana.com/docs/rpc/http/getsignaturestatuses)
			fmt.Println("Transaction sent (?):", signature)
			return
		}

		log.Fatalf("failed to send tx: %v", err)
	}

	fmt.Println("Transaction sent:", signature)
}

// https://github.com/blocto/solana-go-sdk/tree/main/docs/_examples/client/send-tx
func createExampleTransaction(privKeyBase58 string) (types.Transaction, error) {
	conn := rpc.New(literpc.DevnetRPCURL) // NOTE: Devnet is used

	resp, err := conn.GetLatestBlockhash(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return types.Transaction{}, err
	}

	feePayer, _ := types.AccountFromBase58(privKeyBase58)

	tx, err := types.NewTransaction(types.NewTransactionParam{
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        feePayer.PublicKey,
			RecentBlockhash: resp.Value.Blockhash.String(),
			Instructions: []types.Instruction{
				system.Transfer(system.TransferParam{
					From:   feePayer.PublicKey,
					To:     feePayer.PublicKey,
					Amount: 1,
				}),
			},
		}),
		Signers: []types.Account{feePayer},
	})
	if err != nil {
		return types.Transaction{}, err
	}

	return tx, nil
}
