/** Track mirrors adrientv1.Track (proto snake_case via the gateway). */
export interface Track {
  id: string;
  org_id: string;
  owner_id: string;
  title: string;
  artist: string;
  album: string;
  content_type: string;
  size: number;
  duration_seconds: number;
  /** Legacy inline cover (gRPC responses); the app reads covers through art_url. */
  artwork_data_url?: string;
  stream_url: string;
  created_at: string;
  updated_at: string;
  /** liked is true when the calling user has liked this track. */
  liked?: boolean;
  /** Cover image URL (served from the track's embedded or uploaded art). */
  art_url?: string;
  genre?: string;
  track_no?: number;
  year?: number;
  has_lyrics?: boolean;
  /** The caller's play count and last play time. */
  play_count?: number;
  last_played?: string | null;
}

/** Playlist visibility: only the owner, the whole organisation, or one group. */
export type Visibility = "private" | "org" | "group";

/** PlaylistInfo is a playlist from the extras API (visibility, owner, covers). */
export interface PlaylistInfo {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  owner_name: string;
  mine: boolean;
  visibility: Visibility;
  group_id: string;
  group_name: string;
  track_count: number;
  cover_art: string[];
  updated_at: string;
}

export interface GroupRef {
  id: string;
  name: string;
}

export interface HistoryPlay {
  play_id: string;
  played_at: string;
  seconds_listened: number;
  track_id: string;
  title: string;
  artist: string;
  album: string;
  duration_seconds: number;
  art_url: string;
  stream_url: string;
}

export interface DriveAudio {
  id: string;
  name: string;
  size: number;
  imported: boolean;
}

/** Playlist mirrors adrientv1.Playlist. List responses omit `tracks` and carry
 *  only `track_count`; detail/mutation responses populate `tracks`. */
export interface Playlist {
  id: string;
  org_id: string;
  owner_id: string;
  name: string;
  description: string;
  tracks: Track[];
  track_count: number;
  created_at: string;
  updated_at: string;
}

export interface ListTracksResponse {
  tracks: Track[];
}

export interface ListPlaylistsResponse {
  playlists: Playlist[];
}

/** TrackUpdateInput is the editable metadata sent on update. */
export interface TrackUpdateInput {
  title: string;
  artist: string;
  album: string;
  artwork_data_url: string;
}

/** PlaylistInput is the editable metadata sent on create/update. */
export interface PlaylistInput {
  name: string;
  description: string;
}

/** RetentionMode mirrors the backend: keep songs (until the server-wide radio
 *  cache limit evicts them), or erase after N days. */
export type RetentionMode = "keep" | "days";

/** Station mirrors the radio_station JSON served by the music HTTP layer. */
export interface Station {
  id: string;
  org_id: string;
  name: string;
  stream_url: string;
  genre: string;
  logo_url: string;
  retention_mode: RetentionMode;
  retention_days: number;
  track_count: number;
  /** play_url is adrient's same-origin live-stream proxy for the <audio> element. */
  play_url: string;
}

/** RadioCache is the radio cache usage returned with the station list:
 *  used_bytes/songs are this org's cached radio songs; max_bytes/max_days are
 *  the server-wide limits (0 = unlimited). Liked or playlisted songs are never
 *  evicted. */
export interface RadioCache {
  used_bytes: number;
  songs: number;
  max_bytes: number;
  max_days: number;
}
