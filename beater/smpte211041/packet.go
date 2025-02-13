package smpte211041

import "fmt"

const (
	headerLength            int    = 4
	segmentDataOffsetLength int    = 4
	SiriusXM_Billboard_XML  uint32 = 2097152
	SiriusXM_SCTE104_JSON   uint32 = 2097153
	ExperimentStart         uint32 = 4190395
	ExperimentEnd           uint32 = 4194303
)

var RegisterDataItemTypes = make(map[uint32]string)

func init() {
	RegisterDataItemTypes[256] = "SMPTE ST 2127-2 metadata"

	RegisterDataItemTypes[1048832] = "IPMX (VSF)"
	RegisterDataItemTypes[1048833] = "IPMX (VSF)"
	RegisterDataItemTypes[1048834] = "IPMX (VSF)"
	RegisterDataItemTypes[1048835] = "IPMX (VSF)"
	RegisterDataItemTypes[1048836] = "IPMX (VSF)"
	RegisterDataItemTypes[1048837] = "IPMX (VSF)"
	RegisterDataItemTypes[1048838] = "IPMX (VSF)"
	RegisterDataItemTypes[1048839] = "IPMX (VSF)"
	RegisterDataItemTypes[1048840] = "IPMX (VSF)"
	RegisterDataItemTypes[1048841] = "IPMX (VSF)"
	RegisterDataItemTypes[1048842] = "IPMX (VSF)"
	RegisterDataItemTypes[1048843] = "IPMX (VSF)"
	RegisterDataItemTypes[1048844] = "IPMX (VSF)"
	RegisterDataItemTypes[1048845] = "IPMX (VSF)"
	RegisterDataItemTypes[1048846] = "IPMX (VSF)"
	RegisterDataItemTypes[1048847] = "IPMX (VSF)"

	RegisterDataItemTypes[1048832] = "VSF TR-10-10 (HDMI InfoFrame data)"
	RegisterDataItemTypes[2097152] = "SiriusXM Billboard XML"
	RegisterDataItemTypes[2097153] = "SiriusXM SCTE104 JSON"
}

type Payload struct {
	Header

	SegmentationPacket bool
	Payload            []byte
	SegmentContents    []uint32
	DataItemContents   []uint32
}

type Header struct {
	DataItemType            uint32
	DataItemTypeDescription string
	KBit                    bool
	NumDataItemContents     uint32
	SegmentDataOffset       uint32
}

func (p *Payload) String() string {
	out := "SMPTE 2110-41 Packet:\n"

	out += fmt.Sprintf("\tData Item Type: 0x%06X (%s) \n", p.DataItemType, p.DataItemTypeDescription)
	out += fmt.Sprintf("\tKBit Set: %v\n", p.KBit)
	out += fmt.Sprintf("\tNumber of Data Item Contents: %d\n", p.NumDataItemContents)

	if p.KBit {
		out += fmt.Sprintf("\tSegmentation Offset: 0x%06X\n", p.SegmentDataOffset)
	}

	return out
}

func (p *Header) Unmarshal(buf []byte) (n int, err error) {
	if len(buf) < headerLength {
		return 0, fmt.Errorf("%w: %d < %d", errHeaderSizeInsufficient, len(buf), headerLength)
	}

	/*
	 *  0                   1                   2                   3
	 *  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                 Data Item Type           |K| Data Item Length |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                      Segment Data Offfset                     |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                        Segment Contents                       |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                        *               *                      |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                        Segment Contents                       |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 */

	// Combine all 4 bytes into a single 32-bit value
	combined := (uint32(buf[0]) << 24) | (uint32(buf[1]) << 16) | (uint32(buf[2]) << 8) | uint32(buf[3])

	// Extract DataItemType (first 22 bits)
	p.DataItemType = (combined >> 10) & 0x3FFFFF

	p.DataItemTypeDescription = "none"

	if p.DataItemType >= ExperimentStart && p.DataItemType <= ExperimentEnd {
		p.DataItemTypeDescription = "Experimental"
	}

	if val, ok := RegisterDataItemTypes[p.DataItemType]; ok {
		p.DataItemTypeDescription = val
	}

	// Extract KBit (23rd bit)
	kBit := uint8((combined >> 9) & 0x1)
	if kBit == 1 {
		p.KBit = true
	}

	// Extract Payload Length (remaining 9 bits)
	p.NumDataItemContents = combined & 0x1FF

	return headerLength, nil
}

func (p *Payload) UnmarshalSegmentation(buf []byte) error {
	n, err := p.Header.Unmarshal(buf)
	if err != nil {
		return err
	}

	payload := buf[n:]

	if len(payload) < segmentDataOffsetLength {
		return fmt.Errorf("%w: %d < %d", errHeaderSizeInsufficientForOffset, len(payload), segmentDataOffsetLength)
	}

	// SMPTE Advisory Note for ST 2110-41:2024
	// The number of Segment Contents words within each Data Item Package shall be determined by subtracting 1 from the Data Item Length field.
	p.NumDataItemContents -= 1
	//

	offsetCombined := (uint32(payload[0]) << 24) | (uint32(payload[1]) << 16) | (uint32(payload[2]) << 8) | uint32(payload[3])
	p.SegmentDataOffset = offsetCombined

	p.Payload = payload[segmentDataOffsetLength:]

	p.SegmentContents = GroupPayloadToContents(p.Payload)
	p.SegmentationPacket = true

	if len(p.SegmentContents) < int(p.NumDataItemContents) {
		return fmt.Errorf("%w: %d < %d", errPayloadTooSmall, len(p.SegmentContents), p.NumDataItemContents)
	}

	return nil
}

func (p *Payload) Unmarshal(buf []byte) error {
	n, err := p.Header.Unmarshal(buf)
	if err != nil {
		return err
	}

	p.Payload = buf[n:]
	p.DataItemContents = GroupPayloadToContents(p.Payload)

	if len(p.DataItemContents) < int(p.NumDataItemContents) {
		return fmt.Errorf("%w: %d < %d", errPayloadTooSmall, len(p.DataItemContents), p.NumDataItemContents)
	}

	return nil
}

// GroupPayloadToContents groups a slice of bytes into 32-bit words
func GroupPayloadToContents(data []byte) []uint32 {
	var words []uint32
	for i := 0; i+3 < len(data); i += 4 {
		word := (uint32(data[i]) << 24) | (uint32(data[i+1]) << 16) | (uint32(data[i+2]) << 8) | uint32(data[i+3])
		words = append(words, word)
	}

	return words
}

// ExpandSegmentContentsToBytes converts 32-bit words back into a slice of bytes
func ExpandContentsToBytes(words []uint32) []byte {
	var bytes []byte
	for _, word := range words {
		bytes = append(bytes, byte(word>>24), byte(word>>16), byte(word>>8), byte(word))
	}
	return bytes
}
