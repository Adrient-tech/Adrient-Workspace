import type {
  ActivityItem,
  Bookmark,
  Bootstrap,
  Draft,
  FeedEvent,
  Person,
  Reminder,
  Scheduled,
  Section,
  TChannel,
  TMessage,
  ChatReaction,
  ChannelFile,
  ChannelTab,
} from "./types";

const BASE = "/api/v1/chat/x";

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const j = (await resp.json()) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await resp.json()) as T;
}

const q = (o: Record<string, string | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const teams = {
  bootstrap: () => call<Bootstrap>("GET", "/bootstrap"),
  people: () => call<{ people: Person[] }>("GET", "/people").then((r) => r.people ?? []),
  setProfile: (p: { title: string; status_text: string; status_emoji: string }) =>
    call<{ ok: boolean }>("PUT", "/profile", p),

  createChannel: (input: {
    kind: string;
    name?: string;
    topic?: string;
    description?: string;
    member_ids?: string[];
  }) => call<TChannel>("POST", "/channels", input),
  browse: () => call<{ channels: TChannel[] }>("GET", "/channels/browse").then((r) => r.channels ?? []),
  getChannel: (id: string) => call<TChannel>("GET", `/channels/${id}`),
  updateChannel: (
    id: string,
    patch: Partial<{ name: string; topic: string; description: string; archived: boolean; announce: boolean }>,
  ) => call<TChannel>("PATCH", `/channels/${id}`, patch),
  join: (id: string) => call<TChannel>("POST", `/channels/${id}/join`),
  leave: (id: string) => call<{ ok: boolean }>("POST", `/channels/${id}/leave`),
  addMembers: (id: string, user_ids: string[]) =>
    call<TChannel>("POST", `/channels/${id}/members`, { user_ids }),
  removeMember: (id: string, uid: string) =>
    call<{ ok: boolean }>("DELETE", `/channels/${id}/members/${uid}`),
  typing: (id: string) => call<{ ok: boolean }>("POST", `/channels/${id}/typing`),
  markRead: (id: string) => call<{ ok: boolean }>("POST", `/channels/${id}/read`),
  setPrefs: (
    id: string,
    p: Partial<{ starred: boolean; muted: boolean; section_id: string }>,
  ) => call<{ ok: boolean }>("PATCH", `/channels/${id}/prefs`, p),

  createSection: (name: string) => call<Section>("POST", "/sections", { name }),
  updateSection: (id: string, p: Partial<{ name: string; collapsed: boolean }>) =>
    call<{ ok: boolean }>("PATCH", `/sections/${id}`, p),
  deleteSection: (id: string) => call<{ ok: boolean }>("DELETE", `/sections/${id}`),

  messages: (
    channelId: string,
    o: { before?: string; around?: string; from?: string } = {},
  ) =>
    call<{ messages: TMessage[]; has_more: boolean }>(
      "GET",
      `/channels/${channelId}/messages${q(o)}`,
    ),
  post: (
    channelId: string,
    body: string,
    o: { parent_id?: string; attachment_ids?: string[]; client_nonce?: string } = {},
  ) =>
    call<TMessage>("POST", `/channels/${channelId}/messages`, {
      body,
      parent_id: o.parent_id ?? "",
      attachment_ids: o.attachment_ids ?? [],
      client_nonce: o.client_nonce ?? "",
    }),
  message: (id: string) => call<TMessage>("GET", `/messages/${id}`),
  edit: (id: string, body: string) => call<TMessage>("PATCH", `/messages/${id}`, { body }),
  remove: (id: string) => call<{ ok: boolean }>("DELETE", `/messages/${id}`),
  thread: (id: string) =>
    call<{ parent: TMessage; replies: TMessage[] }>("GET", `/messages/${id}/thread`),
  react: (id: string, emoji: string) =>
    call<{ reaction_details: ChatReaction[] }>("POST", `/messages/${id}/react`, { emoji }),
  pin: (id: string) => call<{ pinned: boolean }>("POST", `/messages/${id}/pin`),
  save: (id: string) => call<{ saved: boolean }>("POST", `/messages/${id}/save`),
  remind: (id: string, at: string, note = "") =>
    call<Reminder>("POST", `/messages/${id}/remind`, { at, note }),

  pins: (channelId: string) =>
    call<{ messages: TMessage[] }>("GET", `/channels/${channelId}/pins`).then((r) => r.messages ?? []),
  bookmarks: (channelId: string) =>
    call<{ bookmarks: Bookmark[] }>("GET", `/channels/${channelId}/bookmarks`).then((r) => r.bookmarks ?? []),
  addBookmark: (channelId: string, title: string, url: string) =>
    call<Bookmark>("POST", `/channels/${channelId}/bookmarks`, { title, url }),
  deleteBookmark: (channelId: string, id: string) =>
    call<{ ok: boolean }>("DELETE", `/bookmarks/${channelId}/${id}`),

  tabs: (channelId: string) =>
    call<{ tabs: ChannelTab[] }>("GET", `/channels/${channelId}/tabs`).then((r) => r.tabs ?? []),
  addTab: (channelId: string, tab: { kind: string; title: string; url: string }) =>
    call<ChannelTab>("POST", `/channels/${channelId}/tabs`, tab),
  renameTab: (channelId: string, id: string, title: string) =>
    call<{ ok: boolean }>("PATCH", `/channels/${channelId}/tabs/${id}`, { title }),
  deleteTab: (channelId: string, id: string) =>
    call<{ ok: boolean }>("DELETE", `/channels/${channelId}/tabs/${id}`),
  files: (channelId: string) =>
    call<{ files: ChannelFile[] }>("GET", `/channels/${channelId}/files`).then((r) => r.files ?? []),

  saved: () => call<{ messages: TMessage[] }>("GET", "/saved").then((r) => r.messages ?? []),
  sent: () => call<{ messages: TMessage[] }>("GET", "/sent").then((r) => r.messages ?? []),
  activity: () => call<{ items: ActivityItem[] }>("GET", "/activity").then((r) => r.items ?? []),
  activitySeen: () => call<{ ok: boolean }>("POST", "/activity/seen"),
  reminders: () => call<{ reminders: Reminder[] }>("GET", "/reminders").then((r) => r.reminders ?? []),
  deleteReminder: (id: string) => call<{ ok: boolean }>("DELETE", `/reminders/${id}`),

  drafts: () => call<{ drafts: Draft[] }>("GET", "/drafts").then((r) => r.drafts ?? []),
  saveDraft: (channel_id: string, parent_id: string, body: string) =>
    call<{ ok: boolean }>("PUT", "/drafts", { channel_id, parent_id, body }),
  deleteDraft: (id: string) => call<{ ok: boolean }>("DELETE", `/drafts/${id}`),
  scheduled: () => call<{ scheduled: Scheduled[] }>("GET", "/scheduled").then((r) => r.scheduled ?? []),
  schedule: (channel_id: string, body: string, send_at: string, parent_id = "") =>
    call<Scheduled>("POST", "/scheduled", { channel_id, parent_id, body, send_at }),
  cancelScheduled: (id: string) => call<{ ok: boolean }>("DELETE", `/scheduled/${id}`),

  search: (o: { q?: string; channel_id?: string; sender_id?: string; from?: string; to?: string }) =>
    call<{ messages: TMessage[] }>("GET", `/search${q(o)}`).then((r) => r.messages ?? []),

  huddle: (channelId: string, code: string) =>
    call<{ code: string; url: string }>("POST", `/channels/${channelId}/huddle`, { code }),
};

/** Opens the per-user live feed and reconnects with backoff. Returns a controller. */
export function openFeed(
  onEvent: (e: FeedEvent) => void,
  onState: (open: boolean) => void,
): { send: (obj: unknown) => void; close: () => void } {
  let ws: WebSocket | null = null;
  let closed = false;
  let attempt = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const connect = () => {
    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    ws = new WebSocket(`${proto}://${window.location.host}${BASE}/live`);
    ws.onopen = () => {
      attempt = 0;
      onState(true);
    };
    ws.onmessage = (ev) => {
      try {
        onEvent(JSON.parse(ev.data) as FeedEvent);
      } catch {
        /* ignore */
      }
    };
    ws.onclose = () => {
      onState(false);
      if (closed) return;
      attempt++;
      timer = setTimeout(connect, Math.min(15000, 500 * 2 ** attempt));
    };
  };
  connect();
  return {
    send: (obj) => {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
    },
    close: () => {
      closed = true;
      if (timer) clearTimeout(timer);
      ws?.close();
    },
  };
}
