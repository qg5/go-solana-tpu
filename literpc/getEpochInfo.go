package literpc

type GetEpochInfoResult struct {
	SlotsInEpoch uint64 `json:"slotsInEpoch"`
}

func (client *Client) GetEpochInfo(commitment string) (GetEpochInfoResult, error) {
	params := []interface{}{
		map[string]string{"commitment": commitment},
	}

	var response GetEpochInfoResult
	if err := client.call("getEpochInfo", params, &response); err != nil {
		return GetEpochInfoResult{}, err
	}

	return response, nil
}
