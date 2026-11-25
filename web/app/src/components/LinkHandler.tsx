import { useCallback, useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { openLink } from "../lib/openWith";
import { interceptHref } from "../lib/linkIntercept";
import { useAppPane } from "./AppPane";
import { isEmbedded, postToParent, withEmbed } from "../lib/embed";

/**
 * Global in-app navigation: same-origin workspace links use react-router
 * instead of a full reload. In an embedded pane, links to apps that are not
 * embeddable ask the parent window to navigate instead.
 */
export function LinkHandler() {
  const navigate = useNavigate();
  const location = useLocation();
  const pane = useAppPane();
  const paneRef = useRef(pane);
  paneRef.current = pane;

  useEffect(() => {
    const embedded = isEmbedded();
    const onClick = (e: MouseEvent) => {
      const el = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
      if (!el) return;
      // <a href="/sheets/1" data-open-pane> opens the side pane instead of navigating.
      if (el.hasAttribute("data-open-pane") && !e.defaultPrevented && !e.metaKey && !e.ctrlKey && !e.shiftKey && e.button === 0) {
        const t = openLink(el.getAttribute("href") ?? "");
        if (t) {
          e.preventDefault();
          if (embedded) postToParent({ type: "adrient:navigate", href: t.href });
          else paneRef.current.openInPane(t);
          return;
        }
      }
      const current = window.location.pathname + window.location.search + window.location.hash;
      const path = interceptHref(el, e, window.location.origin, current);
      if (!path) {
        // External link inside a pane must not replace the pane's page.
        if (embedded && !e.defaultPrevented && !el.target && /^https?:/i.test(el.getAttribute("href") ?? "")) {
          e.preventDefault();
          window.open(el.href, "_blank", "noopener,noreferrer");
        }
        return;
      }
      e.preventDefault();
      if (embedded) {
        const t = openLink(path);
        if (t?.embeddable) navigate(withEmbed(path));
        else postToParent({ type: "adrient:navigate", href: path });
        return;
      }
      navigate(path);
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, [navigate]);

  // Embedded pages tell the parent their title.
  useEffect(() => {
    if (!isEmbedded()) return;
    const send = () => postToParent({ type: "adrient:title", title: document.title });
    send();
    const el = document.querySelector("title");
    if (!el) return;
    const mo = new MutationObserver(send);
    mo.observe(el, { childList: true, characterData: true, subtree: true });
    return () => mo.disconnect();
  }, [location.pathname]);

  return null;
}

/** Opens any URL through the resolver: in-app navigate, external in a new tab. */
export function useOpenLink() {
  const navigate = useNavigate();
  return useCallback(
    (url: string) => {
      const t = openLink(url);
      if (!t) return false;
      if (t.external) window.open(t.href, "_blank", "noopener,noreferrer");
      else navigate(t.href);
      return true;
    },
    [navigate],
  );
}
