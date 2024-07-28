package literpc

import (
	"encoding/base64"
	"fmt"
)

type SimulateTransaction struct {
	Value *SimulateTransactionValue `json:"value"`
}

type SimulateTransactionValue struct {
	Err interface{} `json:"err,omitempty"`
}

func (client *LiteRpcClient) SimulateTransaction(transaction, commitment string) error {
	params := []interface{}{
		transaction,
		map[string]string{"encoding": "base64", "commitment": commitment},
	}

	var response SimulateTransaction
	if err := client.call("simulateTransaction", params, &response); err != nil {
		return err
	}

	if response.Value.Err != nil {
		return fmt.Errorf("simulation failed: %v", response.Value.Err)
	}

	return nil
}

func (client *LiteRpcClient) SimulateRawTransaction(serializedTx []byte, commitment string) error {
	encoded := base64.StdEncoding.EncodeToString(serializedTx)
	return client.SimulateTransaction(encoded, commitment)
}
