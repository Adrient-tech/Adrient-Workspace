import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { Box, Button, IconButton, Sheet, Tooltip, Typography } from "@mui/joy";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import OpenInNewRoundedIcon from "@mui/icons-material/OpenInNewRounded";
import OpenInFullRoundedIcon from "@mui/icons-material/OpenInFullRounded";
import CloseFullscreenRoundedIcon from "@mui/icons-material/CloseFullscreenRounded";
import { AppIcon } from "./AppIcon";
import { apps } from "../catalog/apps";
import { appLabel, openLink, type OpenTarget } from "../lib/openWith";
import { PANE_FRAME_NAME, parseEmbedMessage, withEmbed, withoutEmbed } from "../lib/embed";

const MIN_W = 320;
const DEFAULT_W = 520;

interface PaneState {
  target: OpenTarget;
  mode: "pane" | "tab";
}

interface AppPaneApi {
  /** Opens an embeddable target in the side pane; others navigate or open a new tab. */
  openInPane: (target: OpenTarget | string) => void;
  closePane: () => void;
  isOpen: boolean;
}

const Ctx = createContext<AppPaneApi | null>(null);

/** useAppPane gives any app access to the shared right-side pane. */
export function useAppPane(): AppPaneApi {
  const v = useContext(Ctx);
  if (v) return v;
  // Outside a provider (tests, public routes): degrade to a full navigation.
  return {
    isOpen: false,
    closePane: () => {},
    openInPane: (t) => {
      const target = typeof t === "string" ? openLink(t) : t;
      if (!target) return;
      if (target.external) window.open(target.href, "_blank", "noopener,noreferrer");
      else window.location.assign(target.href);
    },
  };
}

export function AppPaneProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [state, setState] = useState<PaneState | null>(null);
  const [width, setWidth] = useState(DEFAULT_W);
  const [title, setTitle] = useState("");
  const frameRef = useRef<HTMLIFrameElement>(null);

  const closePane = useCallback(() => setState(null), []);

  const openInPane = useCallback(
    (t: OpenTarget | string) => {
      const target = typeof t === "string" ? openLink(t) : t;
      if (!target) return;
      if (target.external) {
        window.open(target.href, "_blank", "noopener,noreferrer");
      } else if (!target.embeddable) {
        navigate(target.href);
      } else {
        setTitle("");
        setState({ target, mode: "pane" });
      }
    },
    [navigate],
  );

  // Messages from the embedded page.
  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (e.origin !== window.location.origin) return;
      if (e.source !== frameRef.current?.contentWindow) return;
      const m = parseEmbedMessage(e.data);
      if (!m) return;
      if (m.type === "adrient:title") setTitle(m.title);
      else if (m.type === "adrient:navigate") {
        const t = openLink(m.href);
        if (!t) return;
        if (t.external) window.open(t.href, "_blank", "noopener,noreferrer");
        else {
          setState(null);
          navigate(t.href);
        }
      }
    };
    window.addEventListener("message", onMsg);
    return () => window.removeEventListener("message", onMsg);
  }, [navigate]);

  // Push page content aside while the side pane is open.
  const open = state !== null;
  const pane = state?.mode === "pane";
  useEffect(() => {
    document.body.style.paddingRight = pane ? `${width}px` : "";
    return () => {
      document.body.style.paddingRight = "";
    };
  }, [pane, width]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setState(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  const api = useMemo<AppPaneApi>(() => ({ openInPane, closePane, isOpen: open }), [openInPane, closePane, open]);

  // Current location inside the iframe (same origin), falling back to the opened target.
  const currentHref = (): string => {
    try {
      const l = frameRef.current?.contentWindow?.location;
      if (l && l.pathname !== "blank") return withoutEmbed(l.pathname + l.search + l.hash);
    } catch {
      /* cross-origin: ignore */
    }
    return state ? withoutEmbed(state.target.href) : "/";
  };

  const startResize = (e: React.PointerEvent) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      const w = window.innerWidth - ev.clientX;
      setWidth(Math.max(MIN_W, Math.min(w, window.innerWidth - 240)));
    };
    const up = () => {
      document.body.classList.remove("adrient-resizing");
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    document.body.classList.add("adrient-resizing");
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const app = state ? apps.find((a) => a.id === state.target.app) : undefined;
  const label = state ? appLabel(state.target.app) : "";

  return (
    <Ctx.Provider value={api}>
      {children}
      {state && (
        <Sheet
          component="aside"
          role="complementary"
          aria-label={`${label} pane`}
          data-testid="app-pane"
          data-mode={state.mode}
          variant="outlined"
          sx={{
            position: "fixed",
            top: 0,
            right: 0,
            bottom: 0,
            width: state.mode === "tab" ? "100vw" : width,
            zIndex: 1300,
            display: "flex",
            flexDirection: "column",
            borderRadius: 0,
            boxShadow: "lg",
          }}
        >
          {state.mode === "pane" && (
            <Box
              onPointerDown={startResize}
              data-testid="app-pane-resize"
              sx={{ position: "absolute", left: -3, top: 0, bottom: 0, width: 6, cursor: "col-resize", zIndex: 1 }}
            />
          )}
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider" }}>
            {app && <AppIcon iconName={app.iconName} color={app.accentColor} iconUrl={app.iconUrl} label={app.name} size={24} />}
            <Typography level="title-sm" noWrap sx={{ flex: 1 }} data-testid="app-pane-title">
              {title || label}
            </Typography>
            <Tooltip title={`Open in ${label}`}>
              <Button
                size="sm"
                variant="soft"
                data-testid="app-pane-open"
                aria-label={`Open in ${label}`}
                onClick={() => {
                  const href = currentHref();
                  setState(null);
                  navigate(href);
                }}
              >
                Open in {label}
              </Button>
            </Tooltip>
            <Tooltip title={state.mode === "tab" ? "Side pane" : "Full tab"}>
              <IconButton
                size="sm"
                variant="plain"
                aria-label={state.mode === "tab" ? "Side pane" : "Full tab"}
                onClick={() => setState({ ...state, mode: state.mode === "tab" ? "pane" : "tab" })}
              >
                {state.mode === "tab" ? <CloseFullscreenRoundedIcon /> : <OpenInFullRoundedIcon />}
              </IconButton>
            </Tooltip>
            <Tooltip title="Pop out">
              <IconButton
                size="sm"
                variant="plain"
                data-testid="app-pane-popout"
                aria-label="Pop out"
                onClick={() => {
                  window.open(currentHref(), "_blank", "noopener");
                  setState(null);
                }}
              >
                <OpenInNewRoundedIcon />
              </IconButton>
            </Tooltip>
            <Tooltip title="Close">
              <IconButton size="sm" variant="plain" aria-label="Close pane" data-testid="app-pane-close" onClick={closePane}>
                <CloseRoundedIcon />
              </IconButton>
            </Tooltip>
          </Box>
          <Box
            component="iframe"
            ref={frameRef}
            key={state.target.href}
            name={PANE_FRAME_NAME}
            title={`${label} pane`}
            src={withEmbed(state.target.href)}
            sx={{ flex: 1, border: 0, width: "100%", "body.adrient-resizing &": { pointerEvents: "none" } }}
          />
        </Sheet>
      )}
    </Ctx.Provider>
  );
}
