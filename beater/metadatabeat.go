package beater

import (
	"encoding/xml"
	"fmt"
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
	RootPool   pond.Pool
	GroupPools = map[string]pond.Pool{}
)

// metadatabeat configuration.
type metadatabeat struct {
	done     chan struct{}
	config   metadataCfg.Config
	client   beat.Client
	listener *connection.UDPMulticastListener
	cache    cache.StoreMap
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

	cfg := &connection.ListenerConfig{
		Port:       bt.config.Port,
		Addresses:  bt.config.Addresses,
		Interfaces: bt.config.Interfaces,
	}

	// start the multicast udp listener and subscribe to all addresses in the config
	bt.listener, err = connection.NewListener(cfg, bt.MsgHandler)
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
	// if the format is 0 then log and return
	if mdsupdate.F == 0 {
		logp.Warn("%s: format is missing or is explicity 0 for stream: %s", src, dst)
		return
	}

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

		// mandatory fields for format 1 and 2
		ev := mapstr.M{
			EventFieldMap["Q"]:    e.Q,    // queue_depth
			EventFieldMap["Pts"]:  e.Pts,  // pts
			EventFieldMap["SrcT"]: e.SrcT, // source_type
			EventFieldMap["ConT"]: e.ConT, // content_type
			EventFieldMap["Bc"]:   e.Bc,   // bus_code
			EventFieldMap["Disp"]: e.Disp, // display
		}

		// integer fields which can't do anything about default values
		ev.Put(EventFieldMap["Pldur"], e.Pldur) // playout_duration

		// optional fields for format 1 and 2
		updateMapString("AID", e.AID, ev)     // asset_id
		updateMapString("Art", e.Art, ev)     // artist
		updateMapString("Titl", e.Titl, ev)   // title
		updateMapString("Pid", e.Pid, ev)     // pid
		updateMapString("Com1", e.Com1, ev)   // comment_1
		updateMapString("Com2", e.Com2, ev)   // comment_2
		updateMapString("ArtID", e.ArtID, ev) // artist_id
		updateMapNumber("Dur", e.Dur, ev)     // duration
		updateMapString("MID", e.MID, ev)     // music_db_id
		updateMapString("Rid", e.Rid, ev)     // reconciliation_id
		updateMapNumber("Bt", e.Bt, ev)       // parent_type_id
		updateMapString("Bdur", e.Bdur, ev)   // parent_duration
		updateMapString("Bpn", e.Bpn, ev)     // parent_name
		updateMapNumber("Bct", e.Bct, ev)     // child_count
		updateMapNumber("Bi", e.Bi, ev)       // child_index
		updateMapString("Actn", e.Actn, ev)   // action
		updateMapString("Actp", e.Actp, ev)   // action_param
		updateMapNumber("Sgdur", e.Sgdur, ev) // segue_duration
		updateMapString("Upc", e.Upc, ev)     // upc
		updateMapString("Isrc", e.Isrc, ev)   // isrc

		// format 2 fields
		if mdsupdate.F == 2 {
			updateMapString("EID", e.EID, ev)     // sports_event_id
			updateMapString("Scor", e.Scor, ev)   // sports_score
			updateMapString("Xtitl", e.Xtitl, ev) // highband_title
			updateMapString("Xart", e.Xart, ev)   // highband_artist
			updateMapString("Stitl", e.Stitl, ev) // lowband_title
			updateMapString("Sart", e.Sart, ev)   // lowband_artist
			updateMapString("Wtitl", e.Wtitl, ev) // streaming_title
			updateMapString("Wart", e.Wart, ev)   // streaming_artist
			updateMapString("Hmod", e.Hmod, ev)   // hybrid_mod
			updateMapString("Eph", e.Eph, ev)     // ephemeral
			updateMapString("Xpid", e.Xpid, ev)   // highband_pid
			updateMapNumber("Soff", e.Soff, ev)   // start_offset
			updateMapString("ScID", e.ScID, ev)   // schedule_id
			updateMapString("Apid", e.Apid, ev)   // apg_sports_pids
		}

		event.PutValue("ev", ev)

		events = append(events, event)
	}

	bt.client.PublishAll(events)
}
