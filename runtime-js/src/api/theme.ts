// Theme: the system's light/dark mode and accent color. Needs Info (on by
// default).

import { invoke, listen } from "./internal.js";

/** The system theme. */
export interface Theme {
  dark: boolean;
  /** Accent color as "#rrggbbaa"; "" when unknown. */
  accent: string;
}

/** Returns the current system theme. */
export function get(): Promise<Theme> {
  return invoke("theme.get");
}

/** Called with the new theme when the user switches light/dark or the
 * accent color. */
export function onChange(fn: (theme: Theme) => void): () => void {
  return listen("theme.change", fn);
}

/** CSS custom properties of the self-drawn UI's design tokens, name →
 * value ("--tango-color-primary" → "#1677ff"). Present when the Go side
 * called uivars.SyncPage (package sys/theme/uivars); which names appear
 * depends on the ui/theme in use. */
export type Vars = Record<string, string>;

/** Returns the design-token variables injected into this page, or an
 * empty object when the app does not inject them. The same values are on
 * :root, so CSS can use var(--tango-color-primary) directly. */
export function vars(): Vars {
  const v = (globalThis as any).__tangoUIVarsState;
  return v && typeof v === "object" ? { ...v } : {};
}

/** Called with the new variables whenever they change (the system
 * switched light/dark or accent, or the app refreshed them). Returns a
 * function that unsubscribes. */
export function onVarsChange(fn: (vars: Vars) => void): () => void {
  const target = globalThis as any;
  if (typeof target.addEventListener !== "function") return () => {};
  const h = (e: any) => fn({ ...(e && e.detail) });
  target.addEventListener("tangouivars", h);
  return () => target.removeEventListener("tangouivars", h);
}
