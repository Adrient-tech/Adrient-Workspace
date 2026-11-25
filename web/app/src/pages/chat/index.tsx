/**
 * Chat (Teams/Slack-class): channels + DMs sidebar with sections, threads,
 * Activity, Later, Drafts & sent, search, people, huddles. Built in React/Joy;
 * the backend is internal/chat (REST under /api/v1/chat/x plus the live feed).
 * One Adrient org == one workspace, so there is no workspace switcher.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Alert, Box, Button, CircularProgress, Drawer, IconButton, Snackbar, Typography } from "@mui/joy";
import MenuIcon from "@mui/icons-material/Menu";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { Sidebar } from "./Sidebar";
import { ChannelView } from "./ChannelView";
import { ThreadPanel } from "./ThreadPanel";
import { ActivityView, BrowseView, DraftsView, LaterView, PeopleView, SearchView } from "./Views";
import { MeetingsView } from "./Meetings";
import { CallsView, FilesView } from "./CallsFiles";
import { Rail, type Section } from "./Rail";
import { ScheduleMeetingDialog } from "./ScheduleMeeting";
import { meetNow } from "./meetNow";
import { channelLabel } from "./lib";
import { useNavigate } from "react-router-dom";
import { NewChannelDialog, ProfileDialog } from "./Dialogs";
import { parsePath, viewPath } from "./lib";
import { teams } from "./teamsApi";
import { useTeams } from "./useTeams";
import type { Person, TChannel, View } from "./types";

interface ChatAppProps {
  user: User;
}

export default function ChatApp({ user }: ChatAppProps) {
  const state = useTeams();
  const [view, setView] = useState<View | null>(() => parsePath(window.location.pathname, window.location.search));
  const [thread, setThread] = useState<{ channelId: string; rootId: string } | null>(null);
  const [newDlg, setNewDlg] = useState<{ tab: number } | null>(null);
  const [profile, setProfile] = useState<Person | null>(null);
  const [drawer, setDrawer] = useState(false);
  const [section, setSection] = useState<Section>("chat");
  const [schedule, setSchedule] = useState<{ channel?: TChannel } | null>(null);
  const navigate = useNavigate();
  const [extra, setExtra] = useState<TChannel | null>(null);
  const [notifAsk, setNotifAsk] = useState(
    () => "Notification" in window && Notification.permission === "default" && localStorage.getItem("chat.notif.dismissed") !== "1",
  );
  const [snack, setSnack] = useState<{ title: string; body?: string } | null>(null);
  const toast = useCallback((title: string, body?: string) => setSnack({ title, body }), []);

  const go = useCallback((v: View) => {
    setView(v);
    setThread(null);
    setDrawer(false);
    window.history.pushState(null, "", viewPath(v));
  }, []);

  useEffect(() => {
    const onPop = () => {
      setView(parsePath(window.location.pathname, window.location.search));
      setThread(null);
    };
    const onOpen = (e: Event) => {
      const d = (e as CustomEvent<{ channelId?: string; msgId?: string }>).detail;
      if (d?.channelId) go({ kind: "channel", id: d.channelId, msgId: d.msgId });
    };
    window.addEventListener("popstate", onPop);
    window.addEventListener("chat:open", onOpen);
    return () => {
      window.removeEventListener("popstate", onPop);
      window.removeEventListener("chat:open", onOpen);
    };
  }, [go]);

  // Default landing: #general, else the first channel, else Activity.
  useEffect(() => {
    if (view || state.loading) return;
    const mine = state.channels.filter((c) => c.is_member && !c.archived);
    const pick = mine.find((c) => c.name === "general") ?? mine.find((c) => c.kind === "public") ?? mine[0];
    const v: View = pick ? { kind: "channel", id: pick.id } : { kind: "activity" };
    setView(v);
    window.history.replaceState(null, "", viewPath(v));
  }, [view, state.loading, state.channels]);

  const channelId = view?.kind === "channel" ? view.id : "";
  const known = useMemo(() => state.channels.find((c) => c.id === channelId), [state.channels, channelId]);
  useEffect(() => {
    setExtra(null);
    if (!channelId || known) return;
    if (state.loading) return;
    teams.getChannel(channelId).then(setExtra).catch(() => setExtra(null));
  }, [channelId, known, state.loading]);
  const channel = known ?? (extra && extra.id === channelId ? extra : undefined);

  const openDM = useCallback(
    async (id: string) => {
      try {
        const ch = await teams.createChannel({ kind: "dm", member_ids: id === state.me ? [] : [id] });
        await state.reload();
        go({ kind: "channel", id: ch.id });
      } catch (e) {
        toast("Could not open DM", (e as Error).message);
      }
    },
    [state, go, toast],
  );

  // PersonCard "Chat": /chat?dm=<userId> on arrival, or an event when Chat is open.
  const dmRef = useRef(openDM);
  useEffect(() => {
    dmRef.current = openDM;
  });
  useEffect(() => {
    const dm = new URLSearchParams(window.location.search).get("dm");
    if (dm && !state.loading) {
      window.history.replaceState(null, "", window.location.pathname);
      void dmRef.current(dm);
    }
  }, [state.loading]);
  useEffect(() => {
    const onAct = (e: Event) => {
      const d = (e as CustomEvent<{ action?: string; id?: string }>).detail;
      if (d?.action === "chat" && d.id) void dmRef.current(d.id);
    };
    window.addEventListener("adrient:person-action", onAct);
    return () => window.removeEventListener("adrient:person-action", onAct);
  }, []);

  const closeThread = useCallback(() => setThread(null), []);
  const openThread = useCallback(
    (m: { id: string; channel_id: string; parent_id?: string }) => setThread({ channelId: m.channel_id, rootId: m.parent_id || m.id }),
    [],
  );

  // Follow the open conversation: channels live under Teams, chats under Chat.
  useEffect(() => {
    if (channel) setSection(channel.kind === "public" || channel.kind === "private" ? "teams" : "chat");
  }, [channel?.id, channel?.kind]); // eslint-disable-line react-hooks/exhaustive-deps

  const doMeetNow = useCallback(async () => {
    try {
      const path = await meetNow(channel?.is_member ? channel : undefined, channel ? channelLabel(channel, state.me, state.peopleMap) : "");
      navigate(path);
    } catch (e) {
      toast("Could not start the meeting", (e as Error).message);
    }
  }, [channel, state.me, state.peopleMap, navigate, toast]);

  const railCounts = useMemo(() => {
    const live = state.channels.filter((c) => c.is_member && !c.archived && !c.muted);
    const isCh = (c: TChannel) => c.kind === "public" || c.kind === "private";
    const sum = (l: TChannel[]) => l.reduce((n, c) => n + c.unread_count, 0);
    return { activity: state.counts.activity, chat: sum(live.filter((c) => !isCh(c))), teams: sum(live.filter(isCh)) };
  }, [state.channels, state.counts.activity]);

  const sidebarCore = (
    <Sidebar
      section={section}
      onSchedule={() => setSchedule({ channel: channel?.is_member ? channel : undefined })}
      onMeetNow={() => void doMeetNow()}
      state={state}
      view={view ?? { kind: "activity" }}
      go={go}
      onNewChannel={(tab) => setNewDlg({ tab })}
      onProfile={() => setProfile(state.people.find((p) => p.id === state.me) ?? null)}
      onQuery={(q) => go({ kind: "search", q })}
    />
  );

  const sidebar = (
    <Box sx={{ display: "flex", height: "100%" }}>
      <Rail
        view={view ?? { kind: "activity" }}
        section={section}
        setSection={(s) => {
          setSection(s);
          // Jump to the most recent conversation of that kind.
          const isCh = (c: TChannel) => c.kind === "public" || c.kind === "private";
          const mine = state.channels.filter((c) => c.is_member && !c.archived && (s === "teams" ? isCh(c) : !isCh(c)));
          const cur = channel && (s === "teams") === isCh(channel) ? channel : undefined;
          const pick = cur ?? mine.sort((a, b) => (b.last_message_at ?? "").localeCompare(a.last_message_at ?? ""))[0];
          if (pick) go({ kind: "channel", id: pick.id });
          else if (s === "teams") go({ kind: "browse" });
        }}
        go={go}
        counts={railCounts}
      />
      {sidebarCore}
    </Box>
  );

  const selfPerson = state.people.find((p) => p.id === state.me) ?? null;

  let main: React.ReactNode = (
    <Box sx={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
      <CircularProgress />
    </Box>
  );
  if (view && !state.loading) {
    switch (view.kind) {
      case "channel":
        main = channel ? (
          <ChannelView
            key={channel.id}
            state={state}
            channel={channel}
            focusMsgId={view.msgId}
            onOpenThread={openThread}
            onLeft={() => {
              void state.reload();
              go({ kind: "browse" });
            }}
            toast={toast}
          />
        ) : (
          <Typography sx={{ p: 4 }}>This channel doesn't exist or you don't have access to it.</Typography>
        );
        break;
      case "activity":
        main = <ActivityView state={state} go={go} toast={toast} />;
        break;
      case "later":
        main = <LaterView state={state} go={go} toast={toast} />;
        break;
      case "drafts":
        main = <DraftsView state={state} go={go} toast={toast} />;
        break;
      case "people":
        main = <PeopleView state={state} onOpenProfile={setProfile} onDM={(id) => void openDM(id)} />;
        break;
      case "meetings":
        main = <MeetingsView onSchedule={() => setSchedule({})} onMeetNow={() => void doMeetNow()} />;
        break;
      case "calls":
        main = <CallsView state={state} toast={toast} />;
        break;
      case "files":
        main = <FilesView />;
        break;
      case "browse":
        main = <BrowseView state={state} go={go} onNew={() => setNewDlg({ tab: 0 })} />;
        break;
      case "search":
        main = <SearchView state={state} go={go} toast={toast} q={view.q} />;
        break;
    }
  }

  return (
    <>
      <Header user={user} />
      <Box sx={{ display: "flex", height: "calc(100vh - 56px)", minWidth: 0 }} data-testid="chat-app">
        <Box sx={{ display: { xs: "none", md: "flex" } }}>{sidebar}</Box>
        {drawer && (
          <Drawer open onClose={() => setDrawer(false)} size="sm" sx={{ display: { xs: "block", md: "none" } }}>
            {sidebar}
          </Drawer>
        )}

        <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>
          <Box sx={{ display: { xs: "flex", md: "none" }, alignItems: "center", p: 0.5, borderBottom: "1px solid", borderColor: "divider" }}>
            <IconButton onClick={() => setDrawer(true)} aria-label="Open sidebar">
              <MenuIcon />
            </IconButton>
          </Box>
          {notifAsk && (
            <Alert
              size="sm"
              variant="soft"
              color="primary"
              sx={{ borderRadius: 0 }}
              endDecorator={
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button
                    size="sm"
                    onClick={async () => {
                      await Notification.requestPermission();
                      setNotifAsk(false);
                    }}
                  >
                    Enable
                  </Button>
                  <Button
                    size="sm"
                    variant="plain"
                    onClick={() => {
                      localStorage.setItem("chat.notif.dismissed", "1");
                      setNotifAsk(false);
                    }}
                  >
                    Not now
                  </Button>
                </Box>
              }
            >
              Get desktop notifications for mentions and DMs when this tab is in the background.
            </Alert>
          )}
          <Box sx={{ flex: 1, display: "flex", minHeight: 0, minWidth: 0 }}>
            <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>{main}</Box>
            {thread && channel && thread.channelId === channel.id && view?.kind === "channel" && (
              <ThreadPanel key={thread.rootId} state={state} channel={channel} rootId={thread.rootId} onClose={closeThread} toast={toast} />
            )}
          </Box>
        </Box>
      </Box>

      {newDlg && (
        <NewChannelDialog
          open
          initialTab={newDlg.tab}
          onClose={() => setNewDlg(null)}
          people={state.people}
          me={state.me}
          onCreated={async (ch) => {
            await state.reload();
            go({ kind: "channel", id: ch.id });
          }}
        />
      )}
      {profile && (
        <ProfileDialog
          open
          person={profile}
          editable={profile.id === state.me}
          presence={state.presence(profile.id)}
          onClose={() => setProfile(null)}
          onDM={(id) => void openDM(id)}
          onSaved={() => void state.reloadPeople()}
        />
      )}
      {selfPerson ? null : null}
      {schedule && (
        <ScheduleMeetingDialog
          state={state}
          channel={schedule.channel}
          onClose={() => setSchedule(null)}
          onDone={(m) => toast(m)}
        />
      )}

      {/* In-app notifications (mentions, DMs, reminders) */}
      {state.toasts.map((t, i) => (
        <Snackbar
          key={t.id}
          open
          variant="solid"
          color="primary"
          anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
          sx={{ bottom: 16 + i * 84, cursor: "pointer", maxWidth: 380 }}
          onClick={() => {
            if (t.channelId) go({ kind: "channel", id: t.channelId, msgId: t.msgId });
            state.dismissToast(t.id);
          }}
          data-testid="chat-toast"
        >
          <Box>
            <Typography level="title-sm" sx={{ color: "inherit" }}>
              {t.title}
            </Typography>
            <Typography level="body-sm" sx={{ color: "inherit", opacity: 0.9 }}>
              {t.body}
            </Typography>
          </Box>
        </Snackbar>
      ))}
      <Snackbar
        open={!!snack}
        autoHideDuration={4000}
        onClose={() => setSnack(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
        variant="soft"
      >
        <Box>
          <Typography level="title-sm">{snack?.title}</Typography>
          {snack?.body && <Typography level="body-xs">{snack.body}</Typography>}
        </Box>
      </Snackbar>
    </>
  );
}
