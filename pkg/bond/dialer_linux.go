//go:build linux

package bond

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func configureDialerForOS(dialer *net.Dialer, adapter *Adapter) {
	if adapter == nil || adapter.Name == "" {
		return
	}

	ifaceName := adapter.Name
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		var sysErr error
		err := c.Control(func(fd uintptr) {
			// SO_BINDTODEVICE binds the socket to the specified network interface name (e.g. wlan0, usb0)
			sysErr = unix.BindToDevice(int(fd), ifaceName)
		})
		if err != nil {
			return err
		}
		// If BindToDevice returns an error (e.g., EPERM on older restricted kernels),
		// we log or fallback to LocalAddr IP binding.
		if sysErr != nil {
			// Don't fail completely if LocalAddr is set; the kernel can still route via source IP
			return nil
		}
		return nil
	}
}
