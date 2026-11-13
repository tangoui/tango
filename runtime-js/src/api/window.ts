// Window: the window (Tango surface) the page lives in controls itself.
// Needs the Window capability (on by default).

import { invoke, listen, type Point, type Rect, type Size } from "./internal.js";

export type { Point, Rect, Size } from "./internal.js";

/** Window state. */
export type WindowState = "normal" | "minimized" | "maximized" | "fullscreen";

/** Size limits of the content area; 0 or missing means no limit. */
export interface SizeLimits {
  minWidth?: number;
  minHeight?: number;
  maxWidth?: number;
  maxHeight?: number;
}

/** Files dropped on the page (windows created with AcceptFiles). */
export interface FilesDrop {
  paths: string[];
  /** Drop point in page coordinates; 0, 0 where the platform does not tell. */
  x: number;
  y: number;
}

/** The window's id, unique for the app's lifetime (tango.Surface.ID). */
export function id(): Promise<number> {
  return invoke("window.id");
}

/** Sets the window title. */
export function setTitle(title: string): Promise<void> {
  return invoke("window.setTitle", title);
}

/** Returns the content size. */
export function size(): Promise<Size> {
  return invoke("window.size");
}

/** Resizes the content area. */
export function setSize(width: number, height: number): Promise<void> {
  return invoke("window.setSize", width, height);
}

/** Sets minimum and maximum content size. */
export function setSizeLimits(limits: SizeLimits): Promise<void> {
  return invoke("window.setSizeLimits", limits.minWidth ?? 0, limits.minHeight ?? 0, limits.maxWidth ?? 0, limits.maxHeight ?? 0);
}

/** Returns the window's top-left corner on the desktop. */
export function position(): Promise<Point> {
  return invoke("window.position");
}

/** Moves the window's top-left corner to (x, y) on the desktop. */
export function setPosition(x: number, y: number): Promise<void> {
  return invoke("window.setPosition", x, y);
}

/** Returns position and content size together. */
export function bounds(): Promise<Rect> {
  return invoke("window.bounds");
}

/** Moves and resizes the window in one step. */
export function setBounds(rect: Rect): Promise<void> {
  return invoke("window.setBounds", rect);
}

/** Centers the window on its display. */
export function center(): Promise<void> {
  return invoke("window.center");
}

/** Returns the window state. */
export function state(): Promise<WindowState> {
  return invoke("window.state");
}

/** Minimizes, maximizes, makes fullscreen or restores the window. */
export function setState(state: WindowState): Promise<void> {
  return invoke("window.setState", state);
}

/** Minimizes the window. */
export function minimize(): Promise<void> {
  return invoke("window.minimize");
}

/** Maximizes the window. */
export function maximize(): Promise<void> {
  return invoke("window.maximize");
}

/** Maximizes the window, or restores it when maximized. */
export function toggleMaximize(): Promise<void> {
  return invoke("window.toggleMaximize");
}

/** Makes the window fullscreen. */
export function fullscreen(): Promise<void> {
  return invoke("window.fullscreen");
}

/** Restores the window to the normal state. */
export function restore(): Promise<void> {
  return invoke("window.restore");
}

/** Keeps the window above other windows, or stops doing so. */
export function setAlwaysOnTop(on: boolean): Promise<void> {
  return invoke("window.setAlwaysOnTop", on);
}

/** Reports whether the window is kept above others. */
export function isAlwaysOnTop(): Promise<boolean> {
  return invoke("window.alwaysOnTop");
}

/** Shows the window. */
export function show(): Promise<void> {
  return invoke("window.show");
}

/** Hides the window. */
export function hide(): Promise<void> {
  return invoke("window.hide");
}

/** Brings the window to the front and focuses it. */
export function focus(): Promise<void> {
  return invoke("window.focus");
}

/** Closes the window (OnClose handlers in Go are not asked). The promise
 * resolves just before the window goes away. */
export function close(): Promise<void> {
  return invoke("window.close");
}

/** Returns the page zoom; 1 is 100%. */
export function zoom(): Promise<number> {
  return invoke("window.zoom");
}

/** Sets the page zoom; 1 is 100%. */
export function setZoom(factor: number): Promise<void> {
  return invoke("window.setZoom", factor);
}

/** Reloads the page. */
export function reload(): Promise<void> {
  return invoke("window.reload");
}

/** Opens the system print dialog for the page. */
export function print(): Promise<void> {
  return invoke("window.print");
}

/** Opens the WebView inspector where the platform has one and the window
 * was created with Debug; does nothing otherwise. */
export function openDevTools(): Promise<void> {
  return invoke("window.openDevTools");
}

/** Returns the window title last set (from Go or setTitle). */
export function title(): Promise<string> {
  return invoke("window.title");
}

/** Reports whether the window is shown (minimized windows count as shown;
 * hidden ones, or all windows of a hidden app, do not). */
export function isVisible(): Promise<boolean> {
  return invoke("window.isVisible");
}

/** Reports whether the window has keyboard focus. */
export function isFocused(): Promise<boolean> {
  return invoke("window.isFocused");
}

/** Called with the new content size when the window is resized. */
export function onResize(fn: (size: Size) => void): () => void {
  return listen("window.resize", fn);
}

/** Called with the new position when the window moves. */
export function onMove(fn: (pos: Point) => void): () => void {
  return listen("window.move", fn);
}

/** Called when the window is minimized, maximized, made fullscreen or
 * restored (by the user or by code). */
export function onStateChange(fn: (state: WindowState) => void): () => void {
  return listen<{ state: WindowState }>("window.state", (d) => fn(d.state));
}

/** Called when files are dropped on the page (the window must be created
 * with SurfaceOptions.AcceptFiles). */
export function onFilesDropped(fn: (drop: FilesDrop) => void): () => void {
  return listen("window.filesDropped", fn);
}

/** Called when the window gains keyboard focus (reported by the window
 * system, so it also fires when focus moves between the page and native
 * parts of the window). */
export function onFocus(fn: () => void): () => void {
  return listen<null>("window.focus", () => fn());
}

/** Called when the window loses keyboard focus. */
export function onBlur(fn: () => void): () => void {
  return listen<null>("window.blur", () => fn());
}
