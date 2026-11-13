//go:build tango_dev

package uihost

// ConnectDev connects this test process to a dev host as a worker.
func ConnectDev(addr, dir string) error {
	dev.mu.Lock()
	devExit = func() {}
	dev.mu.Unlock()
	return connectDev(addr, dir)
}

// ResetDev drops the worker connection and state.
func ResetDev() {
	dev.mu.Lock()
	cl := dev.cl
	dev.cl, dev.app, dev.wins, dev.seq, dev.proxies, dev.hidden = nil, nil, nil, 0, nil, false
	dev.snap, dev.snapLoaded, dev.early, dev.menus = nil, false, nil, nil
	dev.gen++ // drop late messages of the closed connection
	dev.mu.Unlock()
	dev.persist = devRegistry()
	if cl != nil {
		cl.Close()
	}
}
