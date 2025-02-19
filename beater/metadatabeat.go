package beater

import (
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/mapstr"

	"github.com/thetherington/metadatabeat/beater/cache"
	"github.com/thetherington/metadatabeat/beater/connection"
	"github.com/thetherington/metadatabeat/beater/rtp"
	"github.com/thetherington/metadatabeat/beater/smpte211041"
	metadataCfg "github.com/thetherington/metadatabeat/config"
)

var (
	RootPool      pond.Pool
	GroupPools    = map[string]pond.Pool{}
	EventFieldMap = map[string]string{}
)

// metadatabeat configuration.
type metadatabeat struct {
	done     chan struct{}
	config   metadataCfg.Config
	client   beat.Client
	listener *connection.UDPMulticastListener
	cache    cache.StoreMap
}

func init() {
	// save the struct "es" tags
	var err error
	if EventFieldMap, err = parseStructTags(MdseventType{}); err != nil {
		log.Fatal(err)
	}
}

// New creates an instance of metadatabeat.
func New(b *beat.Beat, cfg *config.C) (beat.Beater, error) {
	c := metadataCfg.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// global pool number of workers are number of addresses
	RootPool = pond.NewPool(len(c.Addresses))

	// create and map sub pools for each address
	for _, address := range c.Addresses {
		GroupPools[address] = RootPool.NewSubpool(1)
	}

	bt := &metadatabeat{
		done:   make(chan struct{}),
		config: c,
		cache:  cache.NewCacheStore(c.Addresses, c.Port),
	}

	return bt, nil
}

// Run starts metadatabeat.
func (bt *metadatabeat) Run(b *beat.Beat) error {
	logp.Info("metadatabeat is running! Hit CTRL-C to stop it.")

	var err error

	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}

	// start the multicast udp listener and subscribe to all addresses in the config
	bt.listener, err = connection.NewListener(bt.config, bt.MsgHandler)
	if err != nil {
		return err
	}

	// wait here until done channel is closed
	<-bt.done

	return nil
}

// Stop stops metadatabeat.
func (bt *metadatabeat) Stop() {
	// close udp listener port
	bt.listener.Close()

	// disconnect client
	bt.client.Close()

	// stop the worker pools
	RootPool.StopAndWait()

	close(bt.done)
}

// Handler function for every UDP datagram
func (bt *metadatabeat) MsgHandler(dst net.IP, src net.Addr, n int, b []byte) {
	group, ok := bt.cache.Get(dst.String())
	if !ok {
		logp.Err("missing multicast in group cache: %s", dst.String())
		return
	}

	pool := GroupPools[dst.String()]

	pool.Go(func() {
		data := b[:n]

		rtpPacket := &rtp.Packet{}

		if err := rtpPacket.Unmarshal(data); err != nil {
			logp.Err("%s: error unmarshaling rtp packet: %v", dst.String(), err)
			return
		}

		// heartbeat packet, update sequence then ignore
		if len(rtpPacket.Payload) == 0 {
			if err := group.UpdateSequence(rtpPacket.SequenceNumber); err != nil {
				logp.Err("%s: error updating sequence number for rtp packet: %v", dst.String(), err)
			}
			return
		}

		st2110pkt := &smpte211041.Payload{}

		if err := st2110pkt.UnmarshalSegmentation(rtpPacket.Payload); err != nil {
			logp.Err("%s: error unmarshaling ST-2110-41 data: %v", dst.String(), err)
			return
		}

		// check the DIT if this is a Billboard XML Metadata packet
		if st2110pkt.DataItemType != smpte211041.SiriusXM_Billboard_XML {
			logp.Err("%s: error DataItemType is not SiriusXM_Billboard_XML", dst.String())
			// return
		}

		mdsContents, err := group.UpdateContents(&cache.MetadataPacket{
			LastSequence:      rtpPacket.SequenceNumber,
			PayloadContents:   st2110pkt.SegmentContents,
			PayloadLength:     st2110pkt.NumDataItemContents,
			SegmentDataOffset: st2110pkt.SegmentDataOffset,
			Kbit:              st2110pkt.KBit,
		})
		// error if there's out of order and return
		if err != nil {
			logp.Debug("msg", "%v", err)
			return
		}

		// kbit indicates last packet in the segmentation
		if mdsContents.Kbit {
			// reset group
			defer group.ResetContents()

			// the payload should be finished based on the Segment Data Offset and Payload length
			if group.ContentsComplete() {
				// start with xml header
				doc := []byte(fmt.Sprintf("%s\n", `<?xml version="1.0" encoding="UTF-8"?>`))

				// dump all the 32bit words into a byte slice
				doc = append(doc,
					smpte211041.ExpandContentsToBytes(mdsContents.PayloadContents)...,
				)

				var mdsdata MdsUpdate

				if err := xml.Unmarshal(doc, &mdsdata); err != nil {
					logp.Err("%s: error unmarshaling XML: %v", dst.String(), err)
					return
				}

				// completed model, build beat events
				bt.BuildEvents(src.String(), dst.String(), mdsdata)
			}
		}
	})
}

// Event builder for beat events
func (bt *metadatabeat) BuildEvents(src string, dst string, mdsupdate MdsUpdate) {
	var events []beat.Event

	for _, e := range mdsupdate.Ev {
		event := beat.Event{
			Timestamp: time.Now(),
			Fields: mapstr.M{
				"type": "metadatabeat",
				"mds": mapstr.M{
					"format":  mdsupdate.F,
					"version": mdsupdate.V,
				},
				"node": mapstr.M{
					"src": strings.Split(src, ":")[0],
					"dst": dst,
				},
			},
		}

		if mdsupdate.F == 1 {
			ev := mapstr.M{}

			ev.Put(EventFieldMap["Q"], e.Q)
			ev.Put(EventFieldMap["Dur"], e.Dur)
			ev.Put(EventFieldMap["Disp"], e.Disp)
			ev.Put(EventFieldMap["Pldur"], e.Pldur)

			if e.Art != "" {
				ev.Put(EventFieldMap["Art"], e.Art)
			}
			if e.Titl != "" {
				ev.Put(EventFieldMap["Titl"], e.Titl)
			}
			if e.Com1 != "" {
				ev.Put(EventFieldMap["Com1"], e.Com1)
			}
			if e.Com2 != "" {
				ev.Put(EventFieldMap["Com2"], e.Com2)
			}
			if e.Bc != "" {
				ev.Put(EventFieldMap["Bc"], e.Bc)
			}
			if e.Pts != "" {
				ev.Put(EventFieldMap["Pts"], e.Pts)
			}
			if e.SrcT != "" {
				ev.Put(EventFieldMap["SrcT"], e.SrcT)
			}
			if e.AID != "" {
				ev.Put(EventFieldMap["AID"], e.AID)
			}
			if e.ConT != "" {
				ev.Put(EventFieldMap["ConT"], e.ConT)
			}
			if e.MID != "" {
				ev.Put(EventFieldMap["MID"], e.MID)
			}
			if e.ArtID != "" {
				ev.Put(EventFieldMap["ArtID"], e.ArtID)
			}
			if e.Pid != "" {
				ev.Put(EventFieldMap["Pid"], e.Pid)
			}
			if e.Rid != "" {
				ev.Put(EventFieldMap["Rid"], e.Rid)
			}
			if e.Bdur != "" {
				ev.Put(EventFieldMap["Bdur"], e.Bdur)
			}

			event.PutValue("ev", ev)
		}

		events = append(events, event)
	}

	bt.client.PublishAll(events)
}
