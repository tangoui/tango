// Shell: hand URLs and files to the OS. openURL needs OpenURL, openPath and
// revealPath need OpenPath; both are off by default.

import { invoke } from "./internal.js";

/** Opens url with the default handler. Only schemes allowed in Go
 * (shell.AllowSchemes; http, https and mailto by default) are accepted;
 * others reject with "forbidden". */
export function openURL(url: string): Promise<void> {
  return invoke("shell.openURL", url);
}

/** Opens a file or folder with its default app. */
export function openPath(path: string): Promise<void> {
  return invoke("shell.openPath", path);
}

/** Shows a file or folder in Finder / Explorer / the file manager. */
export function revealPath(path: string): Promise<void> {
  return invoke("shell.revealPath", path);
}
