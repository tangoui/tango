// Package rolenames holds the localized labels of the native menu roles and
// the default dialog buttons the desktop backends supply themselves, in the
// wording of each platform (Apple, Microsoft and GNOME style guides).
//
// Keys are prefixed by platform ("mac.", "win.", "gtk.") because the same
// role has different wording per platform (macOS "Quit %s", Windows "Exit",
// GTK "_Quit"). "%s" stands for the application name.
//
// The locale is i18n.Locale() at the time of the call: menus already built
// keep their labels until the backend builds them again.
package rolenames

import (
	"strings"

	"github.com/tangoui/tango/i18n"
)

// Locales lists the locales of the table; every key exists in each of them.
var Locales = []string{"en", "zh-CN", "zh-TW", "ja"}

var table = map[string]map[string]string{
	"en": {
		// macOS (Apple Human Interface Guidelines menu titles)
		"mac.appMenu":          "%s",
		"mac.about":            "About %s",
		"mac.hide":             "Hide %s",
		"mac.hideOthers":       "Hide Others",
		"mac.unhide":           "Show All",
		"mac.quit":             "Quit %s",
		"mac.editMenu":         "Edit",
		"mac.undo":             "Undo",
		"mac.redo":             "Redo",
		"mac.cut":              "Cut",
		"mac.copy":             "Copy",
		"mac.paste":            "Paste",
		"mac.delete":           "Delete",
		"mac.selectAll":        "Select All",
		"mac.windowMenu":       "Window",
		"mac.minimize":         "Minimize",
		"mac.zoom":             "Zoom",
		"mac.close":            "Close Window",
		"mac.front":            "Bring All to Front",
		"mac.togglefullscreen": "Enter Full Screen",
		"mac.ok":               "OK",
		"mac.cancel":           "Cancel",
		"mac.yes":              "Yes",
		"mac.no":               "No",

		// Windows
		"win.about":      "About",
		"win.aboutApp":   "About %s",
		"win.quit":       "Exit",
		"win.hide":       "Hide",
		"win.editMenu":   "Edit",
		"win.undo":       "Undo",
		"win.redo":       "Redo",
		"win.cut":        "Cut",
		"win.copy":       "Copy",
		"win.paste":      "Paste",
		"win.selectAll":  "Select All",
		"win.windowMenu": "Window",
		"win.minimize":   "Minimize",
		"win.close":      "Close",
		"win.ok":         "OK",
		"win.cancel":     "Cancel",
		"win.yes":        "Yes",
		"win.no":         "No",

		// GTK (mnemonic syntax: "_" marks the access key)
		"gtk.fileMenu":   "_File",
		"gtk.about":      "_About",
		"gtk.quit":       "_Quit",
		"gtk.editMenu":   "_Edit",
		"gtk.undo":       "_Undo",
		"gtk.redo":       "_Redo",
		"gtk.cut":        "Cu_t",
		"gtk.copy":       "_Copy",
		"gtk.paste":      "_Paste",
		"gtk.selectAll":  "Select _All",
		"gtk.windowMenu": "_Window",
		"gtk.minimize":   "Mi_nimize",
		"gtk.close":      "_Close",
		"gtk.ok":         "_OK",
		"gtk.cancel":     "_Cancel",
		"gtk.yes":        "_Yes",
		"gtk.no":         "_No",
	},
	"zh-CN": {
		"mac.appMenu":          "%s",
		"mac.about":            "关于 %s",
		"mac.hide":             "隐藏 %s",
		"mac.hideOthers":       "隐藏其他",
		"mac.unhide":           "全部显示",
		"mac.quit":             "退出 %s",
		"mac.editMenu":         "编辑",
		"mac.undo":             "撤销",
		"mac.redo":             "重做",
		"mac.cut":              "剪切",
		"mac.copy":             "拷贝",
		"mac.paste":            "粘贴",
		"mac.delete":           "删除",
		"mac.selectAll":        "全选",
		"mac.windowMenu":       "窗口",
		"mac.minimize":         "最小化",
		"mac.zoom":             "缩放",
		"mac.close":            "关闭窗口",
		"mac.front":            "前置全部窗口",
		"mac.togglefullscreen": "进入全屏幕",
		"mac.ok":               "好",
		"mac.cancel":           "取消",
		"mac.yes":              "是",
		"mac.no":               "否",

		"win.about":      "关于",
		"win.aboutApp":   "关于 %s",
		"win.quit":       "退出",
		"win.hide":       "隐藏",
		"win.editMenu":   "编辑",
		"win.undo":       "撤销",
		"win.redo":       "重做",
		"win.cut":        "剪切",
		"win.copy":       "复制",
		"win.paste":      "粘贴",
		"win.selectAll":  "全选",
		"win.windowMenu": "窗口",
		"win.minimize":   "最小化",
		"win.close":      "关闭",
		"win.ok":         "确定",
		"win.cancel":     "取消",
		"win.yes":        "是",
		"win.no":         "否",

		"gtk.fileMenu":   "文件(_F)",
		"gtk.about":      "关于(_A)",
		"gtk.quit":       "退出(_Q)",
		"gtk.editMenu":   "编辑(_E)",
		"gtk.undo":       "撤销(_U)",
		"gtk.redo":       "重做(_R)",
		"gtk.cut":        "剪切(_T)",
		"gtk.copy":       "复制(_C)",
		"gtk.paste":      "粘贴(_P)",
		"gtk.selectAll":  "全选(_A)",
		"gtk.windowMenu": "窗口(_W)",
		"gtk.minimize":   "最小化(_N)",
		"gtk.close":      "关闭(_C)",
		"gtk.ok":         "确定(_O)",
		"gtk.cancel":     "取消(_C)",
		"gtk.yes":        "是(_Y)",
		"gtk.no":         "否(_N)",
	},
	"zh-TW": {
		"mac.appMenu":          "%s",
		"mac.about":            "關於 %s",
		"mac.hide":             "隱藏 %s",
		"mac.hideOthers":       "隱藏其他",
		"mac.unhide":           "顯示全部",
		"mac.quit":             "結束 %s",
		"mac.editMenu":         "編輯",
		"mac.undo":             "還原",
		"mac.redo":             "重做",
		"mac.cut":              "剪下",
		"mac.copy":             "拷貝",
		"mac.paste":            "貼上",
		"mac.delete":           "刪除",
		"mac.selectAll":        "全選",
		"mac.windowMenu":       "視窗",
		"mac.minimize":         "縮到最小",
		"mac.zoom":             "縮放",
		"mac.close":            "關閉視窗",
		"mac.front":            "將此程式所有視窗移至最前",
		"mac.togglefullscreen": "進入全螢幕",
		"mac.ok":               "好",
		"mac.cancel":           "取消",
		"mac.yes":              "是",
		"mac.no":               "否",

		"win.about":      "關於",
		"win.aboutApp":   "關於 %s",
		"win.quit":       "結束",
		"win.hide":       "隱藏",
		"win.editMenu":   "編輯",
		"win.undo":       "復原",
		"win.redo":       "取消復原",
		"win.cut":        "剪下",
		"win.copy":       "複製",
		"win.paste":      "貼上",
		"win.selectAll":  "全選",
		"win.windowMenu": "視窗",
		"win.minimize":   "最小化",
		"win.close":      "關閉",
		"win.ok":         "確定",
		"win.cancel":     "取消",
		"win.yes":        "是",
		"win.no":         "否",

		"gtk.fileMenu":   "檔案(_F)",
		"gtk.about":      "關於(_A)",
		"gtk.quit":       "結束(_Q)",
		"gtk.editMenu":   "編輯(_E)",
		"gtk.undo":       "復原(_U)",
		"gtk.redo":       "重做(_R)",
		"gtk.cut":        "剪下(_T)",
		"gtk.copy":       "複製(_C)",
		"gtk.paste":      "貼上(_P)",
		"gtk.selectAll":  "全選(_A)",
		"gtk.windowMenu": "視窗(_W)",
		"gtk.minimize":   "最小化(_N)",
		"gtk.close":      "關閉(_C)",
		"gtk.ok":         "確定(_O)",
		"gtk.cancel":     "取消(_C)",
		"gtk.yes":        "是(_Y)",
		"gtk.no":         "否(_N)",
	},
	"ja": {
		"mac.appMenu":          "%s",
		"mac.about":            "%sについて",
		"mac.hide":             "%sを非表示",
		"mac.hideOthers":       "ほかを非表示",
		"mac.unhide":           "すべてを表示",
		"mac.quit":             "%sを終了",
		"mac.editMenu":         "編集",
		"mac.undo":             "取り消す",
		"mac.redo":             "やり直す",
		"mac.cut":              "カット",
		"mac.copy":             "コピー",
		"mac.paste":            "ペースト",
		"mac.delete":           "削除",
		"mac.selectAll":        "すべてを選択",
		"mac.windowMenu":       "ウインドウ",
		"mac.minimize":         "しまう",
		"mac.zoom":             "拡大/縮小",
		"mac.close":            "ウインドウを閉じる",
		"mac.front":            "すべてを手前に移動",
		"mac.togglefullscreen": "フルスクリーンにする",
		"mac.ok":               "OK",
		"mac.cancel":           "キャンセル",
		"mac.yes":              "はい",
		"mac.no":               "いいえ",

		"win.about":      "バージョン情報",
		"win.aboutApp":   "%s のバージョン情報",
		"win.quit":       "終了",
		"win.hide":       "非表示",
		"win.editMenu":   "編集",
		"win.undo":       "元に戻す",
		"win.redo":       "やり直し",
		"win.cut":        "切り取り",
		"win.copy":       "コピー",
		"win.paste":      "貼り付け",
		"win.selectAll":  "すべて選択",
		"win.windowMenu": "ウィンドウ",
		"win.minimize":   "最小化",
		"win.close":      "閉じる",
		"win.ok":         "OK",
		"win.cancel":     "キャンセル",
		"win.yes":        "はい",
		"win.no":         "いいえ",

		"gtk.fileMenu":   "ファイル(_F)",
		"gtk.about":      "情報(_A)",
		"gtk.quit":       "終了(_Q)",
		"gtk.editMenu":   "編集(_E)",
		"gtk.undo":       "元に戻す(_U)",
		"gtk.redo":       "やり直す(_R)",
		"gtk.cut":        "切り取り(_T)",
		"gtk.copy":       "コピー(_C)",
		"gtk.paste":      "貼り付け(_P)",
		"gtk.selectAll":  "すべて選択(_A)",
		"gtk.windowMenu": "ウィンドウ(_W)",
		"gtk.minimize":   "最小化(_N)",
		"gtk.close":      "閉じる(_C)",
		"gtk.ok":         "OK(_O)",
		"gtk.cancel":     "キャンセル(_C)",
		"gtk.yes":        "はい(_Y)",
		"gtk.no":         "いいえ(_N)",
	},
}

// Lookup returns the label of key in locale, matched against Locales
// ("zh-HK" uses zh-TW, an unknown language uses English). ok is false when
// key is unknown.
func Lookup(locale, key string) (string, bool) {
	loc := i18n.Match([]string{locale}, Locales)
	if s, ok := table[loc][key]; ok {
		return s, true
	}
	s, ok := table["en"][key]
	return s, ok
}

// Text returns the label of key in the current locale (i18n.Locale), or
// key itself when it is not in the table. The "%s" placeholder is kept.
func Text(key string) string {
	if s, ok := Lookup(i18n.Locale(), key); ok {
		return s
	}
	return key
}

// Format is Text with "%s" replaced by name.
func Format(key, name string) string {
	return strings.ReplaceAll(Text(key), "%s", name)
}
