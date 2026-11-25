import PersonRemoveIcon from "@mui/icons-material/PersonRemove";
import { useEffect, useRef, useState } from "react";
import { Avatar, Box, Button, IconButton, Input, Tooltip, Typography } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";
import SendIcon from "@mui/icons-material/Send";
import MicOffIcon from "@mui/icons-material/MicOff";
import VideocamOffIcon from "@mui/icons-material/VideocamOff";
import PanToolIcon from "@mui/icons-material/PanTool";
import ScreenShareIcon from "@mui/icons-material/ScreenShare";
import PushPinIcon from "@mui/icons-material/PushPin";
import PushPinOutlinedIcon from "@mui/icons-material/PushPinOutlined";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import type { ChatEntry } from "./types";

const PANEL_BG = "#16231f";
const BORDER = "1px solid rgba(255,255,255,0.1)";

function PanelShell({
  title,
  onClose,
  children,
  width = 320,
  testId,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  width?: number;
  testId: string;
}) {
  return (
    <Box
      data-testid={testId}
      sx={{
        display: "flex",
        flexDirection: "column",
        width: { xs: "100%", sm: width },
        bgcolor: PANEL_BG,
        borderLeft: BORDER,
        height: "100%",
        flexShrink: 0,
        minHeight: 0,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", px: 2, py: 1.5, borderBottom: BORDER }}>
        <Typography level="title-sm" sx={{ color: "#fff", flex: 1 }}>
          {title}
        </Typography>
        <IconButton
          size="sm"
          variant="plain"
          sx={{ color: "#aaa" }}
          onClick={onClose}
          aria-label={`Close ${title}`}
        >
          <CloseIcon />
        </IconButton>
      </Box>
      {children}
    </Box>
  );
}

export function ChatPanel({
  messages,
  localId,
  onSend,
  onClose,
}: {
  messages: ChatEntry[];
  localId: string;
  onSend: (text: string) => void;
  onClose: () => void;
}) {
  const [draft, setDraft] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    bottomRef.current?.scrollIntoView?.({ block: "end" });
  }, [messages]);
  const send = () => {
    const t = draft.trim();
    if (!t) return;
    onSend(t);
    setDraft("");
  };
  return (
    <PanelShell title="In-call messages" onClose={onClose} testId="chat-panel">
      <Box sx={{ flex: 1, overflowY: "auto", px: 2, py: 1 }}>
        {messages.length === 0 && (
          <Typography level="body-xs" sx={{ color: "#8aa39b", textAlign: "center", mt: 4 }}>
            No messages yet. Messages are only visible to people in the call and disappear when it ends.
          </Typography>
        )}
        {messages.map((m, i) => {
          const mine = m.fromId === localId;
          const prev = messages[i - 1];
          const grouped = !!prev && prev.fromId === m.fromId && m.sentAt - prev.sentAt < 120_000;
          return (
            <Box
              key={m.id}
              data-testid="chat-message"
              data-mine={mine ? "1" : "0"}
              sx={{ display: "flex", flexDirection: "column", alignItems: mine ? "flex-end" : "flex-start", mt: grouped ? 0.25 : 1.25 }}
            >
              {!grouped && (
                <Box sx={{ display: "flex", alignItems: "baseline", gap: 0.75, mb: 0.25, flexDirection: mine ? "row-reverse" : "row" }}>
                  {!mine && (
                    <Typography level="body-xs" sx={{ color: "#a8c7fa", fontWeight: 600 }}>
                      {m.fromName}
                    </Typography>
                  )}
                  <Typography level="body-xs" sx={{ color: "#6f857e", fontSize: 11 }}>
                    {new Date(m.sentAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </Typography>
                </Box>
              )}
              <Box
                data-testid={mine ? "bubble-mine" : "bubble-other"}
                sx={{
                  maxWidth: "85%",
                  px: 1.25,
                  py: 0.6,
                  borderRadius: "lg",
                  bgcolor: mine ? "rgba(34,197,150,0.22)" : "#24382f",
                  border: "1px solid",
                  borderColor: mine ? "rgba(126,224,195,0.45)" : "rgba(255,255,255,0.08)",
                }}
              >
                <Typography level="body-sm" sx={{ color: "#e8eaed", wordBreak: "break-word", whiteSpace: "pre-wrap" }}>
                  {m.text}
                </Typography>
              </Box>
            </Box>
          );
        })}
        <div ref={bottomRef} />
      </Box>
      <Box sx={{ display: "flex", gap: 1, px: 2, py: 1.5, borderTop: BORDER }}>
        <Input
          size="sm"
          placeholder="Send a message"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              send();
            }
          }}
          sx={{ flex: 1, bgcolor: "#24382f", color: "#fff", "--Input-placeholderColor": "#8aa39b" }}
          slotProps={{ input: { "aria-label": "Chat message", maxLength: 2000 } }}
        />
        <IconButton
          size="sm"
          variant="solid"
          color="primary"
          onClick={send}
          disabled={!draft.trim()}
          aria-label="Send message"
        >
          <SendIcon />
        </IconButton>
      </Box>
    </PanelShell>
  );
}

export interface PersonRow {
  id: string;
  name: string;
  isLocal?: boolean;
  audioMuted: boolean;
  videoOff: boolean;
  handRaised: boolean;
  sharing: boolean;
  speaking: boolean;
}

export interface HostControls {
  locked: boolean;
  waitingRoom: boolean;
  waiters: { identity: string; name: string }[];
  recording?: { enabled: boolean; on: boolean };
  onMute: (id: string) => void;
  onRemove: (id: string) => void;
  onMuteAll: () => void;
  onToggleLock: () => void;
  onToggleWaitingRoom: () => void;
  onEnd: () => void;
  onAdmit: (identity: string) => void;
  onDeny: (identity: string) => void;
  onToggleRecording?: () => void;
  transcript?: { on: boolean; downloadUrl: string };
  onToggleTranscript?: () => void;
}

export function PeoplePanel({
  people,
  pinnedId,
  onTogglePin,
  onClose,
  host,
}: {
  people: PersonRow[];
  pinnedId: string | null;
  onTogglePin: (id: string) => void;
  onClose: () => void;
  host?: HostControls;
}) {
  const hands = people.filter((p) => p.handRaised).length;
  return (
    <PanelShell title={`People (${people.length})`} onClose={onClose} width={300} testId="people-panel">
      {host && (
        <Box sx={{ px: 2, py: 1, display: "flex", flexWrap: "wrap", gap: 0.75, borderBottom: "1px solid rgba(255,255,255,0.1)" }} data-testid="host-controls">
          <Button size="sm" variant="soft" color="neutral" onClick={host.onMuteAll} data-testid="host-mute-all">
            Mute all
          </Button>
          <Button size="sm" variant={host.locked ? "solid" : "soft"} color={host.locked ? "warning" : "neutral"} onClick={host.onToggleLock} data-testid="host-lock">
            {host.locked ? "Unlock meeting" : "Lock meeting"}
          </Button>
          <Button size="sm" variant={host.waitingRoom ? "solid" : "soft"} color={host.waitingRoom ? "primary" : "neutral"} onClick={host.onToggleWaitingRoom} data-testid="host-waiting-room">
            {host.waitingRoom ? "Waiting room: on" : "Waiting room: off"}
          </Button>
          {host.recording?.enabled && host.onToggleRecording && (
            <Button size="sm" variant={host.recording.on ? "solid" : "soft"} color={host.recording.on ? "danger" : "neutral"} onClick={host.onToggleRecording} data-testid="host-record">
              {host.recording.on ? "Stop recording" : "Record"}
            </Button>
          )}
          {host.onToggleTranscript && (
            <Button size="sm" variant={host.transcript?.on ? "solid" : "soft"} color={host.transcript?.on ? "danger" : "neutral"} onClick={host.onToggleTranscript} data-testid="host-transcript">
              {host.transcript?.on ? "Stop transcript" : "Start transcript"}
            </Button>
          )}
          {host.transcript && (
            <Button size="sm" variant="soft" color="neutral" component="a" href={host.transcript.downloadUrl} download="meeting-transcript.txt" data-testid="host-transcript-download">
              Download transcript
            </Button>
          )}
          <Button size="sm" variant="solid" color="danger" onClick={host.onEnd} data-testid="host-end">
            End for everyone
          </Button>
        </Box>
      )}
      {host && host.waiters.length > 0 && (
        <Box sx={{ px: 2, py: 1, borderBottom: "1px solid rgba(255,255,255,0.1)" }} data-testid="waiting-list">
          <Typography level="body-xs" sx={{ color: "#f4a11d", mb: 0.5 }}>
            Waiting to join ({host.waiters.length})
          </Typography>
          {host.waiters.map((w) => (
            <Box key={w.identity} sx={{ display: "flex", alignItems: "center", gap: 0.75, py: 0.5 }}>
              <Typography level="body-sm" sx={{ color: "#e8eaed", flex: 1 }} noWrap>
                {w.name}
              </Typography>
              <Button size="sm" variant="solid" color="primary" onClick={() => host.onAdmit(w.identity)} aria-label={`Admit ${w.name}`}>
                Admit
              </Button>
              <Button size="sm" variant="soft" color="neutral" onClick={() => host.onDeny(w.identity)} aria-label={`Deny ${w.name}`}>
                Deny
              </Button>
            </Box>
          ))}
        </Box>
      )}
      {hands > 0 && (
        <Typography level="body-xs" sx={{ color: "#f4a11d", px: 2, py: 0.75 }}>
          {hands} raised {hands === 1 ? "hand" : "hands"}
        </Typography>
      )}
      <Box sx={{ flex: 1, overflowY: "auto" }}>
        {people.map((p) => (
          <Box
            key={p.id}
            data-testid="people-row"
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              px: 2,
              py: 1,
              borderBottom: "1px solid rgba(255,255,255,0.06)",
              outline: p.speaking ? "1px solid #3fd1a5" : "none",
              outlineOffset: "-1px",
            }}
          >
            <Avatar size="sm" color="primary" variant="soft">
              {p.name.charAt(0).toUpperCase()}
            </Avatar>
            <Typography
              level="body-sm"
              sx={{ color: "#e8eaed", flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
            >
              {p.name}
              {p.isLocal ? " (You)" : ""}
            </Typography>
            {p.handRaised && <PanToolIcon sx={{ fontSize: 16, color: "#f4a11d" }} titleAccess="Hand raised" />}
            {p.sharing && <ScreenShareIcon sx={{ fontSize: 16, color: "#9be7cf" }} titleAccess="Presenting" />}
            {p.audioMuted && <MicOffIcon sx={{ fontSize: 16, color: "#ff8a80" }} titleAccess="Muted" />}
            {p.videoOff && <VideocamOffIcon sx={{ fontSize: 16, color: "#ff8a80" }} titleAccess="Camera off" />}
            {host && !p.isLocal && (
              <>
                <Tooltip title="Mute">
                  <IconButton size="sm" variant="plain" sx={{ color: "#9aa0a6" }} onClick={() => host.onMute(p.id)} aria-label={`Mute ${p.name}`} data-testid="row-mute">
                    <MicOffIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                <Tooltip title="Remove from meeting">
                  <IconButton size="sm" variant="plain" sx={{ color: "#ff8a80" }} onClick={() => host.onRemove(p.id)} aria-label={`Remove ${p.name}`} data-testid="row-remove">
                    <PersonRemoveIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              </>
            )}
            <Tooltip title={pinnedId === p.id ? "Unpin" : "Pin"}>
              <IconButton
                size="sm"
                variant="plain"
                sx={{ color: pinnedId === p.id ? "#7ee0c3" : "#9aa0a6" }}
                onClick={() => onTogglePin(p.id)}
                aria-label={pinnedId === p.id ? `Unpin ${p.name}` : `Pin ${p.name}`}
              >
                {pinnedId === p.id ? <PushPinIcon fontSize="small" /> : <PushPinOutlinedIcon fontSize="small" />}
              </IconButton>
            </Tooltip>
          </Box>
        ))}
      </Box>
    </PanelShell>
  );
}

export function InfoPanel({
  name,
  code,
  link,
  onClose,
}: {
  name: string;
  code: string | null;
  link: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState<"link" | "code" | null>(null);
  const copy = (kind: "link" | "code", text: string) =>
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopied(kind);
        setTimeout(() => setCopied(null), 1500);
      })
      .catch(() => {});
  return (
    <PanelShell title="Meeting details" onClose={onClose} testId="info-panel">
      <Box sx={{ p: 2, display: "flex", flexDirection: "column", gap: 2 }}>
        <Typography level="title-md" sx={{ color: "#fff" }}>
          {name}
        </Typography>
        <Box>
          <Typography level="body-xs" sx={{ color: "#8aa39b" }}>
            Joining link
          </Typography>
          <Typography level="body-sm" sx={{ color: "#e8eaed", wordBreak: "break-all", my: 0.5 }}>
            {link}
          </Typography>
          <Button
            size="sm"
            variant="soft"
            color="primary"
            startDecorator={<ContentCopyIcon />}
            onClick={() => void copy("link", link)}
          >
            {copied === "link" ? "Copied" : "Copy joining link"}
          </Button>
        </Box>
        {code && (
          <Box>
            <Typography level="body-xs" sx={{ color: "#8aa39b" }}>
              Meeting code
            </Typography>
            <Typography
              level="title-md"
              sx={{ color: "#e8eaed", fontFamily: "monospace", letterSpacing: 1, my: 0.5 }}
            >
              {code}
            </Typography>
            <Button
              size="sm"
              variant="soft"
              color="primary"
              startDecorator={<ContentCopyIcon />}
              onClick={() => void copy("code", code)}
            >
              {copied === "code" ? "Copied" : "Copy code"}
            </Button>
          </Box>
        )}
        <Typography level="body-xs" sx={{ color: "#8aa39b" }}>
          Anyone in your organisation with the link or code can join. Calls run through the workspace's own media server.
        </Typography>
      </Box>
    </PanelShell>
  );
}
