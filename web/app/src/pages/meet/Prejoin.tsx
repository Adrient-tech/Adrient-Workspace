import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Button, Container, IconButton, Input, Sheet, Tooltip, Typography } from "@mui/joy";
import MicIcon from "@mui/icons-material/Mic";
import MicOffIcon from "@mui/icons-material/MicOff";
import VideocamIcon from "@mui/icons-material/Videocam";
import VideocamOffIcon from "@mui/icons-material/VideocamOff";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { MediaSettings } from "./MediaSettings";
import { useLocalMedia, type LocalMedia } from "./media";
import { meetingLink } from "./api";
import type { MeetRoom } from "./types";

interface Props {
  room: MeetRoom;
  user: User;
  lm: LocalMedia;
  speakerId: string;
  onSpeakerChange: (id: string) => void;
  onJoin: (name: string) => void;
  onCancel: () => void;
}

/** Google-Meet-style green room: preview, device pickers, name, join. */
export function Prejoin({
  room,
  user,
  lm,
  speakerId,
  onSpeakerChange,
  onJoin,
  onCancel,
}: Props) {
  useLocalMedia(lm);
  const [name, setName] = useState(
    user.display_name || user.email || "Guest",
  );
  const [copied, setCopied] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);

  const stream = useMemo(
    () => (lm.camTrack ? new MediaStream([lm.camTrack]) : null),
    [lm.camTrack],
  );
  useEffect(() => {
    if (videoRef.current) videoRef.current.srcObject = stream;
  }, [stream]);

  const link = room.code ? meetingLink(room.code) : window.location.href;
  const showVideo = lm.camOn && !!stream;

  return (
    <>
      <Header user={user} />
      <Container maxWidth="lg" sx={{ py: { xs: 2, md: 5 } }}>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", md: "minmax(0,1.5fr) minmax(0,1fr)" },
            gap: 4,
            alignItems: "start",
          }}
        >
          <Box>
            <Box
              sx={{
                position: "relative",
                aspectRatio: "16/9",
                bgcolor: "#10201c",
                borderRadius: "lg",
                overflow: "hidden",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              {showVideo ? (
                <video
                  ref={videoRef}
                  autoPlay
                  muted
                  playsInline
                  data-testid="prejoin-preview"
                  style={{
                    width: "100%",
                    height: "100%",
                    objectFit: "cover",
                    transform: "scaleX(-1)",
                  }}
                />
              ) : (
                <Typography level="title-md" sx={{ color: "#cfe3dd" }}>
                  {lm.camOn ? "Starting camera…" : "Camera is off"}
                </Typography>
              )}
              <Box
                sx={{
                  position: "absolute",
                  bottom: 12,
                  left: 0,
                  right: 0,
                  display: "flex",
                  justifyContent: "center",
                  gap: 1.5,
                }}
              >
                <Tooltip title={lm.micOn ? "Turn off microphone (Ctrl+D)" : "Turn on microphone (Ctrl+D)"}>
                  <IconButton
                    size="lg"
                    variant="solid"
                    color={lm.micOn ? "neutral" : "danger"}
                    onClick={() => lm.setMic(!lm.micOn)}
                    aria-label={lm.micOn ? "Mute microphone" : "Unmute microphone"}
                    aria-pressed={!lm.micOn}
                    sx={{ borderRadius: "50%" }}
                  >
                    {lm.micOn ? <MicIcon /> : <MicOffIcon />}
                  </IconButton>
                </Tooltip>
                <Tooltip title={lm.camOn ? "Turn off camera (Ctrl+E)" : "Turn on camera (Ctrl+E)"}>
                  <IconButton
                    size="lg"
                    variant="solid"
                    color={lm.camOn ? "neutral" : "danger"}
                    onClick={() => void lm.setCam(!lm.camOn)}
                    aria-label={lm.camOn ? "Turn off camera" : "Turn on camera"}
                    aria-pressed={!lm.camOn}
                    sx={{ borderRadius: "50%" }}
                  >
                    {lm.camOn ? <VideocamIcon /> : <VideocamOffIcon />}
                  </IconButton>
                </Tooltip>
              </Box>
            </Box>
            {lm.error && (
              <Sheet color="warning" variant="soft" sx={{ mt: 1.5, p: 1.5, borderRadius: "md" }}>
                <Typography level="body-sm">{lm.error}</Typography>
              </Sheet>
            )}
          </Box>

          <Sheet variant="outlined" sx={{ p: 3, borderRadius: "lg" }}>
            <Typography level="h3">Ready to join?</Typography>
            <Typography level="title-md" sx={{ mt: 0.5 }} noWrap>
              {room.name}
            </Typography>
            {room.code && (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, mt: 0.5 }}>
                <Typography
                  level="body-sm"
                  sx={{ fontFamily: "monospace", letterSpacing: 1 }}
                >
                  {room.code}
                </Typography>
                <Tooltip title={copied ? "Copied" : "Copy meeting link"}>
                  <IconButton
                    size="sm"
                    variant="plain"
                    aria-label="Copy meeting link"
                    onClick={() =>
                      navigator.clipboard
                        .writeText(link)
                        .then(() => {
                          setCopied(true);
                          setTimeout(() => setCopied(false), 1500);
                        })
                        .catch(() => {})
                    }
                  >
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              </Box>
            )}

            <Typography level="title-sm" sx={{ mt: 2.5, mb: 0.5 }}>
              Your name
            </Typography>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              slotProps={{ input: { maxLength: 40, "aria-label": "Your name" } }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && name.trim()) onJoin(name.trim());
              }}
            />

            <Box sx={{ mt: 2.5 }}>
              <MediaSettings
                lm={lm}
                speakerId={speakerId}
                onSpeakerChange={onSpeakerChange}
              />
            </Box>

            <Box sx={{ display: "flex", gap: 1, mt: 3 }}>
              <Button
                fullWidth
                size="lg"
                disabled={!name.trim()}
                onClick={() => onJoin(name.trim())}
              >
                Join now
              </Button>
              <Button size="lg" variant="plain" color="neutral" onClick={onCancel}>
                Cancel
              </Button>
            </Box>
          </Sheet>
        </Box>
      </Container>
    </>
  );
}
