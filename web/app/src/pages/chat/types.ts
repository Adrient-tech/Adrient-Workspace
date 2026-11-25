/** ChatChannel mirrors adrientv1.ChatChannel (proto snake_case via the gateway). */
export interface ChatChannel {
  id: string;
  org_id: string;
  kind: "dm" | "group";
  name: string;
  member_ids: string[];
  created_at: string;
  updated_at: string;
  last_message_at: string;
  unread_count: number;
}

/** ChatAttachment mirrors adrientv1.ChatAttachment. */
export interface ChatAttachment {
  id: string;
  name: string;
  mime_type: string;
  size: number;
  url: string;
}

/** ChatReaction mirrors adrientv1.ChatReaction. */
export interface ChatReaction {
  emoji: string;
  count: number;
  /** True when the requesting user has reacted with this emoji. */
  me: boolean;
}

/** ChatMessage mirrors adrientv1.ChatMessage. */
export interface ChatMessage {
  id: string;
  channel_id: string;
  org_id: string;
  sender_id: string;
  sender_name: string;
  body: string;
  /** @deprecated Use reaction_details instead. */
  reactions: string; // JSON object {"emoji": count}
  sent_at: string;
  attachments?: ChatAttachment[];
  /** Structured reaction aggregates. */
  reaction_details?: ChatReaction[];
  /** ID of the parent message for thread replies. Empty for top-level. */
  parent_id?: string;
  /** Number of thread replies (populated for top-level messages). */
  reply_count?: number;
  /** Echoed back from PostChatMessage for optimistic-UI deduplication. */
  client_nonce?: string;
  /** Set client-side only on optimistic messages; cleared on confirmation. */
  _optimistic?: true;
}

export interface ListChannelsResponse {
  channels: ChatChannel[];
}

export interface ListMessagesResponse {
  messages: ChatMessage[];
}

export interface ReactResponse {
  reaction_details: ChatReaction[];
}

export interface CreateChannelInput {
  kind: "dm" | "group";
  name?: string;
  member_ids?: string[];
}

export interface PostMessageInput {
  channel_id: string;
  body: string;
  attachment_ids?: string[];
}

/** WS envelope sent by the server over the chat WebSocket. */
export type ChatWSEvent =
  | { type: "message"; channel_id: string; message: ChatMessage }
  | { type: "presence"; channel_id: string; online: string[] }
  | { type: "deleted"; channel_id: string; id: string };

// ---- Teams-class chat (REST under /api/v1/chat/x) ---------------------------

export type ChannelKind = "dm" | "group" | "public" | "private";

/** TChannel mirrors internal/chat.TChannel. */
export interface TChannel {
  id: string;
  kind: ChannelKind;
  name: string;
  topic: string;
  description: string;
  archived: boolean;
  /** Announcement channel: only the creator posts top-level messages. */
  announce?: boolean;
  /** When the caller last read the channel (server time), "" if never. */
  last_read_at?: string;
  created_by: string;
  huddle_code: string;
  member_ids: string[];
  last_message_at: string;
  created_at: string;
  unread_count: number;
  mention_count: number;
  starred: boolean;
  muted: boolean;
  section_id: string;
  is_member: boolean;
}

/** TMessage mirrors internal/chat.TMessage (legacy proto messages omit some fields). */
export interface TMessage extends ChatMessage {
  edited_at?: string;
  last_reply_at?: string;
  mention_ids?: string[];
  pinned?: boolean;
  saved?: boolean;
}

export interface Section {
  id: string;
  name: string;
  position: number;
  collapsed: boolean;
}

export interface Person {
  id: string;
  name: string;
  email: string;
  title: string;
  status_text: string;
  status_emoji: string;
}

export interface Bookmark {
  id: string;
  title: string;
  url: string;
}

export interface Draft {
  id: string;
  channel_id: string;
  parent_id: string;
  body: string;
  updated_at: string;
}

export interface Scheduled {
  id: string;
  channel_id: string;
  parent_id: string;
  body: string;
  send_at: string;
  status: string;
}

export interface Reminder {
  id: string;
  message_id: string;
  channel_id: string;
  note: string;
  remind_at: string;
  fired: boolean;
}

export interface ActivityItem {
  kind: "mention" | "thread_reply" | "reminder";
  at: string;
  message: TMessage;
  note?: string;
  unread: boolean;
}

export interface Bootstrap {
  channels: TChannel[];
  sections: Section[];
  me: string;
  me_name: string;
  activity_unread: number;
  drafts: number;
  scheduled: number;
  saved: number;
  online: string[];
  away: string[];
}

/** Events on the per-user live feed. */
export type FeedEvent =
  | { type: "message"; channel_id: string; message: TMessage; notify?: string[] }
  | { type: "edited"; channel_id: string; message: TMessage }
  | { type: "deleted"; channel_id: string; id: string; parent_id?: string }
  | {
      type: "reaction";
      channel_id: string;
      message_id: string;
      reactions: { emoji: string; user_ids: string[] }[];
    }
  | { type: "pins"; channel_id: string; message_id?: string; pinned?: boolean }
  | { type: "channel"; channel_id: string }
  | { type: "sections" }
  | { type: "read"; channel_id: string }
  | { type: "saved"; message_id: string; saved: boolean }
  | { type: "scheduled" }
  | { type: "reminder"; note: string; message_id: string; message?: TMessage }
  | { type: "presence"; online: string[]; away: string[] }
  | { type: "typing"; channel_id: string; user_id: string };

/** What the main pane shows. */
export type View =
  | { kind: "channel"; id: string; msgId?: string }
  | { kind: "activity" }
  | { kind: "later" }
  | { kind: "drafts" }
  | { kind: "people" }
  | { kind: "browse" }
  | { kind: "meetings" }
  | { kind: "calls" }
  | { kind: "files" }
  | { kind: "search"; q: string };

/** ChannelTab is an app or link pinned to a channel (Teams-style tab). */
export interface ChannelTab {
  id: string;
  kind: "doc" | "sheet" | "slides" | "whiteboard" | "tasks" | "project" | "link";
  title: string;
  url: string;
}

/** ChannelFile is an attachment shared in a channel. */
export interface ChannelFile extends ChatAttachment {
  message_id: string;
  sent_at: string;
}
