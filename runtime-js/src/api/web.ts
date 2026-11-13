// Web: the calling page's WebView. Every part is off by default and has its
// own switch in Go (jsapi.WebFind, WebCapture, WebCookies, WebDownloads,
// WebControl). Permission requests, HTTP authentication, new windows and the
// navigation policy are decided in Go only.

import { invoke, listen } from "./internal.js";

export interface FindOptions {
  matchCase?: boolean;
  /** Search towards the start of the page. */
  backward?: boolean;
  /** Stop at the end instead of wrapping around. */
  noWrap?: boolean;
}

export interface FindResult {
  /** Number of matches; -1 where the platform does not count. */
  matches: number;
  /** 1-based index of the highlighted match; 0 when unknown or none. */
  active: number;
  found: boolean;
}

/** Searches the page and highlights the first match (WebFind). */
export function find(text: string, options: FindOptions = {}): Promise<FindResult> {
  return invoke("web.find", text, options);
}

/** Highlights the next match of the current search (WebFind). */
export function findNext(): Promise<FindResult> {
  return invoke("web.findNext");
}

/** Highlights the previous match of the current search (WebFind). */
export function findPrevious(): Promise<FindResult> {
  return invoke("web.findPrevious");
}

/** Ends the search and clears the highlight (WebFind). */
export function stopFind(): Promise<void> {
  return invoke("web.stopFind");
}

export interface PDFOptions {
  landscape?: boolean;
  noBackground?: boolean;
  /** Inches; 0 or absent for the platform default. */
  pageWidth?: number;
  pageHeight?: number;
}

/** Renders the page as PDF; resolves to base64 (WebCapture). */
export function printToPDF(options: PDFOptions = {}): Promise<string> {
  return invoke("web.printToPDF", options);
}

/** Captures the visible page as PNG; resolves to base64 (WebCapture). */
export function screenshot(): Promise<string> {
  return invoke("web.screenshot");
}

export interface Cookie {
  name: string;
  value: string;
  /** "example.com" (host only) or ".example.com" (with subdomains). */
  domain: string;
  path?: string;
  /** Milliseconds since the epoch; null or absent for a session cookie. */
  expires?: number | null;
  secure?: boolean;
  httpOnly?: boolean;
  sameSite?: "" | "lax" | "strict" | "none";
}

/** Cookies that would be sent to url, or all cookies (WebCookies). */
export function cookies(url?: string): Promise<Cookie[]> {
  return url === undefined ? invoke("web.cookies") : invoke("web.cookies", url);
}

/** Adds or replaces a cookie (WebCookies). */
export function setCookie(cookie: Cookie): Promise<void> {
  return invoke("web.setCookie", cookie);
}

/** Deletes the cookie with the same name, domain and path (WebCookies). */
export function deleteCookie(cookie: Pick<Cookie, "name" | "domain" | "path">): Promise<void> {
  return invoke("web.deleteCookie", cookie);
}

/** Deletes the cookies of domain and its subdomains, or all (WebCookies). */
export function clearCookies(domain = ""): Promise<void> {
  return invoke("web.clearCookies", domain);
}

export type DataKind = "cookies" | "cache" | "storage" | "all";

/** Clears website data of every site; no argument clears all (WebCookies). */
export function clearData(kinds?: DataKind[]): Promise<void> {
  return kinds === undefined ? invoke("web.clearData") : invoke("web.clearData", kinds);
}

export interface Download {
  id: number;
  state: "progress" | "done" | "failed" | "canceled";
  received: number;
  /** -1 when unknown. */
  total: number;
  path: string;
  error?: string;
}

/** Subscribes to progress and completion of this page's downloads
 * (WebDownloads). Returns unsubscribe. */
export function onDownload(fn: (d: Download) => void): () => void {
  return listen<Download>("web.download", fn);
}

/** Cancels a download of this page (WebDownloads). */
export function cancelDownload(id: number): Promise<void> {
  return invoke("web.cancelDownload", id);
}

/** Changes the User-Agent of later requests; "" restores the platform's
 * (WebControl). */
export function setUserAgent(ua: string): Promise<void> {
  return invoke("web.setUserAgent", ua);
}
