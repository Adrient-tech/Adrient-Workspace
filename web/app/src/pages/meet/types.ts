import type { Track } from "livekit-client";

/** MeetRoom mirrors adrientv1.MeetRoom (proto snake_case via the gateway). */
export interface MeetRoom {
  id: string;
  org_id: string;
  owner_id: string;
  name: string;
  created_at: string;
  /** Short meeting code, e.g. "abc-defg-hij". Present when returned by the codes surface. */
  code?: string;
  /** True once the meeting ended (last person left, host ended it, room closed). */
  ended?: boolean;
  ended_at?: string;
  ended_reason?: string;
  duration_s?: number;
  participant_count?: number;
}

export interface ListMeetRoomsResponse {
  rooms: MeetRoom[];
}

// ---- WebRTC signaling ----

export type SignalType =
  | "join"
  | "leave"
  | "offer"
  | "answer"
  | "candidate"
  | "presence"
  | "chat"
  | "media_state"
  | "hand_raise"
  | "roster_state"
  | "meeting_ended"
  | "reaction"
  | "screen_share"
  | "lobby"
  | "room_state";

export interface PeerInfo {
  id: string;
  name: string;
  audio_muted?: boolean;
  video_off?: boolean;
  hand_raised?: boolean;
  sharing?: boolean;
}

export interface SignalMessage {
  type: SignalType;
  from?: string;
  to?: string;
  name?: string;
  peers?: PeerInfo[];
  /** SDP offer/answer or RTCIceCandidateInit */
  payload?: unknown;

  // Chat
  text?: string;

  // Media/hand state
  audio_muted?: boolean;
  video_off?: boolean;
  hand_raised?: boolean;
  emoji?: string;
  sharing?: boolean;
}

/** An in-call chat message displayed in the chat panel. */
export interface ChatEntry {
  id: string; // client-generated UUID for key
  fromId: string;
  fromName: string;
  text: string;
  sentAt: number; // Date.now()
}

/** A remote participant, built from the LiveKit room plus hub-only flags. */
export interface RemotePeer {
  id: string;
  name: string;
  /** Audio (+video) of the tile, used for level analysis and the "has video" test. */
  stream: MediaStream | null;
  /** LiveKit tracks attached to the tile's <video> (adaptive stream needs attach()). */
  audioTrack: Track | null;
  videoTrack: Track | null;
  audioMuted: boolean;
  videoOff: boolean;
  handRaised: boolean;
  sharing: boolean;
}

/** Someone waiting for the host to let them in. */
export interface Waiter {
  identity: string;
  name: string;
}

export interface SfuState {
  locked: boolean;
  waiting_room: boolean;
  is_host: boolean;
  recording_enabled: boolean;
  recording: boolean;
  can_record?: boolean;
  waiting: Waiter[];
  transcribing?: boolean;
}

export type HostAction =
  | "mute"
  | "mute_all"
  | "remove"
  | "lock"
  | "unlock"
  | "waiting_room_on"
  | "waiting_room_off"
  | "admit"
  | "deny"
  | "transcript_on"
  | "transcript_off"
  | "end";
