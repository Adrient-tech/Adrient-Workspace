import { useEffect, useRef, useState } from "react";
import {
  Box,
  Button,
  Container,
  Typography,
  Sheet,
  IconButton,
  Avatar,
  Input,
  CircularProgress,
  Modal,
  ModalDialog,
  ModalClose,
  Tooltip,
  Divider,
} from "@mui/joy";
import VideocamIcon from "@mui/icons-material/Videocam";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import BoltIcon from "@mui/icons-material/Bolt";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { listRooms, createMeeting, deleteRoom, resolveCode, meetingLink, restartMeeting } from "./api";
import { fmtMeetDuration } from "./meetStatus";
import type { MeetRoom } from "./types";
import { LocalMedia } from "./media";
import { Prejoin } from "./Prejoin";
import { callStore, safeReturn, useActiveCall } from "./callStore";
import { useLocation, useNavigate } from "react-router-dom";

// ---------------------------------------------------------------------------
// Lobby — room list + create/join.
// ---------------------------------------------------------------------------
interface LobbyProps {
  user: User;
  onJoin: (room: MeetRoom) => void;
}

function Lobby({ user, onJoin }: LobbyProps) {
  const [rooms, setRooms] = useState<MeetRoom[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  // "Join with a code" state
  const [joinCode, setJoinCode] = useState("");
  const [joinError, setJoinError] = useState<string | null>(null);
  const [joinBusy, setJoinBusy] = useState(false);

  async function reload() {
    try {
      setRooms(await listRooms());
    } catch (e) {
      setError((e as Error).message);
    }
  }

  useEffect(() => {
    reload();
  }, []);

  async function handleCreate() {
    if (busy) return;
    setBusy(true);
    try {
      const room = await createMeeting(newName.trim() || "Quick meeting");
      await reload();
      setCreateOpen(false);
      setNewName("");
      onJoin(room);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleInstant() {
    if (busy) return;
    setBusy(true);
    try {
      const room = await createMeeting("Instant meeting");
      await reload();
      onJoin(room);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleJoinByCode() {
    const code = joinCode.trim().toLowerCase();
    if (!code) return;
    setJoinError(null);
    setJoinBusy(true);
    try {
      const room = await resolveCode(code);
      onJoin(room);
    } catch (e) {
      const msg = (e as Error).message;
      setJoinError(
        msg.includes("404")
          ? "Meeting not found for that code."
          : `Error: ${msg}`,
      );
    } finally {
      setJoinBusy(false);
    }
  }

  async function handleDelete(id: string, e: React.MouseEvent) {
    e.stopPropagation();
    if (!window.confirm("Delete this room?")) return;
    setDeletingId(id);
    try {
      await deleteRoom(id);
      await reload();
    } catch {
      // ignore
    } finally {
      setDeletingId(null);
    }
  }

  // Suppress unused variable warning
  void user;

  return (
    <>
      <Header user={user} />
      <Container maxWidth="md" sx={{ py: 4 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 3 }}>
          <Typography level="h2" sx={{ flex: 1 }}>
            Meet
          </Typography>
          <Button
            variant="soft"
            color="primary"
            startDecorator={<BoltIcon />}
            loading={busy}
            onClick={() => void handleInstant()}
          >
            Instant meeting
          </Button>
          <Button
            variant="solid"
            color="primary"
            startDecorator={<AddIcon />}
            onClick={() => setCreateOpen(true)}
          >
            New meeting
          </Button>
        </Box>

        {/* Join with a code */}
        <Sheet variant="outlined" sx={{ borderRadius: "md", p: 2, mb: 3 }}>
          <Typography level="title-sm" sx={{ mb: 1 }}>
            Join with a code
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input
              placeholder="abc-defg-hij"
              value={joinCode}
              onChange={(e) => {
                setJoinCode(e.target.value);
                setJoinError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !joinBusy) void handleJoinByCode();
              }}
              sx={{ flex: 1, fontFamily: "monospace", letterSpacing: 1 }}
              aria-label="Meeting code"
              error={!!joinError}
            />
            <Button
              variant="soft"
              loading={joinBusy}
              onClick={() => void handleJoinByCode()}
              disabled={!joinCode.trim()}
            >
              Join
            </Button>
          </Box>
          {joinError && (
            <Typography level="body-xs" color="danger" sx={{ mt: 0.5 }}>
              {joinError}
            </Typography>
          )}
        </Sheet>

        {error && (
          <Sheet
            color="danger"
            variant="soft"
            sx={{ p: 2, mb: 2, borderRadius: "md" }}
          >
            <Typography color="danger">{error}</Typography>
          </Sheet>
        )}

        {rooms === null && !error && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
            <CircularProgress />
          </Box>
        )}

        {rooms !== null && rooms.length === 0 && (
          <Sheet
            variant="soft"
            sx={{ p: 6, borderRadius: "md", textAlign: "center" }}
          >
            <VideocamIcon sx={{ fontSize: 48, opacity: 0.3, mb: 1 }} />
            <Typography level="body-lg" sx={{ opacity: 0.7 }}>
              No meeting rooms yet. Create one to get started.
            </Typography>
          </Sheet>
        )}

        {rooms !== null && rooms.length > 0 && (
          <Sheet
            variant="outlined"
            sx={{ borderRadius: "md", overflow: "hidden" }}
          >
            {rooms.map((room, i) => (
              <Box
                key={room.id}
                data-testid={`room-${room.id}`}
                onClick={() => onJoin(room)}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  px: { xs: 2, sm: 3 },
                  py: 1.5,
                  cursor: "pointer",
                  minHeight: 56,
                  borderTop: i === 0 ? "none" : "1px solid",
                  borderColor: "divider",
                  "&:hover": { bgcolor: "background.level1" },
                  "&:hover .row-actions": { opacity: 1 },
                }}
              >
                <Avatar
                  variant="soft"
                  color="primary"
                  sx={{ bgcolor: "#2A9D8F20", "--Avatar-size": "40px" }}
                >
                  <VideocamIcon sx={{ color: "#2A9D8F" }} />
                </Avatar>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography level="body-sm" sx={{ fontWeight: 600 }} noWrap>
                    {room.name}
                  </Typography>
                  <Typography
                    level="body-xs"
                    sx={{
                      opacity: 0.6,
                      fontFamily: room.code ? "monospace" : undefined,
                    }}
                  >
                    {room.code
                      ? room.code
                      : `Created ${new Date(room.created_at).toLocaleDateString()}`}
                  </Typography>
                </Box>
                <Button size="sm" variant="soft" color="primary">
                  Join
                </Button>
                <Box
                  className="row-actions"
                  sx={{
                    display: "flex",
                    opacity: { xs: 1, md: 0 },
                    transition: "opacity 120ms",
                  }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {room.code && (
                    <Tooltip title="Copy meeting link">
                      <IconButton
                        size="sm"
                        variant="plain"
                        aria-label="Copy meeting link"
                        onClick={() =>
                          void navigator.clipboard
                            .writeText(meetingLink(room.code!))
                            .catch(() => {})
                        }
                      >
                        <ContentCopyIcon />
                      </IconButton>
                    </Tooltip>
                  )}
                  <Tooltip title="Delete room">
                    <IconButton
                      size="sm"
                      variant="plain"
                      color="danger"
                      loading={deletingId === room.id}
                      onClick={(e) => void handleDelete(room.id, e)}
                      aria-label="Delete room"
                    >
                      <DeleteIcon />
                    </IconButton>
                  </Tooltip>
                </Box>
              </Box>
            ))}
          </Sheet>
        )}

        <Divider sx={{ my: 4 }} />
        <Box sx={{ textAlign: "center", opacity: 0.6 }}>
          <Typography level="body-sm">
            Meet uses peer-to-peer WebRTC — your video goes directly to other
            participants.
          </Typography>
        </Box>
      </Container>

      {/* Create room modal */}
      {createOpen && (
        <Modal
          open
          onClose={() => {
            if (!busy) {
              setCreateOpen(false);
              setNewName("");
            }
          }}
        >
          <ModalDialog
            sx={{ width: { xs: "100vw", sm: 400 }, maxWidth: "100vw" }}
          >
            <ModalClose />
            <Typography level="h4">New meeting</Typography>
            <Typography level="body-sm" sx={{ opacity: 0.7, mt: 0.5 }}>
              Give your meeting a name, or leave blank for a default.
            </Typography>
            <Input
              autoFocus
              placeholder="Meeting name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !busy) void handleCreate();
              }}
              sx={{ mt: 2 }}
              aria-label="Meeting name"
            />
            <Box
              sx={{
                display: "flex",
                justifyContent: "flex-end",
                gap: 1,
                mt: 2,
              }}
            >
              <Button
                variant="plain"
                color="neutral"
                onClick={() => {
                  setCreateOpen(false);
                  setNewName("");
                }}
                disabled={busy}
              >
                Cancel
              </Button>
              <Button loading={busy} onClick={() => void handleCreate()}>
                Create &amp; join
              </Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// MeetApp — lobby, green room and call, all driven by the URL:
//   /meet                 lobby
//   /meet/<code>          green room, then the call (same URL, refresh-safe)
//   /meet/room/<id>       rooms without a short code
// The call itself is owned by callStore + CallHost (app root), so it keeps
// running while the user visits other apps, like Teams.
// ---------------------------------------------------------------------------
interface MeetAppProps {
  user: User;
}

const CODE_RE = /^[a-z]{3}-[a-z]{4}-[a-z]{3}$/;

function pathForRoom(room: MeetRoom): string {
  return room.code ? `/meet/${room.code}` : `/meet/room/${room.id}`;
}

async function resolvePath(pathname: string): Promise<MeetRoom | null> {
  const parts = pathname.split("/").filter(Boolean); // ["meet", ...]
  if (parts[1] === "room" && parts[2]) {
    const rooms = await listRooms();
    const r = rooms.find((x) => x.id === parts[2]);
    if (!r) throw new Error("404");
    return r;
  }
  const code = (parts[1] ?? "").toLowerCase();
  if (!CODE_RE.test(code)) return null;
  return resolveCode(code);
}

export default function MeetApp({ user }: MeetAppProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const active = useActiveCall();
  const [pendingRoom, setPendingRoom] = useState<MeetRoom | null>(null);
  const [speakerId, setSpeakerId] = useState("");
  const lmRef = useRef<LocalMedia | null>(null);
  const [resolving, setResolving] = useState(false);
  const [resolveError, setResolveError] = useState<string | null>(null);
  const [endedRoom, setEndedRoom] = useState<MeetRoom | null>(null);
  const [restarting, setRestarting] = useState(false);
  const [reload, setReload] = useState(0);

  const params = new URLSearchParams(location.search);
  const returnTo = safeReturn(params.get("return"));
  const audioOnly = params.get("audio") === "1";

  const releasePreview = () => {
    // Only stop the preview camera if it is not the live call's media.
    if (lmRef.current && lmRef.current !== callStore.get()?.lm) lmRef.current.stop();
    lmRef.current = null;
  };

  const joinCall = (room: MeetRoom, name: string, lm: LocalMedia, back: string) => {
    callStore.start({
      room,
      displayName: name,
      lm,
      speakerId,
      callPath: pathForRoom(room),
      returnTo: back,
    });
    lmRef.current = null;
    setPendingRoom(null);
  };

  // Resolve the URL into a green room (or straight back into a live call).
  useEffect(() => {
    if (location.pathname === "/meet" || location.pathname === "/meet/") {
      setPendingRoom(null);
      releasePreview();
      return;
    }
    // Already in this call: CallHost shows it full screen.
    if (active && active.callPath === location.pathname) return;
    let cancelled = false;
    setResolving(true);
    setResolveError(null);
    resolvePath(location.pathname)
      .then((room) => {
        if (cancelled) return;
        setResolving(false);
        if (!room) {
          navigate("/meet", { replace: true });
          return;
        }
        if (room.ended) {
          setEndedRoom(room);
          return;
        }
        setEndedRoom(null);
        const lm = new LocalMedia();
        const rejoin = callStore.rejoinInfo(pathForRoom(room));
        if (rejoin) {
          // Refresh during a call: rejoin directly, no green room.
          void lm.start(true, !audioOnly);
          joinCall(room, rejoin.name, lm, safeReturn(rejoin.returnTo) || returnTo);
          return;
        }
        releasePreview();
        lmRef.current = lm;
        void lm.start(true, !audioOnly);
        setPendingRoom(room);
      })
      .catch((e: Error) => {
        if (cancelled) return;
        setResolving(false);
        setResolveError(e.message.includes("404") ? "Meeting not found or it has ended." : `Could not join: ${e.message}`);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname, reload]);

  // Stop the preview camera when leaving Meet without joining.
  useEffect(() => () => releasePreview(), []);

  const backToLobby = () => {
    releasePreview();
    setPendingRoom(null);
    navigate(returnTo || "/meet");
  };

  if (active && active.callPath === location.pathname) return null;

  if (resolving) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", height: "100vh" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (resolveError) {
    return (
      <>
        <Header user={user} />
        <Container maxWidth="sm" sx={{ py: 8, textAlign: "center" }}>
          <Sheet color="danger" variant="soft" sx={{ p: 4, borderRadius: "md" }}>
            <Typography color="danger" level="title-md">
              {resolveError}
            </Typography>
            <Button
              sx={{ mt: 2 }}
              onClick={() => {
                setResolveError(null);
                navigate(returnTo || "/meet");
              }}
            >
              {returnTo ? "Go back" : "Back to Meet"}
            </Button>
          </Sheet>
        </Container>
      </>
    );
  }

  if (endedRoom) {
    const isHost = endedRoom.owner_id === user.id;
    const n = endedRoom.participant_count ?? 0;
    return (
      <>
        <Header user={user} />
        <Container maxWidth="sm" sx={{ py: 8, textAlign: "center" }} data-testid="meet-ended">
          <Sheet variant="outlined" sx={{ p: 4, borderRadius: "md" }}>
            <Typography level="h3">This meeting has ended</Typography>
            <Typography level="body-md" sx={{ mt: 1, opacity: 0.8 }}>
              {endedRoom.name || "Meeting"}
              {endedRoom.duration_s ? ` · ${fmtMeetDuration(endedRoom.duration_s)}` : ""}
              {n ? ` · ${n} ${n === 1 ? "participant" : "participants"}` : ""}
            </Typography>
            <Box sx={{ mt: 3, display: "flex", gap: 1, justifyContent: "center" }}>
              <Button
                variant="outlined"
                color="neutral"
                onClick={() => {
                  setEndedRoom(null);
                  navigate(returnTo || "/meet");
                }}
              >
                {returnTo ? "Go back" : "Back to Meet"}
              </Button>
              {isHost && (
                <Button
                  loading={restarting}
                  data-testid="meet-restart"
                  onClick={() => {
                    setRestarting(true);
                    void restartMeeting(endedRoom.code ?? "")
                      .then(() => {
                        setEndedRoom(null);
                        setReload((n) => n + 1); // re-resolve into the green room of the fresh meeting
                      })
                      .finally(() => setRestarting(false));
                  }}
                >
                  Start a new meeting
                </Button>
              )}
            </Box>
            {!isHost && (
              <Typography level="body-xs" sx={{ mt: 2, opacity: 0.6 }}>
                Ask the host to start it again.
              </Typography>
            )}
          </Sheet>
        </Container>
      </>
    );
  }

  if (pendingRoom && lmRef.current) {
    const lm = lmRef.current;
    return (
      <Prejoin
        room={pendingRoom}
        user={user}
        lm={lm}
        speakerId={speakerId}
        onSpeakerChange={setSpeakerId}
        onCancel={backToLobby}
        onJoin={(name) => joinCall(pendingRoom, name, lm, returnTo)}
      />
    );
  }

  return (
    <Lobby
      user={user}
      onJoin={(room) => {
        const q = location.search;
        navigate(pathForRoom(room) + q);
      }}
    />
  );
}
