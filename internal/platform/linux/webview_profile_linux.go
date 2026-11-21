//go:build linux && !android

package linux

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/tangoui/tango/internal/platform"
)

var webProfile struct {
	newManager func(string, string, string, string, uintptr) ptr
	newContext func(ptr) ptr
	newView    func(uintptr, string, ptr, string, ptr, uintptr) ptr

	cookieManager func(ptr) ptr
	cookieStorage func(ptr, string, int32)

	inspectorView  func(ptr) ptr
	inspectorClose func(ptr)
}

func bindWebProfile() {
	var bd binder
	bd.opt(&webProfile.newManager, "webkit_website_data_manager_new", libWebKit)
	bd.opt(&webProfile.newContext, "webkit_web_context_new_with_website_data_manager", libWebKit)
	bd.opt(&webProfile.newView, "g_object_new", libGObject)
	bd.opt(&webProfile.cookieManager, "webkit_web_context_get_cookie_manager", libWebKit)
	bd.opt(&webProfile.cookieStorage, "webkit_cookie_manager_set_persistent_storage", libWebKit)
	bd.opt(&webProfile.inspectorView, "webkit_web_inspector_get_web_view", libWebKit)
	bd.opt(&webProfile.inspectorClose, "webkit_web_inspector_close", libWebKit)
}

func (b *backend) SupportsWebProfiles() bool {
	wv.once.Do(bindWebExt)
	x := &webProfile
	return x.newManager != nil && x.newContext != nil && x.newView != nil && webInteraction.webViewType != nil &&
		x.cookieManager != nil && x.cookieStorage != nil
}

func (b *backend) webProfileContext(name string) (ptr, error) {
	if !b.SupportsWebProfiles() {
		return 0, platform.ErrUnsupported
	}
	if ctx := b.webProfiles[name]; ctx != 0 {
		return ctx, nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return 0, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return 0, err
	}
	appID := b.opts.ID
	if appID == "" {
		appID = b.opts.Name
	}
	if appID == "" {
		appID, err = os.Executable()
		if err != nil {
			return 0, err
		}
	}
	app := platform.WebProfileKey(appID)
	key := platform.WebProfileKey(name)
	config = filepath.Join(config, "tango", app, "web-profiles", key)
	cache = filepath.Join(cache, "tango", app, "web-profiles", key)
	for _, dir := range []string{config, cache} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return 0, err
		}
	}
	manager := webProfile.newManager("base-data-directory", config, "base-cache-directory", cache, 0)
	if manager == 0 {
		return 0, errors.New("tango/linux: failed to create web profile data manager")
	}
	defer gObjectUnref(manager)
	ctx := webProfile.newContext(manager)
	if ctx == 0 {
		return 0, errors.New("tango/linux: failed to create web profile context")
	}
	cookies := webProfile.cookieManager(ctx)
	if cookies == 0 {
		gObjectUnref(ctx)
		return 0, errors.New("tango/linux: failed to get web profile cookie manager")
	}
	const cookieStorageSQLite = 1
	webProfile.cookieStorage(cookies, filepath.Join(config, "cookies.sqlite"), cookieStorageSQLite)
	if b.webProfiles == nil {
		b.webProfiles = make(map[string]ptr)
	}
	b.webProfiles[name] = ctx
	b.registerSchemeIn(ctx)
	return ctx, nil
}

func (s *surface) DevToolsOpen() (bool, error) {
	if s.web == 0 || webProfile.inspectorView == nil {
		return false, platform.ErrUnsupported
	}
	return webProfile.inspectorView(webkitWebViewGetInspector(s.web)) != 0, nil
}

func (s *surface) CloseDevTools() error {
	if s.web == 0 || webProfile.inspectorClose == nil {
		return platform.ErrUnsupported
	}
	webProfile.inspectorClose(webkitWebViewGetInspector(s.web))
	return nil
}

func (e *embeddedWeb) DevToolsOpen() (bool, error) { return e.w.DevToolsOpen() }
func (e *embeddedWeb) CloseDevTools() error        { return e.w.CloseDevTools() }
