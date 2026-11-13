// Screens: the displays. Needs Info (on by default).

import { invoke, listen, type Rect } from "./internal.js";

/** One display, in logical pixels. */
export interface Display {
  id: string;
  name: string;
  bounds: Rect;
  /** bounds without the menu bar, dock and taskbars. */
  workArea: Rect;
  /** Device pixels per logical pixel. */
  scale: number;
  primary: boolean;
}

/** Returns every display, the primary one first. */
export function all(): Promise<Display[]> {
  return invoke("screens.all");
}

/** Returns the primary display. */
export function primary(): Promise<Display> {
  return invoke("screens.primary");
}

/** Returns the display the calling window is on. */
export function current(): Promise<Display> {
  return invoke("screens.current");
}

/** Called with the new list when displays are added, removed, rearranged
 * or rescaled. */
export function onChange(fn: (displays: Display[]) => void): () => void {
  return listen("screens.change", fn);
}
