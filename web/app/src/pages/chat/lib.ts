// Pure helpers for the Teams-class chat UI (no React, unit-tested in lib.test.ts).

import type {
  FeedEvent,
  Person,
  Section,
  TChannel,
  TMessage,
  View,
  ChatReaction,
} from "./types";

export const COLORS = ["#126C5A", "#3D5A80", "#C46B45", "#7A5980", "#2A9D8F", "#D9A441", "#E0777D", "#5B9279"];

export function colorFor(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return COLORS[h % COLORS.length];
}

export function initials(name: string): string {
  const parts = (name || "?").trim().split(/\s+/);
  return ((parts[0]?.[0] ?? "?") + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toUpperCase();
}

export type PeopleMap = Record<string, Person>;

export function personName(p: PeopleMap, id: string): string {
  return p[id]?.name || p[id]?.email || "Unknown";
}

/** Display label of a channel for user `me`. */
export function channelLabel(ch: TChannel, me: string, people: PeopleMap): string {
  if (ch.kind === "public" || ch.kind === "private") return ch.name;
  if (ch.kind === "group" && ch.name) return ch.name;
  const others = ch.member_ids.filter((id) => id !== me);
  if (others.length === 0) return `${personName(people, me)} (you)`;
  const names = others.map((id) => personName(people, id).split(" ")[0]);
  return names.join(", ");
}

export interface SidebarGroup {
  key: string;
  title: string;
  /** custom section id (for rename/delete), "" for built-ins */
  sectionId: string;
  builtin: "starred" | "channels" | "dms" | "custom";
  channels: TChannel[];
  collapsed?: boolean;
  unread: number;
  mentions: number;
}

const byRecent = (a: TChannel, b: TChannel) =>
  (b.last_message_at || b.created_at).localeCompare(a.last_message_at || a.created_at);

/**
 * Organises channels the Slack way: Starred, each custom section, Channels
 * (public + private), Direct messages (DMs + group DMs). Archived channels are
 * hidden. A channel in a custom section is not repeated under its default one.
 */
export function groupSidebar(
  channels: TChannel[],
  sections: Section[],
  me: string,
  people: PeopleMap,
): SidebarGroup[] {
  const live = channels.filter((c) => !c.archived && c.is_member);
  const sectionIds = new Set(sections.map((s) => s.id));
  const starred = live.filter((c) => c.starred);
  const rest = live.filter((c) => !c.starred);
  const custom = (id: string) => rest.filter((c) => c.section_id === id);
  const isCustom = (c: TChannel) => c.section_id !== "" && sectionIds.has(c.section_id);
  const plain = rest.filter((c) => !isCustom(c));
  const label = (c: TChannel) => channelLabel(c, me, people).toLowerCase();
  const chans = plain.filter((c) => c.kind === "public" || c.kind === "private").sort((a, b) => label(a).localeCompare(label(b)));
  const dms = plain.filter((c) => c.kind === "dm" || c.kind === "group").sort(byRecent);
  const mk = (
    key: string,
    title: string,
    sectionId: string,
    builtin: SidebarGroup["builtin"],
    list: TChannel[],
    collapsed?: boolean,
  ): SidebarGroup => ({
    key,
    title,
    sectionId,
    builtin,
    channels: list,
    collapsed,
    unread: list.reduce((n, c) => n + (c.muted ? 0 : c.unread_count), 0),
    mentions: list.reduce((n, c) => n + c.mention_count, 0),
  });
  const out: SidebarGroup[] = [];
  if (starred.length) out.push(mk("starred", "Starred", "", "starred", starred.sort(byRecent)));
  for (const s of [...sections].sort((a, b) => a.position - b.position)) {
    out.push(mk(`s:${s.id}`, s.name, s.id, "custom", custom(s.id).sort(byRecent), s.collapsed));
  }
  out.push(mk("channels", "Channels", "", "channels", chans));
  out.push(mk("dms", "Direct messages", "", "dms", dms));
  return out;
}

/** Applies a new message from the live feed to the channel list. */
export function applyIncoming(
  channels: TChannel[],
  msg: TMessage,
  me: string,
  activeAndVisible: boolean,
): TChannel[] {
  return channels.map((c) => {
    if (c.id !== msg.channel_id) return c;
    const mine = msg.sender_id === me;
    const topLevel = !msg.parent_id;
    const mentioned = (msg.mention_ids ?? []).includes(me);
    return {
      ...c,
      last_message_at: topLevel ? msg.sent_at : c.last_message_at,
      unread_count: !mine && topLevel && !activeAndVisible ? c.unread_count + 1 : c.unread_count,
      mention_count: !mine && mentioned && !activeAndVisible ? c.mention_count + 1 : c.mention_count,
    };
  });
}

/** Decide whether a live message should notify user `me`. */
export function shouldNotify(
  notify: string[] | undefined,
  msg: TMessage,
  ch: TChannel | undefined,
  me: string,
): boolean {
  if (msg.sender_id === me) return false;
  if (!notify || !notify.includes(me)) return false;
  if (ch?.muted && !(msg.mention_ids ?? []).includes(me)) return false;
  return true;
}

/** Folds a reaction feed event into a message for viewer `me`. */
export function foldReactions(
  m: TMessage,
  reactions: { emoji: string; user_ids: string[] }[],
  me: string,
): TMessage {
  const details: ChatReaction[] = reactions.map((r) => ({
    emoji: r.emoji,
    count: r.user_ids.length,
    me: r.user_ids.includes(me),
  }));
  return { ...m, reaction_details: details };
}

/** Parses (null = no explicit view) /chat, /chat/c/<id>, /chat/c/<id>/m/<msg>, /chat/activity, ... */
export function parsePath(pathname: string, search = ""): View | null {
  const parts = pathname.replace(/^\/chat\/?/, "").split("/").filter(Boolean);
  if (parts[0] === "c" && parts[1]) {
    return parts[2] === "m" && parts[3]
      ? { kind: "channel", id: parts[1], msgId: parts[3] }
      : { kind: "channel", id: parts[1] };
  }
  switch (parts[0]) {
    case "activity":
      return { kind: "activity" };
    case "later":
      return { kind: "later" };
    case "drafts":
      return { kind: "drafts" };
    case "people":
      return { kind: "people" };
    case "browse":
      return { kind: "browse" };
    case "meetings":
      return { kind: "meetings" };
    case "calls":
      return { kind: "calls" };
    case "files":
      return { kind: "files" };
    case "search":
      return { kind: "search", q: new URLSearchParams(search).get("q") ?? "" };
  }
  return null;
}

export function viewPath(v: View): string {
  switch (v.kind) {
    case "channel":
      return v.msgId ? `/chat/c/${v.id}/m/${v.msgId}` : `/chat/c/${v.id}`;
    case "search":
      return `/chat/search?q=${encodeURIComponent(v.q)}`;
    default:
      return `/chat/${v.kind}`;
  }
}

export function permalink(channelId: string, msgId: string): string {
  return `${window.location.origin}/chat/c/${channelId}/m/${msgId}`;
}

// ---- time -------------------------------------------------------------------

export function fmtClock(iso: string): string {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

export function fmtShort(iso: string, now = new Date()): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (d.toDateString() === now.toDateString()) return fmtClock(iso);
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

export function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

export function dayLabel(iso: string, now = new Date()): string {
  const d = new Date(iso);
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === now.toDateString()) return "Today";
  if (d.toDateString() === y.toDateString()) return "Yesterday";
  return d.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

/** True when `m` continues the same author's run (compact row, no header). */
export function continuesRun(prev: TMessage | undefined, m: TMessage): boolean {
  if (!prev || prev.sender_id !== m.sender_id) return false;
  if (dayKey(prev.sent_at) !== dayKey(m.sent_at)) return false;
  return new Date(m.sent_at).getTime() - new Date(prev.sent_at).getTime() < 5 * 60_000;
}

/** Local <input type=datetime-local> value for now + minutes. */
export function localInputValue(date: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}T${p(date.getHours())}:${p(date.getMinutes())}`;
}

/** Common send-later / remind presets. */
export function presets(now = new Date()): { label: string; at: Date }[] {
  const at = (h: number, addDays = 0) => {
    const d = new Date(now);
    d.setDate(d.getDate() + addDays);
    d.setHours(h, 0, 0, 0);
    return d;
  };
  const list: { label: string; at: Date }[] = [
    { label: "In 20 minutes", at: new Date(now.getTime() + 20 * 60_000) },
    { label: "In 1 hour", at: new Date(now.getTime() + 60 * 60_000) },
    { label: "In 3 hours", at: new Date(now.getTime() + 3 * 60 * 60_000) },
  ];
  if (now.getHours() < 9) list.push({ label: "Today at 9:00 AM", at: at(9) });
  list.push({ label: "Tomorrow at 9:00 AM", at: at(9, 1) });
  const toMonday = ((8 - now.getDay()) % 7) || 7;
  list.push({ label: "Next Monday at 9:00 AM", at: at(9, toMonday) });
  return list;
}

/** Unread totals for the title/badge. */
export function totals(channels: TChannel[]): { unread: number; mentions: number } {
  let unread = 0;
  let mentions = 0;
  for (const c of channels) {
    if (c.archived || !c.is_member) continue;
    mentions += c.mention_count;
    if (!c.muted) unread += c.unread_count;
  }
  return { unread, mentions };
}

/**
 * Applies non-"message" feed events to a list of messages (channel view,
 * thread, saved list, ...). "message" is handled by the owning view because
 * it depends on what the view is showing.
 */
export function applyFeedToList(list: TMessage[], e: FeedEvent, me: string): TMessage[] {
  switch (e.type) {
    case "edited":
      return list.map((m) => (m.id === e.message.id ? { ...m, body: e.message.body, edited_at: e.message.edited_at, mention_ids: e.message.mention_ids } : m));
    case "deleted": {
      let out = list.filter((m) => m.id !== e.id);
      if (e.parent_id) out = out.map((m) => (m.id === e.parent_id ? { ...m, reply_count: Math.max(0, (m.reply_count ?? 1) - 1) } : m));
      return out;
    }
    case "reaction":
      return list.map((m) => (m.id === e.message_id ? foldReactions(m, e.reactions, me) : m));
    case "pins":
      return e.message_id ? list.map((m) => (m.id === e.message_id ? { ...m, pinned: e.pinned } : m)) : list;
    case "saved":
      return list.map((m) => (m.id === e.message_id ? { ...m, saved: e.saved } : m));
    default:
      return list;
  }
}
