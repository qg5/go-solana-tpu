package tpu

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"

	"github.com/blocto/solana-go-sdk/types"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/qg5/go-solana-tpu/quic"
)

type TPUClient struct {
	conn   *rpc.Client
	config *TPUClientConfig

	cache *TPUClientCache
}

type TPUClientConfig struct {
	maxFanoutSlots uint64 // (default: 100)
	maxRetries     int    // (default: 5)
}

type TPUClientCache struct {
	leaders      []*net.UDPAddr
	leaderMap    map[string]*net.UDPAddr
	slotsInEpoch uint64
}

var (
	ErrNoLeadersFound         = errors.New("no leader found")
	ErrMaxRetries             = errors.New("max retries were hit")
	ErrUnsupportedTransaction = errors.New("this type of transaction is not supported")
)

// New creates a new tpu client and calls the Update method
func New(conn *rpc.Client, config *TPUClientConfig) (*TPUClient, error) {
	if config == nil {
		config = defaultTPUClientConfig()
	}

	tpuClient := &TPUClient{
		conn:   conn,
		config: config,
		cache: &TPUClientCache{
			leaderMap: make(map[string]*net.UDPAddr),
		},
	}

	if err := tpuClient.Update(); err != nil {
		return nil, err
	}

	return tpuClient, nil
}

func defaultTPUClientConfig() *TPUClientConfig {
	return &TPUClientConfig{
		maxFanoutSlots: 100,
		maxRetries:     5,
	}
}

// Update retrieves the latest epoch info and cluster nodes
func (t *TPUClient) Update() error {
	if err := t.getEpochInfo(); err != nil {
		return err
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
	if err := t.getLeaderSockets(); err != nil {
		return err
	}

	return t.sendRawTransaction(serializedTx)
}

func (t *TPUClient) getEpochInfo() error {
	epochInfo, err := t.conn.GetEpochInfo(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return err
	}

	t.cache.slotsInEpoch = epochInfo.SlotsInEpoch
	return nil
}

func (t *TPUClient) getLeaderSockets() error {
	startSlot, err := t.conn.GetSlot(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return err
	}

	fanout := uint64(math.Min(float64(2*t.config.maxFanoutSlots), float64(t.cache.slotsInEpoch)))
	slotLeaders, err := t.conn.GetSlotLeaders(context.Background(), startSlot, fanout)
	if err != nil {
		return err
	}

	t.filterValidLeaders(slotLeaders)
	if len(t.cache.leaders) == 0 {
		return ErrNoLeadersFound
	}

	return nil
}

func (t *TPUClient) getClusterNodes() error {
	clusters, err := t.conn.GetClusterNodes(context.Background())
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		if cluster.TPUQUIC != nil {
			addr, err := net.ResolveUDPAddr("udp", *cluster.TPUQUIC)
			if err != nil {
				continue
			}

			t.cache.leaderMap[cluster.Pubkey.String()] = addr
		}
	}

	return nil
}

func (t *TPUClient) sendRawTransaction(txBytes []byte) error {
	var (
		wg             sync.WaitGroup
		currentRetries int
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, leaderAddr := range t.cache.leaders {
		wg.Add(1)

		go func(addr *net.UDPAddr) {
			defer wg.Done()

			err := quic.SendTransaction(ctx, addr.String(), txBytes)
			if err != nil {
				currentRetries++
			}

			if currentRetries > t.config.maxRetries {
				cancel()
			}

		}(leaderAddr)
	}

	wg.Wait()

	if currentRetries > t.config.maxRetries {
		return ErrMaxRetries
	}

	return nil
}

func (t *TPUClient) filterValidLeaders(slotLeaders []solana.PublicKey) {
	seenLeader := make(map[string]struct{})
	var leaderTPUSockets []*net.UDPAddr
	checkedSlots := 0

	for _, leader := range slotLeaders {
		leaderString := leader.String()
		if _, exists := seenLeader[leaderString]; !exists {
			seenLeader[leaderString] = struct{}{}
			if tpuSocket, ok := t.cache.leaderMap[leaderString]; ok {
				leaderTPUSockets = append(leaderTPUSockets, tpuSocket)
			}
		}

		checkedSlots++
		if checkedSlots >= int(t.config.maxFanoutSlots) {
			break
		}
	}

	t.cache.leaders = leaderTPUSockets
}
