package connection

import (
	"fmt"
	"log"
	"net"

	metadataCfg "github.com/thetherington/metadatabeat/config"
	"golang.org/x/net/ipv4"
)

const (
	maxDatagramSize = 8192
)

type handler func(net.IP, net.Addr, int, []byte)

type UDPMulticastListener struct {
	p      *ipv4.PacketConn
	iifs   []*net.Interface
	mcasts []net.IP
}

// Listen binds to the UDP address and port given and writes packets received
// from that address to a buffer which is passed to a hander
func NewListener(cfg metadataCfg.Config, handler handler) (*UDPMulticastListener, error) {
	if len(cfg.Addresses) == 0 || len(cfg.Interfaces) == 0 {
		return nil, fmt.Errorf("%w: %d addresses / %d multicasts", errMissingArguments, len(cfg.Addresses), len(cfg.Interfaces))
	}

	uml := &UDPMulticastListener{
		iifs:   make([]*net.Interface, 0),
		mcasts: make([]net.IP, 0),
	}

	// parse the interfaces and validate
	for _, i := range cfg.Interfaces {
		iif, err := net.InterfaceByName(i)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errUnknownInterface, i)
		}

		uml.iifs = append(uml.iifs, iif)
	}

	// parse and validate the multicast address
	for _, m := range cfg.Addresses {
		group := net.ParseIP(m)

		if group == nil || !group.IsMulticast() {
			return nil, fmt.Errorf("%w: %s", errInvalidMulticast, m)
		}

		uml.mcasts = append(uml.mcasts, group)
	}

	c, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	uml.p = ipv4.NewPacketConn(c)

	for _, iif := range uml.iifs {
		for _, mcast := range uml.mcasts {
			if err := uml.p.JoinGroup(iif, &net.UDPAddr{IP: mcast}); err != nil {
				return nil, fmt.Errorf("%w", err)
			}
		}
	}

	if err := uml.p.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	// start reading
	go func() {
		// loop forever
		for {
			buffer := make([]byte, maxDatagramSize)

			numBytes, cm, src, err := uml.p.ReadFrom(buffer)
			if err != nil {
				log.Println("ReadFromUDP failed:", err)
				return
			}

			if cm.Dst.IsMulticast() {
				if uml.IsSubcribed(cm) {
					// worker pool?
					handler(cm.Dst, src, numBytes, buffer)
				} else {
					continue
				}
			}
		}
	}()

	return uml, nil
}

func (u *UDPMulticastListener) IsSubcribed(cm *ipv4.ControlMessage) bool {
	for _, m := range u.mcasts {
		if cm.Dst.Equal(m) {
			return true
		}
	}

	return false
}

func (u *UDPMulticastListener) Close() error {
	for _, iif := range u.iifs {
		for _, mcast := range u.mcasts {
			if err := u.p.LeaveGroup(iif, &net.UDPAddr{IP: mcast}); err != nil {
				log.Println(err)
			}
		}
	}

	return u.p.Close()
}
