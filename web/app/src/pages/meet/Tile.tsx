import { useEffect, useRef } from "react";
import { Avatar, Box, IconButton, Tooltip } from "@mui/joy";
import MicOffIcon from "@mui/icons-material/MicOff";
import PanToolIcon from "@mui/icons-material/PanTool";
import PushPinIcon from "@mui/icons-material/PushPin";
import PushPinOutlinedIcon from "@mui/icons-material/PushPinOutlined";
import ScreenShareIcon from "@mui/icons-material/ScreenShare";
import type { Track } from "livekit-client";
import type { Rect } from "./lib";

export interface TileProps {
  id: string;
  label: string;
  stream: MediaStream | null;
  /** Remote LiveKit tracks: attached (not srcObject) so adaptive stream sees the element. */
  lkVideo?: Track | null;
  lkAudio?: Track | null;
  isLocal?: boolean;
  audioMuted?: boolean;
  showVideo: boolean;
  handRaised?: boolean;
  sharing?: boolean;
  speaking?: boolean;
  pinned?: boolean;
  reaction?: string | null;
  rect: Rect;
  speakerId?: string;
  onTogglePin: (id: string) => void;
  registerVideo: (id: string, el: HTMLVideoElement | null) => void;
}

/** One participant tile, absolutely positioned by the layout engine. */
export function Tile({
  id,
  label,
  stream,
  lkVideo,
  lkAudio,
  isLocal,
  audioMuted,
  showVideo,
  handRaised,
  sharing,
  speaking,
  pinned,
  reaction,
  rect,
  speakerId,
  onTogglePin,
  registerVideo,
}: TileProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const isRemoteTile = !isLocal;

  const remote = !!(lkVideo || lkAudio) || !!isRemoteTile;
  useEffect(() => {
    const el = videoRef.current;
    if (!el || remote) return;
    if (el.srcObject !== stream) el.srcObject = stream;
  }, [stream, showVideo, remote]);

  // Remote media: attach() lets LiveKit size/pause simulcast layers per element.
  useEffect(() => {
    const el = videoRef.current;
    if (!el || !remote) return;
    lkVideo?.attach(el);
    lkAudio?.attach(el);
    return () => {
      lkVideo?.detach(el);
      lkAudio?.detach(el);
    };
  }, [lkVideo, lkAudio, remote]);

  // Keep the <video> mounted (even when hidden) so remote audio keeps playing.
  useEffect(() => {
    registerVideo(id, videoRef.current);
    return () => registerVideo(id, null);
  }, [id, registerVideo]);

  useEffect(() => {
    const el = videoRef.current as
      | (HTMLVideoElement & { setSinkId?: (id: string) => Promise<void> })
      | null;
    if (el?.setSinkId && speakerId && !isLocal) {
      el.setSinkId(speakerId).catch(() => {});
    }
  }, [speakerId, isLocal]);

  const small = rect.w < 220;

  return (
    <Box
      data-testid={`tile-${isLocal ? "local" : id}`}
      data-speaking={speaking ? "true" : "false"}
      data-pinned={pinned ? "true" : "false"}
      sx={{
        position: "absolute",
        left: rect.x,
        top: rect.y,
        width: rect.w,
        height: rect.h,
        transition: "left 200ms, top 200ms, width 200ms, height 200ms",
        bgcolor: "#1c2f2a",
        borderRadius: "12px",
        overflow: "hidden",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        outline: speaking ? "3px solid #3fd1a5" : "3px solid transparent",
        outlineOffset: "-3px",
        "&:hover .tile-actions": { opacity: 1 },
      }}
    >
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted={!!isLocal}
        aria-label={`${label}'s video`}
        style={{
          position: "absolute",
          inset: 0,
          width: "100%",
          height: "100%",
          objectFit: sharing ? "contain" : "cover",
          background: sharing ? "#000" : undefined,
          transform: isLocal && !sharing ? "scaleX(-1)" : undefined,
          visibility: showVideo ? "visible" : "hidden",
        }}
      />
      {!showVideo && (
        <Avatar
          color="primary"
          variant="solid"
          sx={{
            "--Avatar-size": small ? "36px" : "72px",
            fontSize: small ? 16 : 30,
            boxShadow: speaking ? "0 0 0 6px rgba(63,209,165,0.35)" : undefined,
          }}
        >
          {label.charAt(0).toUpperCase()}
        </Avatar>
      )}

      <Box
        sx={{
          position: "absolute",
          left: 8,
          right: 8,
          bottom: 8,
          display: "flex",
          alignItems: "center",
          gap: 0.5,
        }}
      >
        <Box
          sx={{
            bgcolor: "rgba(0,0,0,0.55)",
            color: "#fff",
            px: 1,
            py: 0.25,
            borderRadius: "sm",
            fontSize: small ? 11 : 13,
            fontWeight: 500,
            maxWidth: "100%",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {sharing ? `${label} (presenting)` : label}
        </Box>
        {audioMuted && (
          <Box
            title="Microphone muted"
            data-testid="tile-muted"
            sx={{
              bgcolor: "rgba(0,0,0,0.65)",
              color: "#ff8a80",
              borderRadius: "50%",
              width: 22,
              height: 22,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
          >
            <MicOffIcon sx={{ fontSize: 14 }} />
          </Box>
        )}
        {sharing && <ScreenShareIcon sx={{ fontSize: 16, color: "#9be7cf" }} />}
      </Box>

      {handRaised && (
        <Box
          data-testid="tile-hand"
          title="Hand raised"
          sx={{
            position: "absolute",
            top: 8,
            left: 8,
            bgcolor: "#f4a11d",
            color: "#000",
            borderRadius: "50%",
            width: 28,
            height: 28,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            boxShadow: "0 1px 4px rgba(0,0,0,0.4)",
          }}
        >
          <PanToolIcon sx={{ fontSize: 16 }} />
        </Box>
      )}

      {reaction && (
        <Box
          data-testid="tile-reaction"
          sx={{
            position: "absolute",
            top: 8,
            right: 44,
            fontSize: 28,
            lineHeight: 1,
            filter: "drop-shadow(0 1px 2px rgba(0,0,0,0.5))",
          }}
        >
          {reaction}
        </Box>
      )}

      <Box
        className="tile-actions"
        sx={{
          position: "absolute",
          top: 6,
          right: 6,
          opacity: pinned ? 1 : { xs: 1, md: 0 },
          transition: "opacity 120ms",
        }}
      >
        <Tooltip title={pinned ? "Unpin" : "Pin to spotlight"}>
          <IconButton
            size="sm"
            variant="solid"
            color={pinned ? "primary" : "neutral"}
            onClick={() => onTogglePin(id)}
            aria-label={pinned ? `Unpin ${label}` : `Pin ${label}`}
            aria-pressed={!!pinned}
            sx={{ borderRadius: "50%", bgcolor: pinned ? undefined : "rgba(0,0,0,0.55)" }}
          >
            {pinned ? <PushPinIcon fontSize="small" /> : <PushPinOutlinedIcon fontSize="small" />}
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  );
}
