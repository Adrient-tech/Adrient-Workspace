import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { Box, CircularProgress, Input, Modal, ModalDialog, Typography } from "@mui/joy";
import SearchRoundedIcon from "@mui/icons-material/SearchRounded";
import { useNavigate } from "react-router-dom";
import { AppIcon } from "../../components/AppIcon";
import { useOpenLink } from "../../components/LinkHandler";
import { search as apiSearch } from "../../api/client";
import { searchDirectory, type Member } from "../../api/directory";
import type { SearchGroup } from "../../api/types";
import { useVisibleApps } from "../../catalog/useVisibleApps";
import { usePinnedApps } from "../../catalog/pins";
import { externalHref } from "../../catalog/apps";
import { buildItems, type PaletteItem } from "./paletteLogic";
import { QUICK_ACTIONS, runQuickAction } from "./quickActions";
import { closePalette, isCommandKey, openPalette, shouldIgnoreCommandKey, usePalette } from "./paletteStore";

const AskAdrient = lazy(() => import("../../lib/ai/AskAdrient"));

const KIND_LABEL: Record<PaletteItem["kind"], string> = {
  app: "Apps",
  action: "Quick actions",
  file: "Files and results",
  person: "People",
  ask: "Ask",
};

/** CommandPaletteHost listens for Ctrl-K / Cmd-K and renders the palette. It is
 *  mounted once in the app shell for signed-in users. */
export function CommandPaletteHost() {
  const { open, initial } = usePalette();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isCommandKey(e) && !shouldIgnoreCommandKey(e.target)) {
        e.preventDefault();
        openPalette();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  return open ? <PaletteDialog initial={initial} /> : null;
}

function PaletteDialog({ initial }: { initial: string }) {
  const navigate = useNavigate();
  const openUrl = useOpenLink();
  const { apps } = useVisibleApps();
  const { pins } = usePinnedApps();
  const [query, setQuery] = useState(initial);
  const [groups, setGroups] = useState<SearchGroup[]>([]);
  const [people, setPeople] = useState<Member[]>([]);
  const [loading, setLoading] = useState(false);
  const [sel, setSel] = useState(0);
  const [ask, setAsk] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Debounced server search (files, mail, events...) and people directory.
  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setGroups([]);
      setPeople([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    let alive = true;
    const t = setTimeout(() => {
      Promise.allSettled([apiSearch(q, 20), searchDirectory(q)]).then(([s, d]) => {
        if (!alive) return;
        setGroups(s.status === "fulfilled" ? (s.value.groups ?? []) : []);
        setPeople(d.status === "fulfilled" ? d.value : []);
        setLoading(false);
      });
    }, 220);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [query]);

  const items = useMemo(
    () => buildItems({ query, apps, pinned: pins, actions: QUICK_ACTIONS, groups, people }),
    [query, apps, pins, groups, people],
  );

  useEffect(() => setSel(0), [query]);
  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-idx="${sel}"]`)?.scrollIntoView({ block: "nearest" });
  }, [sel]);

  function run(item: PaletteItem) {
    switch (item.kind) {
      case "ask":
        setAsk(item.target);
        return;
      case "app": {
        const app = apps.find((a) => a.id === item.target);
        closePalette();
        const href = app ? externalHref(app) : undefined;
        if (href) window.open(href, "_blank", "noopener,noreferrer");
        else navigate(`/${item.target}`);
        return;
      }
      case "action":
        closePalette();
        void runQuickAction(item.target, navigate).catch(() => navigate(`/${QUICK_ACTIONS.find((a) => a.id === item.target)?.appId ?? ""}`));
        return;
      case "file":
        closePalette();
        if (!openUrl(item.target)) navigate(item.target);
        return;
      case "person":
        closePalette();
        navigate(`/contacts?q=${encodeURIComponent(item.target)}`);
        return;
    }
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(s + 1, items.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(s - 1, 0));
    } else if (e.key === "Enter" && items[sel]) {
      e.preventDefault();
      run(items[sel]);
    }
  }

  if (ask !== null) {
    return (
      <Suspense fallback={null}>
        <AskAdrient question={ask} onClose={closePalette} />
      </Suspense>
    );
  }

  let lastKind = "";
  return (
    <Modal open onClose={closePalette} slotProps={{ backdrop: { sx: { backdropFilter: "blur(6px)", bgcolor: "rgba(8, 26, 22, 0.42)" } } }}>
      <ModalDialog
        layout="center"
        data-testid="command-palette"
        sx={{
          p: 0,
          width: "min(640px, calc(100vw - 24px))",
          maxHeight: "min(560px, calc(100vh - 80px))",
          alignSelf: "flex-start",
          mt: { xs: 4, sm: "12vh" },
          overflow: "hidden",
          gap: 0,
        }}
      >
        <Box sx={{ px: 2, py: 1.5, borderBottom: "1px solid", borderColor: "divider" }}>
          <Input
            autoFocus
            variant="plain"
            size="lg"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search apps, files, people, or ask Adrient…"
            aria-label="Global search"
            startDecorator={
              loading ? <CircularProgress size="sm" thickness={2} sx={{ "--CircularProgress-size": "18px" }} /> : <SearchRoundedIcon />
            }
            endDecorator={
              <Typography level="body-xs" sx={{ px: 0.75, py: 0.25, border: "1px solid", borderColor: "divider", borderRadius: "sm" }}>
                Esc
              </Typography>
            }
            sx={{ "--Input-focusedThickness": "0px", bgcolor: "transparent", px: 0 }}
          />
        </Box>
        <Box ref={listRef} role="listbox" sx={{ overflowY: "auto", py: 0.75, px: 0.75 }}>
          {items.map((it, i) => {
            const header = it.kind !== lastKind ? KIND_LABEL[it.kind] : null;
            lastKind = it.kind;
            return (
              <Box key={it.key}>
                {header && (
                  <Typography level="body-xs" sx={{ px: 1.25, pt: 1, pb: 0.5, fontWeight: 700, letterSpacing: "0.06em", textTransform: "uppercase", color: "text.tertiary" }}>
                    {header}
                  </Typography>
                )}
                <Box
                  role="option"
                  aria-selected={i === sel}
                  data-idx={i}
                  data-testid={`palette-${it.kind}-${it.target}`}
                  onMouseMove={() => setSel(i)}
                  onClick={() => run(it)}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1.5,
                    px: 1.25,
                    py: 0.9,
                    borderRadius: "md",
                    cursor: "pointer",
                    bgcolor: i === sel ? "background.level1" : "transparent",
                    outline: i === sel ? "1px solid var(--joy-palette-divider)" : "none",
                  }}
                >
                  <AppIcon iconName={it.glyph} size={30} label={it.title} />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography level="title-sm" noWrap>
                      {it.title}
                    </Typography>
                    {it.subtitle && (
                      <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
                        {it.subtitle}
                      </Typography>
                    )}
                  </Box>
                  {i === sel && (
                    <Typography level="body-xs" sx={{ color: "text.tertiary" }}>
                      Enter
                    </Typography>
                  )}
                </Box>
              </Box>
            );
          })}
        </Box>
      </ModalDialog>
    </Modal>
  );
}
