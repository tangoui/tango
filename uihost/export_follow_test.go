package uihost

import "github.com/tangoui/tango"

// FollowStats reports how many windows of a follow the system and whether
// the app's shared i18n subscription is active.
func FollowStats(a *tango.App) (windows int, i18nSubscribed bool) {
	follows.mu.Lock()
	defer follows.mu.Unlock()
	af := follows.apps[a]
	if af == nil {
		return 0, false
	}
	return len(af.list), af.cancelI18n != nil
}
