// Package filetype picks icons for files and folders, the way an IDE's
// file tree does: a file name resolves to a Kind (code, image, archive, …)
// by its exact name or extension, and a Set maps kinds to icons. Sets for
// the three design systems are provided; a Theme layers a file icon theme
// (for example one contributed by a plugin, with bitmap icons made by
// icon.FromImage) over a fallback.
//
//	ic := filetype.ForFlavor(c.Theme.Flavor).Icon("main.go", false, false)
//	nodes = append(nodes, ui.TreeNode{Key: path, Title: "main.go", Icon: ic})
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package filetype

import (
	"path"
	"strings"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/antd"
	"github.com/tangoui/tango/ui/icon/element"
	"github.com/tangoui/tango/ui/icon/lucide"
	"github.com/tangoui/tango/ui/theme"
)

// Kind is a file category.
type Kind uint8

// The kinds, after MinCode's project tree.
const (
	Document Kind = iota // anything not recognized
	Folder
	Code
	TypeScript
	JavaScript
	Vue
	Markup // HTML, XML
	Style  // CSS and preprocessors
	Data   // JSON, YAML, TOML, CSV
	Config // dotfiles, .ini, lock files
	Markdown
	Text
	Image
	Video
	Audio
	Database
	Archive
	Terminal // shell scripts, Dockerfile, Makefile
	PDF
	Word
	Excel
	Slides
	numKinds
)

var kindNames = [numKinds]string{"document", "folder", "code", "typescript", "javascript", "vue", "markup",
	"style", "data", "config", "markdown", "text", "image", "video", "audio", "database", "archive",
	"terminal", "pdf", "word", "excel", "slides"}

func (k Kind) String() string {
	if k < numKinds {
		return kindNames[k]
	}
	return "document"
}

var extKinds = map[string]Kind{}

func init() {
	for k, exts := range map[Kind]string{
		Vue:        "vue svelte astro",
		TypeScript: "ts tsx mts cts",
		JavaScript: "js jsx mjs cjs",
		Markup:     "html htm xml xhtml xsl svgz vm jsp",
		Style:      "css scss sass less styl pcss",
		Data:       "json jsonc json5 yaml yml toml csv tsv ndjson proto graphql gql",
		Markdown:   "md mdx markdown mdwn rst adoc",
		Text:       "txt log text rtf",
		Image:      "png jpg jpeg jfif gif webp bmp ico icns avif tif tiff svg heic psd",
		Video:      "mp4 mov avi mkv webm m4v flv wmv",
		Audio:      "mp3 wav flac aac ogg m4a opus wma mid midi",
		Database:   "sql db sqlite sqlite3 duckdb mdb accdb",
		Archive:    "zip rar 7z tar gz bz2 xz tgz zst jar war ear apk ipa dmg iso deb rpm whl",
		Terminal:   "sh bash zsh fish ps1 psm1 bat cmd",
		Code: "go rs py pyw java kt kts c h cc cpp cxx hpp hh m mm cs php rb swift scala dart lua r ex exs erl " +
			"hrl fs fsx clj cljs groovy sol zig nim v hs ml mli pl pm jl asm s wasm wat",
		Config: "ini conf config cfg properties lock env editorconfig plist reg",
		PDF:    "pdf",
		Word:   "doc docx odt pages",
		Excel:  "xls xlsx ods numbers",
		Slides: "ppt pptx odp key",
	} {
		for _, e := range strings.Fields(exts) {
			extKinds[e] = k
		}
	}
}

var nameKinds = map[string]Kind{}

func init() {
	for k, names := range map[Kind]string{
		Config: ".babelrc .browserslistrc .dockerignore .editorconfig .eslintignore .eslintrc .gitattributes " +
			".gitignore .gitmodules .npmrc .nvmrc .prettierignore .prettierrc .stylelintrc biome.json deno.json " +
			"jsconfig.json tsconfig.json .env",
		Archive: "bun.lock bun.lockb cargo.lock cargo.toml composer.json composer.lock go.mod go.sum go.work " +
			"package-lock.json package.json pnpm-lock.yaml pyproject.toml requirements.txt yarn.lock",
		Terminal: "containerfile dockerfile gemfile justfile makefile procfile rakefile",
		Text:     "license licence copying authors notice",
	} {
		for _, n := range strings.Fields(names) {
			nameKinds[n] = k
		}
	}
}

// KindOf returns the kind of a file (by its base name; separators of
// either kind are accepted) or Folder for a directory.
func KindOf(name string, isDir bool) Kind {
	if isDir {
		return Folder
	}
	n := strings.ToLower(path.Base(strings.ReplaceAll(name, `\`, "/")))
	if n == "" || n == "." || n == "/" {
		return Document
	}
	if k, ok := nameKinds[n]; ok {
		return k
	}
	switch {
	case strings.HasPrefix(n, ".env."):
		return Config
	case strings.HasPrefix(n, "dockerfile.") || strings.HasPrefix(n, "containerfile."):
		return Terminal
	}
	if i := strings.LastIndexByte(n, '.'); i >= 0 {
		if k, ok := extKinds[n[i+1:]]; ok {
			return k
		}
	}
	return Document
}

// Resolver picks the icon of a file or folder.
type Resolver interface {
	Icon(name string, isDir, open bool) *icon.Icon
}

// Set maps kinds to icons. Kinds left nil use Kinds[Document].
type Set struct {
	Kinds      [numKinds]*icon.Icon
	FolderOpen *icon.Icon // an expanded folder; nil uses Kinds[Folder]
}

// Icon implements Resolver.
func (s *Set) Icon(name string, isDir, open bool) *icon.Icon {
	k := KindOf(name, isDir)
	if k == Folder && open && s.FolderOpen != nil {
		return s.FolderOpen
	}
	if ic := s.Kinds[k]; ic != nil {
		return ic
	}
	return s.Kinds[Document]
}

// Ant uses Ant Design Outlined icons.
var Ant = &Set{FolderOpen: antd.FolderOpenOutlined, Kinds: [numKinds]*icon.Icon{
	Document: antd.FileOutlined, Folder: antd.FolderOutlined, Code: antd.CodeOutlined,
	TypeScript: antd.CodeOutlined, JavaScript: antd.CodeOutlined, Vue: antd.CodeOutlined,
	Markup: antd.Html5Outlined, Style: antd.BgColorsOutlined, Data: antd.DatabaseOutlined,
	Config: antd.SettingOutlined, Markdown: antd.FileMarkdownOutlined, Text: antd.FileTextOutlined,
	Image: antd.FileImageOutlined, Video: antd.VideoCameraOutlined, Audio: antd.CustomerServiceOutlined,
	Database: antd.DatabaseOutlined, Archive: antd.FileZipOutlined, Terminal: antd.DesktopOutlined,
	PDF: antd.FilePdfOutlined, Word: antd.FileWordOutlined, Excel: antd.FileExcelOutlined,
	Slides: antd.FilePptOutlined,
}}

// Element uses Element Plus icons (MinCode's file tree mapping).
var Element = &Set{FolderOpen: element.FolderOpened, Kinds: [numKinds]*icon.Icon{
	Document: element.Document, Folder: element.Folder, Code: element.Operation,
	TypeScript: element.Tickets, JavaScript: element.Operation, Vue: element.Grid,
	Markup: element.ChromeFilled, Style: element.Brush, Data: element.DataAnalysis,
	Config: element.Setting, Markdown: element.Notebook, Text: element.Document,
	Image: element.Picture, Video: element.VideoCamera, Audio: element.Headset,
	Database: element.Coin, Archive: element.Box, Terminal: element.Monitor,
	PDF: element.Reading, Word: element.Document, Excel: element.Grid, Slides: element.DataBoard,
}}

// Lucide uses Lucide file icons (shadcn/ui).
var Lucide = &Set{FolderOpen: lucide.FolderOpen, Kinds: [numKinds]*icon.Icon{
	Document: lucide.File, Folder: lucide.Folder, Code: lucide.FileCode,
	TypeScript: lucide.FileCode, JavaScript: lucide.FileCode, Vue: lucide.FileCode,
	Markup: lucide.FileCode, Style: lucide.FileType, Data: lucide.FileBraces,
	Config: lucide.FileCog, Markdown: lucide.FileText, Text: lucide.FileText,
	Image: lucide.FileImage, Video: lucide.FileVideoCamera, Audio: lucide.FileMusic,
	Database: lucide.FileDatabase, Archive: lucide.FileArchive, Terminal: lucide.FileTerminal,
	PDF: lucide.FileText, Word: lucide.FileText, Excel: lucide.FileSpreadsheet, Slides: lucide.Presentation,
}}

// ForFlavor returns the set of a design system.
func ForFlavor(f theme.Flavor) *Set {
	switch f {
	case theme.Element:
		return Element
	case theme.StyleShadcn:
		return Lucide
	}
	return Ant
}

// Theme is a file icon theme in the shape of VS Code's (and MinCode's
// plugin contribution point): icons by exact file or folder name, by
// extension and by kind, over a fallback. Map keys are lower case;
// extensions have no leading dot and may be compound ("d.ts", matched
// before "ts"). Icons may be bitmaps (icon.FromImage).
type Theme struct {
	FileNames   map[string]*icon.Icon
	Extensions  map[string]*icon.Icon
	FolderNames map[string]*icon.Icon
	// FolderNamesOpen are the expanded variants of FolderNames.
	FolderNamesOpen map[string]*icon.Icon
	Kinds           map[Kind]*icon.Icon
	// File, Folder and FolderOpen are the theme's defaults; nil defers to
	// Fallback.
	File, Folder, FolderOpen *icon.Icon
	// Fallback resolves what the theme leaves out; nil means Ant.
	Fallback Resolver
}

// Icon implements Resolver.
func (t *Theme) Icon(name string, isDir, open bool) *icon.Icon {
	n := strings.ToLower(path.Base(strings.ReplaceAll(name, `\`, "/")))
	if isDir {
		if open {
			if ic := t.FolderNamesOpen[n]; ic != nil {
				return ic
			}
		}
		if ic := t.FolderNames[n]; ic != nil {
			return ic
		}
		if open && t.FolderOpen != nil {
			return t.FolderOpen
		}
		if t.Folder != nil {
			return t.Folder
		}
		return t.fallback().Icon(name, isDir, open)
	}
	if ic := t.FileNames[n]; ic != nil {
		return ic
	}
	// Longest extension first: "a.d.ts" tries "d.ts", then "ts".
	for i := strings.IndexByte(n, '.'); i >= 0 && i < len(n)-1; {
		if ic := t.Extensions[n[i+1:]]; ic != nil {
			return ic
		}
		j := strings.IndexByte(n[i+1:], '.')
		if j < 0 {
			break
		}
		i += j + 1
	}
	if ic := t.Kinds[KindOf(n, false)]; ic != nil {
		return ic
	}
	if t.File != nil && KindOf(n, false) == Document {
		return t.File
	}
	return t.fallback().Icon(name, isDir, open)
}

func (t *Theme) fallback() Resolver {
	if t.Fallback != nil {
		return t.Fallback
	}
	return Ant
}
