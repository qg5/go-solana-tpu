package tpu

import "sync"

type TPUClientCache struct {
	peerLeaderNodes []string
	peerNodes       map[string]string
	stakedPeerNodes map[string]uint64
	currentSlot     uint64
	slotsInEpoch    uint64
	mu              sync.RWMutex
}

func (c *TPUClientCache) GetCurrentSlot() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSlot
}

func (c *TPUClientCache) SetCurrentSlot(slot uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSlot = slot
}
