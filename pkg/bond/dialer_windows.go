//go:build windows

package bond

import (
	"encoding/binary"
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	// IPPROTO_IP is the IPv4 socket option level
	IPPROTO_IP = 0
	// IP_UNICAST_IF specifies the outgoing interface for IPv4 traffic on Windows
	IP_UNICAST_IF = 31
)

func configureDialerForOS(dialer *net.Dialer, adapter *Adapter) {
	if adapter == nil {
		return
	}

	iface, err := net.InterfaceByName(adapter.Name)
	if err != nil {
		return
	}
	ifIndex := uint32(iface.Index)

	dialer.Control = func(network, address string, c syscall.RawConn) error {
		_ = c.Control(func(fd uintptr) {
			// On Windows, IP_UNICAST_IF accepts the network interface index in network byte order
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], ifIndex)
			_ = windows.Setsockopt(windows.Handle(fd), IPPROTO_IP, IP_UNICAST_IF, (*byte)(&buf[0]), 4)
		})
		return nil
	}
}
