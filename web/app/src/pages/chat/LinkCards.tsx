/**
 * Rich cards for links and files in chat messages (Teams-style):
 *  - meeting links/codes  -> "Join meeting" card (joins docked in Chat)
 *  - Docs/Sheets/Slides/PDF/Drive links and attachments -> file card with
 *    "Open" (inside Chat, in the side pane) and "Open in <App>"
 *  - other workspace links (invoice, deal, task, ticket, event) -> small card
 * Classification always goes through lib/openWith.
 */
import { useEffect, useState } from "react";
import { Box, Button, CircularProgress, Typography } from "@mui/joy";
import { useNavigate } from "react-router-dom";
import { AppIcon } from "../../components/AppIcon";
import { useAppPane } from "../../components/AppPane";
import { apps } from "../../catalog/apps";
import {
  appForFile,
  appLabel,
  openDriveFile,
  openLink,
  withReturn,
  type AppId,
  type OpenTarget,
} from "../../lib/openWith";
import { meetPath } from "../../lib/calls";
import { fmtMeetDuration, useMeetInfo } from "../meet/meetStatus";
import { useActiveCall } from "../meet/callStore";
import { getFile, uploadFile } from "../drive/api";
import type { ChatAttachment } from "./types";

const URL_RE = /https?:\/\/[^\s<>()[\]"']+/g;
const CODE_RE = /(?:^|[\s(])([a-z]{3}-[a-z]{4}-[a-z]{3})(?=$|[\s).,!?])/g;
const MAX_CARDS = 3;

/** Path to return to after opening another app: the channel (no message suffix) or current path. */
export function chatReturn(): string {
  const m = /^\/chat\/c\/([^/]+)/.exec(window.location.pathname);
  return m ? `/chat/c/${m[1]}` : window.location.pathname;
}

/** FILE_APPS open a Drive file in an editor/viewer. */
const FILE_APPS = new Set<AppId>(["docs", "sheets", "slides", "pdf", "whiteboard", "3d", "drive"]);
/** Editors that need a Drive id to open an uploaded chat attachment. */
const EDITOR_APPS = new Set<AppId>(["docs", "sheets", "slides", "pdf", "whiteboard"]);

export interface CardLink {
  url: string;
  target: OpenTarget;
}

/** Finds the links in a message body that deserve a card (workspace links, meeting codes). */
export function extractCardLinks(body: string, origin = window.location.origin): CardLink[] {
  const seen = new Set<string>();
  const out: CardLink[] = [];
  const add = (raw: string) => {
    const url = raw.replace(/[.,;:!?]+$/, "");
    const t = openLink(url, origin);
    if (!t || t.external || seen.has(t.href)) return;
    if (t.app === "other" || t.app === "chat") return;
    seen.add(t.href);
    out.push({ url, target: t });
  };
  for (const m of body.matchAll(URL_RE)) add(m[0]);
  for (const m of body.matchAll(CODE_RE)) add(m[1]);
  return out.slice(0, MAX_CARDS);
}

function appTile(app: AppId) {
  return apps.find((a) => a.id === app);
}

function CardIcon({ app, size = 32 }: { app: AppId; size?: number }) {
  const t = appTile(app);
  if (!t) return <Box sx={{ width: size, height: size }} />;
  return <AppIcon iconName={t.iconName} color={t.accentColor} iconUrl={t.iconUrl} label={t.name} size={size} />;
}

const cardSx = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: 1.25,
  p: 1,
  pr: 1.25,
  mt: 0.75,
  maxWidth: 460,
  border: "1px solid",
  borderColor: "divider",
  borderRadius: "md",
  bgcolor: "background.surface",
} as const;

const nameCache = new Map<string, Promise<string>>();
function cached(key: string, load: () => Promise<string>): Promise<string> {
  let p = nameCache.get(key);
  if (!p) {
    p = load().catch(() => "");
    nameCache.set(key, p);
  }
  return p;
}

function useAsyncName(key: string, load: () => Promise<string>, fallback: string): string {
  const [name, setName] = useState("");
  useEffect(() => {
    let live = true;
    void cached(key, load).then((n) => live && setName(n));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return name || fallback;
}

function driveIdOf(href: string): string {
  const m = /^\/(?:docs|sheets|slides|pdf|whiteboard|drive\/file)\/([^/?#]+)/.exec(href);
  if (m) return m[1];
  return new URLSearchParams(href.split("?")[1] ?? "").get("file") ?? "";
}

function MeetCard({ code }: { code: string }) {
  const navigate = useNavigate();
  const call = useActiveCall();
  const info = useMeetInfo(code);
  const title = info?.name || "Meeting";
  const ended = !!info?.ended;
  const inCall = call?.callPath === `/meet/${code}`;
  const people = info?.participants ?? 0;
  return (
    <Box sx={{ ...cardSx, opacity: ended ? 0.85 : 1 }} data-testid="meet-card" data-state={ended ? "ended" : "open"}>
      <CardIcon app="meet" />
      <Box sx={{ minWidth: 120, flex: 1 }}>
        <Typography level="title-sm" noWrap>
          {title}
        </Typography>
        {ended ? (
          <Typography level="body-xs" sx={{ opacity: 0.8 }} data-testid="meet-card-ended">
            Meeting ended · {fmtMeetDuration(info?.durationS ?? 0)} · {people} {people === 1 ? "participant" : "participants"}
          </Typography>
        ) : (
          <Typography level="body-xs" sx={{ opacity: 0.7 }}>
            Meeting · {code}
          </Typography>
        )}
      </Box>
      {!ended && (
        <Button
          size="sm"
          disabled={inCall}
          data-testid="meet-card-join"
          onClick={() => navigate(meetPath(code, { returnTo: chatReturn() }))}
        >
          {inCall ? "In call" : "Join meeting"}
        </Button>
      )}
    </Box>
  );
}

function FileCard({ target }: { target: OpenTarget }) {
  const pane = useAppPane();
  const navigate = useNavigate();
  const id = driveIdOf(target.href);
  const fallback = `${appLabel(target.app)} file`;
  const name = useAsyncName(`file:${id}`, () => (id ? getFile(id).then((f) => f.name) : Promise.resolve("")), fallback);
  return (
    <Box sx={cardSx} data-testid="file-card">
      <CardIcon app={target.app} />
      <Box sx={{ minWidth: 120, flex: 1 }}>
        <Typography level="title-sm" noWrap>
          {name}
        </Typography>
        <Typography level="body-xs" sx={{ opacity: 0.7 }}>
          {appLabel(target.app)}
        </Typography>
      </Box>
      <Button size="sm" variant="soft" data-testid="file-card-open" onClick={() => pane.openInPane(target)}>
        Open
      </Button>
      <Button
        size="sm"
        variant="plain"
        color="neutral"
        data-testid="file-card-open-app"
        onClick={() => navigate(withReturn(target.href, chatReturn()))}
      >
        Open in {appLabel(target.app)}
      </Button>
    </Box>
  );
}

function SmallCard({ target }: { target: OpenTarget }) {
  const pane = useAppPane();
  const navigate = useNavigate();
  return (
    <Box
      role="button"
      tabIndex={0}
      sx={{ ...cardSx, cursor: "pointer", py: 0.75, "&:hover": { bgcolor: "background.level1" } }}
      data-testid="app-link-card"
      onClick={() => (target.embeddable ? pane.openInPane(target) : navigate(withReturn(target.href, chatReturn())))}
      onKeyDown={(e) => {
        if (e.key === "Enter") pane.openInPane(target);
      }}
    >
      <CardIcon app={target.app} size={26} />
      <Typography level="body-sm" sx={{ flex: 1 }} noWrap>
        {appLabel(target.app)} · {target.href.split("?")[0]}
      </Typography>
      <Typography level="body-xs" sx={{ opacity: 0.6 }}>
        {target.embeddable ? "Open" : "Open in app"}
      </Typography>
    </Box>
  );
}

/** Cards for the links found in one message body. */
export function LinkCards({ body }: { body: string }) {
  const links = extractCardLinks(body);
  if (links.length === 0) return null;
  return (
    <Box data-testid="link-cards">
      {links.map((l) =>
        l.target.app === "meet" ? (
          <MeetCard key={l.target.href} code={l.target.href.split("/")[2]?.split("?")[0] ?? ""} />
        ) : FILE_APPS.has(l.target.app) ? (
          <FileCard key={l.target.href} target={l.target} />
        ) : (
          <SmallCard key={l.target.href} target={l.target} />
        ),
      )}
    </Box>
  );
}

// ---- attachments --------------------------------------------------------------

const savedToDrive = new Map<string, Promise<string>>();

/** Copies a chat attachment into Drive once and returns the Drive file id. */
function saveToDrive(att: ChatAttachment): Promise<string> {
  let p = savedToDrive.get(att.id);
  if (!p) {
    p = (async () => {
      const r = await fetch(att.url, { credentials: "same-origin" });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      const blob = await r.blob();
      const f = await uploadFile(new File([blob], att.name, { type: att.mime_type || blob.type }));
      return f.id;
    })();
    p.catch(() => savedToDrive.delete(att.id));
    savedToDrive.set(att.id, p);
  }
  return p;
}

/** True when an attachment gets a file card (editable/viewable in a workspace app). */
export function isCardAttachment(att: ChatAttachment): boolean {
  return EDITOR_APPS.has(appForFile({ name: att.name, mime_type: att.mime_type }));
}

export function AttachmentCard({ att, size }: { att: ChatAttachment; size: string }) {
  const pane = useAppPane();
  const navigate = useNavigate();
  const app = appForFile({ name: att.name, mime_type: att.mime_type });
  const [busy, setBusy] = useState<"" | "pane" | "app">("");
  const [err, setErr] = useState("");
  const go = async (mode: "pane" | "app") => {
    setBusy(mode);
    setErr("");
    try {
      const id = await saveToDrive(att);
      const t = openDriveFile({ name: att.name, mime_type: att.mime_type, drive_id: id });
      if (mode === "pane") pane.openInPane(t);
      else navigate(withReturn(t.href, chatReturn()));
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  };
  return (
    <Box sx={cardSx} data-testid="file-card">
      <CardIcon app={app} />
      <Box sx={{ minWidth: 120, flex: 1 }}>
        <Typography level="title-sm" noWrap>
          {att.name}
        </Typography>
        <Typography level="body-xs" sx={{ opacity: 0.7 }} color={err ? "danger" : undefined}>
          {err ? `Could not open: ${err}` : `${appLabel(app)} · ${size}`}
        </Typography>
      </Box>
      <Button
        size="sm"
        variant="soft"
        data-testid="file-card-open"
        disabled={!!busy}
        startDecorator={busy === "pane" ? <CircularProgress size="sm" /> : undefined}
        onClick={() => void go("pane")}
      >
        Open
      </Button>
      <Button size="sm" variant="plain" color="neutral" data-testid="file-card-open-app" disabled={!!busy} onClick={() => void go("app")}>
        Open in {appLabel(app)}
      </Button>
      <Button size="sm" variant="plain" color="neutral" component="a" href={att.url} download={att.name}>
        Download
      </Button>
    </Box>
  );
}
