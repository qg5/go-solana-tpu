package tpu

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/types"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/qg5/go-solana-tpu/literpc"
	"github.com/qg5/go-solana-tpu/quic"
)

var (
	ErrNoLeadersFound         = errors.New("no leader found")
	ErrMaxRetries             = errors.New("max retries were hit")
	ErrInvalidCommitment      = errors.New("invalid commitment")
	ErrMaxTxSize              = errors.New("transaction too big")
	ErrMaxFanoutSlots         = errors.New("the fanout slots exceed the max (100)")
	ErrUnsupportedTransaction = errors.New("this type of transaction is not supported")
)

const (
	// Maximum number of slots used to build TPU socket fanout set.
	// https://github.com/solana-labs/solana/blob/27eff8408b7223bb3c4ab70523f8a8dca3ca6645/tpu-client/src/tpu_client.rs#L39
	MAX_FANOUT_SLOTS = 100

	// Defines the maximum transaction size that can be sent.
	// https://github.com/solana-labs/solana/blob/master/sdk/src/packet.rs#L21
	MAX_TX_SIZE = 1232

	// This defines how long a new slot takes to appear, by default Solana has a new one created every 400ms
	DEFAULT_SLOT_CREATION_INTERVAL = 400 * time.Millisecond

	DEFAULT_FANOUT_SLOTS = 12
)

type TPUClient struct {
	rpc    *literpc.Client
	config *TPUClientConfig
	cache  *TPUClientCache
	mu     sync.Mutex
}

type TPUClientConfig struct {
	// The range of upcoming slots to include when determining which
	// leaders to send transactions to (default: 12, max: 100)
	FanoutSlots int

	// The amount of attempts the program will do until it errors out (default: 5)
	// You can set the value to 0 to remove the retry limit
	MaxRetries int

	// The commitment to be used for the RPC calls (default "confirmed")
	Commitment string

	// Whether you want only nodes with a stake or not (default: false)
	// This option is experimental as of now
	OnlyStaked bool

	// Whether to skip transaction simulation or not (default: false)
	SkipSimulation bool
}

// New creates a new tpu client and calls the Update method
//
// TPUClientConfig is configured with default values for maximum optimization.
func New(rpcURL string, config *TPUClientConfig) (*TPUClient, error) {
	config = populateConfig(config)

	if config.FanoutSlots > MAX_FANOUT_SLOTS {
		return nil, ErrMaxFanoutSlots
	}

	if err := validateCommitment(config.Commitment); err != nil {
		return nil, err
	}

	rpc := literpc.New(rpcURL)

	tpuClient := &TPUClient{
		rpc:    rpc,
		config: config,
		cache: &TPUClientCache{
			peerNodes: make(map[string]string),
		},
	}

	if tpuClient.config.OnlyStaked {
		tpuClient.cache.stakedPeerNodes = make(map[string]uint64)
	}

	go tpuClient.startSlotUpdates()

	if err := tpuClient.Update(); err != nil {
		return nil, err
	}

	return tpuClient, nil
}

func populateConfig(config *TPUClientConfig) *TPUClientConfig {
	if config == nil {
		return defaultConfig()
	}

	if config.FanoutSlots == 0 {
		config.FanoutSlots = DEFAULT_FANOUT_SLOTS
	}

	if config.Commitment == "" {
		config.Commitment = "confirmed"
	}

	return config
}

func defaultConfig() *TPUClientConfig {
	return &TPUClientConfig{
		FanoutSlots:    DEFAULT_FANOUT_SLOTS,
		MaxRetries:     5,
		Commitment:     "confirmed",
		OnlyStaked:     false,
		SkipSimulation: false,
	}
}

func validateCommitment(commitment string) error {
	switch commitment {
	case "processed", "confirmed", "finalized":
		return nil
	default:
		return ErrInvalidCommitment
	}
}

// Update retrieves the latest epoch info and cluster nodes
func (t *TPUClient) Update() error {
	if err := t.getEpochInfo(); err != nil {
		return err
	}

	if t.config.OnlyStaked {
		if err := t.getStakedNodes(); err != nil {
			return err
		}
	}

	if err := t.getClusterNodes(); err != nil {
		return err
	}

	return nil
}

// SendTransaction currently only supports blocto and gagliardetto transactions
//
// This function will serialize the transaction to bytes and return the signature or an error
func (t *TPUClient) SendTransaction(tx any) (string, error) {
	var (
		signature    string
		err          error
		serializedTx []byte
	)

	switch v := tx.(type) {
	case solana.Transaction:
		serializedTx, err = v.MarshalBinary()
		if err != nil {
			return "", err
		}

		signature = v.Signatures[0].String()
	case types.Transaction:
		serializedTx, err = v.Serialize()
		if err != nil {
			return "", err
		}

		signature = base58.Encode(v.Signatures[0])
	default:
		return "", ErrUnsupportedTransaction
	}

	if err = t.SendRawTransaction(serializedTx); err != nil {
		return "", err
	}

	return signature, nil
}

// SendRawTransaction sends a raw transaction to the leaders
func (t *TPUClient) SendRawTransaction(serializedTx []byte) error {
	if len(serializedTx) > MAX_TX_SIZE {
		return ErrMaxTxSize
	}

	if !t.config.SkipSimulation {
		err := t.rpc.SimulateRawTransaction(serializedTx, t.config.Commitment)
		if err != nil {
			return err
		}
	}

	if err := t.getLeaderSockets(); err != nil {
		return err
	}

	return t.sendRawTransaction(serializedTx)
}

func (t *TPUClient) getEpochInfo() error {
	epochInfo, err := t.rpc.GetEpochInfo(t.config.Commitment)
	if err != nil {
		return err
	}

	t.cache.slotsInEpoch = epochInfo.SlotsInEpoch
	return nil
}

func (t *TPUClient) getLeaderSockets() error {
	fanout := math.Min(float64(2*t.config.FanoutSlots), float64(t.cache.slotsInEpoch))
	slotLeaders, err := t.rpc.GetSlotLeaders(t.cache.GetCurrentSlot(), uint64(fanout))
	if err != nil {
		return err
	}

	t.cache.peerLeaderNodes = t.filterValidLeaders(slotLeaders)
	if len(t.cache.peerLeaderNodes) == 0 {
		return ErrNoLeadersFound
	}

	return nil
}

func (t *TPUClient) getClusterNodes() error {
	clusters, err := t.rpc.GetClusterNodes()
	if err != nil {
		return err
	}

	isStakedCheck := len(t.cache.stakedPeerNodes) != 0

	for _, cluster := range clusters {
		if cluster.TPUQUIC == nil {
			continue
		}

		clusterPubKey := cluster.Pubkey

		if isStakedCheck {
			if _, ok := t.cache.stakedPeerNodes[clusterPubKey]; ok {
				t.cache.peerNodes[clusterPubKey] = *cluster.TPUQUIC
			}
		} else {
			t.cache.peerNodes[clusterPubKey] = *cluster.TPUQUIC
		}
	}

	return nil
}

func (t *TPUClient) getStakedNodes() error {
	accounts, err := t.rpc.GetVoteAccounts()
	if err != nil {
		return err
	}

	for _, current := range accounts.Current {
		t.cache.stakedPeerNodes[current.NodePubkey] = current.ActivatedStake
	}

	return nil
}

func (t *TPUClient) sendRawTransaction(serializedTx []byte) error {
	var (
		wg             sync.WaitGroup
		currentRetries int
	)

	if t.config.MaxRetries == 0 {
		t.config.MaxRetries = len(t.cache.peerLeaderNodes) + 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, leaderAddr := range t.cache.peerLeaderNodes {
		wg.Add(1)

		go func(addr string) {
			defer wg.Done()
			if quic.SendTransaction(ctx, addr, serializedTx) != nil {
				t.mu.Lock()
				currentRetries++
				t.mu.Unlock()
			}

			t.mu.Lock()
			if currentRetries > t.config.MaxRetries {
				cancel()
			}
			t.mu.Unlock()
		}(leaderAddr)
	}

	wg.Wait()
	if currentRetries > t.config.MaxRetries {
		return ErrMaxRetries
	}

	return nil
}

func (t *TPUClient) startSlotUpdates() error {
	startSlot, err := t.rpc.GetSlot(t.config.Commitment)
	if err != nil {
		return err
	}

	t.cache.SetCurrentSlot(startSlot)
	incrementedSlot := startSlot

	ticker := time.NewTicker(DEFAULT_SLOT_CREATION_INTERVAL)
	defer ticker.Stop()

	for range ticker.C {
		incrementedSlot++
		t.cache.SetCurrentSlot(incrementedSlot)
	}

	return nil
}

func (t *TPUClient) filterValidLeaders(slotLeaders []string) []string {
	var peerLeaderNodes []string
	seenLeader := make(map[string]struct{}, len(slotLeaders))

	for i, leader := range slotLeaders {
		if i >= t.config.FanoutSlots {
			break
		}

		if _, seen := seenLeader[leader]; !seen {
			seenLeader[leader] = struct{}{}
			if tpuSocket, ok := t.cache.peerNodes[leader]; ok {
				peerLeaderNodes = append(peerLeaderNodes, tpuSocket)
			}
		}
	}

	return peerLeaderNodes
}
