import { searchDirectory } from "../../api/directory";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  Input,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Typography,
  Card,
} from "@mui/joy";
import SearchIcon from "@mui/icons-material/Search";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import { MessageItem, UserAvatar } from "./Message";
import { buildActions } from "./actions";
import { applyFeedToList, channelLabel, fmtShort } from "./lib";
import { teams } from "./teamsApi";
import type { TeamsState } from "./useTeams";
import type { ActivityItem, Draft, Person, Reminder, Scheduled, TChannel, TMessage, View } from "./types";

interface BaseProps {
  state: TeamsState;
  go: (v: View) => void;
  toast: (title: string, body?: string) => void;
}

function Title({ children, sub }: { children: React.ReactNode; sub?: string }) {
  return (
    <Box sx={{ px: 3, py: 1.5, borderBottom: "1px solid", borderColor: "divider" }}>
      <Typography level="title-lg" sx={{ fontWeight: 800 }}>
        {children}
      </Typography>
      {sub && (
        <Typography level="body-sm" sx={{ opacity: 0.7 }}>
          {sub}
        </Typography>
      )}
    </Box>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <Typography level="body-md" sx={{ p: 4, textAlign: "center", opacity: 0.65 }}>
      {children}
    </Typography>
  );
}

function useMsgList(state: TeamsState, initial: TMessage[]) {
  const [list, setList] = useState<TMessage[]>(initial);
  useEffect(() => setList(initial), [initial]);
  useEffect(
    () =>
      state.subscribe((e) => {
        if (e.type === "edited" || e.type === "deleted" || e.type === "reaction" || e.type === "pins" || e.type === "saved") {
          setList((cur) => applyFeedToList(cur, e, state.me));
        }
      }),
    [state],
  );
  return [list, setList] as const;
}

const chanOf = (chs: TChannel[], id: string) => chs.find((c) => c.id === id);

// ---- Activity ---------------------------------------------------------------

export function ActivityView({ state, go, toast }: BaseProps) {
  const [items, setItems] = useState<ActivityItem[] | null>(null);
  const load = useCallback(() => {
    teams.activity().then(setItems).catch(() => setItems([]));
  }, []);
  useEffect(() => {
    load();
    return state.subscribe((e) => {
      if (e.type === "message" || e.type === "reminder") load();
    });
  }, [load, state]);
  // Mark as seen once the list has been shown (keep the unread highlight for this visit).
  useEffect(() => {
    if (items && items.some((i) => i.unread)) {
      const t = setTimeout(() => void teams.activitySeen().then(() => state.refreshCounts()), 1500);
      return () => clearTimeout(t);
    }
  }, [items, state]);
  const mentionNames = useMemo(() => state.people.map((p) => p.name), [state.people]);
  const [msgs, setMsgs] = useMsgList(state, useMemo(() => (items ?? []).map((i) => i.message), [items]));
  const actions = useMemo(
    () => buildActions({ update: (id, p) => setMsgs((c) => c.map((m) => (m.id === id ? { ...m, ...p } : m))), remove: (id) => setMsgs((c) => c.filter((m) => m.id !== id)), toast }),
    [setMsgs, toast],
  );
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="activity-view">
      <Title sub="Mentions of you, replies in threads you are part of, and reminders">Activity</Title>
      <Box sx={{ flex: 1, overflowY: "auto", py: 1 }}>
        {items === null && <CircularProgress size="sm" sx={{ m: 3 }} />}
        {items?.length === 0 && <Empty>Nothing yet. When someone @mentions you or replies in your threads it shows up here.</Empty>}
        {items?.map((it, idx) => {
          const m = msgs.find((x) => x.id === it.message.id) ?? it.message;
          const ch = chanOf(state.channels, m.channel_id);
          const where = ch ? channelLabel(ch, state.me, state.peopleMap) : "";
          const verb = it.kind === "mention" ? "mentioned you in" : it.kind === "thread_reply" ? "replied in a thread in" : `Reminder${it.note ? `: ${it.note}` : ""} ·`;
          if (it.kind === "reminder" && !it.message.id) {
            return (
              <Card key={`r${idx}`} size="sm" sx={{ mx: 2, my: 0.5 }}>
                <Typography level="body-sm">⏰ Reminder: {it.note || "your reminder"}</Typography>
              </Card>
            );
          }
          return (
            <Box key={`${it.kind}-${m.id}-${idx}`} sx={{ borderLeft: "3px solid", borderColor: it.unread ? "primary.500" : "transparent" }}>
              <MessageItem
                m={m}
                me={state.me}
                meName={state.meName}
                people={state.peopleMap}
                mentionNames={mentionNames}
                actions={{ ...actions, openThread: undefined }}
                inThread
                context={`${verb} ${where}`}
                onContextClick={() => go({ kind: "channel", id: m.channel_id, msgId: m.parent_id || m.id })}
              />
              <Box sx={{ px: 2, pl: 7.5 }}>
                <Button size="sm" variant="plain" onClick={() => go({ kind: "channel", id: m.channel_id, msgId: m.id })}>
                  Open in channel
                </Button>
              </Box>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
}

// ---- generic message list (Later, Search) -----------------------------------

function MessageResults({ state, go, toast, messages, empty, removeOnUnsave }: BaseProps & { messages: TMessage[]; empty: string; removeOnUnsave?: boolean }) {
  const [list, setList] = useMsgList(state, messages);
  const mentionNames = useMemo(() => state.people.map((p) => p.name), [state.people]);
  const actions = useMemo(
    () =>
      buildActions({
        update: (id, p) =>
          setList((c) => (removeOnUnsave && p.saved === false ? c.filter((m) => m.id !== id) : c.map((m) => (m.id === id ? { ...m, ...p } : m)))),
        remove: (id) => setList((c) => c.filter((m) => m.id !== id)),
        toast,
      }),
    [setList, toast, removeOnUnsave],
  );
  if (list.length === 0) return <Empty>{empty}</Empty>;
  return (
    <>
      {list.map((m) => {
        const ch = chanOf(state.channels, m.channel_id);
        return (
          <Box key={m.id} sx={{ pb: 0.5 }}>
            <MessageItem
              m={m}
              me={state.me}
              meName={state.meName}
              people={state.peopleMap}
              mentionNames={mentionNames}
              actions={{ ...actions, openThread: undefined }}
              inThread
              context={ch ? channelLabel(ch, state.me, state.peopleMap) : "Channel"}
              onContextClick={() => go({ kind: "channel", id: m.channel_id, msgId: m.parent_id || m.id })}
            />
            <Box sx={{ pl: 7.5 }}>
              <Button size="sm" variant="plain" onClick={() => go({ kind: "channel", id: m.channel_id, msgId: m.id })}>
                Jump to message
              </Button>
            </Box>
          </Box>
        );
      })}
    </>
  );
}

export function LaterView(props: BaseProps) {
  const [msgs, setMsgs] = useState<TMessage[] | null>(null);
  const [rems, setRems] = useState<Reminder[]>([]);
  useEffect(() => {
    teams.saved().then(setMsgs).catch(() => setMsgs([]));
    teams.reminders().then((r) => setRems(r.filter((x) => !x.fired))).catch(() => undefined);
  }, []);
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="later-view">
      <Title sub="Messages you saved to come back to">Later</Title>
      <Box sx={{ flex: 1, overflowY: "auto", py: 1 }}>
        {rems.length > 0 && (
          <Box sx={{ px: 2, pb: 1 }}>
            <Typography level="title-sm">Upcoming reminders</Typography>
            {rems.map((r) => (
              <Box key={r.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.5 }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  ⏰ {new Date(r.remind_at).toLocaleString()} {r.note && `· ${r.note}`}
                </Typography>
                {r.channel_id && r.message_id && (
                  <Button size="sm" variant="plain" onClick={() => props.go({ kind: "channel", id: r.channel_id, msgId: r.message_id })}>
                    Open
                  </Button>
                )}
                <IconButton size="sm" variant="plain" aria-label="Cancel reminder" onClick={() => void teams.deleteReminder(r.id).then(() => setRems((c) => c.filter((x) => x.id !== r.id)))}>
                  <DeleteOutlineIcon sx={{ fontSize: 18 }} />
                </IconButton>
              </Box>
            ))}
          </Box>
        )}
        {msgs === null ? <CircularProgress size="sm" sx={{ m: 3 }} /> : <MessageResults {...props} messages={msgs} removeOnUnsave empty="No saved messages. Use the bookmark icon on a message to save it for later." />}
      </Box>
    </Box>
  );
}

// ---- Drafts & sent ----------------------------------------------------------

export function DraftsView({ state, go, toast }: BaseProps) {
  const [drafts, setDrafts] = useState<Draft[]>([]);
  const [sched, setSched] = useState<Scheduled[]>([]);
  const [sent, setSent] = useState<TMessage[] | null>(null);
  const reload = useCallback(() => {
    teams.drafts().then(setDrafts).catch(() => undefined);
    teams.scheduled().then(setSched).catch(() => undefined);
    teams.sent().then(setSent).catch(() => setSent([]));
  }, []);
  useEffect(() => {
    reload();
    return state.subscribe((e) => {
      if (e.type === "scheduled" || (e.type === "message" && e.message.sender_id === state.me)) reload();
    });
  }, [reload, state]);
  const label = (id: string) => {
    const c = chanOf(state.channels, id);
    return c ? channelLabel(c, state.me, state.peopleMap) : "Channel";
  };
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="drafts-view">
      <Title sub="Unsent drafts, scheduled messages and what you sent recently">Drafts &amp; sent</Title>
      <Tabs defaultValue={0} sx={{ flex: 1, minHeight: 0, bgcolor: "transparent" }}>
        <TabList sx={{ px: 2 }}>
          <Tab>Drafts {drafts.length > 0 && <Chip size="sm" sx={{ ml: 0.5 }}>{drafts.length}</Chip>}</Tab>
          <Tab>Scheduled {sched.length > 0 && <Chip size="sm" sx={{ ml: 0.5 }}>{sched.length}</Chip>}</Tab>
          <Tab>Sent</Tab>
        </TabList>
        <TabPanel value={0} sx={{ overflowY: "auto" }}>
          {drafts.length === 0 && <Empty>No drafts. Anything you start typing is saved here automatically.</Empty>}
          {drafts.map((d) => (
            <Card key={d.id} size="sm" sx={{ mb: 1 }}>
              <Typography level="title-sm">To: {label(d.channel_id)}</Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{d.body}</Typography>
              <Box sx={{ display: "flex", gap: 1 }}>
                <Button size="sm" onClick={() => go({ kind: "channel", id: d.channel_id })}>
                  Continue
                </Button>
                <Button size="sm" variant="plain" color="danger" onClick={() => void teams.deleteDraft(d.id).then(() => { reload(); void state.refreshCounts(); })}>
                  Discard
                </Button>
              </Box>
            </Card>
          ))}
        </TabPanel>
        <TabPanel value={1} sx={{ overflowY: "auto" }}>
          {sched.length === 0 && <Empty>Nothing scheduled. Use the clock icon in the composer to send later.</Empty>}
          {sched.map((s) => (
            <Card key={s.id} size="sm" sx={{ mb: 1 }}>
              <Typography level="title-sm">
                To: {label(s.channel_id)} · {new Date(s.send_at).toLocaleString()}
              </Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{s.body}</Typography>
              <Box>
                <Button
                  size="sm"
                  variant="soft"
                  color="danger"
                  onClick={() =>
                    void teams.cancelScheduled(s.id).then(() => {
                      toast("Scheduled message cancelled");
                      reload();
                      void state.refreshCounts();
                    })
                  }
                >
                  Cancel send
                </Button>
              </Box>
            </Card>
          ))}
        </TabPanel>
        <TabPanel value={2} sx={{ overflowY: "auto", px: 0 }}>
          {sent === null ? <CircularProgress size="sm" /> : <MessageResults state={state} go={go} toast={toast} messages={sent} empty="You haven't sent anything yet." />}
        </TabPanel>
      </Tabs>
    </Box>
  );
}

// ---- People -----------------------------------------------------------------

export function PeopleView({ state, onOpenProfile, onDM }: { state: TeamsState; onOpenProfile: (p: Person) => void; onDM: (id: string) => void }) {
  const [q, setQ] = useState("");
  // The roster holds the first 200 people; a search goes to the server so
  // everyone in a large organisation can be found.
  const [found, setFound] = useState<Person[] | null>(null);
  useEffect(() => {
    const t = q.trim();
    if (!t) {
      setFound(null);
      return;
    }
    let live = true;
    const h = setTimeout(() => {
      searchDirectory(t)
        .then((ms) => live && setFound(ms.map((m) => state.people.find((p) => p.id === m.id) ?? ({ id: m.id, name: m.name, email: m.email, title: "", status_text: "", status_emoji: "" } as Person))))
        .catch(() => live && setFound([]));
    }, 250);
    return () => {
      live = false;
      clearTimeout(h);
    };
  }, [q, state.people]);
  const list = found ?? state.people;
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="people-view">
      <Title sub={found ? `${found.length}${found.length >= 50 ? "+" : ""} matching` : `${state.people.length >= 200 ? "First 200" : state.people.length} people. Search to find anyone.`}>People</Title>
      <Box sx={{ p: 2 }}>
        <Input startDecorator={<SearchIcon />} placeholder="Search people" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search people" } }} />
      </Box>
      <Box sx={{ flex: 1, overflowY: "auto", px: 2, display: "grid", gap: 1, gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))", alignContent: "start" }}>
        {list.map((p) => {
          const pr = state.presence(p.id);
          return (
            <Card key={p.id} size="sm" variant="outlined" sx={{ flexDirection: "row", alignItems: "center", gap: 1.5, cursor: "pointer" }} onClick={() => onOpenProfile(p)} data-testid="person-card">
              <Box sx={{ position: "relative" }}>
                <UserAvatar id={p.id} name={p.name} size={44} />
                <Box sx={{ position: "absolute", right: 0, bottom: 0, width: 12, height: 12, borderRadius: "50%", border: "2px solid", borderColor: "background.surface", bgcolor: pr === "online" ? "#2eb67d" : pr === "away" ? "#e8a317" : "#9aa5a0" }} />
              </Box>
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography level="title-sm" noWrap>
                  {p.name} {p.id === state.me && "(you)"}
                </Typography>
                <Typography level="body-xs" noWrap sx={{ opacity: 0.7 }}>
                  {p.title || p.email}
                </Typography>
                {p.status_text && (
                  <Typography level="body-xs" noWrap>
                    {p.status_emoji} {p.status_text}
                  </Typography>
                )}
              </Box>
              <Button size="sm" variant="soft" onClick={(e) => { e.stopPropagation(); onDM(p.id); }}>
                {p.id === state.me ? "Notes" : "Message"}
              </Button>
            </Card>
          );
        })}
      </Box>
    </Box>
  );
}

// ---- Browse channels --------------------------------------------------------

export function BrowseView({ state, go, onNew }: { state: TeamsState; go: (v: View) => void; onNew: () => void }) {
  const [list, setList] = useState<TChannel[] | null>(null);
  const [q, setQ] = useState("");
  const load = useCallback(() => teams.browse().then(setList).catch(() => setList([])), []);
  useEffect(() => {
    void load();
  }, [load]);
  const shown = (list ?? []).filter((c) => `${c.name} ${c.topic}`.toLowerCase().includes(q.toLowerCase()));
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="browse-view">
      <Title sub="Public channels in your workspace">Browse channels</Title>
      <Box sx={{ p: 2, display: "flex", gap: 1 }}>
        <Input sx={{ flex: 1 }} startDecorator={<SearchIcon />} placeholder="Search channels" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search channels" } }} />
        <Button onClick={onNew}>Create channel</Button>
      </Box>
      <Box sx={{ flex: 1, overflowY: "auto", px: 2 }}>
        {list === null && <CircularProgress size="sm" />}
        {list && shown.length === 0 && <Empty>No public channels match.</Empty>}
        {shown.map((c) => (
          <Card key={c.id} size="sm" variant="outlined" sx={{ mb: 1, flexDirection: "row", alignItems: "center" }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm"># {c.name}</Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                {c.member_ids.length} members{c.topic ? ` · ${c.topic}` : ""}
              </Typography>
            </Box>
            {c.is_member ? (
              <Button size="sm" variant="soft" onClick={() => go({ kind: "channel", id: c.id })}>
                Open
              </Button>
            ) : (
              <>
                <Button size="sm" variant="plain" onClick={() => go({ kind: "channel", id: c.id })}>
                  Preview
                </Button>
                <Button
                  size="sm"
                  onClick={async () => {
                    await teams.join(c.id);
                    await state.reload();
                    go({ kind: "channel", id: c.id });
                  }}
                >
                  Join
                </Button>
              </>
            )}
          </Card>
        ))}
      </Box>
    </Box>
  );
}

// ---- Search -----------------------------------------------------------------

export function SearchView({ state, go, toast, q: initialQ }: BaseProps & { q: string }) {
  const [q, setQ] = useState(initialQ);
  const [channelId, setChannelId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [res, setRes] = useState<TMessage[] | null>(null);
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    async (query: string) => {
      setBusy(true);
      try {
        setRes(await teams.search({ q: query, channel_id: channelId, from, to }));
      } catch {
        setRes([]);
      } finally {
        setBusy(false);
      }
    },
    [channelId, from, to],
  );
  useEffect(() => {
    setQ(initialQ);
    if (initialQ) void run(initialQ);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialQ]);
  return (
    <Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }} data-testid="search-view">
      <Title>Search</Title>
      <Box sx={{ p: 2, display: "flex", gap: 1, flexWrap: "wrap" }}>
        <Input
          sx={{ flex: 1, minWidth: 220 }}
          autoFocus
          startDecorator={<SearchIcon />}
          placeholder="Search messages"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void run(q)}
          slotProps={{ input: { "aria-label": "Search query" } }}
        />
        <select aria-label="In channel" value={channelId} onChange={(e) => setChannelId(e.target.value)} style={{ padding: "6px 8px", borderRadius: 6 }}>
          <option value="">Everywhere</option>
          {state.channels.filter((c) => c.is_member).map((c) => (
            <option key={c.id} value={c.id}>
              {channelLabel(c, state.me, state.peopleMap)}
            </option>
          ))}
        </select>
        <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "From date" } }} />
        <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "aria-label": "To date" } }} />
        <Button loading={busy} onClick={() => void run(q)}>
          Search
        </Button>
      </Box>
      <Box sx={{ flex: 1, overflowY: "auto" }}>
        {res === null ? <Empty>Type a word and press Enter.</Empty> : <MessageResults state={state} go={go} toast={toast} messages={res} empty={`No messages match “${q}”.`} />}
      </Box>
    </Box>
  );
}

export { fmtShort };
