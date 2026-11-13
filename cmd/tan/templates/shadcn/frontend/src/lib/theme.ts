import { useSyncExternalStore } from "react"
import { Theme } from "@tangoui/runtime"

// Light or dark, as the variables Go injects say (--tango-color-scheme;
// see main.go). Outside Tango, with no variables, prefers-color-scheme.
const media = window.matchMedia("(prefers-color-scheme: dark)")

function read(): boolean {
  const scheme = Theme.vars()["--tango-color-scheme"]
  return scheme ? scheme === "dark" : media.matches
}

let dark = read()
const listeners = new Set<() => void>()

// Keeps the .dark class on <html> (Tailwind's dark: variant and shadcn/ui
// components use it) in step with the injected variables.
function update() {
  dark = read()
  document.documentElement.classList.toggle("dark", dark)
  for (const l of listeners) l()
}

update()
Theme.onVarsChange(update)
media.addEventListener("change", update)

/** Reports whether the page is dark, re-rendering when that changes. */
export function useDark(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => dark,
  )
}
