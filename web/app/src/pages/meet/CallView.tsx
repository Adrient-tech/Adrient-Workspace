import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Badge,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dropdown,
  GlobalStyles,
  IconButton,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Tooltip,
  Typography,
} from "@mui/joy";
import VideocamIcon from "@mui/icons-material/Videocam";
import OpenInFullIcon from "@mui/icons-material/OpenInFull";
import CloseFullscreenIcon from "@mui/icons-material/CloseFullscreen";
import VideocamOffIcon from "@mui/icons-material/VideocamOff";
import MicIcon from "@mui/icons-material/Mic";
import MicOffIcon from "@mui/icons-material/MicOff";
import { HangupIcon } from "../../components/HangupIcon";
import ScreenShareIcon from "@mui/icons-material/ScreenShare";
import StopScreenShareIcon from "@mui/icons-material/StopScreenShare";
import PeopleIcon from "@mui/icons-material/People";
import ChatIcon from "@mui/icons-material/Chat";
import PanToolIcon from "@mui/icons-material/PanTool";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import EmojiEmotionsIcon from "@mui/icons-material/EmojiEmotions";
import PictureInPictureAltIcon from "@mui/icons-material/PictureInPictureAlt";
import GridViewIcon from "@mui/icons-material/GridView";
import SettingsIcon from "@mui/icons-material/Settings";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import StopCircleIcon from "@mui/icons-material/StopCircle";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  ConnectionState,
  DisconnectReason,
  LocalAudioTrack,
  LocalVideoTrack,
  ParticipantEvent,
  Room,
  RoomEvent,
  Track,
} from "livekit-client";
import { openSignalSocket, meetingLink } from "./api";
import { connIdFor, fetchSfuState, hostControl, joinWhenAdmitted, postTranscript, recordingControl, transcriptDownloadUrl } from "./sfu";
import { startTranscribing, transcriptionSupported } from "./transcribe";
import {
  REACTION_EMOJIS,
  computeGridLayout,
  computeSpotlightLayout,
  pickFeatured,
  shortcutAction,
  videoQualityFor,
  type LayoutMode,
  type Rect,
} from "./lib";
import { LocalMedia, sounds, useLocalMedia, useSpeakers } from "./media";
import { playSound } from "../../lib/sounds";
import { MediaSettings } from "./MediaSettings";
import { ChatPanel, InfoPanel, PeoplePanel, type HostControls, type PersonRow } from "./Panels";
import { Tile } from "./Tile";
import type { ChatEntry, HostAction, MeetRoom, RemotePeer, SignalMessage, Waiter } from "./types";

type SfuPhase =
  | "joining"
  | "waiting"
  | "connecting"
  | "open"
  | "reconnecting"
  | "denied"
  | "locked"
  | "removed"
  | "ended"
  | "unavailable"
  | "error";

const BLOCKED_MESSAGES: Partial<Record<SfuPhase, string>> = {
  waiting: "Waiting for the host to let you in…",
  denied: "The host did not admit you to this meeting.",
  locked: "This meeting is locked by the host.",
  removed: "The host removed you from this meeting.",
  ended: "The host ended this meeting for everyone.",
  unavailable: "Video calls are not set up on this server.",
  error: "Could not connect to the call.",
};

const LAYOUT_LABELS: Record<LayoutMode, string> = {
  auto: "Auto",
  tiled: "Tiled",
  spotlight: "Spotlight",
  sidebar: "Sidebar",
};

type Panel = "none" | "chat" | "people" | "info";

interface Props {
  room: MeetRoom;
  displayName: string;
  lm: LocalMedia;
  speakerId: string;
  onSpeakerChange: (id: string) => void;
  onLeave: () => void;
  /** Leave the call running and go back to another app (call bar shows). */
  onMinimize?: () => void;
  returnLabel?: string;
  /** False while the call runs in the background: keyboard shortcuts off. */
  active?: boolean;
  /** Docked inside another app (Chat): fill the container instead of the viewport. */
  docked?: boolean;
  /** Docked only: open the call full screen. */
  onPopOut?: () => void;
  /** Docked only: hide the docked view and keep the call in the call bar. */
  onUndock?: () => void;
}

interface Floater {
  key: string;
  emoji: string;
  left: number;
}

const barBtn = { borderRadius: "50%" } as const;

export function CallView({
  room,
  displayName,
  lm,
  speakerId,
  onSpeakerChange,
  onLeave,
  onMinimize,
  returnLabel,
  active = true,
  docked = false,
  onPopOut,
  onUndock,
}: Props) {
  useLocalMedia(lm);
  const [sfu, setSfu] = useState<SfuPhase>("joining");
  const [sfuMsg, setSfuMsg] = useState("");
  const [isHost, setIsHost] = useState(false);
  const [tick, setTick] = useState(0);
  const [hands, setHands] = useState<Record<string, boolean>>({});
  const [waiters, setWaiters] = useState<Waiter[]>([]);
  const [roomFlags, setRoomFlags] = useState({ locked: false, waitingRoom: false });
  const [transcribing, setTranscribing] = useState(false);
  const [recording, setRecording] = useState({ enabled: false, on: false, canRecord: false });
    const [consentSeen, setConsentSeen] = useState(false);
  const [wsState, setWsState] = useState<"connecting" | "open" | "error">("connecting");
  const [chat, setChat] = useState<ChatEntry[]>([]);
  const [unread, setUnread] = useState(0);
  const [panel, setPanel] = useState<Panel>("none");
  const [handRaised, setHandRaised] = useState(false);
  const [screenTrack, setScreenTrack] = useState<MediaStreamTrack | null>(null);
  const [layout, setLayout] = useState<LayoutMode>("auto");
  const [pinnedId, setPinnedId] = useState<string | null>(null);
  const [reactOpen, setReactOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [tileReactions, setTileReactions] = useState<Record<string, string>>({});
  const [floaters, setFloaters] = useState<Floater[]>([]);
  const [stage, setStage] = useState({ w: 0, h: 0 });

  const wsRef = useRef<WebSocket | null>(null);
  const lkRef = useRef<Room | null>(null);
  const pubChain = useRef<Promise<void>>(Promise.resolve());
  const localPubs = useRef<{ audio?: LocalAudioTrack; video?: LocalVideoTrack; screen?: LocalVideoTrack }>({});
  const screenTrackRef = useRef<MediaStreamTrack | null>(null);
  const panelRef = useRef<Panel>("none");
  const videoElsRef = useRef<Map<string, HTMLVideoElement>>(new Map());
  const stageRef = useRef<HTMLDivElement>(null);
  const reactTimers = useRef<Map<string, number>>(new Map());
  panelRef.current = panel;

  const shareLink = room.code ? meetingLink(room.code) : window.location.href;
  const micOn = lm.micOn && !!lm.micTrack;
  const camOn = lm.camOn && !!lm.camTrack;
  const sharing = !!screenTrack;

  // ---- hub signalling (chat, reactions, hand raise, lobby) ----
  const sendSignal = useCallback((msg: SignalMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  const connId = useMemo(() => connIdFor(room.id), [room.id]);
  const handRaisedRef = useRef(false);
  handRaisedRef.current = handRaised;

  // ---- LiveKit publishing (serialised so replace/unpublish never race) ----
  const queue = useCallback((fn: () => Promise<void>) => {
    pubChain.current = pubChain.current.then(fn).catch((e) => console.warn("[meet] publish failed", e));
  }, []);

  const publishKind = useCallback(
    (kind: "audio" | "video", track: MediaStreamTrack | null) => {
      queue(async () => {
        const lk = lkRef.current;
        if (lk && lk.state !== ConnectionState.Connected && lk.state !== ConnectionState.Disconnected) {
          // Connecting / reconnecting: wait (bounded) instead of dropping the publish.
          await new Promise<void>((res) => {
            const done = () => {
              lk.off(RoomEvent.Connected, done).off(RoomEvent.Reconnected, done);
              window.clearTimeout(t);
              res();
            };
            const t = window.setTimeout(done, 15000);
            lk.on(RoomEvent.Connected, done).on(RoomEvent.Reconnected, done);
          });
        }
        if (!lk || lk !== lkRef.current || lk.state !== ConnectionState.Connected) {
          return;
        }
        const lp = lk.localParticipant;
        const cur = localPubs.current[kind];
        if (!track) {
          if (cur) {
            localPubs.current[kind] = undefined;
            await lp.unpublishTrack(cur, false);
          }
          return;
        }
        if (cur) {
          if (cur.mediaStreamTrack !== track) await cur.replaceTrack(track, true);
          return;
        }
        if (kind === "audio") {
          const t = new LocalAudioTrack(track, undefined, true);
          localPubs.current.audio = t;
          await lp.publishTrack(t, { source: Track.Source.Microphone });
          if (!lm.micOn) await t.mute();
        } else {
          const t = new LocalVideoTrack(track, undefined, true);
          localPubs.current.video = t;
          await lp.publishTrack(t, {
            source: Track.Source.Camera,
            simulcast: true,
            videoEncoding: { maxBitrate: lm.quality.maxBitrate, maxFramerate: lm.quality.frameRate },
          });
        }
      });
    },
    [lm, queue],
  );

  // ---- device tracks -> LiveKit ----
  useEffect(() => {
    lm.cancelStop();
    lm.onPublish = (kind, track) => {
      publishKind(kind, track);
      if (kind === "video") {
        sendSignal({ type: "media_state", audio_muted: !lm.micOn, video_off: !track });
      }
    };
    return () => {
      lm.onPublish = () => {};
      lm.scheduleStop();
    };
  }, [lm, sendSignal, publishKind]);

  const flashReaction = useCallback((id: string, emoji: string) => {
    setTileReactions((p) => ({ ...p, [id]: emoji }));
    const prev = reactTimers.current.get(id);
    if (prev) window.clearTimeout(prev);
    reactTimers.current.set(
      id,
      window.setTimeout(() => {
        setTileReactions((p) => {
          const n = { ...p };
          delete n[id];
          return n;
        });
      }, 4000),
    );
    const key = `${id}-${Date.now()}-${Math.random()}`;
    setFloaters((f) => [...f.slice(-12), { key, emoji, left: 10 + Math.random() * 70 }]);
    window.setTimeout(() => setFloaters((f) => f.filter((x) => x.key !== key)), 3200);
  }, []);

  const handleSignal = useCallback(
    (msg: SignalMessage) => {
      switch (msg.type) {
        case "roster_state":
          setHands((prev) => {
            const n = { ...prev };
            for (const p of msg.peers ?? []) n[p.id] = !!p.hand_raised;
            return n;
          });
          break;
        case "join":
          // Tell the newcomer our hand state (the SFU carries media, the hub carries this).
          if (handRaisedRef.current) sendSignal({ type: "hand_raise", hand_raised: true });
          break;
        case "leave":
          if (msg.from) {
            setHands((prev) => {
              const n = { ...prev };
              delete n[msg.from!];
              return n;
            });
          }
          break;
        case "chat": {
          if (!msg.from) break;
          setChat((prev) => [
            ...prev,
            {
              id: `${msg.from}-${Date.now()}-${Math.random()}`,
              fromId: msg.from!,
              fromName: msg.name ?? msg.from!,
              text: msg.text ?? "",
              sentAt: Date.now(),
            },
          ]);
          if (panelRef.current !== "chat") setUnread((n) => n + 1);
          sounds.message();
          break;
        }
        case "meeting_ended":
          setSfu("ended");
          break;
        case "hand_raise":
          if (msg.from) {
            setHands((prev) => ({ ...prev, [msg.from!]: !!msg.hand_raised }));
            if (msg.hand_raised) sounds.hand();
          }
          break;
        case "reaction":
          if (msg.from && msg.emoji) flashReaction(msg.from, msg.emoji);
          break;
        case "lobby": {
          const p = msg.payload as { waiting?: Waiter[] } | undefined;
          setWaiters(p?.waiting ?? []);
          break;
        }
        case "room_state": {
          const p = msg.payload as { locked?: boolean; waiting_room?: boolean; recording?: boolean; transcribing?: boolean } | undefined;
          setRoomFlags({ locked: !!p?.locked, waitingRoom: !!p?.waiting_room });
          setTranscribing(!!p?.transcribing);
          if (p && typeof p.recording === "boolean") setRecording((r) => ({ ...r, on: !!p.recording }));
          break;
        }
      }
    },
    [sendSignal, flashReaction],
  );

  // ---- hub websocket ----
  useEffect(() => {
    const ws = openSignalSocket(room.id, displayName, connId);
    wsRef.current = ws;
    setWsState("connecting");
    ws.onopen = () => {
      setWsState("open");
      ws.send(
        JSON.stringify({
          type: "media_state",
          audio_muted: !lm.micOn,
          video_off: !lm.camTrack,
        } satisfies SignalMessage),
      );
    };
    ws.onerror = () => setWsState("error");
    ws.onclose = () => setWsState("error");
    ws.onmessage = (evt) => {
      try {
        handleSignal(JSON.parse(evt.data as string) as SignalMessage);
      } catch {
        /* ignore malformed frame */
      }
    };
    return () => {
      ws.onclose = null;
      ws.close();
      wsRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room.id, handleSignal]);

  // ---- LiveKit room: join (waiting room aware) + events ----
  useEffect(() => {
    const ac = new AbortController();
    let cancelled = false;
    let lk: Room | null = null;
    const bump = () => setTick((n) => n + 1);
    setSfu("joining");
    void (async () => {
      let res;
      try {
        res = await joinWhenAdmitted(room.id, displayName, connId, ac.signal, () => setSfu("waiting"));
      } catch {
        return; // aborted
      }
      if (cancelled) return;
      if (res.status !== "ok") {
        setSfuMsg(res.status === "unavailable" || res.status === "error" ? (res.message ?? "") : "");
        setSfu(res.status === "waiting" ? "waiting" : res.status);
        return;
      }
      setIsHost(res.isHost);
      setSfu("connecting");
      lk = new Room({
        adaptiveStream: true,
        dynacast: true,
        publishDefaults: { simulcast: true },
      });
      lkRef.current = lk;
      lk.on(RoomEvent.ParticipantConnected, () => {
        sounds.join();
        bump();
      })
        .on(RoomEvent.ParticipantDisconnected, (p) => {
          sounds.leave();
          setPinnedId((cur) => (cur === p.identity ? null : cur));
          bump();
        })
        .on(RoomEvent.TrackSubscribed, bump)
        .on(RoomEvent.TrackUnsubscribed, bump)
        .on(RoomEvent.TrackMuted, bump)
        .on(RoomEvent.TrackUnmuted, bump)
        .on(RoomEvent.TrackPublished, bump)
        .on(RoomEvent.TrackUnpublished, bump)
        .on(RoomEvent.ParticipantNameChanged, bump)
        .on(RoomEvent.RecordingStatusChanged, (on: boolean) => setRecording((r) => ({ ...r, on })))
        .on(RoomEvent.Reconnecting, () => setSfu("reconnecting"))
        .on(RoomEvent.Reconnected, () => {
          setSfu("open");
          bump();
        })
        .on(RoomEvent.Disconnected, (reason) => {
          if (cancelled) return;
          if (reason === DisconnectReason.PARTICIPANT_REMOVED) setSfu("removed");
          else if (reason === DisconnectReason.ROOM_DELETED) setSfu("ended");
          else setSfu("error");
        });
      // The host muted our mic on the server: reflect it in our own controls.
      lk.localParticipant.on(ParticipantEvent.TrackMuted, (pub) => {
        if (pub.source === Track.Source.Microphone && lm.micOn) lm.setMic(false);
      });
      try {
        await lk.connect(res.url, res.token);
      } catch {
        if (!cancelled) setSfu("error");
        return;
      }
      if (cancelled) {
        void lk.disconnect();
        return;
      }
      setSfu("open");
      if (lm.micTrack) publishKind("audio", lm.micTrack);
      if (lm.camTrack) publishKind("video", lm.camTrack);
      const st = screenTrackRef.current;
      if (st) publishScreen(st);
      bump();
    })();
    return () => {
      cancelled = true;
      ac.abort();
      localPubs.current = {};
      const cur = lk;
      lkRef.current = null;
      if (cur) void cur.disconnect();
      setTick((n) => n + 1);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room.id]);

  // Host: load lock / waiting-room / recording state once connected.
  useEffect(() => {
    if (sfu !== "open") return;
    void fetchSfuState(room.id).then((s) => {
      if (!s) return;
      setRoomFlags({ locked: s.locked, waitingRoom: s.waiting_room });
      setTranscribing(!!s.transcribing);
      setRecording({ enabled: s.recording_enabled, on: s.recording, canRecord: !!s.can_record });
      setWaiters(s.waiting ?? []);
    });
  }, [sfu, room.id]);

  // ---- remote participants, from the LiveKit room ----
  const streamCache = useRef(new Map<string, { key: string; stream: MediaStream | null }>());
  const remotePeers: RemotePeer[] = useMemo(() => {
    const lk = lkRef.current;
    if (!lk) return [];
    const out: RemotePeer[] = [];
    lk.remoteParticipants.forEach((p) => {
      const cam = p.getTrackPublication(Track.Source.Camera);
      const scr = p.getTrackPublication(Track.Source.ScreenShare);
      const mic = p.getTrackPublication(Track.Source.Microphone);
      const camT = cam?.track && !cam.isMuted ? cam.track : null;
      const scrT = scr?.track ?? null;
      const micT = mic?.track ?? null;
      const videoTrack = scrT ?? camT;
      const key = `${micT?.mediaStreamTrack.id ?? ""}|${videoTrack?.mediaStreamTrack.id ?? ""}`;
      let c = streamCache.current.get(p.identity);
      if (!c || c.key !== key) {
        const tracks = [micT?.mediaStreamTrack, videoTrack?.mediaStreamTrack].filter(
          (t): t is MediaStreamTrack => !!t,
        );
        c = { key, stream: tracks.length ? new MediaStream(tracks) : null };
        streamCache.current.set(p.identity, c);
      }
      out.push({
        id: p.identity,
        name: p.name || p.identity,
        stream: c.stream,
        audioTrack: micT,
        videoTrack,
        audioMuted: !mic || mic.isMuted,
        videoOff: !videoTrack,
        handRaised: !!hands[p.identity],
        sharing: !!scrT,
      });
    });
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, hands]);

  // ---- adaptive capture quality ----
  const participantCount = 1 + remotePeers.length;
  useEffect(() => {
    void lm.setQuality(videoQualityFor(participantCount));
  }, [participantCount, lm]);


  // ---- controls ----
  const toggleMic = useCallback(() => {
    lm.setMic(!lm.micOn);
    sendSignal({ type: "media_state", audio_muted: !lm.micOn, video_off: !lm.camTrack });
  }, [lm, sendSignal]);

  const toggleCam = useCallback(async () => {
    await lm.setCam(!lm.camOn);
    sendSignal({ type: "media_state", audio_muted: !lm.micOn, video_off: !lm.camTrack });
  }, [lm, sendSignal]);

  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      const a = shortcutAction(e);
      if (!a) return;
      e.preventDefault();
      if (a === "mic") toggleMic();
      else void toggleCam();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggleMic, toggleCam, active]);

  // Mute changes made outside the call screen (the call bar) still reach peers.
  const lastMic = useRef(lm.micOn);
  useEffect(
    () =>
      lm.subscribe(() => {
        if (lm.micOn === lastMic.current) return;
        lastMic.current = lm.micOn;
        playSound(lm.micOn ? "unmute" : "mute");
        const a = localPubs.current.audio;
        if (a) void (lm.micOn ? a.unmute() : a.mute()).catch(() => {});
        sendSignal({ type: "media_state", audio_muted: !lm.micOn, video_off: !lm.camTrack });
      }),
    [lm, sendSignal],
  );

  const publishScreen = useCallback(
    (t: MediaStreamTrack) => {
      queue(async () => {
        const lk = lkRef.current;
        if (!lk || lk.state !== ConnectionState.Connected || localPubs.current.screen) return;
        const lt = new LocalVideoTrack(t, undefined, true);
        localPubs.current.screen = lt;
        await lk.localParticipant.publishTrack(lt, {
          source: Track.Source.ScreenShare,
          simulcast: false,
          videoEncoding: { maxBitrate: 2_500_000, maxFramerate: 15 },
        });
      });
    },
    [queue],
  );

  const stopScreen = useCallback(() => {
    screenTrackRef.current?.stop();
    screenTrackRef.current = null;
    setScreenTrack(null);
    queue(async () => {
      const cur = localPubs.current.screen;
      localPubs.current.screen = undefined;
      if (cur && lkRef.current) await lkRef.current.localParticipant.unpublishTrack(cur, false);
    });
    sendSignal({ type: "screen_share", sharing: false });
  }, [sendSignal, queue]);

  const toggleScreen = async () => {
    if (screenTrackRef.current) {
      stopScreen();
      return;
    }
    try {
      const s = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
      const t = s.getVideoTracks()[0];
      if (!t) return;
      screenTrackRef.current = t;
      setScreenTrack(t);
      publishScreen(t);
      sendSignal({ type: "screen_share", sharing: true });
      playSound("share");
      t.onended = () => stopScreen();
    } catch {
      /* cancelled or unsupported */
    }
  };

  // ---- host controls ----
  const hostDo = useCallback(
    (action: HostAction, identity?: string) => {
      void hostControl(room.id, action, identity).then(() => {
        if (action === "lock" || action === "unlock" || action === "waiting_room_on" || action === "waiting_room_off" || action === "transcript_on" || action === "transcript_off") {
          void fetchSfuState(room.id).then((st) => {
            if (!st) return;
            setRoomFlags({ locked: st.locked, waitingRoom: st.waiting_room });
            setTranscribing(!!st.transcribing);
          });
        }
        if (action === "admit" || action === "deny") setWaiters((w) => w.filter((x) => x.identity !== identity));
      });
    },
    [room.id],
  );
  const toggleRecording = useCallback(() => {
    const next = !recording.on;
    void recordingControl(room.id, next ? "start" : "stop").then((ok) => {
      if (ok) {
        setRecording((r) => ({ ...r, on: next }));
        if (next) setConsentSeen(true);
      }
    });
  }, [room.id, recording.on]);
  const recOn = recording.on;
  // Recording start/stop tones (not on the first render, and not for a recording that was already running when we joined).
  const prevRec = useRef<boolean | null>(null);
  useEffect(() => {
    if (sfu !== "open") return;
    if (prevRec.current !== null && prevRec.current !== recOn) playSound(recOn ? "rec_start" : "rec_stop");
    prevRec.current = recOn;
  }, [recOn, sfu]);
  // The meeting ended for everyone (host ended it): one closing tone.
  const endedTone = useRef(false);
  useEffect(() => {
    if (sfu === "ended" && !endedTone.current) {
      endedTone.current = true;
      playSound("meeting_ended");
    }
  }, [sfu]);

  // While the host has the transcript on, turn this browser's own microphone speech into transcript lines.
  useEffect(() => {
    if (!transcribing || sfu !== "open" || !micOn || !transcriptionSupported()) return;
    return startTranscribing((text) => void postTranscript(room.id, text).catch(() => false));
  }, [transcribing, sfu, micOn, room.id]);
  useEffect(() => {
    if (!recOn) setConsentSeen(false);
  }, [recOn]);

  const toggleHand = () => {
    const next = !handRaised;
    setHandRaised(next);
    sendSignal({ type: "hand_raise", hand_raised: next });
  };

  const react = (emoji: string) => {
    sendSignal({ type: "reaction", emoji });
    flashReaction("local", emoji);
    setReactOpen(false);
  };

  const sendChat = (text: string) => {
    setChat((p) => [
      ...p,
      { id: `local-${Date.now()}-${Math.random()}`, fromId: "local", fromName: displayName, text, sentAt: Date.now() },
    ]);
    sendSignal({ type: "chat", name: displayName, text });
  };

  const openPanel = (p: Exclude<Panel, "none">) => {
    setPanel((cur) => (cur === p ? "none" : p));
    if (p === "chat") setUnread(0);
  };
  useEffect(() => {
    if (panel === "chat") setUnread(0);
  }, [panel, chat.length]);

  const leave = () => {
    screenTrackRef.current?.stop();
    wsRef.current?.close();
    void lkRef.current?.disconnect();
    lm.stop();
    onLeave();
  };

  const togglePin = useCallback((id: string) => setPinnedId((p) => (p === id ? null : id)), []);
  const registerVideo = useCallback((id: string, el: HTMLVideoElement | null) => {
    if (el) videoElsRef.current.set(id, el);
    else videoElsRef.current.delete(id);
  }, []);

  // ---- streams for tiles / audio analysis ----
  const localStream = useMemo(() => {
    const t = screenTrack ?? lm.camTrack;
    return t ? new MediaStream([t]) : null;
  }, [screenTrack, lm.camTrack]);
  const localAudio = useMemo(
    () => (lm.micTrack ? new MediaStream([lm.micTrack]) : null),
    [lm.micTrack],
  );

  const streamsForAudio = useMemo(() => {
    const m: Record<string, MediaStream | null> = { local: localAudio };
    remotePeers.forEach((p) => (m[p.id] = p.stream));
    return m;
  }, [localAudio, remotePeers]);
  const speakers = useSpeakers(streamsForAudio);

  // ---- layout ----
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setStage({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    setStage({ w: el.clientWidth, h: el.clientHeight });
    return () => ro.disconnect();
  }, []);

  const tileIds = useMemo(
    () => [
      { id: "local", sharing },
      ...remotePeers.map((p) => ({ id: p.id, sharing: p.sharing })),
    ],
    [remotePeers, sharing],
  );
  const featuredId = pickFeatured(layout, tileIds, pinnedId, speakers.active);
  const rects: Record<string, Rect> = useMemo(() => {
    const out: Record<string, Rect> = {};
    const pad = 8;
    const w = Math.max(0, stage.w - pad * 2);
    const h = Math.max(0, stage.h - pad * 2);
    if (!featuredId) {
      const g = computeGridLayout(tileIds.length, w, h);
      tileIds.forEach((t, i) => (out[t.id] = shift(g.rects[i], pad)));
    } else {
      const rest = tileIds.filter((t) => t.id !== featuredId);
      const sp = computeSpotlightLayout(rest.length, w, h, {
        stripSize: layout === "sidebar" ? 260 : layout === "spotlight" ? 150 : 200,
      });
      out[featuredId] = shift(sp.featured, pad);
      rest.forEach((t, i) => (out[t.id] = shift(sp.strip[i], pad)));
    }
    return out;
  }, [stage, tileIds, featuredId, layout]);

  const togglePip = async () => {
    try {
      if (document.pictureInPictureElement) {
        await document.exitPictureInPicture();
        return;
      }
      const id =
        featuredId ??
        speakers.active ??
        remotePeers.find((p) => p.stream)?.id ??
        "local";
      const el = videoElsRef.current.get(id) ?? videoElsRef.current.get("local");
      await el?.requestPictureInPicture();
    } catch {
      /* not allowed */
    }
  };

  const people: PersonRow[] = [
    {
      id: "local",
      name: displayName,
      isLocal: true,
      audioMuted: !micOn,
      videoOff: !camOn,
      handRaised,
      sharing,
      speaking: !!speakers.speaking.local,
    },
    ...remotePeers.map((p) => ({
      id: p.id,
      name: p.name,
      audioMuted: p.audioMuted,
      videoOff: p.videoOff,
      handRaised: p.handRaised,
      sharing: p.sharing,
      speaking: !!speakers.speaking[p.id],
    })),
  ];

  const hostControls: HostControls | undefined = isHost
    ? {
        locked: roomFlags.locked,
        waitingRoom: roomFlags.waitingRoom,
        waiters,
        recording: { enabled: recording.enabled && recording.canRecord, on: recOn },
        onMute: (id) => hostDo("mute", id),
        onRemove: (id) => hostDo("remove", id),
        onMuteAll: () => hostDo("mute_all"),
        onToggleLock: () => hostDo(roomFlags.locked ? "unlock" : "lock"),
        onToggleWaitingRoom: () => hostDo(roomFlags.waitingRoom ? "waiting_room_off" : "waiting_room_on"),
        onEnd: () => {
          void hostControl(room.id, "end").finally(leave);
        },
        onAdmit: (id) => hostDo("admit", id),
        onDeny: (id) => hostDo("deny", id),
        onToggleRecording: toggleRecording,
        transcript: { on: transcribing, downloadUrl: transcriptDownloadUrl(room.id) },
        onToggleTranscript: () => hostDo(transcribing ? "transcript_off" : "transcript_on"),
      }
    : undefined;
  const blockedMsg = BLOCKED_MESSAGES[sfu];
  const blocked = !!blockedMsg && sfu !== "reconnecting";

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: docked ? "100%" : "100vh", minHeight: 0, bgcolor: "#0d1a17" }} data-testid="call-view" data-docked={docked ? "1" : "0"}>
      <GlobalStyles
        styles={{
          "@keyframes meetFloatUp": {
            "0%": { transform: "translateY(0) scale(0.8)", opacity: 0 },
            "15%": { opacity: 1 },
            "100%": { transform: "translateY(-260px) scale(1.4)", opacity: 0 },
          },
        }}
      />
      {/* Top bar */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          px: { xs: 1.5, sm: 3 },
          py: 1,
          borderBottom: "1px solid rgba(255,255,255,0.1)",
          flexShrink: 0,
          gap: 1,
        }}
      >
        {onMinimize && (
          <Tooltip title={`Back to ${returnLabel ?? "workspace"} (call keeps running)`}>
            <IconButton
              size="sm"
              variant="plain"
              onClick={onMinimize}
              aria-label={`Back to ${returnLabel ?? "workspace"}`}
              data-testid="call-minimize"
              sx={{ color: "#fff", "&:hover": { bgcolor: "rgba(255,255,255,0.12)" } }}
            >
              <ArrowBackIcon />
            </IconButton>
          </Tooltip>
        )}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography level="title-md" sx={{ color: "#fff" }} noWrap>
            {room.name}
          </Typography>
          {room.code && (
            <Typography level="body-xs" sx={{ color: "rgba(255,255,255,0.55)", letterSpacing: 1 }}>
              {room.code}
            </Typography>
          )}
        </Box>
        {recOn && (
          <Chip size="sm" variant="solid" color="danger" startDecorator={<FiberManualRecordIcon sx={{ fontSize: 12 }} />} data-testid="rec-indicator">
            REC
          </Chip>
        )}
        {transcribing && (
          <Chip size="sm" variant="soft" color="warning" data-testid="transcript-indicator" title={transcriptionSupported() ? "Your speech is turned into text for the meeting transcript" : "This browser cannot transcribe speech, so you are not in the transcript"}>
            Transcript on
          </Chip>
        )}
        {wsState === "connecting" && <CircularProgress size="sm" />}
        {wsState === "error" && (
          <Typography level="body-xs" sx={{ color: "#ff8a80" }}>
            Signalling disconnected
          </Typography>
        )}
        {lm.bgBusy && (
          <Typography level="body-xs" sx={{ color: "#9be7cf" }}>
            Loading background effect…
          </Typography>
        )}
        <Chip size="sm" variant="soft" color="neutral" startDecorator={<PeopleIcon sx={{ fontSize: 14 }} />} data-testid="participant-count">
          {participantCount}
        </Chip>
        {onUndock && (
          <Tooltip title="Undock (call keeps running in the call bar)">
            <IconButton size="sm" variant="plain" onClick={onUndock} aria-label="Undock call" data-testid="call-undock" sx={{ color: "#fff", "&:hover": { bgcolor: "rgba(255,255,255,0.12)" } }}>
              <CloseFullscreenIcon />
            </IconButton>
          </Tooltip>
        )}
        {onPopOut && (
          <Tooltip title="Full screen">
            <IconButton size="sm" variant="plain" onClick={onPopOut} aria-label="Full screen" data-testid="call-popout" sx={{ color: "#fff", "&:hover": { bgcolor: "rgba(255,255,255,0.12)" } }}>
              <OpenInFullIcon />
            </IconButton>
          </Tooltip>
        )}
      </Box>

      {lm.error && (
        <Box sx={{ bgcolor: "#3d2b00", px: 3, py: 0.75, flexShrink: 0 }}>
          <Typography level="body-sm" sx={{ color: "#ffd166" }}>
            {lm.error}
          </Typography>
        </Box>
      )}

      {recOn && !consentSeen && !recording.canRecord && (
        <Box
          data-testid="recording-consent"
          sx={{ bgcolor: "#4a1d1d", px: 2, py: 0.75, flexShrink: 0, display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}
        >
          <Typography level="body-sm" sx={{ color: "#ffd9d9", flex: 1 }}>
            This meeting is being recorded. By staying you agree to be recorded.
          </Typography>
          <Button size="sm" variant="soft" color="neutral" onClick={() => setConsentSeen(true)} data-testid="recording-consent-ok">
            OK
          </Button>
          <Button size="sm" variant="solid" color="danger" onClick={leave} data-testid="recording-consent-leave">
            Leave
          </Button>
        </Box>
      )}

      <Box sx={{ flex: 1, display: "flex", overflow: "hidden", minHeight: 0 }}>
        <Box ref={stageRef} sx={{ flex: 1, position: "relative", overflow: "hidden", minWidth: 0 }} data-testid="stage">
          {stage.w > 0 &&
            tileIds.map((t) => {
              const isLocal = t.id === "local";
              const peer = isLocal ? null : remotePeers.find((p) => p.id === t.id)!;
              const rect = rects[t.id];
              if (!rect) return null;
              const isSharing = isLocal ? sharing : peer!.sharing;
              const stream = isLocal ? localStream : peer!.stream;
              const hasVideo = isLocal
                ? !!localStream
                : !!peer!.stream && peer!.stream.getVideoTracks().length > 0 && !peer!.videoOff;
              return (
                <Tile
                  key={t.id}
                  id={t.id}
                  label={isLocal ? `${displayName} (You)` : peer!.name}
                  stream={stream}
                  lkVideo={isLocal ? null : peer!.videoTrack}
                  lkAudio={isLocal ? null : peer!.audioTrack}
                  isLocal={isLocal}
                  audioMuted={isLocal ? !micOn : peer!.audioMuted}
                  showVideo={hasVideo || (isSharing && !!stream)}
                  handRaised={isLocal ? handRaised : peer!.handRaised}
                  sharing={isSharing}
                  speaking={!!speakers.speaking[t.id]}
                  pinned={pinnedId === t.id}
                  reaction={tileReactions[t.id]}
                  rect={rect}
                  speakerId={speakerId}
                  onTogglePin={togglePin}
                  registerVideo={registerVideo}
                />
              );
            })}
          {isHost && waiters.length > 0 && (
            <Box
              data-testid="lobby-banner"
              sx={{ position: "absolute", top: 12, left: "50%", transform: "translateX(-50%)", zIndex: 5, bgcolor: "#16231f", border: "1px solid #f4a11d", borderRadius: "lg", px: 2, py: 1, display: "flex", flexDirection: "column", gap: 0.75, maxWidth: "90%" }}
            >
              {waiters.slice(0, 3).map((w) => (
                <Box key={w.identity} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Typography level="body-sm" sx={{ color: "#fff" }}>
                    {w.name} wants to join
                  </Typography>
                  <Button size="sm" color="primary" onClick={() => hostDo("admit", w.identity)} data-testid="lobby-admit">
                    Admit
                  </Button>
                  <Button size="sm" variant="soft" color="neutral" onClick={() => hostDo("deny", w.identity)} data-testid="lobby-deny">
                    Deny
                  </Button>
                </Box>
              ))}
            </Box>
          )}
          {(sfu === "joining" || sfu === "connecting" || sfu === "reconnecting") && (
            <Box sx={{ position: "absolute", top: 8, left: 8, zIndex: 4 }}>
              <Chip size="sm" variant="soft" color="neutral" startDecorator={<CircularProgress size="sm" />} data-testid="sfu-connecting">
                {sfu === "reconnecting" ? "Reconnecting…" : "Connecting…"}
              </Chip>
            </Box>
          )}
          {blocked && (
            <Box
              data-testid="call-blocked"
              data-phase={sfu}
              sx={{ position: "absolute", inset: 0, zIndex: 6, bgcolor: "rgba(13,26,23,0.94)", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 2, textAlign: "center", p: 3 }}
            >
              {sfu === "waiting" && <CircularProgress />}
              <Typography level="title-lg" sx={{ color: "#fff" }}>
                {sfuMsg || blockedMsg}
              </Typography>
              <Button color="neutral" variant="soft" onClick={leave} data-testid="call-blocked-leave">
                {sfu === "waiting" ? "Cancel" : "Leave"}
              </Button>
            </Box>
          )}
          {floaters.map((f) => (
            <Box
              key={f.key}
              sx={{
                position: "absolute",
                bottom: 24,
                left: `${f.left}%`,
                fontSize: 40,
                pointerEvents: "none",
                animation: "meetFloatUp 3s ease-out forwards",
              }}
            >
              {f.emoji}
            </Box>
          ))}
        </Box>

        {panel === "chat" && (
          <ChatPanel messages={chat} localId="local" onSend={sendChat} onClose={() => setPanel("none")} />
        )}
        {panel === "people" && (
          <PeoplePanel people={people} pinnedId={pinnedId} onTogglePin={togglePin} onClose={() => setPanel("none")} host={hostControls} />
        )}
        {panel === "info" && (
          <InfoPanel name={room.name} code={room.code ?? null} link={shareLink} onClose={() => setPanel("none")} />
        )}
      </Box>

      {/* Controls */}
      <Box
        sx={{
          position: "relative",
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          flexWrap: "wrap",
          gap: docked ? 0.75 : 1.25,
          py: docked ? 1 : 1.5,
          px: 1,
          flexShrink: 0,
          ...(docked ? { "& .MuiIconButton-sizeLg": { "--IconButton-size": "40px" } } : {}),
        }}
        data-testid="call-controls"
      >
        {reactOpen && (
          <Box
            data-testid="reaction-picker"
            sx={{
              position: "absolute",
              bottom: "100%",
              bgcolor: "#16231f",
              border: "1px solid rgba(255,255,255,0.15)",
              borderRadius: "xl",
              px: 1,
              py: 0.5,
              display: "flex",
              gap: 0.5,
              mb: 0.5,
            }}
          >
            {REACTION_EMOJIS.map((e) => (
              <IconButton key={e} variant="plain" onClick={() => react(e)} aria-label={`React ${e}`} sx={{ fontSize: 24 }}>
                {e}
              </IconButton>
            ))}
          </Box>
        )}
        <Tooltip title={micOn ? "Turn off microphone (Ctrl+D)" : "Turn on microphone (Ctrl+D)"}>
          <IconButton
            size="lg"
            variant="solid"
            color={micOn ? "neutral" : "danger"}
            onClick={toggleMic}
            disabled={!lm.micTrack}
            aria-label={micOn ? "Mute microphone" : "Unmute microphone"}
            aria-pressed={!micOn}
            sx={barBtn}
          >
            {micOn ? <MicIcon /> : <MicOffIcon />}
          </IconButton>
        </Tooltip>
        <Tooltip title={camOn ? "Turn off camera (Ctrl+E)" : "Turn on camera (Ctrl+E)"}>
          <IconButton
            size="lg"
            variant="solid"
            color={camOn ? "neutral" : "danger"}
            onClick={() => void toggleCam()}
            aria-label={camOn ? "Turn off camera" : "Turn on camera"}
            aria-pressed={!camOn}
            sx={barBtn}
          >
            {camOn ? <VideocamIcon /> : <VideocamOffIcon />}
          </IconButton>
        </Tooltip>
        <Tooltip title={sharing ? "Stop presenting" : "Present screen"}>
          <IconButton
            size="lg"
            variant={sharing ? "solid" : "soft"}
            color={sharing ? "primary" : "neutral"}
            onClick={() => void toggleScreen()}
            aria-label={sharing ? "Stop presenting" : "Present screen"}
            sx={barBtn}
          >
            {sharing ? <StopScreenShareIcon /> : <ScreenShareIcon />}
          </IconButton>
        </Tooltip>
        {recording.canRecord && (
          <Tooltip title={recOn ? "Stop recording" : "Record meeting"}>
            <IconButton
              size="lg"
              variant={recOn ? "solid" : "soft"}
              color={recOn ? "danger" : "neutral"}
              onClick={toggleRecording}
              aria-label={recOn ? "Stop recording" : "Record meeting"}
              aria-pressed={recOn}
              data-testid="record-btn"
              sx={barBtn}
            >
              {recOn ? <StopCircleIcon /> : <FiberManualRecordIcon sx={{ color: "#ff5252" }} />}
            </IconButton>
          </Tooltip>
        )}
        <Tooltip title={reactOpen ? "" : "Send a reaction"}>
          <IconButton
            size="lg"
            variant={reactOpen ? "solid" : "soft"}
            color={reactOpen ? "primary" : "neutral"}
            onClick={() => setReactOpen((o) => !o)}
            aria-label="Send a reaction"
            sx={barBtn}
          >
            <EmojiEmotionsIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title={handRaised ? "Lower hand" : "Raise hand"}>
          <IconButton
            size="lg"
            variant={handRaised ? "solid" : "soft"}
            color={handRaised ? "warning" : "neutral"}
            onClick={toggleHand}
            aria-label={handRaised ? "Lower hand" : "Raise hand"}
            aria-pressed={handRaised}
            sx={barBtn}
          >
            <PanToolIcon />
          </IconButton>
        </Tooltip>
        <Dropdown>
          <Tooltip title={`Layout: ${LAYOUT_LABELS[layout]}`}>
            <MenuButton
              slots={{ root: IconButton }}
              slotProps={{
                root: { size: "lg", variant: "soft", color: "neutral", sx: barBtn, "aria-label": "Change layout" },
              }}
            >
              <GridViewIcon />
            </MenuButton>
          </Tooltip>
          <Menu placement="top" size="sm">
            {(Object.keys(LAYOUT_LABELS) as LayoutMode[]).map((m) => (
              <MenuItem key={m} selected={layout === m} onClick={() => setLayout(m)}>
                {LAYOUT_LABELS[m]}
              </MenuItem>
            ))}
          </Menu>
        </Dropdown>
        {typeof document !== "undefined" && document.pictureInPictureEnabled && (
          <Tooltip title="Picture in picture">
            <IconButton size="lg" variant="soft" color="neutral" onClick={() => void togglePip()} aria-label="Picture in picture" sx={barBtn}>
              <PictureInPictureAltIcon />
            </IconButton>
          </Tooltip>
        )}
        <Tooltip title="Settings, devices and effects">
          <IconButton size="lg" variant="soft" color="neutral" onClick={() => setSettingsOpen(true)} aria-label="Settings" sx={barBtn}>
            <SettingsIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="Meeting details">
          <IconButton
            size="lg"
            variant={panel === "info" ? "solid" : "soft"}
            color={panel === "info" ? "primary" : "neutral"}
            onClick={() => openPanel("info")}
            aria-label="Meeting details"
            sx={barBtn}
          >
            <InfoOutlinedIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="People">
          <IconButton
            size="lg"
            variant={panel === "people" ? "solid" : "soft"}
            color={panel === "people" ? "primary" : "neutral"}
            onClick={() => openPanel("people")}
            aria-label="Show participants"
            sx={barBtn}
          >
            <PeopleIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="Chat">
          <IconButton
            size="lg"
            variant={panel === "chat" ? "solid" : "soft"}
            color={panel === "chat" ? "primary" : "neutral"}
            onClick={() => openPanel("chat")}
            aria-label="Show chat"
            sx={barBtn}
          >
            <Badge badgeContent={unread > 0 ? unread : undefined} color="danger" size="sm">
              <ChatIcon />
            </Badge>
          </IconButton>
        </Tooltip>
        <Tooltip title="Leave call">
          <IconButton size="lg" variant="solid" color="danger" onClick={leave} aria-label="Leave call" sx={{ ...barBtn, px: 3 }}>
            <HangupIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {settingsOpen && (
        <Modal open onClose={() => setSettingsOpen(false)}>
          <ModalDialog sx={{ width: { xs: "100vw", sm: 420 }, maxWidth: "100vw" }}>
            <ModalClose />
            <Typography level="h4" sx={{ mb: 1 }}>
              Settings
            </Typography>
            <MediaSettings lm={lm} speakerId={speakerId} onSpeakerChange={onSpeakerChange} />
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

function shift(r: Rect | undefined, pad: number): Rect {
  const b = r ?? { x: 0, y: 0, w: 0, h: 0 };
  return { x: b.x + pad, y: b.y + pad, w: b.w, h: b.h };
}
