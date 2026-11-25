import { inAppHref } from "./openWith";

/** Server-owned paths that need a real page load. */
export const SERVER_PREFIXES = /^\/(api|auth|oauth|login|logout|healthz|metrics|dl|download|files|ui)(\/|$|\?)/;

/**
 * Pure decision for the global click handler: returns the in-app href to
 * navigate to, or null when the browser should handle the click itself.
 */
export function interceptHref(
  a: Pick<HTMLAnchorElement, "href" | "target" | "hasAttribute" | "getAttribute">,
  ev: Pick<MouseEvent, "button" | "metaKey" | "ctrlKey" | "shiftKey" | "altKey" | "defaultPrevented">,
  origin: string,
  current: string,
): string | null {
  if (ev.defaultPrevented || ev.button !== 0) return null;
  if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return null;
  if (a.hasAttribute("download") || a.hasAttribute("data-no-intercept")) return null;
  const raw = a.getAttribute("href") ?? "";
  if (!raw || raw.startsWith("#")) return null;
  const proto = /^([a-z][a-z0-9+.-]*):/i.exec(raw)?.[1]?.toLowerCase();
  if (proto && proto !== "http" && proto !== "https") return null;
  const target = a.target;
  if (target && target !== "_blank" && target !== "_self") return null;
  const path = inAppHref(a.href, origin);
  if (!path || SERVER_PREFIXES.test(path)) return null;
  // Hash-only change on the current page: let the browser scroll.
  if (path.split("#")[0] === current.split("#")[0] && path.includes("#")) return null;
  return path;
}

