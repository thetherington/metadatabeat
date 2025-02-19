package cache

import (
	"fmt"
	"sync"
)

type StoreMap map[string]*Group

type Group struct {
	MetadataPacket

	McastAddress  string
	SenderAddress string
	Port          int
	OrderFlag     bool

	Placeholder []*MetadataPacket

	Mu sync.Mutex
}

type MetadataPacket struct {
	LastSequence      uint16
	PayloadContents   []uint32
	PayloadLength     uint32
	SegmentDataOffset uint32
	Kbit              bool
}

func NewCacheStore(addresses []string, port int) StoreMap {
	cache := map[string]*Group{}

	for _, a := range addresses {
		cache[a] = NewGroup(a, port)
	}

	return cache
}

func NewGroup(address string, port int) *Group {
	g := &Group{
		McastAddress: address,
		Port:         port,
	}

	g.ResetContents()

	return g
}

func (sm StoreMap) Get(key string) (*Group, bool) {
	g, ok := sm[key]
	return g, ok
}

func (sm StoreMap) Put(address string, port int) *Group {
	sm[address] = NewGroup(address, port)

	return sm[address]
}

func (sm StoreMap) Delete(address string) {
	delete(sm, address)
}

func (g *Group) UpdateSequence(s uint16) error {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if !g.IsSequenceLast(s) {
		err := fmt.Errorf("missed heartbeat %s, old sequence %d vs. new sequence: %d", g.McastAddress, g.LastSequence, s)
		g.LastSequence = s
		return err
	}

	g.LastSequence = s
	return nil
}

func (g *Group) UpdateContents(pkt *MetadataPacket) (*MetadataPacket, error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	// out of order payload with an offset. save into placeholder
	if !g.IsSequenceLast(pkt.LastSequence) && pkt.SegmentDataOffset > 0 {
		g.Placeholder = append(g.Placeholder, pkt)
		g.OrderFlag = true

		return nil, fmt.Errorf("sequence out of order, pkt saved in placeholder. sequence %d", pkt.LastSequence)

	}

	// resemble the packet from the placeholder or add packet to placeholder
	if len(g.Placeholder) > 0 && g.OrderFlag {
		return nil, nil
	}

	// regular sequential data or start of new data
	g.LastSequence = pkt.LastSequence

	g.PayloadContents = append(g.PayloadContents, pkt.PayloadContents...)
	g.PayloadLength = pkt.SegmentDataOffset + pkt.PayloadLength
	g.Kbit = pkt.Kbit

	return &MetadataPacket{
		PayloadContents: g.PayloadContents,
		PayloadLength:   g.PayloadLength,
		Kbit:            g.Kbit,
	}, nil
}

func (g *Group) IsSequenceLast(s uint16) bool {
	if g.LastSequence == 0 {
		return true
	}

	if (s - g.LastSequence) == 1 {
		return true
	}

	return false
}

// Reset the Group content
func (g *Group) ResetContents() {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	g.OrderFlag = false
	g.PayloadLength = 0
	g.PayloadContents = make([]uint32, 0)
	g.Placeholder = make([]*MetadataPacket, 0)
	g.Kbit = false
}

// Helper function that validates if PayloadLength == len(PayloadContents)
func (g *Group) ContentsComplete() bool {
	return g.PayloadLength == uint32(len(g.PayloadContents))
}
