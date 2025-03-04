package connection

import (
	"fmt"
	"log"
	"net"

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

type ListenerConfig struct {
	Port       int
	Addresses  []string
	Interfaces []string
}

// Listen binds to the UDP address and port given and writes packets received
// from that address to a buffer which is passed to a hander
func NewListener(cfg *ListenerConfig, handler handler) (*UDPMulticastListener, error) {
	if len(cfg.Addresses) == 0 || len(cfg.Interfaces) == 0 {
		return nil, fmt.Errorf("%w: %d addresses / %d multicasts", errMissingArguments, len(cfg.Addresses), len(cfg.Interfaces))
	}

	lis := &UDPMulticastListener{
		iifs:   make([]*net.Interface, 0),
		mcasts: make([]net.IP, 0),
	}

	// parse the interfaces and validate
	for _, i := range cfg.Interfaces {
		iif, err := net.InterfaceByName(i)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errUnknownInterface, i)
		}

		lis.iifs = append(lis.iifs, iif)
	}

	// parse and validate the multicast address
	for _, m := range cfg.Addresses {
		group := net.ParseIP(m)

		if group == nil || !group.IsMulticast() {
			return nil, fmt.Errorf("%w: %s", errInvalidMulticast, m)
		}

		lis.mcasts = append(lis.mcasts, group)
	}

	c, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	lis.p = ipv4.NewPacketConn(c)

	for _, iif := range lis.iifs {
		for _, mcast := range lis.mcasts {
			if err := lis.p.JoinGroup(iif, &net.UDPAddr{IP: mcast}); err != nil {
				return nil, fmt.Errorf("%w", err)
			}
		}
	}

	if err := lis.p.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	// start reading and loop forever
	go func() {
		for {
			buffer := make([]byte, maxDatagramSize)

			numBytes, cm, src, err := lis.p.ReadFrom(buffer)
			if err != nil {
				log.Println("ReadFromUDP failed:", err)
				return
			}

			if cm.Dst.IsMulticast() && lis.IsSubcribed(cm) {
				handler(cm.Dst, src, numBytes, buffer)
			}
		}
	}()

	return lis, nil
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
