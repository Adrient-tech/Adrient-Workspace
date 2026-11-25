/** Chat > Calls (history with call back) and Chat > Files (recent Drive files). */
import { useEffect, useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, Typography } from "@mui/joy";
import CallOutlinedIcon from "@mui/icons-material/CallOutlined";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import CallReceivedIcon from "@mui/icons-material/CallReceived";
import CallMadeIcon from "@mui/icons-material/CallMade";
import { useNavigate } from "react-router-dom";
import { listCallHistory, startCall, type CallLogEntry } from "../../lib/calls";
import { AppIcon } from "../../components/AppIcon";
import { HangupIcon } from "../../components/HangupIcon";
import { useAppPane } from "../../components/AppPane";
import { apps } from "../../catalog/apps";
import { appForFile, openDriveFile, withReturn } from "../../lib/openWith";
import { listRecent } from "../drive/api";
import type { DriveFile } from "../drive/types";
import { UserAvatar } from "./Message";
import type { TeamsState } from "./useTeams";

// ---- calls log ----------------------------------------------------------------
// History lives on the server (chat_calls): rings, answers, declines, misses
// and call end are recorded there, so every browser and device shows the same list.

/** Kept for callers that logged outgoing calls locally; the server logs them now. */
export function logOutgoingCall(_c: { at: string; userIds: string[]; title: string; video: boolean }) {
  /* no-op: the server records the ring */
}

function fmtDuration(s: number): string {
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return m < 60 ? `${m}m ${s % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

const STATUS_COLOR: Record<string, "danger" | "success" | "neutral" | "warning"> = {
  missed: "danger",
  answered: "success",
  declined: "neutral",
  ringing: "warning",
};

export function CallsView({ state, toast }: { state: TeamsState; toast: (t: string, b?: string) => void }) {
  const navigate = useNavigate();
  const [rows, setRows] = useState<CallLogEntry[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  useEffect(() => {
    let live = true;
    listCallHistory()
      .then((r) => live && setRows(r))
      .catch((e: Error) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, []);

  const callBack = async (r: CallLogEntry, video: boolean) => {
    const targets = r.direction === "in" ? [r.caller_id] : r.peer_ids;
    if (!targets.length) return;
    setBusy(r.id + video);
    try {
      const who = state.peopleMap[targets[0]]?.name ?? "Call";
      const res = await startCall({ userIds: targets, title: who, video, returnTo: "/chat/calls" });
      navigate(res.path);
    } catch (e) {
      toast("Could not start the call", (e as Error).message);
      setBusy("");
    }
  };

  return (
    <Box sx={{ flex: 1, overflowY: "auto", p: 3, maxWidth: 760 }} data-testid="calls-view">
      <Typography level="title-lg" sx={{ mb: 2 }}>
        Calls
      </Typography>
      {error && <Alert color="danger">{error}</Alert>}
      {!rows && !error && <CircularProgress size="sm" />}
      {rows && rows.length === 0 && <Typography level="body-sm">No calls yet. Start one from a chat with the call buttons.</Typography>}
      {rows?.map((r) => {
        const otherId = r.direction === "in" ? r.caller_id : (r.peer_ids[0] ?? "");
        const name = state.peopleMap[otherId]?.name ?? "Someone";
        const extra = r.direction === "in" ? r.peer_ids.length - 1 : r.peer_ids.length - 1;
        return (
          <Box key={r.id} data-testid="call-row" data-status={r.status} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider" }}>
            <UserAvatar id={otherId} name={name} size={34} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm" noWrap color={r.status === "missed" ? "danger" : undefined}>
                {name}
                {extra > 0 ? ` +${extra}` : ""}
              </Typography>
              <Typography level="body-xs" sx={{ opacity: 0.8, display: "flex", alignItems: "center", gap: 0.5 }}>
                {r.direction === "in" ? <CallReceivedIcon sx={{ fontSize: 14 }} /> : <CallMadeIcon sx={{ fontSize: 14 }} />}
                {r.direction === "in" ? "Incoming" : "Outgoing"} {r.video ? "video" : "voice"} call ·{" "}
                {new Date(r.started_at).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}
                {r.duration_s > 0 ? ` · ${fmtDuration(r.duration_s)}` : ""}
              </Typography>
            </Box>
            <Chip size="sm" variant="soft" color={STATUS_COLOR[r.status] ?? "neutral"} data-testid="call-status" startDecorator={r.status === "missed" || r.status === "declined" ? <HangupIcon sx={{ fontSize: 14 }} /> : undefined}>
              {r.status === "answered" ? "Answered" : r.status === "missed" ? "Missed" : r.status === "declined" ? "Declined" : "Ringing"}
            </Chip>
            <Button size="sm" variant="soft" startDecorator={<CallOutlinedIcon />} loading={busy === r.id + false} onClick={() => void callBack(r, false)} data-testid="call-back-voice">
              Call back
            </Button>
            <Button size="sm" variant="plain" startDecorator={<VideocamOutlinedIcon />} loading={busy === r.id + true} onClick={() => void callBack(r, true)} data-testid="call-back-video">
              Video
            </Button>
          </Box>
        );
      })}
    </Box>
  );
}

export function FilesView() {
  const pane = useAppPane();
  const navigate = useNavigate();
  const [files, setFiles] = useState<DriveFile[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    listRecent()
      .then((f) => setFiles(f.slice(0, 50)))
      .catch((e: Error) => setError(e.message));
  }, []);
  return (
    <Box sx={{ flex: 1, overflowY: "auto", p: 3, maxWidth: 760 }} data-testid="files-view">
      <Typography level="title-lg" sx={{ mb: 2 }}>
        Files
      </Typography>
      {error && <Alert color="danger">{error}</Alert>}
      {!files && !error && <CircularProgress size="sm" />}
      {files && files.length === 0 && <Typography level="body-sm">No recent files.</Typography>}
      {files?.map((f) => {
        const t = openDriveFile({ name: f.name, mime_type: f.mime_type, drive_id: f.id });
        const tile = apps.find((a) => a.id === appForFile({ name: f.name, mime_type: f.mime_type }));
        return (
          <Box key={f.id} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider" }}>
            {tile && <AppIcon iconName={tile.iconName} color={tile.accentColor} iconUrl={tile.iconUrl} label={tile.name} size={30} />}
            <Typography level="title-sm" sx={{ flex: 1 }} noWrap>
              {f.name}
            </Typography>
            <Button size="sm" variant="soft" onClick={() => pane.openInPane(t)}>
              Open
            </Button>
            <Button size="sm" variant="plain" color="neutral" onClick={() => navigate(withReturn(t.href, "/chat/files"))}>
              {t.label}
            </Button>
          </Box>
        );
      })}
    </Box>
  );
}
