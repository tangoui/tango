// App: app-wide information, control and events.
// info and the on* events need Info (on by default); quit, hide, show,
// setBadge, setProgress and requestAttention need AppControl.

import { invoke, listen, report } from "./internal.js";

/** What env.Info reports in Go. */
export interface Environment {
  /** Go's GOOS: "darwin", "windows", "linux", "ios", "android". */
  os: string;
  /** Go's GOARCH. */
  arch: string;
  /** "macOS", "Windows", "Ubuntu 24.04.1 LTS", ... */
  osName: string;
  osVersion: string;
  /** false in builds with -tags tango_release. */
  debug: boolean;
  /** "WebKit", "WebView2", "WebKitGTK"; "" when unknown. */
  webView: string;
  webViewVersion: string;
}

/** A later launch forwarded by the single package. */
export interface SecondInstance {
  args: string[];
  cwd: string;
}

/** Returns the OS, architecture, build mode and WebView engine. */
export function info(): Promise<Environment> {
  return invoke("env.info");
}

/** Quits the app. */
export function quit(): Promise<void> {
  return invoke("app.quit");
}

/** Hides all of the app's windows (Cmd+H on macOS). */
export function hide(): Promise<void> {
  return invoke("app.hide");
}

/** Shows the windows hide() hid and brings the app to the front. */
export function show(): Promise<void> {
  return invoke("app.show");
}

/** Shows text on the dock icon / taskbar button; "" clears. Windows and
 * Linux only show numbers ("3", "99+"). */
export function setBadge(text: string): Promise<void> {
  return invoke("app.setBadge", text);
}

/** Shows a progress bar in [0, 1] on the dock icon / taskbar button;
 * null or a negative value hides it. */
export function setProgress(progress: number | null): Promise<void> {
  return invoke("app.setProgress", progress);
}

/** Bounces the dock icon / flashes the taskbar button. */
export function requestAttention(critical = false): Promise<void> {
  return invoke("app.requestAttention", critical);
}

const taken = new Set<string>();

// Subscribes to a launch event and, the first time in this page, also
// delivers the ones that arrived before any page existed.
function launch<T>(kind: string, fn: (data: T) => void): () => void {
  const off = listen<T>("app." + kind, fn);
  if (!taken.has(kind)) {
    taken.add(kind);
    invoke<T[]>("app.takePending", kind).then(
      (list) => {
        for (const d of list) {
          try {
            fn(d);
          } catch (e) {
            report(e);
          }
        }
      },
      () => taken.delete(kind),
    );
  }
  return off;
}

/** Called with deep links (custom URL schemes), including the one that
 * launched the app. */
export function onOpenURL(fn: (url: string) => void): () => void {
  return launch<{ url: string }>("openURL", (d) => fn(d.url));
}

/** Called with files opened with the app (file manager, Dock, command
 * line), including those it was launched with. */
export function onOpenFiles(fn: (paths: string[]) => void): () => void {
  return launch<{ paths: string[] }>("openFiles", (d) => fn(d.paths));
}

/** Called when the Dock icon is clicked while no window is visible (macOS). */
export function onReopen(fn: () => void): () => void {
  return launch<null>("reopen", () => fn());
}

/** Called when a second launch of the app forwarded its arguments (the Go
 * side uses single.Claim; jsapi.Install forwards launches by itself). */
export function onSecondInstance(fn: (launch: SecondInstance) => void): () => void {
  return listen("app.secondInstance", fn);
}
