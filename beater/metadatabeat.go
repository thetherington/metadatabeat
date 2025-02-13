package beater

import (
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/mapstr"

	"github.com/thetherington/metadatabeat/beater/connection"
	"github.com/thetherington/metadatabeat/beater/rtp"
	"github.com/thetherington/metadatabeat/beater/smpte211041"
	metadataCfg "github.com/thetherington/metadatabeat/config"
)

// metadatabeat configuration.
type metadatabeat struct {
	done            chan struct{}
	config          metadataCfg.Config
	client          beat.Client
	listener        *connection.UDPMulticastListener
	eventFieldMap   map[string]string
	payloadContents []uint32
	payloadLength   uint32
}

// New creates an instance of metadatabeat.
func New(b *beat.Beat, cfg *config.C) (beat.Beater, error) {
	c := metadataCfg.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// save the struct "es" tags
	m, err := parseStructTags(MdseventType{})
	if err != nil {
		return nil, err
	}

	bt := &metadatabeat{
		done:            make(chan struct{}),
		config:          c,
		eventFieldMap:   m,
		payloadContents: make([]uint32, 0),
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

	ml, err := connection.NewListener(bt.config, bt.MsgHandler)
	if err != nil {
		return err
	}

	bt.listener = ml

	<-bt.done

	// logp.Info("Event sent")
	return nil
}

// Stop stops metadatabeat.
func (bt *metadatabeat) Stop() {
	bt.listener.Close()
	bt.client.Close()

	close(bt.done)
}

func (bt *metadatabeat) MsgHandler(dst net.IP, src net.Addr, n int, b []byte) {
	data := b[:n]

	rtpPacket := &rtp.Packet{}

	if err := rtpPacket.Unmarshal(data); err != nil {
		log.Printf("error unmarshaling rtp packet: %v", err)
		return
	}

	// heartbeat packet
	// TODO: store sequence number
	if len(rtpPacket.Payload) < 8 {
		return
	}

	st2110pkt := &smpte211041.Payload{}

	if err := st2110pkt.UnmarshalSegmentation(rtpPacket.Payload); err != nil {
		log.Printf("error unmarshaling ST-2110-41 data: %v", err)
		return
	}

	// check the DIT if this is a Billboard XML Metadata packet
	if st2110pkt.DataItemType != smpte211041.SiriusXM_Billboard_XML {
		// return
	}

	// append data contents
	bt.payloadContents = append(bt.payloadContents, st2110pkt.SegmentContents...)

	// TODO: not sure what the segmentDataOffset value would be for more than 2 segments
	bt.payloadLength = st2110pkt.SegmentDataOffset + st2110pkt.NumDataItemContents

	// kbit indicates last packet in the segmentation
	if st2110pkt.KBit {
		// the payload should be finished based on the Segment Data Offset and Payload length
		if bt.payloadLength == uint32(len(bt.payloadContents)) {
			// start with xml header
			doc := []byte(fmt.Sprintf("%s\n", `<?xml version="1.0" encoding="UTF-8"?>`))

			// dump all the 32bit words into a byte slice
			doc = append(doc, smpte211041.ExpandContentsToBytes(bt.payloadContents)...)

			var metaDataPayload MdsUpdate

			if err := xml.Unmarshal(doc, &metaDataPayload); err != nil {
				log.Println("unmarshal xml failed", err)
			} else {
				// completed model
				events := bt.BuildEvents(metaDataPayload)

				bt.client.PublishAll(events)
				// fmt.Printf("%+v\n", metaDataPayload)
			}
		}

		// reset
		bt.payloadContents = make([]uint32, 0)
		bt.payloadLength = 0
	}
}

func (b *metadatabeat) BuildEvents(mdsupdate MdsUpdate) []beat.Event {
	var events []beat.Event

	for _, e := range mdsupdate.Ev {
		event := beat.Event{
			Timestamp: time.Now(),
			Fields: mapstr.M{
				"type":    "metadatabeat",
				"format":  mdsupdate.F,
				"version": mdsupdate.V,
			},
		}

		if mdsupdate.F == 1 {
			event.PutValue(b.eventFieldMap["Q"], e.Q)
			event.PutValue(b.eventFieldMap["Art"], e.Art)
			event.PutValue(b.eventFieldMap["Titl"], e.Titl)
			event.PutValue(b.eventFieldMap["Com1"], e.Com1)
			event.PutValue(b.eventFieldMap["Com2"], e.Com2)
			event.PutValue(b.eventFieldMap["Dur"], e.Dur)
			event.PutValue(b.eventFieldMap["Bc"], e.Bc)
			event.PutValue(b.eventFieldMap["Pts"], e.Pts)
			event.PutValue(b.eventFieldMap["SrcT"], e.SrcT)
			event.PutValue(b.eventFieldMap["AID"], e.AID)
			event.PutValue(b.eventFieldMap["ConT"], e.ConT)
			event.PutValue(b.eventFieldMap["MID"], e.MID)
			event.PutValue(b.eventFieldMap["ArtID"], e.ArtID)
			event.PutValue(b.eventFieldMap["Pid"], e.Pid)
			event.PutValue(b.eventFieldMap["Disp"], e.Disp)
			event.PutValue(b.eventFieldMap["Rid"], e.Rid)
			event.PutValue(b.eventFieldMap["Bdur"], e.Bdur)
			event.PutValue(b.eventFieldMap["Pldur"], e.Pldur)
		}

		events = append(events, event)
	}

	return events
}
