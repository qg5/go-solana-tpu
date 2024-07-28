package literpc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

const (
	MainnetBetaRPCURL = "https://api.mainnet-beta.solana.com"
	DevnetRPCURL      = "https://api.devnet.solana.com"
	TestnetRPCURL     = "https://api.testnet.solana.com"
)

type LiteRpcClient struct {
	endpoint string
}

func New(endpoint string) *LiteRpcClient {
	return &LiteRpcClient{
		endpoint: endpoint,
	}
}

type JsonRPCResponse struct {
	JsonRPC string          `json:"jsonrpc"`
	Id      int             `json:"int"`
	Result  json.RawMessage `json:"result"`
}

func (client *LiteRpcClient) call(method string, params interface{}, result interface{}) error {
	payload, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(client.endpoint, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var jsonResponse JsonRPCResponse
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return err
	}

	if err := json.Unmarshal(jsonResponse.Result, result); err != nil {
		return err
	}

	return nil
}
