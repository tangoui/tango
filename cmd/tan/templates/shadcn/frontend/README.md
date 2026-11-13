# Front end

React 19 + TypeScript + Vite, styled with Tailwind CSS v4 and
[shadcn/ui](https://ui.shadcn.com) components.

- `src/components/ui/` holds the shadcn/ui components (Button, Dialog,
  DropdownMenu, Select, Input, Label and Sonner for toasts). They were
  generated with the shadcn CLI (`shadcn@4.23.0`, style `base-nova`, on
  Base UI) and are your code now: edit them freely. shadcn/ui is MIT
  licensed, © shadcn.
- Add more with the CLI, run from this directory:

      npx shadcn@latest add tabs

  `components.json` tells it where things go; `@/` is an alias of `src/`
  (vite.config.ts and the tsconfig files).
- Colors and radius come from Go: main.go calls `uivars.SyncPage` with
  `theme.Shadcn()` / `theme.Shadcn(theme.Dark())`, which injects
  `--background`, `--primary`, `--radius`, ... into the page and swaps them
  when the system switches between light and dark. `src/index.css` only
  maps them to Tailwind (`@theme inline`); `src/lib/theme.ts` keeps the
  `.dark` class on `<html>` in step, and the Sonner toaster follows it.
  Restyle in main.go (`theme.ShadcnOptions{Base: "zinc", Radius: 12,
  Primary: ...}`), or override single values on `:root` in index.css.
- **After `npx shadcn add`, check `src/index.css`.** Some components (and
  `shadcn init`/`shadcn add` with a theme) write their color variables back
  as `:root { --background: ...; }` and `.dark { ... }` blocks. Those
  declarations override what Go injects (the page then ignores
  `theme.Shadcn` and no longer follows light/dark), so delete every such
  `:root`/`.dark` variable that Go already provides — keep only deliberate
  single overrides. To find them, run from this directory:

      grep -nE '^\s*--(background|foreground|card|popover|primary|secondary|muted|accent|destructive|border|input|ring|chart-[0-9]|sidebar[a-z-]*|radius)\s*:' src/index.css

  It should print nothing (the `--color-*` / `--radius-*` mappings inside
  `@theme inline` do not match and are fine). Also check that the CLI did
  not add a `.dark { ... }` block or rewrite `@theme inline`.
- `src/App.tsx` is the demo page. On macOS the window has no system title
  bar (see main.go) and the page draws one: elements with the CSS custom
  property `--tango-drag: drag` move the window, and controls inside set
  `--tango-drag: no-drag` so they keep their clicks. Windows and Linux keep
  the system title bar, so the page does not draw its own there.
