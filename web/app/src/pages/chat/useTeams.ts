import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { openFeed, teams } from "./teamsApi";
import { playSound } from "../../lib/sounds";
import { applyIncoming, shouldNotify, totals, channelLabel, type PeopleMap } from "./lib";
import type { FeedEvent, Person, Section, TChannel, TMessage } from "./types";

export interface Toast {
  id: number;
  title: string;
  body: string;
  channelId?: string;
  msgId?: string;
}

type Listener = (e: FeedEvent) => void;

/**
 * Central state for the Teams chat: channels, sections, people, presence and
 * the live feed. Views subscribe to raw feed events through `subscribe`.
 */
export function useTeams() {
  const [me, setMe] = useState("");
  const [meName, setMeName] = useState("");
  const [channels, setChannels] = useState<TChannel[]>([]);
  const [sections, setSections] = useState<Section[]>([]);
  const [people, setPeople] = useState<Person[]>([]);
  const [online, setOnline] = useState<string[]>([]);
  const [away, setAway] = useState<string[]>([]);
  const [counts, setCounts] = useState({ activity: 0, drafts: 0, scheduled: 0, saved: 0 });
  const [loading, setLoading] = useState(true);
  const [live, setLive] = useState(false);
  const [toasts, setToasts] = useState<Toast[]>([]);

  const meRef = useRef("");
  const channelsRef = useRef<TChannel[]>([]);
  const peopleRef = useRef<PeopleMap>({});
  const activeRef = useRef<string>("");
  const listeners = useRef(new Set<Listener>());
  const toastId = useRef(0);
  const feedRef = useRef<ReturnType<typeof openFeed> | null>(null);

  meRef.current = me;
  channelsRef.current = channels;
  const peopleMap = useMemo<PeopleMap>(() => {
    const m: PeopleMap = {};
    for (const p of people) m[p.id] = p;
    peopleRef.current = m;
    return m;
  }, [people]);

  const reload = useCallback(async () => {
    try {
      const b = await teams.bootstrap();
      setMe(b.me);
      setMeName(b.me_name);
      meRef.current = b.me;
      setChannels(b.channels ?? []);
      setSections(b.sections ?? []);
      setOnline(b.online ?? []);
      setAway(b.away ?? []);
      setCounts({ activity: b.activity_unread, drafts: b.drafts, scheduled: b.scheduled, saved: b.saved });
    } catch {
      /* keep stale state */
    } finally {
      setLoading(false);
    }
  }, []);

  const reloadPeople = useCallback(async () => {
    try {
      setPeople(await teams.people());
    } catch {
      /* ignore */
    }
  }, []);

  const refreshCounts = useCallback(async () => {
    try {
      const b = await teams.bootstrap();
      setCounts({ activity: b.activity_unread, drafts: b.drafts, scheduled: b.scheduled, saved: b.saved });
    } catch {
      /* ignore */
    }
  }, []);

  const notifyOS = useCallback((t: Omit<Toast, "id">) => {
    const id = ++toastId.current;
    setToasts((cur) => [...cur.slice(-3), { ...t, id }]);
    setTimeout(() => setToasts((cur) => cur.filter((x) => x.id !== id)), 7000);
    if (document.hidden && "Notification" in window && Notification.permission === "granted") {
      try {
        const n = new Notification(t.title, { body: t.body, tag: t.channelId ?? "chat" });
        n.onclick = () => {
          window.focus();
          if (t.channelId) window.dispatchEvent(new CustomEvent("chat:open", { detail: t }));
          n.close();
        };
      } catch {
        /* some browsers throw outside a user gesture */
      }
    }
  }, []);

  const handle = useCallback(
    (e: FeedEvent) => {
      const myId = meRef.current;
      switch (e.type) {
        case "presence":
          setOnline(e.online ?? []);
          setAway(e.away ?? []);
          break;
        case "message": {
          const msg = e.message;
          const visible = !document.hidden && activeRef.current === e.channel_id;
          const known = channelsRef.current.some((c) => c.id === e.channel_id);
          if (!known) {
            void reload();
          } else {
            setChannels((cur) => applyIncoming(cur, msg, myId, visible));
          }
          if (visible && msg.sender_id !== myId && !msg.parent_id) void teams.markRead(e.channel_id);
          const ch = channelsRef.current.find((c) => c.id === e.channel_id);
          if (shouldNotify(e.notify, msg, ch, myId) && !visible) {
            const isMention = (msg.mention_ids ?? []).includes(myId);
            playSound(isMention ? "mention" : ch?.kind === "dm" ? "dm" : "message");
            const where = ch ? channelLabel(ch, myId, peopleRef.current) : "chat";
            const mention = (msg.mention_ids ?? []).includes(myId);
            notifyOS({
              title: mention ? `${msg.sender_name} mentioned you in ${where}` : `${msg.sender_name} in ${where}`,
              body: msg.body.slice(0, 140),
              channelId: e.channel_id,
              msgId: msg.id,
            });
          }
          if ((msg.mention_ids ?? []).includes(myId) || msg.parent_id) void refreshCounts();
          break;
        }
        case "channel":
        case "sections":
        case "read":
          void reload();
          break;
        case "scheduled":
          void refreshCounts();
          break;
        case "saved":
          void refreshCounts();
          break;
        case "reminder":
          playSound("task");
          notifyOS({
            title: "Reminder",
            body: e.note || e.message?.body?.slice(0, 140) || "You asked to be reminded",
            channelId: e.message?.channel_id,
            msgId: e.message?.id,
          });
          void refreshCounts();
          break;
      }
      listeners.current.forEach((l) => l(e));
    },
    [reload, refreshCounts, notifyOS],
  );

  useEffect(() => {
    void reload();
    void reloadPeople();
    const feed = openFeed(handle, (open) => {
      setLive(open);
      if (open) void reload();
    });
    feedRef.current = feed;
    // Away after 5 idle minutes or when the tab is hidden for a while.
    let idle: ReturnType<typeof setTimeout> | undefined;
    let isAway = false;
    const setAwayState = (a: boolean) => {
      if (a !== isAway) {
        isAway = a;
        feed.send({ type: "away", away: a });
      }
    };
    const bump = () => {
      setAwayState(false);
      if (idle) clearTimeout(idle);
      idle = setTimeout(() => setAwayState(true), 5 * 60_000);
    };
    const vis = () => (document.hidden ? undefined : bump());
    bump();
    window.addEventListener("mousemove", bump);
    window.addEventListener("keydown", bump);
    document.addEventListener("visibilitychange", vis);
    return () => {
      feed.close();
      if (idle) clearTimeout(idle);
      window.removeEventListener("mousemove", bump);
      window.removeEventListener("keydown", bump);
      document.removeEventListener("visibilitychange", vis);
    };
  }, [handle, reload, reloadPeople]);

  // Title badge.
  const t = useMemo(() => totals(channels), [channels]);
  useEffect(() => {
    const n = t.mentions || t.unread;
    document.title = n > 0 ? `(${n}) Chat · Adrient Workspace` : "Chat · Adrient Workspace";
  }, [t]);

  const subscribe = useCallback((l: Listener) => {
    listeners.current.add(l);
    return () => {
      listeners.current.delete(l);
    };
  }, []);

  const setActive = useCallback((id: string) => {
    activeRef.current = id;
  }, []);

  const patchChannel = useCallback((id: string, patch: Partial<TChannel>) => {
    setChannels((cur) => cur.map((c) => (c.id === id ? { ...c, ...patch } : c)));
  }, []);

  const dismissToast = useCallback((id: number) => setToasts((c) => c.filter((x) => x.id !== id)), []);

  const presence = useCallback(
    (id: string): "online" | "away" | "offline" =>
      online.includes(id) ? "online" : away.includes(id) ? "away" : "offline",
    [online, away],
  );

  return {
    me, meName, channels, sections, people, peopleMap, counts, loading, live, toasts,
    totals: t, reload, reloadPeople, refreshCounts, subscribe, setActive, patchChannel,
    dismissToast, presence, notifyOS,
  };
}

export type TeamsState = ReturnType<typeof useTeams>;
export type { TMessage };
