// Dialog: native message, open and save dialogs, attached to the calling
// window. Needs Dialogs (off by default). A dismissed dialog rejects with
// TangoError code "canceled".

import { invoke } from "./internal.js";

/** Icon / severity of a message dialog. */
export type MessageKind = "info" | "warning" | "error" | "question";

export interface MessageOptions {
  kind?: MessageKind;
  title?: string;
  message: string;
  /** Button labels; default a single "OK". */
  buttons?: string[];
}

/** Restricts the files an open/save dialog offers. */
export interface FileFilter {
  name: string;
  /** Glob patterns such as "*.png". */
  patterns: string[];
}

export interface FileOptions {
  title?: string;
  /** Initial directory. */
  directory?: string;
  filters?: FileFilter[];
  /** Offer a "New Folder" button (macOS, GTK; Windows always does). */
  canCreateDirs?: boolean;
  showHidden?: boolean;
}

export interface OpenOptions extends FileOptions {
  /** Allow choosing several files. */
  multiple?: boolean;
  /** Choose folders instead of files. */
  folders?: boolean;
}

export interface SaveOptions extends FileOptions {
  /** Default file name. */
  filename?: string;
}

/** Shows a message dialog and resolves to the index of the chosen button.
 * Dismissing a multi-button dialog rejects with "canceled"; a single-button
 * dialog always resolves to 0. */
export function message(opts: MessageOptions): Promise<number> {
  return invoke("dialog.message", opts);
}

/** Shows an open dialog and resolves to the chosen paths (one unless
 * multiple). */
export function open(opts: OpenOptions = {}): Promise<string[]> {
  return invoke("dialog.open", opts);
}

/** Shows a save dialog and resolves to the chosen path. */
export function save(opts: SaveOptions = {}): Promise<string> {
  return invoke("dialog.save", opts);
}
