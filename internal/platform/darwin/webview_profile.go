//go:build darwin && !ios

package darwin

import "github.com/tangoui/tango/internal/platform"

func (b *backend) SupportsWebProfiles() bool {
	return profileWebViewEnabled && respondsTo(class("WKWebsiteDataStore"), "dataStoreForIdentifier:")
}

func webProfileStore(name string) id {
	key := platform.WebProfileKey(name)
	uuid := send(alloc("NSUUID"), "initWithUUIDString:", uintptr(nsString(
		key[:8]+"-"+key[8:12]+"-"+key[12:16]+"-"+key[16:20]+"-"+key[20:])))
	defer release(uuid)
	return send(class("WKWebsiteDataStore"), "dataStoreForIdentifier:", uintptr(uuid))
}

func (s *surface) inspector() id {
	if s.web != 0 && respondsTo(s.web, "_inspector") {
		return send(s.web, "_inspector")
	}
	return 0
}

func (s *surface) DevToolsOpen() (bool, error) {
	i := s.inspector()
	if i == 0 || !respondsTo(i, "isVisible") {
		return false, platform.ErrUnsupported
	}
	return sendBool(i, "isVisible"), nil
}

func (s *surface) CloseDevTools() error {
	i := s.inspector()
	if i == 0 || !respondsTo(i, "close") {
		return platform.ErrUnsupported
	}
	send(i, "close")
	return nil
}

func (e *embedded) DevToolsOpen() (bool, error) { return e.s.DevToolsOpen() }
func (e *embedded) CloseDevTools() error        { return e.s.CloseDevTools() }
