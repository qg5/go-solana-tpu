package literpc

type GetClusterNodes struct {
	Pubkey  string  `json:"pubkey"`
	TPUQUIC *string `json:"tpuQuic,omitempty"`
}

func (client *LiteRpcClient) GetClusterNodes() ([]GetClusterNodes, error) {
	var response []GetClusterNodes
	if err := client.call("getClusterNodes", nil, &response); err != nil {
		return nil, err
	}

	return response, nil
}
