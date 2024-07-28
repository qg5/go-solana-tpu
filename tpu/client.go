package tpu

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/blocto/solana-go-sdk/types"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/qg5/go-solana-tpu/quic"
)

var (
	ErrNoLeadersFound         = errors.New("no leader found")
	ErrFailedSimulation       = errors.New("simulation failed")
	ErrMaxRetries             = errors.New("max retries were hit")
	ErrInvalidCommitment      = errors.New("invalid commitment")
	ErrUnsupportedTransaction = errors.New("this type of transaction is not supported")
)

type TPUClient struct {
	conn   *rpc.Client
	config *TPUClientConfig

	cache *TPUClientCache
}

type TPUClientConfig struct {
	// The amount of slots that will be checked (default: 100)
	MaxFanoutSlots uint64

	// The amount of attempts the program will do until it errors out (default: 5)
	MaxRetries int

	// The commitment to be used for the RPC calls (default "confirmed")
	Commitment rpc.CommitmentType

	// Whether you want only nodes with a stake or not (default: false)
	// This option is experimental as of now
	OnlyStaked bool

	// Whether to skip transaction simulation or not (default: false)
	SkipSimulation bool
}

type TPUClientCache struct {
	peerLeaderNodes []string
	peerNodes       map[string]string
	stakedPeerNodes map[string]uint64
	slotsInEpoch    uint64
}

// New creates a new tpu client and calls the Update method
//
// TPUClientConfig is configured with default values for maximum optimization.
func New(conn *rpc.Client, config *TPUClientConfig) (*TPUClient, error) {
	if config == nil {
		config = defaultTPUClientConfig()
	}

	if err := validateCommitment(config.Commitment); err != nil {
		return nil, err
	}

	tpuClient := &TPUClient{
		conn:   conn,
		config: config,
		cache: &TPUClientCache{
			peerNodes: make(map[string]string),
		},
	}

	if tpuClient.config.OnlyStaked {
		tpuClient.cache.stakedPeerNodes = make(map[string]uint64)
	}

	if err := tpuClient.Update(); err != nil {
		return nil, err
	}

	return tpuClient, nil
}

func defaultTPUClientConfig() *TPUClientConfig {
	return &TPUClientConfig{
		MaxFanoutSlots: 100,
		MaxRetries:     5,
		Commitment:     rpc.CommitmentConfirmed,
		OnlyStaked:     false,
		SkipSimulation: false,
	}
}

func validateCommitment(commitment rpc.CommitmentType) error {
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

		accounts, err := t.conn.GetVoteAccounts(context.Background(), &rpc.GetVoteAccountsOpts{})
		if err != nil {
			return err
		}

		for _, current := range accounts.Current {
			t.cache.stakedPeerNodes[current.NodePubkey.String()] = current.ActivatedStake
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

// SendRawTransaction sends a raw transaction to the tpu client
func (t *TPUClient) SendRawTransaction(serializedTx []byte) error {
	if !t.config.SkipSimulation {
		out, err := t.conn.SimulateRawTransactionWithOpts(context.Background(), serializedTx, &rpc.SimulateTransactionOpts{
			Commitment: t.config.Commitment,
		})

		if out.Value.Err != nil {
			return fmt.Errorf("%w: %v", ErrFailedSimulation, out.Value.Err)
		}

		if err != nil {
			return fmt.Errorf("%w: %v", ErrFailedSimulation, err)
		}
	}

	if err := t.getLeaderSockets(); err != nil {
		return err
	}

	return t.sendRawTransaction(serializedTx)
}

func (t *TPUClient) getEpochInfo() error {
	epochInfo, err := t.conn.GetEpochInfo(context.Background(), t.config.Commitment)
	if err != nil {
		return err
	}

	t.cache.slotsInEpoch = epochInfo.SlotsInEpoch
	return nil
}

func (t *TPUClient) getLeaderSockets() error {
	startSlot, err := t.conn.GetSlot(context.Background(), t.config.Commitment)
	if err != nil {
		return err
	}

	fanout := uint64(math.Min(float64(2*t.config.MaxFanoutSlots), float64(t.cache.slotsInEpoch)))
	slotLeaders, err := t.conn.GetSlotLeaders(context.Background(), startSlot, fanout)
	if err != nil {
		return err
	}

	t.filterValidLeaders(slotLeaders)
	if len(t.cache.peerLeaderNodes) == 0 {
		return ErrNoLeadersFound
	}

	return nil
}

func (t *TPUClient) getClusterNodes() error {
	clusters, err := t.conn.GetClusterNodes(context.Background())
	if err != nil {
		return err
	}

	isStakedCheck := len(t.cache.stakedPeerNodes) != 0

	for _, cluster := range clusters {
		if cluster.TPUQUIC == nil {
			continue
		}

		clusterPubKey := cluster.Pubkey.String()

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

func (t *TPUClient) sendRawTransaction(serializedTx []byte) error {
	var (
		wg             sync.WaitGroup
		currentRetries int
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, leaderAddr := range t.cache.peerLeaderNodes {
		wg.Add(1)

		go func(addr string) {
			defer wg.Done()

			err := quic.SendTransaction(ctx, addr, serializedTx)
			if err != nil {
				currentRetries++
			}

			if currentRetries > t.config.MaxRetries {
				cancel()
			}

		}(leaderAddr)
	}

	wg.Wait()

	if currentRetries > t.config.MaxRetries {
		return ErrMaxRetries
	}

	return nil
}

func (t *TPUClient) filterValidLeaders(slotLeaders []solana.PublicKey) {
	seenLeader := make(map[string]struct{})
	var leaderTPUSockets []string
	checkedSlots := 0

	for _, leader := range slotLeaders {
		leaderString := leader.String()
		if _, exists := seenLeader[leaderString]; !exists {
			seenLeader[leaderString] = struct{}{}
			if tpuSocket, ok := t.cache.peerNodes[leaderString]; ok {
				leaderTPUSockets = append(leaderTPUSockets, tpuSocket)
			}
		}

		checkedSlots++
		if checkedSlots >= int(t.config.MaxFanoutSlots) {
			break
		}
	}

	t.cache.peerLeaderNodes = leaderTPUSockets
}
