package literpc

func (client *LiteRpcClient) GetSlot(commitment string) (uint64, error) {
	params := []interface{}{
		map[string]string{"commitment": commitment},
	}

	var response uint64
	if err := client.call("getSlot", params, &response); err != nil {
		return 0, err
	}

	return response, nil
}
