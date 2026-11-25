import { firstUnreadIndex, parseSlash } from "./slash";
import { activeTypers, TYPING_TTL_MS, typingLabel, typingSignal } from "./typing";
import { ChatAssistant } from "./ai/ChatAssistant";
import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Sheet,
  Tooltip,
  Typography,
} from "@mui/joy";
import TagIcon from "@mui/icons-material/Tag";
import LockIcon from "@mui/icons-material/Lock";
import StarBorderIcon from "@mui/icons-material/StarBorder";
import StarIcon from "@mui/icons-material/Star";
import PushPinOutlinedIcon from "@mui/icons-material/PushPinOutlined";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import HeadsetMicOutlinedIcon from "@mui/icons-material/HeadsetMicOutlined";
import EventIcon from "@mui/icons-material/Event";
import GroupOutlinedIcon from "@mui/icons-material/GroupOutlined";
import NotificationsOffOutlinedIcon from "@mui/icons-material/NotificationsOffOutlined";
import NotificationsNoneIcon from "@mui/icons-material/NotificationsNone";
import LinkIcon from "@mui/icons-material/Link";
import { ChatComposer } from "./ChatComposer";
import { MessageItem, UserAvatar, type MessageActions } from "./Message";
import { DetailsDialog, SendLaterButton } from "./Dialogs";
import { buildActions } from "./actions";
import { applyFeedToList, channelLabel, continuesRun, dayKey, dayLabel } from "./lib";
import { teams } from "./teamsApi";
import { createMeeting } from "../meet/api";
import { meetPath, startCall } from "../../lib/calls";
import { useNavigate } from "react-router-dom";
import CallOutlinedIcon from "@mui/icons-material/CallOutlined";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import { AddTabDialog, FilesPanel, PinnedTabView, TabBar, type TabId } from "./ChannelTabs";
import { useActiveCall, useRegisterDock } from "../meet/callStore";
import { ScheduleMeetingDialog } from "./ScheduleMeeting";
import { logOutgoingCall } from "./CallsFiles";
import { meetNow } from "./meetNow";
import PersonAddAltOutlinedIcon from "@mui/icons-material/PersonAddAltOutlined";
import EventOutlinedIcon from "@mui/icons-material/EventOutlined";
import SettingsOutlinedIcon from "@mui/icons-material/SettingsOutlined";
import type { TeamsState } from "./useTeams";
import type { Bookmark, ChannelTab, TChannel, TMessage } from "./types";

interface Props {
  state: TeamsState;
  channel: TChannel;
  focusMsgId?: string;
  onOpenThread: (m: TMessage) => void;
  onLeft: () => void;
  toast: (title: string, body?: string) => void;
}

export function ChannelView({ state, channel, focusMsgId, onOpenThread, onLeft, toast }: Props) {
  const { me, meName, peopleMap, people } = state;
  const navigate = useNavigate();
  const [messages, setMessages] = useState<TMessage[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [history, setHistory] = useState<string>(""); // non-empty: viewing history (label)
  const [highlight, setHighlight] = useState("");
  const [pins, setPins] = useState<TMessage[]>([]);
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [showPins, setShowPins] = useState(false);
  const [showDetails, setShowDetails] = useState(false);
  const [draftInit, setDraftInit] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState<TabId>("posts");
  const [tabs, setTabs] = useState<ChannelTab[]>([]);
  const [addTab, setAddTab] = useState(false);
  const [scheduling, setScheduling] = useState(false);
  const call = useActiveCall();
  const chatPath = `/chat/c/${channel.id}`;
  const docked = !!call && !call.undocked && call.returnTo === chatPath;
  const dockRef = useRegisterDock(chatPath);
  const scroller = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const channelId = channel.id;
  const label = channelLabel(channel, me, peopleMap);
  const mentionNames = useMemo(() => [...people.map((p) => p.name), ...people.map((p) => p.email)].filter(Boolean), [people]);

  const scrollToBottom = useCallback(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, []);

  const loadLatest = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const r = await teams.messages(channelId);
      setMessages(r.messages ?? []);
      setHasMore(r.has_more);
      setHistory("");
      stick.current = true;
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [channelId]);

  const loadAround = useCallback(
    async (msgId: string) => {
      setLoading(true);
      try {
        const r = await teams.messages(channelId, { around: msgId });
        setMessages(r.messages ?? []);
        setHasMore(r.has_more);
        setHistory("Viewing an earlier message");
        stick.current = false;
        setHighlight(msgId);
        setTimeout(() => setHighlight(""), 3500);
      } catch {
        await loadLatest();
        toast("Message not found", "It may have been deleted.");
      } finally {
        setLoading(false);
      }
    },
    [channelId, loadLatest, toast],
  );

  const reloadSide = useCallback(() => {
    teams.pins(channelId).then(setPins).catch(() => setPins([]));
    teams.bookmarks(channelId).then(setBookmarks).catch(() => setBookmarks([]));
    teams.tabs(channelId).then(setTabs).catch(() => setTabs([]));
  }, [channelId]);

  // Load on channel / focus change.
  useEffect(() => {
    setMessages([]);
    setDraftInit(null);
    if (focusMsgId) void loadAround(focusMsgId);
    else void loadLatest();
    reloadSide();
    teams
      .drafts()
      .then((ds) => setDraftInit(ds.find((d) => d.channel_id === channelId && d.parent_id === "")?.body ?? ""))
      .catch(() => setDraftInit(""));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, focusMsgId]);

  // The read position as it was when the channel was opened, for the "New" divider.
  const readSnap = useRef({ id: "", at: "" });
  if (readSnap.current.id !== channelId) readSnap.current = { id: channelId, at: channel.last_read_at ?? "" };

  // Mark read when opened / when unread appears while visible.
  useEffect(() => {
    state.setActive(channelId);
    if (channel.is_member && (channel.unread_count > 0 || channel.mention_count > 0)) {
      void teams.markRead(channelId).then(() => state.patchChannel(channelId, { unread_count: 0, mention_count: 0 }));
    }
    return () => state.setActive("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, channel.unread_count, channel.mention_count, channel.is_member]);

  // Live updates.
  useEffect(
    () =>
      state.subscribe((e) => {
        if (e.type === "message" && e.channel_id === channelId) {
          const m = e.message;
          if (m.parent_id) {
            setMessages((cur) =>
              cur.map((x) => (x.id === m.parent_id ? { ...x, reply_count: (x.reply_count ?? 0) + 1, last_reply_at: m.sent_at } : x)),
            );
          } else {
            setMessages((cur) => (cur.some((x) => x.id === m.id) ? cur : history ? cur : [...cur, m]));
          }
        } else if (e.type === "pins" || e.type === "channel") {
          if ("channel_id" in e && e.channel_id === channelId) reloadSide();
          if (e.type === "pins") setMessages((cur) => applyFeedToList(cur, e, me));
        } else if (e.type === "edited" || e.type === "deleted" || e.type === "reaction" || e.type === "saved") {
          setMessages((cur) => applyFeedToList(cur, e, me));
        }
      }),
    [state, channelId, history, me, reloadSide],
  );

  // Stick to bottom on new content.
  useEffect(() => {
    if (stick.current && !history) scrollToBottom();
  }, [messages, history, scrollToBottom, loading]);

  // Scroll to the focused message.
  useEffect(() => {
    if (!highlight || loading) return;
    const el = scroller.current?.querySelector(`[data-msg-id="${highlight}"]`);
    el?.scrollIntoView({ block: "center" });
  }, [highlight, loading, messages]);

  const actions: MessageActions = useMemo(
    () =>
      buildActions({
        update: (id, patch) => setMessages((cur) => cur.map((m) => (m.id === id ? { ...m, ...patch } : m))),
        remove: (id) => setMessages((cur) => cur.filter((m) => m.id !== id)),
        openThread: onOpenThread,
        toast,
      }),
    [onOpenThread, toast],
  );

  const loadOlder = async () => {
    const first = messages[0];
    if (!first) return;
    const el = scroller.current;
    const prevH = el?.scrollHeight ?? 0;
    const r = await teams.messages(channelId, { before: first.id });
    stick.current = false;
    setMessages((cur) => [...(r.messages ?? []), ...cur]);
    setHasMore(r.has_more);
    requestAnimationFrame(() => {
      if (el) el.scrollTop = el.scrollHeight - prevH;
    });
  };

  const jumpToDate = async (date: string) => {
    if (!date) return;
    setLoading(true);
    try {
      const r = await teams.messages(channelId, { from: date });
      setMessages(r.messages ?? []);
      setHasMore(false);
      setHistory(`Showing messages from ${new Date(date + "T00:00:00").toLocaleDateString()}`);
      stick.current = false;
      if (scroller.current) scroller.current.scrollTop = 0;
      if ((r.messages ?? []).length === 0) toast("Nothing on or after that date");
    } finally {
      setLoading(false);
    }
  };

  const isChanForSlash = channel.kind === "public" || channel.kind === "private";
  const runSlash = async (body: string): Promise<string | null> => {
    const r = parseSlash(body, { isChannel: isChanForSlash });
    switch (r.kind) {
      case "none":
        return body;
      case "send":
        return r.body;
      case "topic":
        await teams.updateChannel(channelId, { topic: r.topic });
        toast(r.topic ? "Topic updated" : "Topic cleared");
        return null;
      case "mute":
        await teams.setPrefs(channelId, { muted: r.muted });
        state.patchChannel(channelId, { muted: r.muted });
        toast(r.muted ? "Channel muted" : "Channel unmuted");
        return null;
      case "star":
        await teams.setPrefs(channelId, { starred: r.starred });
        state.patchChannel(channelId, { starred: r.starred });
        toast(r.starred ? "Channel starred" : "Channel unstarred");
        return null;
      case "huddle":
        await startHuddle();
        return null;
      case "meet":
        await doMeetNow();
        return null;
      case "help":
        toast("Slash commands", r.lines.join("\n"));
        return null;
      case "error":
        toast(r.message);
        return null;
    }
  };

  const send = async ({ body: rawBody, attachmentIds }: { body: string; attachmentIds: string[] }) => {
    const body = await runSlash(rawBody);
    if (body === null) {
      lastDraft.current = "";
      return;
    }
    const nonce = Math.random().toString(36).slice(2);
    const m = await teams.post(channelId, body, { attachment_ids: attachmentIds, client_nonce: nonce });
    if (history) {
      await loadLatest();
    } else {
      setMessages((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, m]));
    }
    stick.current = true;
    lastDraft.current = "";
  };

  // Draft autosave (debounced).
  const draftTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const lastDraft = useRef("");
  const typer = useMemo(() => typingSignal(() => void teams.typing(channelId).catch(() => undefined)), [channelId]);
  const [typers, setTypers] = useState<Record<string, number>>({});
  useEffect(() => {
    setTypers({});
    return state.subscribe((e) => {
      if (e.type === "typing" && e.channel_id === channelId && e.user_id !== me) setTypers((cur) => ({ ...cur, [e.user_id]: Date.now() }));
      else if (e.type === "message" && e.channel_id === channelId) setTypers((cur) => { const n = { ...cur }; delete n[e.message.sender_id]; return n; });
    });
  }, [state, channelId, me]);
  useEffect(() => {
    if (!Object.keys(typers).length) return;
    const t = setTimeout(() => setTypers((cur) => Object.fromEntries(activeTypers(cur, Date.now()).map((id) => [id, cur[id]]))), TYPING_TTL_MS);
    return () => clearTimeout(t);
  }, [typers]);
  const onDraftChange = useCallback(
    (v: string) => {
      typer.typed(v);
      if (v === lastDraft.current) return;
      lastDraft.current = v;
      if (draftTimer.current) clearTimeout(draftTimer.current);
      draftTimer.current = setTimeout(() => {
        void teams.saveDraft(channelId, "", v).then(() => state.refreshCounts()).catch(() => undefined);
      }, 800);
    },
    [channelId, state, typer],
  );
  useEffect(() => {
    lastDraft.current = draftInit ?? "";
  }, [draftInit]);
  useEffect(() => () => draftTimer.current && clearTimeout(draftTimer.current), [channelId]);

  const startHuddle = async () => {
    try {
      let code = channel.huddle_code;
      if (!code) {
        const room = await createMeeting(`Huddle: ${label}`);
        code = room.code ?? "";
      }
      await teams.huddle(channelId, code);
      // Same tab, audio first; Back returns here and the call keeps running.
      navigate(meetPath(code, { returnTo: chatPath, audioOnly: true }));
    } catch (e) {
      toast("Could not start huddle", (e as Error).message);
    }
  };

  // Direct calls from a DM or group DM ring the other people, like Teams.
  const startDirectCall = async (video: boolean) => {
    try {
      const others = channel.member_ids.filter((id) => id !== me);
      const r = await startCall({ userIds: others, title: label, video, returnTo: chatPath });
      logOutgoingCall({ at: new Date().toISOString(), userIds: others, title: label, video });
      navigate(r.path);
    } catch (e) {
      toast("Could not start the call", (e as Error).message);
    }
  };

  const doMeetNow = async () => {
    try {
      navigate(await meetNow(channel, label));
    } catch (e) {
      toast("Could not start the meeting", (e as Error).message);
    }
  };

  const isChan = channel.kind === "public" || channel.kind === "private";
  const sideMembers = channel.member_ids.slice(0, 3);

  const items: React.ReactNode[] = [];
  let prev: TMessage | undefined;
  const unreadAt = firstUnreadIndex(messages, readSnap.current.at, me);
  for (const [idx, m] of messages.entries()) {
    if (idx === unreadAt && !history) {
      items.push(
        <Box key="unread-divider" data-testid="unread-divider" sx={{ display: "flex", alignItems: "center", gap: 1, px: 2, my: 1 }}>
          <Box sx={{ flex: 1, borderTop: "1px solid", borderColor: "danger.400" }} />
          <Typography level="body-xs" color="danger" sx={{ fontWeight: 700 }}>New</Typography>
        </Box>,
      );
    }
    if (!prev || dayKey(prev.sent_at) !== dayKey(m.sent_at)) {
      items.push(
        <Box key={`d-${m.id}`} sx={{ display: "flex", alignItems: "center", gap: 1, px: 2, my: 1.5 }}>
          <Box sx={{ flex: 1, borderTop: "1px solid", borderColor: "divider" }} />
          <Typography level="body-xs" sx={{ fontWeight: 700, px: 1.5, py: 0.25, border: "1px solid", borderColor: "divider", borderRadius: "xl" }}>
            {dayLabel(m.sent_at)}
          </Typography>
          <Box sx={{ flex: 1, borderTop: "1px solid", borderColor: "divider" }} />
        </Box>,
      );
    }
    items.push(
      <MessageItem
        key={m.id}
        m={m}
        me={me}
        meName={meName}
        people={peopleMap}
        mentionNames={mentionNames}
        actions={actions}
        compact={continuesRun(prev, m)}
        highlight={highlight === m.id}
      />,
    );
    prev = m;
  }

  return (
    <Box sx={{ display: "flex", flex: 1, minWidth: 0, minHeight: 0 }}>
    {docked && (
      <Box ref={dockRef} data-testid="call-dock" sx={{ flex: 1, minWidth: 0, bgcolor: "#0d1a17" }} />
    )}
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        minWidth: 0,
        minHeight: 0,
        ...(docked
          ? { width: 380, flexShrink: 0, borderLeft: "1px solid", borderColor: "divider" }
          : { flex: 1 }),
      }}
    >
      {/* Header */}
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 2, py: 1, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }}>
        {isChan ? (
          channel.kind === "private" ? <LockIcon sx={{ fontSize: 18 }} /> : <TagIcon sx={{ fontSize: 18 }} />
        ) : null}
        <Typography level="title-md" sx={{ fontWeight: 700 }} data-testid="channel-title">
          {label}
        </Typography>
        {channel.is_member && (
          <Tooltip title={channel.starred ? "Unstar" : "Star"}>
            <IconButton
              size="sm"
              variant="plain"
              aria-label={channel.starred ? "Unstar channel" : "Star channel"}
              onClick={async () => {
                await teams.setPrefs(channelId, { starred: !channel.starred });
                state.patchChannel(channelId, { starred: !channel.starred });
              }}
            >
              {channel.starred ? <StarIcon sx={{ fontSize: 18, color: "warning.500" }} /> : <StarBorderIcon sx={{ fontSize: 18 }} />}
            </IconButton>
          </Tooltip>
        )}
        {channel.topic && (
          <Typography level="body-sm" sx={{ opacity: 0.7, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", maxWidth: 360 }}>
            {channel.topic}
          </Typography>
        )}
        <Box sx={{ flex: 1 }} />
        <ChatAssistant channelId={channelId} channelName={label} channels={state.channels} messages={messages} reload={() => void loadLatest()} />
        <Button size="sm" variant="plain" color="neutral" startDecorator={<GroupOutlinedIcon />} onClick={() => setShowDetails(true)} aria-label="Members and details">
          <Box sx={{ display: "flex", mr: 0.5 }}>
            {sideMembers.map((id, i) => (
              <Box key={id} sx={{ ml: i ? -0.75 : 0 }}>
                <UserAvatar id={id} name={peopleMap[id]?.name ?? "?"} size={20} />
              </Box>
            ))}
          </Box>
          {channel.member_ids.length}
        </Button>
        {!isChan && channel.is_member && channel.member_ids.length > 1 && (
          <>
            <Tooltip title="Voice call">
              <IconButton size="sm" variant="soft" color="primary" onClick={() => void startDirectCall(false)} aria-label="Voice call" data-testid="dm-voice-call">
                <CallOutlinedIcon />
              </IconButton>
            </Tooltip>
            <Tooltip title="Video call">
              <IconButton size="sm" variant="soft" color="primary" onClick={() => void startDirectCall(true)} aria-label="Video call" data-testid="dm-video-call">
                <VideocamOutlinedIcon />
              </IconButton>
            </Tooltip>
          </>
        )}
        {isChan && channel.is_member && (
          <Tooltip title="Start a huddle (audio call in Meet)">
            <Button size="sm" variant="soft" color="primary" startDecorator={<HeadsetMicOutlinedIcon />} onClick={() => void startHuddle()} aria-label="Start huddle">
              Huddle
            </Button>
          </Tooltip>
        )}
        {channel.is_member && (
          <>
            <Tooltip title="Meet now: start a meeting here">
              <Button size="sm" variant="soft" color="primary" startDecorator={<VideocamOutlinedIcon />} onClick={() => void doMeetNow()} data-testid="header-meet-now">
                Meet now
              </Button>
            </Tooltip>
            <Tooltip title="Schedule a meeting">
              <IconButton size="sm" variant="plain" onClick={() => setScheduling(true)} aria-label="Schedule meeting" data-testid="header-schedule">
                <EventOutlinedIcon />
              </IconButton>
            </Tooltip>
            {channel.kind !== "dm" && (
              <Tooltip title="Add people">
                <IconButton size="sm" variant="plain" onClick={() => setShowDetails(true)} aria-label="Add people" data-testid="header-add-people">
                  <PersonAddAltOutlinedIcon />
                </IconButton>
              </Tooltip>
            )}
          </>
        )}
        <Tooltip title="Pinned messages">
          <Button size="sm" variant="plain" color="neutral" startDecorator={<PushPinOutlinedIcon />} onClick={() => setShowPins(true)} aria-label="Pinned messages">
            {pins.length}
          </Button>
        </Tooltip>
        {channel.is_member && (
          <Tooltip title={channel.muted ? "Unmute" : "Mute notifications"}>
            <IconButton
              size="sm"
              variant="plain"
              aria-label={channel.muted ? "Unmute channel" : "Mute channel"}
              onClick={async () => {
                await teams.setPrefs(channelId, { muted: !channel.muted });
                state.patchChannel(channelId, { muted: !channel.muted });
              }}
            >
              {channel.muted ? <NotificationsOffOutlinedIcon sx={{ fontSize: 18 }} /> : <NotificationsNoneIcon sx={{ fontSize: 18 }} />}
            </IconButton>
          </Tooltip>
        )}
        <Tooltip title="Jump to date">
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <EventIcon sx={{ fontSize: 18, opacity: 0.6, mr: 0.5 }} />
            <Input size="sm" type="date" onChange={(e) => void jumpToDate(e.target.value)} slotProps={{ input: { "aria-label": "Jump to date" } }} sx={{ width: 140 }} />
          </Box>
        </Tooltip>
        <Tooltip title={isChan ? "Channel settings" : "Details"}>
          <IconButton size="sm" variant="plain" onClick={() => setShowDetails(true)} aria-label="Channel details" data-testid="header-settings">
            {isChan ? <SettingsOutlinedIcon sx={{ fontSize: 18 }} /> : <InfoOutlinedIcon sx={{ fontSize: 18 }} />}
          </IconButton>
        </Tooltip>
      </Box>

      <TabBar
        tab={tab}
        setTab={setTab}
        tabs={tabs}
        canEdit={channel.is_member && !channel.archived}
        onAdd={() => setAddTab(true)}
        onRemove={(t) => {
          void teams.deleteTab(channelId, t.id).then(() => {
            setTabs((cur) => cur.filter((x) => x.id !== t.id));
            if (tab === t.id) setTab("posts");
          });
        }}
      />
      {tab === "files" && <FilesPanel channelId={channelId} messages={messages} />}
      {tab !== "posts" && tab !== "files" && tabs.find((t) => t.id === tab) && <PinnedTabView tab={tabs.find((t) => t.id === tab)!} />}
      {tab === "posts" && (
      <>
      {/* Bookmarks bar */}
      {bookmarks.length > 0 && (
        <Box sx={{ display: "flex", gap: 1, px: 2, py: 0.5, borderBottom: "1px solid", borderColor: "divider", overflowX: "auto" }}>
          {bookmarks.map((b) => (
            <Button
              key={b.id}
              size="sm"
              variant="outlined"
              color="neutral"
              component="a"
              href={b.url}
              target={b.url.startsWith("/") ? undefined : "_blank"}
              rel="noopener noreferrer"
              startDecorator={<LinkIcon />}
              endDecorator={
                <Box
                  component="span"
                  role="button"
                  aria-label={`Remove bookmark ${b.title}`}
                  onClick={(ev: React.MouseEvent) => {
                    ev.preventDefault();
                    void teams.deleteBookmark(channelId, b.id).then(reloadSide);
                  }}
                  sx={{ opacity: 0.5, "&:hover": { opacity: 1 } }}
                >
                  ✕
                </Box>
              }
              sx={{ whiteSpace: "nowrap" }}
            >
              {b.title}
            </Button>
          ))}
        </Box>
      )}

      {history && (
        <Alert size="sm" variant="soft" color="warning" sx={{ borderRadius: 0 }} endDecorator={<Button size="sm" variant="plain" onClick={() => void loadLatest()}>Jump to present</Button>}>
          {history}
        </Alert>
      )}

      {/* Messages */}
      <Box
        ref={scroller}
        data-testid="message-list"
        onScroll={(e) => {
          const el = e.currentTarget;
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
        sx={{ flex: 1, overflowY: "auto", py: 1 }}
      >
        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
            <CircularProgress size="sm" />
          </Box>
        )}
        {error && (
          <Alert color="danger" sx={{ m: 2 }}>
            {error}
          </Alert>
        )}
        {!loading && hasMore && (
          <Box sx={{ textAlign: "center", py: 1 }}>
            <Button size="sm" variant="plain" onClick={() => void loadOlder()}>
              Load earlier messages
            </Button>
          </Box>
        )}
        {!loading && messages.length === 0 && !error && (
          <Box sx={{ p: 4, textAlign: "center", opacity: 0.7 }}>
            <Typography level="title-md">{isChan ? `Welcome to ${label}` : `This is the start of your conversation`}</Typography>
            <Typography level="body-sm">{channel.description || "Send the first message below."}</Typography>
          </Box>
        )}
        {items.map((n, i) => (
          <Fragment key={i}>{n}</Fragment>
        ))}
      </Box>

      {/* Composer */}
      <Box sx={{ px: 2, pb: 1.5, pt: 0.5 }}>
        <Typography level="body-xs" sx={{ minHeight: 16, opacity: 0.7 }} aria-live="polite" data-testid="typing-line">
          {typingLabel(activeTypers(typers, Date.now()).map((id) => peopleMap[id]?.name ?? ""))}
        </Typography>
        {channel.archived ? (
          <Alert size="sm" variant="soft">This channel is archived and read-only.</Alert>
        ) : !channel.is_member ? (
          <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", display: "flex", alignItems: "center", gap: 1.5 }}>
            <Typography level="body-sm" sx={{ flex: 1 }}>
              You are previewing <b>#{channel.name}</b>. Join to post.
            </Typography>
            <Button
              size="sm"
              onClick={async () => {
                await teams.join(channelId);
                await state.reload();
              }}
            >
              Join channel
            </Button>
          </Sheet>
        ) : channel.announce && channel.created_by && channel.created_by !== me ? (
          <Alert size="sm" variant="soft" data-testid="announce-readonly">This is an announcement channel. Only its owner can post here; reply in a thread.</Alert>
        ) : draftInit === null ? null : (
          <ChatComposer
            key={channelId}
            initialValue={draftInit}
            onChange={onDraftChange}
            placeholder={isChan ? `Message ${label}` : `Message ${label}`}
            onSend={send}
            extraActions={({ value, clear }) => (
              <SendLaterButton
                value={value}
                onSchedule={async (at) => {
                  try {
                    await teams.schedule(channelId, value.trim(), at.toISOString());
                    clear();
                    lastDraft.current = "";
                    void state.refreshCounts();
                    toast("Message scheduled", `Will be sent ${at.toLocaleString()}`);
                  } catch (e) {
                    toast("Could not schedule", (e as Error).message);
                  }
                }}
              />
            )}
          />
        )}
      </Box>

      </>
      )}

      {scheduling && <ScheduleMeetingDialog state={state} channel={channel} onClose={() => setScheduling(false)} onDone={(m) => toast(m)} />}

      {addTab && (
        <AddTabDialog
          channelId={channelId}
          onClose={() => setAddTab(false)}
          onAdded={(t) => {
            setTabs((cur) => (cur.some((x) => x.id === t.id) ? cur : [...cur, t]));
            setTab(t.id);
            setAddTab(false);
          }}
        />
      )}

      {showDetails && (
        <DetailsDialog
          open
          onClose={() => setShowDetails(false)}
          channel={channel}
          me={me}
          people={peopleMap}
          peopleList={people}
          presence={state.presence}
          onChanged={() => {
            void state.reload();
            reloadSide();
          }}
          onLeft={onLeft}
        />
      )}

      <Modal open={showPins} onClose={() => setShowPins(false)}>
        <ModalDialog sx={{ width: 560, maxWidth: "95vw", maxHeight: "85vh", overflow: "auto" }}>
          <ModalClose />
          <Typography level="title-lg">Pinned in {label}</Typography>
          {pins.length === 0 && <Typography level="body-sm">Nothing pinned yet. Use the ⋮ menu on a message to pin it.</Typography>}
          {pins.map((m) => (
            <MessageItem
              key={m.id}
              m={m}
              me={me}
              meName={meName}
              people={peopleMap}
              mentionNames={mentionNames}
              inThread
              actions={{
                ...actions,
                pin: async (x) => {
                  await actions.pin(x);
                  reloadSide();
                },
              }}
            />
          ))}
        </ModalDialog>
      </Modal>
    </Box>
    </Box>
  );
}
