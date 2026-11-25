import { useEffect, useRef, useState } from "react";
import { Box, Button, IconButton, Tooltip, Typography } from "@mui/joy";
import { useLocation, useNavigate } from "react-router-dom";
import MicIcon from "@mui/icons-material/Mic";
import MicOffIcon from "@mui/icons-material/MicOff";
import { HangupIcon } from "../../components/HangupIcon";
import OpenInFullIcon from "@mui/icons-material/OpenInFull";
import { CallView } from "./CallView";
import { callStore, isDocked, useActiveCall, useCallDock } from "./callStore";
import { useLocalMedia } from "./media";

function useElapsed(since: number): string {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);
  const s = Math.max(0, Math.floor((now - since) / 1000));
  const mm = String(Math.floor(s / 60)).padStart(2, "0");
  const ss = String(s % 60).padStart(2, "0");
  return s >= 3600 ? `${Math.floor(s / 3600)}:${mm.slice(-2)}:${ss}` : `${mm}:${ss}`;
}

/**
 * CallHost is mounted once at the app root. While a call is active it keeps
 * the CallView (and so the WebRTC connections) alive on every page:
 * full screen on the call's own URL, hidden behind a small call bar anywhere
 * else in the workspace.
 */
export function CallHost() {
  const call = useActiveCall();
  const location = useLocation();
  const navigate = useNavigate();
  const dock = useCallDock();
  const [rect, setRect] = useState<{ top: number; left: number; width: number; height: number } | null>(null);
  const docked = isDocked(call, dock) && location.pathname !== call?.callPath;
  const dockEl = docked ? dock!.el : null;

  // A call started from a Chat channel returns there once joined, docked.
  const dockedFor = useRef(0);
  useEffect(() => {
    if (!call || dockedFor.current === call.startedAt) return;
    dockedFor.current = call.startedAt;
    if (location.pathname === call.callPath && call.returnTo.startsWith("/chat/c/")) {
      navigate(call.returnTo, { replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [call?.startedAt]);

  // Track the dock placeholder's on-screen rectangle.
  useEffect(() => {
    if (!dockEl) {
      setRect(null);
      return;
    }
    let raf = 0;
    const tick = () => {
      const b = dockEl.getBoundingClientRect();
      // Clamp to the viewport so the control bar (Leave) is never cut off.
      const r = { top: Math.max(0, b.top), left: b.left, width: b.width, height: Math.max(0, Math.min(b.bottom, window.innerHeight) - Math.max(0, b.top)) };
      setRect((p) =>
        p && p.top === r.top && p.left === r.left && p.width === r.width && p.height === r.height ? p : r,
      );
      raf = requestAnimationFrame(tick);
    };
    tick();
    return () => cancelAnimationFrame(raf);
  }, [dockEl]);

  if (!call) return null;

  const onCallPage = location.pathname === call.callPath;
  const showDocked = docked && !!rect;
  const leave = () => {
    const ended = callStore.end();
    const back = ended?.returnTo || "/meet";
    if (location.pathname === ended?.callPath || !ended?.returnTo) navigate(back);
  };

  return (
    <>
      <Box
        data-testid={showDocked ? "call-docked" : undefined}
        sx={
          onCallPage
            ? { position: "fixed", inset: 0, zIndex: 1300 }
            : showDocked
            ? { position: "fixed", top: rect!.top, left: rect!.left, width: rect!.width, height: rect!.height, zIndex: 1100, overflow: "hidden" }
            : { position: "fixed", width: 1, height: 1, overflow: "hidden", opacity: 0, pointerEvents: "none", left: -10, top: -10 }
        }
        aria-hidden={!onCallPage && !showDocked}
      >
        <CallView
          room={call.room}
          displayName={call.displayName}
          lm={call.lm}
          speakerId={call.speakerId}
          onSpeakerChange={(id) => callStore.setSpeaker(id)}
          onLeave={leave}
          onMinimize={showDocked ? undefined : call.returnTo ? () => navigate(call.returnTo) : () => navigate("/")}
          returnLabel={call.returnTo ? labelFor(call.returnTo) : "Workspace"}
          active={onCallPage || showDocked}
          docked={showDocked}
          onPopOut={showDocked ? () => navigate(call.callPath) : undefined}
          onUndock={showDocked ? () => callStore.setUndocked(true) : undefined}
        />
      </Box>
      {!onCallPage && !docked && (
        <CallBar
          onOpen={() => {
            // An undocked chat call goes back into its chat, docked.
            if (call.undocked && call.returnTo.startsWith("/chat/c/")) {
              callStore.setUndocked(false);
              navigate(call.returnTo);
            } else navigate(call.callPath);
          }}
          onLeave={leave}
        />
      )}
    </>
  );
}

function labelFor(path: string): string {
  const seg = path.split("/").filter(Boolean)[0] ?? "";
  if (!seg) return "Workspace";
  return seg.charAt(0).toUpperCase() + seg.slice(1);
}

function CallBar({ onOpen, onLeave }: { onOpen: () => void; onLeave: () => void }) {
  const call = useActiveCall();
  const lm = call?.lm;
  useLocalMedia(lm!);
  const elapsed = useElapsed(call?.startedAt ?? Date.now());
  if (!call || !lm) return null;
  return (
    <Box
      role="region"
      aria-label="Ongoing call"
      data-testid="call-bar"
      sx={{
        position: "fixed",
        right: 16,
        bottom: 16,
        zIndex: 1250,
        display: "flex",
        alignItems: "center",
        gap: 1,
        pl: 2,
        pr: 1,
        py: 1,
        borderRadius: "xl",
        bgcolor: "#0d1a17",
        color: "#fff",
        boxShadow: "lg",
      }}
    >
      <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#22c55e", flexShrink: 0 }} />
      <Box sx={{ minWidth: 0, mr: 1 }}>
        <Typography level="body-sm" sx={{ color: "#fff", fontWeight: 600 }} noWrap>
          {call.room.name || "Meeting"}
        </Typography>
        <Typography level="body-xs" sx={{ color: "rgba(255,255,255,.7)" }}>
          In call · {elapsed}
        </Typography>
      </Box>
      <Tooltip title={lm.micOn ? "Mute" : "Unmute"}>
        <IconButton
          size="sm"
          variant="soft"
          color={lm.micOn ? "neutral" : "danger"}
          onClick={() => lm.setMic(!lm.micOn)}
          aria-label={lm.micOn ? "Mute" : "Unmute"}
          sx={{ borderRadius: "50%" }}
        >
          {lm.micOn ? <MicIcon /> : <MicOffIcon />}
        </IconButton>
      </Tooltip>
      <Button size="sm" variant="soft" startDecorator={<OpenInFullIcon />} onClick={onOpen} data-testid="call-bar-return">
        Return to call
      </Button>
      <Tooltip title="Leave call">
        <IconButton size="sm" color="danger" variant="solid" onClick={onLeave} aria-label="Leave call" sx={{ borderRadius: "50%" }}>
          <HangupIcon />
        </IconButton>
      </Tooltip>
    </Box>
  );
}
