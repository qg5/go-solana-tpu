package literpc

func (client *Client) GetSlotLeaders(start uint64, limit uint64) ([]string, error) {
	params := []interface{}{start, limit}
	var response []string
	if err := client.call("getSlotLeaders", params, &response); err != nil {
		return nil, err
	}

	return response, nil
}
