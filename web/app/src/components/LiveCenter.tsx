import { useEffect, useRef, useState } from "react";
import { Avatar, Box, Button, IconButton, Sheet, Typography } from "@mui/joy";
import { useLocation, useNavigate } from "react-router-dom";
import CallIcon from "@mui/icons-material/Call";
import VideocamIcon from "@mui/icons-material/Videocam";
import { HangupIcon } from "./HangupIcon";
import CloseIcon from "@mui/icons-material/Close";
import NotificationsIcon from "@mui/icons-material/Notifications";
import { live, type LiveEvent } from "../lib/live";
import { answerCall, meetPath } from "../lib/calls";
import { desktopNotify, playSound, soundForNotification, sounds, startSounds } from "../lib/sounds";

interface Ring {
  code: string;
  title: string;
  video: boolean;
  fromId: string;
  fromName: string;
  at: number;
}

interface Toast {
  id: string;
  title: string;
  body: string;
  url: string;
}

const RING_TIMEOUT_MS = 45_000;

function osNotify(title: string, body: string, onClick: () => void) {
  desktopNotify(title, body, onClick);
}

/**
 * LiveCenter is mounted once for signed-in users. It shows incoming calls
 * (ring, accept, decline) and a toast for every new notification from any
 * app, on whatever page the user is on.
 */
export function LiveCenter() {
  const navigate = useNavigate();
  const location = useLocation();
  const here = useRef(location.pathname + location.search);
  here.current = location.pathname + location.search;
  const [ring, setRing] = useState<Ring | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(() => {
    startSounds();
    live.start();
    return live.subscribe((e: LiveEvent) => {
      const s = (k: string) => (typeof e[k] === "string" ? (e[k] as string) : "");
      switch (e.type) {
        case "call_ring": {
          const r: Ring = {
            code: s("code"),
            title: s("title"),
            video: !!e.video,
            fromId: s("from_id"),
            fromName: s("from_name") || "Someone",
            at: Date.now(),
          };
          setRing(r);
          sounds.stop("ringback");
          osNotify(`${r.video ? "Video" : "Voice"} call from ${r.fromName}`, r.title, () =>
            navigate(meetPath(r.code, { returnTo: here.current, audioOnly: !r.video })),
          );
          break;
        }
        case "call_accepted":
          sounds.stop("ringback");
          break;
        case "call_cancelled":
          sounds.stop("ringback");
          setRing((cur) => (cur && cur.code === s("code") ? null : cur));
          break;
        case "call_declined":
          sounds.stop("ringback");
          pushToast({ id: `d${Date.now()}`, title: `${s("from_name") || "They"} declined the call`, body: "", url: "" });
          break;
        case "notification": {
          window.dispatchEvent(new CustomEvent("adrient:notification", { detail: e }));
          if (s("kind") === "call") break; // the ring dialog covers it
          const t: Toast = { id: s("id") || String(Date.now()), title: s("title"), body: s("body"), url: s("url") };
          pushToast(t);
          const tone = soundForNotification(s("kind"));
          if (tone && tone !== "meeting_ended") playSound(tone);
          osNotify(t.title, t.body, () => t.url && navigate(t.url));
          break;
        }
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pushToast = (t: Toast) => {
    setToasts((cur) => [...cur.filter((x) => x.id !== t.id), t].slice(-4));
    window.setTimeout(() => setToasts((cur) => cur.filter((x) => x.id !== t.id)), 6000);
  };

  // Ringtone + timeout while a ring is showing.
  useEffect(() => {
    if (!ring) return;
    void sounds.loop("ringtone");
    const t = window.setTimeout(() => {
      setRing(null);
      pushToast({ id: `m${ring.code}`, title: `Missed call from ${ring.fromName}`, body: ring.title, url: meetPath(ring.code) });
    }, RING_TIMEOUT_MS);
    return () => {
      sounds.stop("ringtone");
      window.clearTimeout(t);
    };
  }, [ring]);

  const accept = () => {
    if (!ring) return;
    void answerCall(ring.fromId, ring.code, "accepted");
    const path = meetPath(ring.code, { returnTo: here.current, audioOnly: !ring.video });
    setRing(null);
    navigate(path);
  };
  const decline = () => {
    if (!ring) return;
    void answerCall(ring.fromId, ring.code, "declined");
    setRing(null);
  };

  return (
    <>
      {ring && (
        <Sheet
          role="alertdialog"
          aria-label={`Incoming call from ${ring.fromName}`}
          data-testid="incoming-call"
          variant="outlined"
          sx={{
            position: "fixed",
            top: 72,
            right: 16,
            zIndex: 1400,
            width: 320,
            p: 2,
            borderRadius: "lg",
            boxShadow: "lg",
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Avatar color="primary" variant="solid">
              {ring.fromName.charAt(0).toUpperCase()}
            </Avatar>
            <Box sx={{ minWidth: 0 }}>
              <Typography level="title-sm" noWrap>
                {ring.fromName}
              </Typography>
              <Typography level="body-xs" noWrap>
                Incoming {ring.video ? "video" : "voice"} call · {ring.title}
              </Typography>
            </Box>
          </Box>
          <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
            <Button color="danger" variant="soft" startDecorator={<HangupIcon />} onClick={decline} data-testid="call-decline">
              Decline
            </Button>
            <Button color="success" startDecorator={ring.video ? <VideocamIcon /> : <CallIcon />} onClick={accept} data-testid="call-accept">
              Accept
            </Button>
          </Box>
        </Sheet>
      )}
      <Box sx={{ position: "fixed", left: 16, bottom: 16, zIndex: 1350, display: "flex", flexDirection: "column", gap: 1 }}>
        {toasts.map((t) => (
          <Sheet
            key={t.id}
            variant="outlined"
            data-testid="live-toast"
            sx={{ width: 320, p: 1.5, borderRadius: "md", boxShadow: "md", display: "flex", gap: 1, alignItems: "flex-start", cursor: t.url ? "pointer" : "default" }}
            onClick={() => {
              if (t.url) navigate(t.url);
              setToasts((cur) => cur.filter((x) => x.id !== t.id));
            }}
          >
            <NotificationsIcon color="primary" fontSize="small" sx={{ mt: 0.25 }} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm">{t.title}</Typography>
              {t.body && (
                <Typography level="body-xs" sx={{ display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
                  {t.body}
                </Typography>
              )}
            </Box>
            <IconButton
              size="sm"
              variant="plain"
              aria-label="Dismiss"
              onClick={(ev) => {
                ev.stopPropagation();
                setToasts((cur) => cur.filter((x) => x.id !== t.id));
              }}
            >
              <CloseIcon fontSize="small" />
            </IconButton>
          </Sheet>
        ))}
      </Box>
    </>
  );
}
