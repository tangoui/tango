// Clipboard: reading needs ClipboardRead, writing ClipboardWrite; both are
// off by default.

import { invoke } from "./internal.js";

/** Returns the clipboard's text ("" when it holds none). */
export function readText(): Promise<string> {
  return invoke("clipboard.readText");
}

/** Replaces the clipboard's content with text. */
export function writeText(text: string): Promise<void> {
  return invoke("clipboard.writeText", text);
}

/** Returns the clipboard's image as base64-encoded PNG, or null. Use
 * `"data:image/png;base64," + img` as an <img> src. */
export function readImage(): Promise<string | null> {
  return invoke("clipboard.readImage");
}

/** Replaces the clipboard's content with a PNG image given as base64 (a
 * "data:image/png;base64," prefix is accepted). */
export function writeImage(pngBase64: string): Promise<void> {
  const i = pngBase64.startsWith("data:") ? pngBase64.indexOf(",") : -1;
  return invoke("clipboard.writeImage", i >= 0 ? pngBase64.slice(i + 1) : pngBase64);
}

/** Returns the paths of files copied in the file manager ([] when none). */
export function readFiles(): Promise<string[]> {
  return invoke("clipboard.readFiles");
}

/** Replaces the clipboard's content with files (absolute paths), as
 * copying them in the file manager does. */
export function writeFiles(paths: string[]): Promise<void> {
  return invoke("clipboard.writeFiles", paths);
}
