// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtp

import (
	"encoding/binary"
	"fmt"
)

// Extension RTP Header extension.
type Extension struct {
	id      uint8
	payload []byte
}

// Header represents an RTP packet header.
type Header struct {
	Version          uint8
	Padding          bool
	Extension        bool
	Marker           bool
	PayloadType      uint8
	SequenceNumber   uint16
	Timestamp        uint32
	SSRC             uint32
	CSRC             []uint32
	ExtensionProfile uint16
	Extensions       []Extension

	// Deprecated: will be removed in a future version.
	PayloadOffset int
}

// Packet represents an RTP Packet.
type Packet struct {
	Header
	Payload     []byte
	PaddingSize byte

	// Deprecated: will be removed in a future version.
	Raw []byte
}

const (
	headerLength            = 4
	versionShift            = 6
	versionMask             = 0x3
	paddingShift            = 5
	paddingMask             = 0x1
	extensionShift          = 4
	extensionMask           = 0x1
	extensionProfileOneByte = 0xBEDE
	extensionProfileTwoByte = 0x1000
	extensionIDReserved     = 0xF
	ccMask                  = 0xF
	markerShift             = 7
	markerMask              = 0x1
	ptMask                  = 0x7F
	seqNumOffset            = 2
	seqNumLength            = 2
	timestampOffset         = 4
	timestampLength         = 4
	ssrcOffset              = 8
	ssrcLength              = 4
	csrcOffset              = 12
	csrcLength              = 4
)

// String helps with debugging by printing packet information in a readable way.
func (p Packet) String() string {
	out := "RTP PACKET:\n"

	out += fmt.Sprintf("\tVersion: %v\n", p.Version)
	out += fmt.Sprintf("\tMarker: %v\n", p.Marker)
	out += fmt.Sprintf("\tPayload Type: %d\n", p.PayloadType)
	out += fmt.Sprintf("\tSequence Number: %d\n", p.SequenceNumber)
	out += fmt.Sprintf("\tTimestamp: %d\n", p.Timestamp)
	out += fmt.Sprintf("\tSSRC: %d (%x)\n", p.SSRC, p.SSRC)
	out += fmt.Sprintf("\tPayload Length: %d\n", len(p.Payload))

	return out
}

// Unmarshal parses the passed byte slice and stores the result in the Header.
// It returns the number of bytes read n and any error.
func (h *Header) Unmarshal(buf []byte) (n int, err error) { //nolint:gocognit,cyclop
	if len(buf) < headerLength {
		return 0, fmt.Errorf("%w: %d < %d", errHeaderSizeInsufficient, len(buf), headerLength)
	}

	/*
	 *  0                   1                   2                   3
	 *  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |V=2|P|X|  CC   |M|     PT      |       sequence number         |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |                           timestamp                           |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |           synchronization source (SSRC) identifier            |
	 * +=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
	 * |            contributing source (CSRC) identifiers             |
	 * |                             ....                              |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 */

	h.Version = buf[0] >> versionShift & versionMask
	h.Padding = (buf[0] >> paddingShift & paddingMask) > 0
	h.Extension = (buf[0] >> extensionShift & extensionMask) > 0
	nCSRC := int(buf[0] & ccMask)
	if cap(h.CSRC) < nCSRC || h.CSRC == nil {
		h.CSRC = make([]uint32, nCSRC)
	} else {
		h.CSRC = h.CSRC[:nCSRC]
	}

	n = csrcOffset + (nCSRC * csrcLength)
	if len(buf) < n {
		return n, fmt.Errorf("size %d < %d: %w", len(buf), n,
			errHeaderSizeInsufficient)
	}

	h.Marker = (buf[1] >> markerShift & markerMask) > 0
	h.PayloadType = buf[1] & ptMask

	h.SequenceNumber = binary.BigEndian.Uint16(buf[seqNumOffset : seqNumOffset+seqNumLength])
	h.Timestamp = binary.BigEndian.Uint32(buf[timestampOffset : timestampOffset+timestampLength])
	h.SSRC = binary.BigEndian.Uint32(buf[ssrcOffset : ssrcOffset+ssrcLength])

	for i := range h.CSRC {
		offset := csrcOffset + (i * csrcLength)
		h.CSRC[i] = binary.BigEndian.Uint32(buf[offset:])
	}

	if h.Extensions != nil {
		h.Extensions = h.Extensions[:0]
	}

	if h.Extension { // nolint: nestif
		if expected := n + 4; len(buf) < expected {
			return n, fmt.Errorf("size %d < %d: %w",
				len(buf), expected,
				errHeaderSizeInsufficientForExtension,
			)
		}

		h.ExtensionProfile = binary.BigEndian.Uint16(buf[n:])
		n += 2
		extensionLength := int(binary.BigEndian.Uint16(buf[n:])) * 4
		n += 2
		extensionEnd := n + extensionLength

		if len(buf) < extensionEnd {
			return n, fmt.Errorf("size %d < %d: %w", len(buf), extensionEnd, errHeaderSizeInsufficientForExtension)
		}

		if h.ExtensionProfile == extensionProfileOneByte || h.ExtensionProfile == extensionProfileTwoByte {
			var (
				extid      uint8
				payloadLen int
			)

			for n < extensionEnd {
				if buf[n] == 0x00 { // padding
					n++

					continue
				}

				if h.ExtensionProfile == extensionProfileOneByte {
					extid = buf[n] >> 4
					payloadLen = int(buf[n]&^0xF0 + 1)
					n++

					if extid == extensionIDReserved {
						break
					}
				} else {
					extid = buf[n]
					n++

					if len(buf) <= n {
						return n, fmt.Errorf("size %d < %d: %w", len(buf), n, errHeaderSizeInsufficientForExtension)
					}

					payloadLen = int(buf[n])
					n++
				}

				if extensionPayloadEnd := n + payloadLen; len(buf) <= extensionPayloadEnd {
					return n, fmt.Errorf("size %d < %d: %w", len(buf), extensionPayloadEnd, errHeaderSizeInsufficientForExtension)
				}

				extension := Extension{id: extid, payload: buf[n : n+payloadLen]}
				h.Extensions = append(h.Extensions, extension)
				n += payloadLen
			}
		} else {
			// RFC3550 Extension
			extension := Extension{id: 0, payload: buf[n:extensionEnd]}
			h.Extensions = append(h.Extensions, extension)
			n += len(h.Extensions[0].payload)
		}
	}

	return n, nil
}

// Unmarshal parses the passed byte slice and stores the result in the Packet.
func (p *Packet) Unmarshal(buf []byte) error {
	n, err := p.Header.Unmarshal(buf)
	if err != nil {
		return err
	}

	end := len(buf)
	if p.Header.Padding {
		if end <= n {
			return errTooSmall
		}
		p.PaddingSize = buf[end-1]
		end -= int(p.PaddingSize)
	} else {
		p.PaddingSize = 0
	}
	if end < n {
		return errTooSmall
	}

	p.Payload = buf[n:end]

	return nil
}

// GetExtensionIDs returns an extension id array.
func (h *Header) GetExtensionIDs() []uint8 {
	if !h.Extension {
		return nil
	}

	if len(h.Extensions) == 0 {
		return nil
	}

	ids := make([]uint8, 0, len(h.Extensions))
	for _, extension := range h.Extensions {
		ids = append(ids, extension.id)
	}

	return ids
}

// GetExtension returns an RTP header extension.
func (h *Header) GetExtension(id uint8) []byte {
	if !h.Extension {
		return nil
	}
	for _, extension := range h.Extensions {
		if extension.id == id {
			return extension.payload
		}
	}

	return nil
}

// Clone returns a deep copy of p.
func (p Packet) Clone() *Packet {
	clone := &Packet{}
	clone.Header = p.Header.Clone()
	if p.Payload != nil {
		clone.Payload = make([]byte, len(p.Payload))
		copy(clone.Payload, p.Payload)
	}
	clone.PaddingSize = p.PaddingSize

	return clone
}

// Clone returns a deep copy h.
func (h Header) Clone() Header {
	clone := h
	if h.CSRC != nil {
		clone.CSRC = make([]uint32, len(h.CSRC))
		copy(clone.CSRC, h.CSRC)
	}
	if h.Extensions != nil {
		ext := make([]Extension, len(h.Extensions))
		for i, e := range h.Extensions {
			ext[i] = e
			if e.payload != nil {
				ext[i].payload = make([]byte, len(e.payload))
				copy(ext[i].payload, e.payload)
			}
		}
		clone.Extensions = ext
	}

	return clone
}
