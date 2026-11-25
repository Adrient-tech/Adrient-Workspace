// Slash commands for the Chat composer. Pure parsing; ChannelView performs the effect.

export type SlashResult =
  | { kind: "none" }
  | { kind: "send"; body: string }
  | { kind: "topic"; topic: string }
  | { kind: "mute"; muted: boolean }
  | { kind: "star"; starred: boolean }
  | { kind: "huddle" }
  | { kind: "meet" }
  | { kind: "help"; lines: string[] }
  | { kind: "error"; message: string };

export const SLASH_HELP: { cmd: string; text: string }[] = [
  { cmd: "/shrug [text]", text: "Append ¯\\_(ツ)_/¯" },
  { cmd: "/tableflip [text]", text: "Append (╯°□°)╯︵ ┻━┻" },
  { cmd: "/unflip [text]", text: "Append ┬─┬ノ( º _ ºノ)" },
  { cmd: "/me <text>", text: "Post an action in italics" },
  { cmd: "/topic <text>", text: "Set the channel topic" },
  { cmd: "/mute, /unmute", text: "Mute or unmute this channel for you" },
  { cmd: "/star, /unstar", text: "Star or unstar this channel" },
  { cmd: "/huddle", text: "Start a huddle in this channel" },
  { cmd: "/meet", text: "Start a meeting with this channel" },
  { cmd: "/help", text: "Show this list" },
];

const FACES: Record<string, string> = {
  shrug: "¯\\_(ツ)_/¯",
  tableflip: "(╯°□°)╯︵ ┻━┻",
  unflip: "┬─┬ノ( º _ ºノ)",
};

/** True when the text is meant as a command: a slash, a lowercase word, then nothing or a space. Paths and "/ " stay plain text. */
export function isSlash(text: string): boolean {
  return /^\/[a-z]+(\s|$)/.test(text);
}

export function parseSlash(input: string, opts: { isChannel: boolean } = { isChannel: true }): SlashResult {
  const text = input.replace(/\s+$/, "");
  if (!isSlash(text)) return { kind: "none" };
  const m = /^\/([a-z]+)(?:\s+([\s\S]*))?$/.exec(text)!;
  const cmd = m[1];
  const arg = (m[2] ?? "").trim();
  if (cmd in FACES) return { kind: "send", body: arg ? `${arg} ${FACES[cmd]}` : FACES[cmd] };
  switch (cmd) {
    case "me":
      return arg ? { kind: "send", body: `_${arg.replace(/_/g, "\\_")}_` } : { kind: "error", message: "Usage: /me <what you are doing>" };
    case "topic":
      if (!opts.isChannel) return { kind: "error", message: "Only channels have a topic." };
      if (arg.length > 250) return { kind: "error", message: "A topic is at most 250 characters." };
      return { kind: "topic", topic: arg };
    case "mute":
      return { kind: "mute", muted: true };
    case "unmute":
      return { kind: "mute", muted: false };
    case "star":
      return { kind: "star", starred: true };
    case "unstar":
      return { kind: "star", starred: false };
    case "huddle":
      return { kind: "huddle" };
    case "meet":
      return { kind: "meet" };
    case "help":
      return { kind: "help", lines: SLASH_HELP.map((h) => `${h.cmd}: ${h.text}`) };
    default:
      return { kind: "error", message: `Unknown command /${cmd}. Type /help for the list.` };
  }
}

/**
 * Index of the first message that is new to the reader: sent after lastReadAt by someone else.
 * Returns -1 when there is none or when the channel was never read (everything would be "new").
 */
export function firstUnreadIndex(msgs: { sent_at: string; sender_id: string }[], lastReadAt: string, me: string): number {
  if (!lastReadAt) return -1;
  const t = Date.parse(lastReadAt.replace(" ", "T").replace(/([+-]\d\d)$/, "$1:00"));
  if (!Number.isFinite(t)) return -1;
  return msgs.findIndex((m) => m.sender_id !== me && Date.parse(m.sent_at) > t);
}
