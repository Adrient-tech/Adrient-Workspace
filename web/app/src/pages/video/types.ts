/** Video mirrors adrientv1.Video (proto snake_case via the gateway). */
export interface Video {
  id: string;
  org_id: string;
  owner_id: string;
  title: string;
  description: string;
  content_type: string;
  size: number;
  duration_seconds: number;
  thumbnail_data_url: string;
  stream_url: string;
  created_at: string;
  updated_at: string;
  progress_seconds?: number;
  progress_percent?: number;
  watched?: boolean;
}

export interface ListVideosResponse {
  videos: Video[];
}

/** VideoUpdateInput is the editable metadata sent on update. */
export interface VideoUpdateInput {
  title: string;
  description: string;
  thumbnail_data_url: string;
}

/** VideoUserShare records that a video has been shared with a specific org user. */
export interface VideoUserShare {
  video_id: string;
  user_id: string;
  user_name: string;
  user_email: string;
  created_at: string;
}

/** VideoShareLink is a public watch link token (YouTube-style). */
export interface VideoShareLink {
  token: string;
  video_id: string;
  org_id: string;
  created_by: string;
  /** ISO-8601 expiry; empty means never expires. */
  expires_at: string;
  created_at: string;
  /** Full public watch URL e.g. https://host/video/watch/{token}. */
  url: string;
}

/** VideoPublicInfo is returned by the unauthenticated token-resolution endpoint. */
export interface VideoPublicInfo {
  title: string;
  description: string;
  content_type: string;
  duration_seconds: number;
  thumbnail_data_url: string;
  /** Relative URL to stream the video (no auth required when using a share token). */
  content_url: string;
}

/** VideoPlaylist mirrors adrientv1.VideoPlaylist. */
export interface VideoPlaylist {
  id: string;
  org_id: string;
  owner_user_id: string;
  name: string;
  created_at: string;
  item_count: number;
}

export interface ListVideoPlaylistsResponse {
  playlists: VideoPlaylist[];
}

export interface ListVideoPlaylistVideosResponse {
  videos: Video[];
}

/** VideoProgress mirrors adrientv1.VideoProgress. */
export interface VideoProgress {
  video_id: string;
  position_seconds: number;
  percent: number;
  watched: boolean;
  updated_at: string;
}

/** VideoCaption mirrors adrientv1.VideoCaption. */
export interface VideoCaption {
  id: string;
  org_id: string;
  video_id: string;
  lang: string;
  label: string;
  /** Relative URL to fetch the .vtt bytes. */
  stream_url: string;
  created_at: string;
}

export interface ListVideoCaptionsResponse {
  captions: VideoCaption[];
}

export type Visibility = "private" | "org" | "group";

/** VideoInfo is a video from the extras API (owner, counts, state of the caller). */
export interface VideoInfo {
  id: string;
  title: string;
  description: string;
  owner_id: string;
  owner_name: string;
  mine: boolean;
  visibility: Visibility;
  group_id: string;
  status: "ready" | "processing";
  duration_seconds: number;
  size: number;
  content_type: string;
  thumb_url: string;
  stream_url: string;
  source_kind: string;
  created_at: string;
  views: number;
  likes: number;
  comments: number;
  liked: boolean;
  progress_seconds: number;
  progress_percent: number;
  watched: boolean;
  watch_later: boolean;
  subscribed: boolean;
  has_captions: boolean;
}

export interface VideoComment {
  id: string;
  parent_id: string;
  user_id: string;
  user_name: string;
  body: string;
  created_at: string;
  likes: number;
  liked: boolean;
  mine: boolean;
  can_delete: boolean;
}

export interface Channel {
  user_id: string;
  name: string;
  subscribers: number;
  videos: number;
  subscribed: boolean;
  mine: boolean;
}

export interface MeetingRecording {
  id: string;
  room: string;
  started_at: string;
  finished_at: string | null;
  file_name: string;
  size: number;
  video_id: string;
}

export interface VideoStats {
  views: number;
  unique_viewers: number;
  watch_seconds: number;
  avg_seconds: number;
  likes: number;
  comments: number;
  daily: { day: string; views: number }[];
}

export interface DriveVideo {
  id: string;
  name: string;
  size: number;
  imported: boolean;
}

export interface GroupRef {
  id: string;
  name: string;
}

export interface FeedParams {
  q?: string;
  duration?: "" | "short" | "medium" | "long";
  date?: "" | "today" | "week" | "month" | "year";
  sort?: "" | "new" | "oldest" | "views" | "title";
  owner?: string;
  subs?: boolean;
  source?: string;
}
