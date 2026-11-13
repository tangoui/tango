//go:build linux && !android

package linux

// platform.Power: UPower (power source, battery), power-profiles-daemon
// (power-saver → LowPower), systemd-logind (PrepareForSleep, Inhibit) on
// the system bus, and the GNOME session manager / freedesktop ScreenSaver
// on the session bus for display-sleep blocks.
//
// PowerSuspending: while a handler is set the backend holds a logind
// "delay" sleep inhibitor, so logind waits (up to InhibitDelayMaxSec,
// 5 s by default) for the handler before suspending; the lock is released
// right after the handler ran and taken again on resume.

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Power = (*backend)(nil)

const (
	upowerBus     = "org.freedesktop.UPower"
	upowerPath    = "/org/freedesktop/UPower"
	upowerDisplay = "/org/freedesktop/UPower/devices/DisplayDevice"
	logindBus     = "org.freedesktop.login1"
	logindPath    = "/org/freedesktop/login1"
	logindIface   = "org.freedesktop.login1.Manager"
	propsIface    = "org.freedesktop.DBus.Properties"
)

// power profile daemons: the current name first, then the original one.
var powerProfileSvcs = [][3]string{
	{"org.freedesktop.UPower.PowerProfiles", "/org/freedesktop/UPower/PowerProfiles", "org.freedesktop.UPower.PowerProfiles"},
	{"net.hadess.PowerProfiles", "/net/hadess/PowerProfiles", "net.hadess.PowerProfiles"},
}

type sleepBlock struct {
	fd        int    // logind inhibitor, -1 if none
	gsmCookie uint32 // org.gnome.SessionManager.Inhibit, 0 if none
	ssCookie  uint32 // org.freedesktop.ScreenSaver.Inhibit, 0 if none
}

var pw struct {
	handler func(platform.PowerEvent)
	unsubs  []func()
	delayFD int // logind delay inhibitor while a handler is set, else -1
	blocks  map[uint64]*sleepBlock
	next    uint64
}

func init() { pw.delayFD = -1 }

func (b *backend) who() string {
	if b.opts.Name != "" {
		return b.opts.Name
	}
	return secretApp(b.opts.ID, "")
}

func (b *backend) PowerStatus() (platform.PowerStatus, error) {
	conn, err := bus(gBusTypeSystem)
	if err != nil {
		return platform.PowerStatus{Battery: -1}, err
	}
	var in upowerInfo
	main, err := dbusGetAll(conn, upowerBus, upowerPath, upowerBus)
	if err != nil {
		if isMissing(err) {
			return platform.PowerStatus{Battery: -1}, fmt.Errorf("%w: UPower is not running (install upower)", platform.ErrUnsupported)
		}
		return platform.PowerStatus{Battery: -1}, err
	}
	in.haveUPower = true
	in.onBattery, _ = dictBool(main, "OnBattery")
	gVariantUnref(main)
	if dev, err := dbusGetAll(conn, upowerBus, upowerDisplay, upowerBus+".Device"); err == nil {
		in.devType, _ = dictUint32(dev, "Type")
		in.devState, _ = dictUint32(dev, "State")
		in.percent, _ = dictDouble(dev, "Percentage")
		in.present, _ = dictBool(dev, "IsPresent")
		gVariantUnref(dev)
	}
	in.profile = activePowerProfile(conn)
	return powerStatus(in), nil
}

func activePowerProfile(conn ptr) string {
	for _, s := range powerProfileSvcs {
		v, err := dbusGet(conn, s[0], s[1], s[2], "ActiveProfile")
		if err != nil {
			continue
		}
		p := gvStr(v)
		gVariantUnref(v)
		return p
	}
	return ""
}

func (b *backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	conn, err := bus(gBusTypeSystem)
	if err != nil {
		return err
	}
	pw.handler = fn
	if fn == nil {
		for _, u := range pw.unsubs {
			u()
		}
		pw.unsubs = nil
		releaseDelay()
		return nil
	}
	if pw.unsubs != nil {
		return nil // already subscribed; the new handler takes over
	}
	changed := func(_, _, _, _ string, _ ptr) {
		if pw.handler != nil {
			pw.handler(platform.PowerStatusChanged)
		}
	}
	pw.unsubs = append(pw.unsubs, subscribe(conn, upowerBus, propsIface, "PropertiesChanged", "", "", changed))
	for _, s := range powerProfileSvcs {
		pw.unsubs = append(pw.unsubs, subscribe(conn, s[0], propsIface, "PropertiesChanged", s[1], "", changed))
	}
	pw.unsubs = append(pw.unsubs, subscribe(conn, logindBus, logindIface, "PrepareForSleep", logindPath, "", func(_, _, _, _ string, params ptr) {
		v := gvChild(params, 0)
		start := gd.vBoolean(v)
		gVariantUnref(v)
		if start {
			if pw.handler != nil {
				pw.handler(platform.PowerSuspending)
			}
			releaseDelay() // let the system sleep now
			return
		}
		b.takeDelay(conn)
		if pw.handler != nil {
			pw.handler(platform.PowerResumed)
		}
	}))
	b.takeDelay(conn)
	return nil
}

func (b *backend) takeDelay(conn ptr) {
	if pw.delayFD >= 0 {
		return
	}
	fd, err := dbusCallFD(conn, logindBus, logindPath, logindIface, "Inhibit",
		logindInhibitArgs("sleep", b.who(), "Saving state before sleep", "delay"))
	if err == nil {
		pw.delayFD = fd
	}
}

func releaseDelay() {
	if pw.delayFD >= 0 {
		syscall.Close(pw.delayFD)
		pw.delayFD = -1
	}
}

func (b *backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	if kind != platform.BlockSystemSleep && kind != platform.BlockDisplaySleep {
		return 0, fmt.Errorf("tango/linux: unknown sleep block kind %d", kind)
	}
	blk := &sleepBlock{fd: -1}
	var errs []error
	if conn, err := bus(gBusTypeSystem); err == nil {
		fd, err := dbusCallFD(conn, logindBus, logindPath, logindIface, "Inhibit", logindInhibitArgs("idle", b.who(), reason, "block"))
		if err == nil {
			blk.fd = fd
		} else {
			errs = append(errs, err)
		}
	} else {
		errs = append(errs, err)
	}
	// Desktop sessions decide idle suspend / screen blanking themselves.
	if conn, err := bus(gBusTypeSession); err == nil {
		args := fmt.Sprintf("(%s, uint32 0, %s, uint32 %d)", gvString(secretApp(b.opts.ID, b.opts.Name)), gvString(reason), gsmInhibitFlags(kind))
		if reply, err := dbusCall(conn, "org.gnome.SessionManager", "/org/gnome/SessionManager", "org.gnome.SessionManager", "Inhibit", args, 3000); err == nil {
			v := gvChild(reply, 0)
			blk.gsmCookie = gVariantGetUint32(v)
			gVariantUnref(v)
			gVariantUnref(reply)
		} else if kind == platform.BlockDisplaySleep {
			// KDE, XFCE, ...: the freedesktop screensaver inhibit.
			if reply, err := dbusCall(conn, "org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver", "Inhibit",
				"("+gvString(b.who())+", "+gvString(reason)+")", 3000); err == nil {
				v := gvChild(reply, 0)
				blk.ssCookie = gVariantGetUint32(v)
				gVariantUnref(v)
				gVariantUnref(reply)
			} else {
				errs = append(errs, err)
			}
		}
	}
	if blk.fd < 0 && blk.gsmCookie == 0 && blk.ssCookie == 0 {
		return 0, fmt.Errorf("%w: no sleep inhibitor available: %w", platform.ErrUnsupported, errors.Join(errs...))
	}
	if pw.blocks == nil {
		pw.blocks = map[uint64]*sleepBlock{}
	}
	pw.next++
	pw.blocks[pw.next] = blk
	return pw.next, nil
}

func (b *backend) UnblockSleep(token uint64) {
	blk := pw.blocks[token]
	if blk == nil {
		return
	}
	delete(pw.blocks, token)
	if blk.fd >= 0 {
		syscall.Close(blk.fd)
	}
	if conn, err := bus(gBusTypeSession); err == nil {
		if blk.gsmCookie != 0 {
			if r, err := dbusCall(conn, "org.gnome.SessionManager", "/org/gnome/SessionManager", "org.gnome.SessionManager", "Uninhibit",
				fmt.Sprintf("(uint32 %d,)", blk.gsmCookie), 3000); err == nil {
				gVariantUnref(r)
			}
		}
		if blk.ssCookie != 0 {
			if r, err := dbusCall(conn, "org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver", "UnInhibit",
				fmt.Sprintf("(uint32 %d,)", blk.ssCookie), 3000); err == nil {
				gVariantUnref(r)
			}
		}
	}
}
