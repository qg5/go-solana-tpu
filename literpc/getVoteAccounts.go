package literpc

type VoteAccounts struct {
	NodePubkey     string `json:"nodePubkey,omitempty"`
	ActivatedStake uint64 `json:"activatedStake,omitempty"`
}

type GetVoteAccounts struct {
	Current []VoteAccounts `json:"current"`
}

func (client *Client) GetVoteAccounts() (GetVoteAccounts, error) {
	var response GetVoteAccounts
	if err := client.call("getVoteAccounts", nil, &response); err != nil {
		return GetVoteAccounts{}, err
	}

	return response, nil
}
