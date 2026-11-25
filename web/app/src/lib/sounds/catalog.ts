/** Every sound the app can play. The file for an id lives at /sounds/<set>/<id>.mp3. */
export const SOUND_IDS = [
  "ringtone",
  "ringback",
  "join",
  "leave",
  "hand",
  "mute",
  "unmute",
  "share",
  "rec_start",
  "rec_stop",
  "meeting_ended",
  "message",
  "dm",
  "mention",
  "mail",
  "calendar",
  "task",
  "approval",
  "alert",
  "success",
  "error",
] as const;
export type SoundId = (typeof SOUND_IDS)[number];

export const SOUND_SETS = ["classic", "soft", "bright"] as const;
export type SoundSet = (typeof SOUND_SETS)[number];
export const SET_LABELS: Record<SoundSet, string> = {
  classic: "Classic",
  soft: "Soft",
  bright: "Bright",
};

export const CATEGORIES = ["calls", "meeting", "chat", "mail", "calendar", "tasks", "approvals", "system"] as const;
export type SoundCategory = (typeof CATEGORIES)[number];

export const CATEGORY_LABELS: Record<SoundCategory, { label: string; help: string }> = {
  calls: { label: "Calls", help: "Ringtone for incoming calls and the ringback you hear while calling" },
  meeting: { label: "Meetings", help: "Join and leave chimes, hand raise, mute ticks, screen share, recording, meeting ended" },
  chat: { label: "Chat", help: "Messages, direct messages and @mentions" },
  mail: { label: "Mail", help: "New mail arriving" },
  calendar: { label: "Calendar", help: "Event reminders" },
  tasks: { label: "Tasks", help: "Task assignments and reminders" },
  approvals: { label: "Approvals", help: "Requests waiting for your approval" },
  system: { label: "System", help: "Alerts and success or error confirmations" },
};

export const CATEGORY_OF: Record<SoundId, SoundCategory> = {
  ringtone: "calls",
  ringback: "calls",
  join: "meeting",
  leave: "meeting",
  hand: "meeting",
  mute: "meeting",
  unmute: "meeting",
  share: "meeting",
  rec_start: "meeting",
  rec_stop: "meeting",
  meeting_ended: "meeting",
  message: "chat",
  dm: "chat",
  mention: "chat",
  mail: "mail",
  calendar: "calendar",
  task: "tasks",
  approval: "approvals",
  alert: "system",
  success: "system",
  error: "system",
};

export const SOUND_LABELS: Record<SoundId, string> = {
  ringtone: "Incoming call ringtone",
  ringback: "Outgoing ringback",
  join: "Someone joined",
  leave: "Someone left",
  hand: "Hand raised",
  mute: "Muted",
  unmute: "Unmuted",
  share: "Screen share started",
  rec_start: "Recording started",
  rec_stop: "Recording stopped",
  meeting_ended: "Meeting ended",
  message: "Chat message",
  dm: "Direct message",
  mention: "@mention",
  mail: "New mail",
  calendar: "Calendar reminder",
  task: "Task reminder",
  approval: "Approval request",
  alert: "System alert",
  success: "Success",
  error: "Error",
};

/** Sounds that repeat until stopped. */
export const LOOPING: ReadonlySet<SoundId> = new Set<SoundId>(["ringtone", "ringback"]);

export function soundUrl(set: SoundSet, id: SoundId): string {
  return `/sounds/${set}/${id}.mp3`;
}

/**
 * Picks the sound for a server notification kind (the `kind` of a live
 * "notification" event). Unknown kinds get the plain message tone.
 */
export function soundForNotification(kind: string): SoundId | null {
  const k = kind.toLowerCase();
  if (k === "call") return null; // the ring dialog plays the ringtone
  if (k === "meet_ended") return "meeting_ended";
  if (k.includes("approv")) return "approval";
  if (k.includes("mention")) return "mention";
  if (k.startsWith("mail")) return "mail";
  if (k.startsWith("calendar") || k.startsWith("event")) return "calendar";
  if (k.startsWith("task") || k.startsWith("keep") || k.includes("reminder")) return "task";
  if (k.includes("alert") || k.startsWith("system") || k.startsWith("security")) return "alert";
  return "message";
}
