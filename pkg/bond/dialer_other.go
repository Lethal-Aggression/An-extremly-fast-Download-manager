//go:build !linux && !windows

package bond

import (
	"net"
)

func configureDialerForOS(dialer *net.Dialer, adapter *Adapter) {
	// Standard OS fallback: dialer.LocalAddr is already configured
}
