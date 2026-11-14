package updateui

import (
	"strings"

	"github.com/tangoui/tango/i18n"
)

// Strings are the texts of the update window. Fields holding a format
// verb are used with fmt.Sprintf: NewVersion takes the new and the
// running version, Released the release date, UpToDate and RolledBack
// the running version.
type Strings struct {
	Title       string // window title
	Checking    string
	NewVersion  string // "%s is available (you have %s)"
	Released    string // "Released %s"
	Notes       string // heading of the release notes
	Skip        string // skip this version
	Later       string // remind me later
	Install     string // download and install
	Downloading string
	Patch       string // the download is an incremental update
	Full        string // the download is the full release
	Cancel      string
	Ready       string // downloaded and verified
	RestartNow  string
	// InstallNextLaunch keeps the download and asks again at the next launch.
	InstallNextLaunch string
	Downloaded        string // offered at launch: an update was downloaded earlier
	Error             string
	Retry             string
	Close             string
	UpToDate          string // "You're up to date (%s)"
	RolledBack        string // "The last update did not start; restored %s"
	Unsupported       string // this installation cannot update itself
	OpenDownloadPage  string
}

var (
	stringsEN = &Strings{
		Title:             "Software Update",
		Checking:          "Checking for updates…",
		NewVersion:        "Version %s is available. You have %s.",
		Released:          "Released %s",
		Notes:             "What's new",
		Skip:              "Skip This Version",
		Later:             "Remind Me Later",
		Install:           "Download and Install",
		Downloading:       "Downloading…",
		Patch:             "Incremental update",
		Full:              "Full installer",
		Cancel:            "Cancel",
		Ready:             "The update is ready. Restart to finish installing it.",
		RestartNow:        "Restart Now",
		InstallNextLaunch: "Install on Next Launch",
		Downloaded:        "An update was downloaded earlier. Restart now to install it?",
		Error:             "The update failed.",
		Retry:             "Try Again",
		Close:             "Close",
		UpToDate:          "You're up to date. Version %s is the newest version available.",
		RolledBack:        "The last update did not start, so version %s was restored.",
		Unsupported:       "This copy of the app cannot update itself. Download the new version instead.",
		OpenDownloadPage:  "Open Download Page",
	}
	stringsZhCN = &Strings{
		Title:             "软件更新",
		Checking:          "正在检查更新…",
		NewVersion:        "新版本 %s 可用，当前版本为 %s。",
		Released:          "发布于 %s",
		Notes:             "更新内容",
		Skip:              "跳过此版本",
		Later:             "稍后提醒",
		Install:           "下载并安装",
		Downloading:       "正在下载…",
		Patch:             "增量更新",
		Full:              "完整安装包",
		Cancel:            "取消",
		Ready:             "更新已准备好，重启后完成安装。",
		RestartNow:        "立即重启",
		InstallNextLaunch: "下次启动时安装",
		Downloaded:        "之前已下载好一个更新，现在重启安装吗？",
		Error:             "更新失败。",
		Retry:             "重试",
		Close:             "关闭",
		UpToDate:          "当前已是最新版本 %s。",
		RolledBack:        "上次更新未能正常启动，已恢复到 %s。",
		Unsupported:       "当前安装方式无法自动更新，请手动下载新版本。",
		OpenDownloadPage:  "打开下载页面",
	}
	stringsZhTW = &Strings{
		Title:             "軟體更新",
		Checking:          "正在檢查更新…",
		NewVersion:        "新版本 %s 可用，目前版本為 %s。",
		Released:          "發布於 %s",
		Notes:             "更新內容",
		Skip:              "略過此版本",
		Later:             "稍後提醒",
		Install:           "下載並安裝",
		Downloading:       "正在下載…",
		Patch:             "增量更新",
		Full:              "完整安裝檔",
		Cancel:            "取消",
		Ready:             "更新已準備好，重新啟動後完成安裝。",
		RestartNow:        "立即重新啟動",
		InstallNextLaunch: "下次啟動時安裝",
		Downloaded:        "先前已下載一個更新，現在重新啟動並安裝嗎？",
		Error:             "更新失敗。",
		Retry:             "重試",
		Close:             "關閉",
		UpToDate:          "目前已是最新版本 %s。",
		RolledBack:        "上次更新未能正常啟動，已還原到 %s。",
		Unsupported:       "目前的安裝方式無法自動更新，請手動下載新版本。",
		OpenDownloadPage:  "開啟下載頁面",
	}
	stringsJA = &Strings{
		Title:             "ソフトウェア・アップデート",
		Checking:          "アップデートを確認しています…",
		NewVersion:        "新しいバージョン %s を利用できます。現在のバージョンは %s です。",
		Released:          "公開日 %s",
		Notes:             "新機能",
		Skip:              "このバージョンをスキップ",
		Later:             "あとで通知",
		Install:           "ダウンロードしてインストール",
		Downloading:       "ダウンロードしています…",
		Patch:             "差分アップデート",
		Full:              "フルインストーラ",
		Cancel:            "キャンセル",
		Ready:             "アップデートの準備ができました。再起動するとインストールが完了します。",
		RestartNow:        "今すぐ再起動",
		InstallNextLaunch: "次回起動時にインストール",
		Downloaded:        "以前にダウンロードしたアップデートがあります。今すぐ再起動してインストールしますか？",
		Error:             "アップデートに失敗しました。",
		Retry:             "再試行",
		Close:             "閉じる",
		UpToDate:          "最新の状態です。バージョン %s が最新です。",
		RolledBack:        "前回のアップデートが起動しなかったため、バージョン %s に戻しました。",
		Unsupported:       "このアプリは自動でアップデートできません。新しいバージョンを手動でダウンロードしてください。",
		OpenDownloadPage:  "ダウンロードページを開く",
	}
)

// builtinStrings picks the built-in texts for a BCP 47 locale: zh-TW,
// zh-HK, zh-MO and zh-Hant* use Traditional Chinese, other zh Simplified,
// ja Japanese, anything else English.
func builtinStrings(locale string) *Strings {
	l := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	switch {
	case l == "zh-tw", l == "zh-hk", l == "zh-mo", strings.HasPrefix(l, "zh-hant"),
		strings.HasPrefix(l, "zh-tw-"), strings.HasPrefix(l, "zh-hk-"), strings.HasPrefix(l, "zh-mo-"):
		return stringsZhTW
	case l == "zh", strings.HasPrefix(l, "zh-"):
		return stringsZhCN
	case l == "ja", strings.HasPrefix(l, "ja-"):
		return stringsJA
	}
	return stringsEN
}

// stringsFor returns opts.Text, or the built-in texts for i18n.Locale().
func stringsFor(opts Options) *Strings {
	if opts.Text != nil {
		return opts.Text
	}
	return builtinStrings(i18n.Locale())
}
