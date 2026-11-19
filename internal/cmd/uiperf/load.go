package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func parseLoadavg(s string) (float64, error) {
	s = strings.TrimSpace(strings.Trim(s, "{}"))
	f := strings.Fields(s)
	if len(f) < 1 {
		return 0, fmt.Errorf("invalid loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}
func systemLoad() (float64, error) {
	o, e := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if e != nil {
		return 0, e
	}
	return parseLoadavg(string(o))
}
func loadGate(read func() (float64, error), sleep func(time.Duration), max float64, waitMax time.Duration) error {
	return loadGateClock(read, sleep, time.Now, max, waitMax)
}
func loadGateClock(read func() (float64, error), sleep func(time.Duration), now func() time.Time, max float64, waitMax time.Duration) error {
	start := now()
	for {
		v, e := read()
		if e != nil {
			return e
		}
		if v < max {
			return nil
		}
		if now().Sub(start) >= waitMax {
			return fmt.Errorf("load %.2f remains at or above %.2f after %s", v, max, waitMax)
		}
		sleep(30 * time.Second)
	}
}
