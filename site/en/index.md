---
layout: home

hero:
  name: Tango
  text: Beautiful GUIs, built in Go
  tagline: A rich component library, three design languages, and light and dark themes. Build a polished GUI, connect native capabilities, and mix in your web stack.
  image:
    src: /logo.svg
    alt: Tango
  actions:
    - theme: brand
      text: Try the GUI live
      link: /components/
    - theme: alt
      text: Start building
      link: /en/guide/ui
    - theme: alt
      text: GitHub
      link: https://github.com/tangoui/tango

features:
  - icon: 🎨
    title: A polished GUI from the start
    details: Compose buttons, inputs, forms, tables, date pickers, trees, overlays and charts in Go, with a consistent design language across controls and workspace layouts.
    link: /components/
  - icon: 🌓
    title: Three styles, light and dark
    details: Ant Design, Element Plus and shadcn themes share semantic tokens. Switch styles, appearance or accent color while keeping the interface consistent.
    link: /en/guide/ui
  - icon: 🖱️
    title: Try the code editor
    details: Edit Go, TypeScript, Python and JavaScript in your browser with syntax highlighting, undo and redo, read-only mode and line numbers. Tables, charts and other components have live demos too.
    link: /components/code-editor
  - icon: 🪟
    title: Go GUI and your web stack
    details: Draw the interface in Go or use Vue, React and TypeScript through a system WebView. On macOS, WebView and Go-drawn content can share a window.
    link: /en/guide/ui
  - icon: 🔌
    title: Connect to the system
    details: Go packages provide windows, menus, tray icons, notifications and clipboard access. Web frontends can use Go bindings, events and streams with backpressure.
    link: /en/guide/system
  - icon: 📦
    title: From development to distribution
    details: tan covers templates, hot reload, builds, packaging, signing and running, with signed auto-updates. The guides explain platform capabilities and verification scope.
    link: /en/guide/packaging

---

## From a good-looking interface to a useful application

**Tango is for developers who want to build polished GUIs in Go.** Components handle layout, interaction and drawing; your Go code owns application state and business logic, with native integration and WebView available when needed.

<div class="gui-showcase">

![Tango Go GUI: navigation, statistics, filters and a data table](/screenshots/gui-preview.png)

A real Go GUI rendered by `examples/ui-gallery`, with navigation, statistics, filters and a data table. [Try the components →](/components/)

</div>

## Give your application a design language

| What you want to build | What Tango provides |
| --- | --- |
| Forms and settings pages | Inputs, selectors, switches, validation and consistent control states |
| Data workspaces and tools | Tables, trees, charts, navigation and workspace layouts |
| A branded application | Three themes, light and dark modes, accent colors and component tokens |
| A GUI connected to the system | Go business logic, windows, menus, tray icons and notifications |
| An existing web interface | System WebView, frontend templates, Go bindings and event communication |

Try the [code editor](/components/code-editor) or the [full GUI demos](/components/), then explore [Go-drawn UI](/en/guide/ui) or the [full component catalog](/guide/components).

Tango is currently 0.x. Go-drawn UI, hybrid interfaces and mobile remain experimental. See [Self-drawn UI](/en/guide/ui) and [Mobile](/en/guide/mobile) for platform capabilities and verification scope.

## In one minute

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan init -template ui hello  # Go-drawn GUI template
cd hello
tan dev                 # opens the window, rebuilds on Go changes
tan build               # release build in build/
```

Continue with [Go-drawn UI](/en/guide/ui). For a web frontend, choose a template in [Getting started](/en/guide/getting-started).

::: info Documentation language
The Chinese documentation is the primary version. English pages for the guides and the API reference are being translated; untranslated pages link to the Chinese original.
:::
