// Menu: native context menus the page describes, shown in its window.
// Needs Menu (on by default). Standard items are limited to edit roles.

import { invoke, report, type Point } from "./internal.js";

/** Standard edit actions the platform labels and performs itself. */
export type MenuRole = "copy" | "paste" | "cut" | "selectAll" | "undo" | "redo";

/** One menu entry. */
export interface MenuItem {
  /** Reported back when chosen; defaults to the item's index. */
  id?: string;
  /** Default "normal"; or "checkbox", "radio", "separator". */
  type?: "normal" | "checkbox" | "radio" | "separator";
  label?: string;
  /** Shown as a hint, e.g. "CmdOrCtrl+C". */
  accelerator?: string;
  disabled?: boolean;
  checked?: boolean;
  /** Radio group within the same submenu. */
  group?: string;
  role?: MenuRole;
  /** PNG icon as base64 (macOS, Windows). */
  icon?: string;
  submenu?: MenuItem[];
  /** Called when this item is chosen (checked is the new state). */
  onClick?: (checked: boolean) => void;
}

/** The item the user chose. */
export interface MenuChoice {
  id: string;
  /** New checked state of checkbox / radio items. */
  checked: boolean;
}

function strip(items: MenuItem[], prefix: string, fns: Map<string, (checked: boolean) => void>): unknown[] {
  return items.map((it, i) => {
    const id = it.id ?? prefix + i;
    if (it.onClick) fns.set(id, it.onClick);
    const { onClick: _, submenu, ...rest } = it;
    const out: Record<string, unknown> = { ...rest, id };
    if (submenu) out.submenu = strip(submenu, id + ".", fns);
    return out;
  });
}

/** Shows items as a context menu at `at` (page coordinates, e.g. the
 * event's clientX/clientY) and resolves to the chosen item, or null when
 * dismissed. The chosen item's onClick runs before the promise resolves. */
export async function popup(items: MenuItem[], at: Point): Promise<MenuChoice | null> {
  const fns = new Map<string, (checked: boolean) => void>();
  const wire = strip(items, "", fns);
  const r = await invoke<{ id: string | null; checked: boolean }>(
    "menu.popup",
    wire,
    Math.round(at.x),
    Math.round(at.y),
  );
  if (r.id === null) return null;
  const fn = fns.get(r.id);
  if (fn) {
    try {
      fn(r.checked);
    } catch (e) {
      report(e);
    }
  }
  return { id: r.id, checked: r.checked };
}

/** Minimal shape of a DOM event target, so this module needs no DOM types. */
export interface EventTargetLike {
  addEventListener(type: string, fn: (ev: any) => void): void;
  removeEventListener(type: string, fn: (ev: any) => void): void;
}

/** Replaces the browser context menu of target (an element, or window)
 * with a native one built by build from the contextmenu event; build may
 * return null to keep the browser menu. Returns a function that undoes it. */
export function bindContextMenu(
  target: EventTargetLike,
  build: (ev: any) => MenuItem[] | null,
): () => void {
  const h = (ev: any) => {
    const items = build(ev);
    if (!items) return;
    ev.preventDefault();
    popup(items, { x: ev.clientX, y: ev.clientY }).catch(report);
  };
  target.addEventListener("contextmenu", h);
  return () => target.removeEventListener("contextmenu", h);
}
