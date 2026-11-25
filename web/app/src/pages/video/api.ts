import type {
  Video,
  ListVideosResponse,
  VideoUpdateInput,
  VideoUserShare,
  VideoShareLink,
  VideoPublicInfo,
  VideoPlaylist,
  ListVideoPlaylistsResponse,
  ListVideoPlaylistVideosResponse,
  VideoProgress,
  VideoCaption,
  ListVideoCaptionsResponse,
  VideoInfo,
  VideoComment,
  Channel,
  MeetingRecording,
  VideoStats,
  DriveVideo,
  GroupRef,
  FeedParams,
  Visibility,
} from "./types";

const API_BASE = "/api/v1";

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${API_BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return (await resp.json()) as T;
}

export async function listVideos(): Promise<Video[]> {
  const r = await jsonFetch<ListVideosResponse>("/videos");
  return r.videos ?? [];
}

export function getVideo(id: string): Promise<Video> {
  return jsonFetch<Video>(`/videos/${id}`);
}

export function updateVideo(
  id: string,
  input: Partial<VideoUpdateInput>,
): Promise<Video> {
  return jsonFetch<Video>(`/videos/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export async function deleteVideo(id: string): Promise<void> {
  await jsonFetch<unknown>(`/videos/${id}`, { method: "DELETE" });
}

/** uploadUrl is the raw multipart upload endpoint (not gRPC-gateway). */
export const uploadUrl = `${API_BASE}/videos/upload`;

/** streamUrl returns the byte-stream path for a video (range-capable). */
export function streamUrl(id: string): string {
  return `${API_BASE}/videos/${id}/content`;
}

/** downloadUrl forces a save-to-disk disposition. */
export function downloadUrl(id: string): string {
  return `${API_BASE}/videos/${id}/content?download=1`;
}

// ---------------------------------------------------------------------------
// Sharing — targeted user shares
// ---------------------------------------------------------------------------

/** shareVideo grants the given user IDs access to a video. */
export async function shareVideo(
  videoId: string,
  userIds: string[],
): Promise<VideoUserShare[]> {
  const r = await jsonFetch<{ shares: VideoUserShare[] }>(
    `/videos/${videoId}/shares`,
    {
      method: "POST",
      body: JSON.stringify({ user_ids: userIds }),
    },
  );
  return r.shares ?? [];
}

/** listVideoShares returns the org users a video has been shared with. */
export async function listVideoShares(
  videoId: string,
): Promise<VideoUserShare[]> {
  const r = await jsonFetch<{ shares: VideoUserShare[] }>(
    `/videos/${videoId}/shares`,
  );
  return r.shares ?? [];
}

/** unshareVideo removes a targeted share grant for one user. */
export async function unshareVideo(
  videoId: string,
  userId: string,
): Promise<void> {
  await jsonFetch<unknown>(`/videos/${videoId}/shares/${userId}`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Sharing — public link tokens
// ---------------------------------------------------------------------------

/** createVideoShareLink creates a new public watch link. expiresAt is an
 *  optional ISO-8601 string; omit or pass "" for a never-expiring link. */
export async function createVideoShareLink(
  videoId: string,
  expiresAt = "",
): Promise<VideoShareLink> {
  return jsonFetch<VideoShareLink>(`/videos/${videoId}/share-links`, {
    method: "POST",
    body: JSON.stringify({ expires_at: expiresAt }),
  });
}

/** listVideoShareLinks returns the active public links for a video. */
export async function listVideoShareLinks(
  videoId: string,
): Promise<VideoShareLink[]> {
  const r = await jsonFetch<{ links: VideoShareLink[] }>(
    `/videos/${videoId}/share-links`,
  );
  return r.links ?? [];
}

/** revokeVideoShareLink revokes a public share link by token. */
export async function revokeVideoShareLink(token: string): Promise<void> {
  await jsonFetch<unknown>(`/videos/share-links/${token}`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Public (unauthenticated) share resolution
// ---------------------------------------------------------------------------

/** getSharedVideoInfo resolves a public share token to its video metadata.
 *  No session required. */
export async function getSharedVideoInfo(
  token: string,
): Promise<VideoPublicInfo> {
  const resp = await fetch(`${API_BASE}/videos/shared/${token}`, {
    headers: { Accept: "application/json" },
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return (await resp.json()) as VideoPublicInfo;
}

/** sharedStreamUrl returns the unauthenticated byte-stream URL for a token. */
export function sharedStreamUrl(token: string): string {
  return `${API_BASE}/videos/shared/${token}/content`;
}

// ---------------------------------------------------------------------------
// Playlists
// ---------------------------------------------------------------------------

export async function listVideoPlaylists(): Promise<VideoPlaylist[]> {
  const r = await jsonFetch<ListVideoPlaylistsResponse>("/video/playlists");
  return r.playlists ?? [];
}

export async function createVideoPlaylist(
  name: string,
): Promise<VideoPlaylist> {
  return jsonFetch<VideoPlaylist>("/video/playlists", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export async function updateVideoPlaylist(
  id: string,
  name: string,
): Promise<VideoPlaylist> {
  return jsonFetch<VideoPlaylist>(`/video/playlists/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ name }),
  });
}

export async function deleteVideoPlaylist(id: string): Promise<void> {
  await jsonFetch<unknown>(`/video/playlists/${id}`, { method: "DELETE" });
}

export async function addToVideoPlaylist(
  playlistId: string,
  videoId: string,
): Promise<void> {
  await jsonFetch<unknown>(`/video/playlists/${playlistId}/items`, {
    method: "POST",
    body: JSON.stringify({ video_id: videoId }),
  });
}

export async function removeFromVideoPlaylist(
  playlistId: string,
  videoId: string,
): Promise<void> {
  await jsonFetch<unknown>(`/video/playlists/${playlistId}/items/${videoId}`, {
    method: "DELETE",
  });
}

export async function listVideoPlaylistVideos(
  playlistId: string,
): Promise<Video[]> {
  const r = await jsonFetch<ListVideoPlaylistVideosResponse>(
    `/video/playlists/${playlistId}/items`,
  );
  return r.videos ?? [];
}

// ---------------------------------------------------------------------------
// Progress
// ---------------------------------------------------------------------------

export async function setVideoProgress(
  videoId: string,
  positionSeconds: number,
  percent: number,
): Promise<VideoProgress> {
  const r = await jsonFetch<{ progress: VideoProgress }>(
    `/videos/${videoId}/progress`,
    {
      method: "PUT",
      body: JSON.stringify({ position_seconds: positionSeconds, percent }),
    },
  );
  return r.progress;
}

export async function getVideoProgress(
  videoId: string,
): Promise<VideoProgress> {
  return jsonFetch<VideoProgress>(`/videos/${videoId}/progress`);
}

// ---------------------------------------------------------------------------
// Captions
// ---------------------------------------------------------------------------

export async function listVideoCaptions(
  videoId: string,
): Promise<VideoCaption[]> {
  const r = await jsonFetch<ListVideoCaptionsResponse>(
    `/videos/${videoId}/captions`,
  );
  return r.captions ?? [];
}

export async function deleteVideoCaption(
  videoId: string,
  id: string,
): Promise<void> {
  await jsonFetch<unknown>(`/videos/${videoId}/captions/${id}`, {
    method: "DELETE",
  });
}

/** captionUploadUrl returns the upload endpoint for a video's captions. */
export function captionUploadUrl(videoId: string): string {
  return `${API_BASE}/videos/${videoId}/captions/upload`;
}

/** uploadVideo posts a multipart form with optional metadata. Reports progress
 *  via the optional callback. Resolves to the created Video. */
export function uploadVideo(
  file: File,
  meta: {
    title?: string;
    description?: string;
    duration_seconds?: number;
    thumbnail_data_url?: string;
    visibility?: string;
    group_id?: string;
  },
  onProgress?: (fraction: number) => void,
): Promise<Video> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("file", file);
    if (meta.title) form.append("title", meta.title);
    if (meta.description) form.append("description", meta.description);
    if (meta.duration_seconds != null)
      form.append("duration_seconds", String(meta.duration_seconds));
    if (meta.thumbnail_data_url)
      form.append("thumbnail_data_url", meta.thumbnail_data_url);
    if (meta.visibility) form.append("visibility", meta.visibility);
    if (meta.group_id) form.append("group_id", meta.group_id);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", uploadUrl);
    xhr.withCredentials = true;
    xhr.upload.onprogress = (e) => {
      if (onProgress && e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText) as Video);
        } catch {
          reject(new Error("bad response"));
        }
      } else {
        reject(new Error(xhr.responseText || `HTTP ${xhr.status}`));
      }
    };
    xhr.onerror = () => reject(new Error("network error"));
    xhr.send(form);
  });
}

// ---------------------------------------------------------------------------
// Extras: feed, watch page, comments, subscriptions, history, Meet, Drive
// ---------------------------------------------------------------------------

const X = "/videos/x";

export async function getFeed(p: FeedParams = {}): Promise<VideoInfo[]> {
  const qs = new URLSearchParams();
  if (p.q) qs.set("q", p.q);
  if (p.duration) qs.set("duration", p.duration);
  if (p.date) qs.set("date", p.date);
  if (p.sort) qs.set("sort", p.sort);
  if (p.owner) qs.set("owner", p.owner);
  if (p.subs) qs.set("subs", "1");
  if (p.source) qs.set("source", p.source);
  const r = await jsonFetch<{ videos?: VideoInfo[] }>(`${X}/feed?${qs}`);
  return r.videos ?? [];
}

export const getVideoInfo = (id: string) => jsonFetch<VideoInfo>(`${X}/videos/${id}`);

export async function getRelated(id: string): Promise<VideoInfo[]> {
  const r = await jsonFetch<{ videos?: VideoInfo[] }>(`${X}/videos/${id}/related`);
  return r.videos ?? [];
}

export function updateVideoInfo(id: string, patch: { title?: string; description?: string; thumbnail_data_url?: string; visibility?: Visibility; group_id?: string }): Promise<VideoInfo> {
  return jsonFetch<VideoInfo>(`${X}/videos/${id}`, { method: "PATCH", body: JSON.stringify(patch) });
}

export async function recordView(id: string, seconds: number): Promise<void> {
  await jsonFetch<unknown>(`${X}/videos/${id}/view`, { method: "POST", body: JSON.stringify({ seconds }) }).catch(() => undefined);
}

export async function reportDuration(id: string, seconds: number): Promise<void> {
  await jsonFetch<unknown>(`${X}/videos/${id}/duration`, { method: "POST", body: JSON.stringify({ seconds }) }).catch(() => undefined);
}

export const getStats = (id: string) => jsonFetch<VideoStats>(`${X}/videos/${id}/stats`);

export async function setLiked(id: string, on: boolean): Promise<void> {
  await jsonFetch<unknown>(`${X}/videos/${id}/like`, { method: on ? "PUT" : "DELETE", body: on ? "{}" : undefined });
}

export async function getComments(id: string): Promise<VideoComment[]> {
  const r = await jsonFetch<{ comments?: VideoComment[] }>(`${X}/videos/${id}/comments`);
  return r.comments ?? [];
}

export async function addComment(id: string, body: string, parentId = ""): Promise<void> {
  await jsonFetch<unknown>(`${X}/videos/${id}/comments`, { method: "POST", body: JSON.stringify({ body, parent_id: parentId }) });
}

export function likeComment(commentId: string): Promise<{ liked: boolean; likes: number }> {
  return jsonFetch(`${X}/comments/${commentId}/like`, { method: "PUT", body: "{}" });
}

export async function deleteComment(commentId: string): Promise<void> {
  await jsonFetch<unknown>(`${X}/comments/${commentId}`, { method: "DELETE" });
}

export const getChannel = (userId: string) => jsonFetch<Channel>(`${X}/channels/${userId}`);

export async function subscribe(userId: string, on: boolean): Promise<void> {
  await jsonFetch<unknown>(`${X}/channels/${userId}/subscribe`, { method: on ? "PUT" : "DELETE", body: on ? "{}" : undefined });
}

export async function listSubscriptions(): Promise<Channel[]> {
  const r = await jsonFetch<{ channels?: Channel[] }>(`${X}/subscriptions`);
  return r.channels ?? [];
}

export async function getWatchLater(): Promise<VideoInfo[]> {
  const r = await jsonFetch<{ videos?: VideoInfo[] }>(`${X}/watch-later`);
  return r.videos ?? [];
}

export async function setWatchLater(id: string, on: boolean): Promise<void> {
  await jsonFetch<unknown>(`${X}/watch-later/${id}`, { method: on ? "PUT" : "DELETE", body: on ? "{}" : undefined });
}

export async function getHistory(): Promise<VideoInfo[]> {
  const r = await jsonFetch<{ videos?: VideoInfo[] }>(`${X}/history`);
  return r.videos ?? [];
}

export async function clearHistory(): Promise<void> {
  await jsonFetch<unknown>(`${X}/history`, { method: "DELETE" });
}

export async function getMeetings(): Promise<MeetingRecording[]> {
  const r = await jsonFetch<{ recordings?: MeetingRecording[] }>(`${X}/meetings`);
  return r.recordings ?? [];
}

export const openMeeting = (recId: string) => jsonFetch<VideoInfo>(`${X}/meetings/${recId}/open`, { method: "POST", body: "{}" });

export async function listGroups(): Promise<GroupRef[]> {
  const r = await jsonFetch<{ groups?: GroupRef[] }>(`${X}/groups`);
  return r.groups ?? [];
}

export async function listDriveVideos(q = ""): Promise<DriveVideo[]> {
  const r = await jsonFetch<{ files?: DriveVideo[] }>(`${X}/drive-files?q=${encodeURIComponent(q)}`);
  return r.files ?? [];
}

export const importVideoFromDrive = (fileId: string, title = "", visibility: Visibility = "private") =>
  jsonFetch<VideoInfo>(`${X}/import-drive`, { method: "POST", body: JSON.stringify({ file_id: fileId, title, visibility }) });
