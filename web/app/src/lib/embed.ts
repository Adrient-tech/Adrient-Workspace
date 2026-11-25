/**
 * Embedded mode: a route opened with ?embed=1 runs inside an AppPane iframe.
 * The parent window owns the Header, CallHost and LiveCenter, so the embedded
 * page hides them and talks to the parent with postMessage.
 */

export type EmbedMessage =
  | { type: "adrient:navigate"; href: string }
  | { type: "adrient:title"; title: string };

/** True when the search string carries embed=1. */
export function isEmbedSearch(search: string): boolean {
  return new URLSearchParams(search).get("embed") === "1";
}

/** Name given to AppPane iframes; it survives in-frame navigation, unlike ?embed=1. */
export const PANE_FRAME_NAME = "adrient-pane";

/** True when the current page is an embedded pane (inside an AppPane iframe). */
export function isEmbedded(): boolean {
  if (typeof window === "undefined" || window.parent === window) return false;
  return isEmbedSearch(window.location.search) || window.name === PANE_FRAME_NAME;
}

/** Adds embed=1 to an in-app href (keeps hash and existing query). */
export function withEmbed(href: string): string {
  if (!href.startsWith("/") || href.startsWith("//")) return href;
  const [rest, hash = ""] = href.split("#");
  if (/[?&]embed=1(&|$)/.test(rest)) return href;
  return `${rest}${rest.includes("?") ? "&" : "?"}embed=1${hash ? `#${hash}` : ""}`;
}

/** Removes embed=1 from an in-app href. */
export function withoutEmbed(href: string): string {
  const [rest, hash = ""] = href.split("#");
  const [path, query = ""] = rest.split("?");
  const params = new URLSearchParams(query);
  params.delete("embed");
  const q = params.toString();
  return `${path}${q ? `?${q}` : ""}${hash ? `#${hash}` : ""}`;
}

/** Sends a message to the parent window (same origin only). No-op when not embedded. */
export function postToParent(msg: EmbedMessage): void {
  if (typeof window === "undefined" || window.parent === window) return;
  window.parent.postMessage(msg, window.location.origin);
}

/** Type guard for messages coming from an embedded pane. */
export function parseEmbedMessage(data: unknown): EmbedMessage | null {
  if (!data || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  if (d.type === "adrient:navigate" && typeof d.href === "string") return { type: d.type, href: d.href };
  if (d.type === "adrient:title" && typeof d.title === "string") return { type: d.type, title: d.title };
  return null;
}
