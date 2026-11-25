import type {
  Track,
  Playlist,
  ListTracksResponse,
  ListPlaylistsResponse,
  TrackUpdateInput,
  PlaylistInput,
  Station,
  RetentionMode,
  RadioCache,
  PlaylistInfo,
  GroupRef,
  HistoryPlay,
  DriveAudio,
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

// --- Tracks ---------------------------------------------------------------

export async function listTracks(): Promise<Track[]> {
  const r = await jsonFetch<ListTracksResponse>("/music/tracks");
  return r.tracks ?? [];
}

export function getTrack(id: string): Promise<Track> {
  return jsonFetch<Track>(`/music/tracks/${id}`);
}

export function updateTrack(
  id: string,
  input: Partial<TrackUpdateInput>,
): Promise<Track> {
  return jsonFetch<Track>(`/music/tracks/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export async function deleteTrack(id: string): Promise<void> {
  await jsonFetch<unknown>(`/music/tracks/${id}`, { method: "DELETE" });
}

// --- Playlists ------------------------------------------------------------

export async function listPlaylists(): Promise<Playlist[]> {
  const r = await jsonFetch<ListPlaylistsResponse>("/music/playlists");
  return r.playlists ?? [];
}

export function getPlaylist(id: string): Promise<Playlist> {
  return jsonFetch<Playlist>(`/music/playlists/${id}`);
}

export function createPlaylist(input: PlaylistInput): Promise<Playlist> {
  return jsonFetch<Playlist>("/music/playlists", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updatePlaylist(
  id: string,
  input: Partial<PlaylistInput>,
): Promise<Playlist> {
  return jsonFetch<Playlist>(`/music/playlists/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export async function deletePlaylist(id: string): Promise<void> {
  await jsonFetch<unknown>(`/music/playlists/${id}`, { method: "DELETE" });
}

export function addTrackToPlaylist(
  playlistId: string,
  trackId: string,
): Promise<Playlist> {
  return jsonFetch<Playlist>(`/music/playlists/${playlistId}/tracks`, {
    method: "POST",
    body: JSON.stringify({ playlist_id: playlistId, track_id: trackId }),
  });
}

export function removeTrackFromPlaylist(
  playlistId: string,
  trackId: string,
): Promise<Playlist> {
  return jsonFetch<Playlist>(
    `/music/playlists/${playlistId}/tracks/${trackId}`,
    { method: "DELETE" },
  );
}

export function reorderPlaylistTrack(
  playlistId: string,
  trackId: string,
  newPosition: number,
): Promise<Playlist> {
  return jsonFetch<Playlist>(
    `/music/playlists/${playlistId}/tracks/${trackId}/position`,
    {
      method: "PATCH",
      body: JSON.stringify({
        playlist_id: playlistId,
        track_id: trackId,
        new_position: newPosition,
      }),
    },
  );
}

// --- Likes ----------------------------------------------------------------

export async function likeTrack(trackId: string): Promise<void> {
  await jsonFetch<unknown>(`/music/tracks/${trackId}/like`, {
    method: "POST",
    body: "{}",
  });
}

export async function unlikeTrack(trackId: string): Promise<void> {
  await jsonFetch<unknown>(`/music/tracks/${trackId}/like`, {
    method: "DELETE",
  });
}

export async function listLikedTracks(): Promise<Track[]> {
  const r = await jsonFetch<{ tracks?: Track[] }>("/music/liked");
  return r.tracks ?? [];
}

// --- Radio ----------------------------------------------------------------

export async function listStations(): Promise<Station[]> {
  return (await listRadio()).stations;
}

/** listRadio returns the stations plus radio cache usage (when reported). */
export async function listRadio(): Promise<{
  stations: Station[];
  cache: RadioCache | null;
}> {
  const r = await jsonFetch<{ stations?: Station[]; cache?: RadioCache }>(
    "/music/radio/stations",
  );
  return { stations: r.stations ?? [], cache: r.cache ?? null };
}

/** createStation adds a custom radio station (any http(s) stream URL). */
export async function createStation(input: {
  name: string;
  stream_url: string;
  genre?: string;
}): Promise<Station> {
  const r = await jsonFetch<{ station: Station }>("/music/radio/stations", {
    method: "POST",
    body: JSON.stringify(input),
  });
  return r.station;
}

/** playStation validates the station and returns it; server-side caching runs
 *  while the play_url stream is open. */
export function playStation(id: string): Promise<Station> {
  return jsonFetch<Station>(`/music/radio/${id}/play`, {
    method: "POST",
    body: "{}",
  });
}

export async function stopStation(id: string): Promise<void> {
  await fetch(`${API_BASE}/music/radio/${id}/stop`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
}

export function setStationRetention(
  id: string,
  mode: RetentionMode,
  days: number,
): Promise<Station> {
  return jsonFetch<Station>(`/music/radio/${id}/retention`, {
    method: "PUT",
    body: JSON.stringify({ retention_mode: mode, retention_days: days }),
  });
}

// --- Blobs (raw HTTP, not gRPC-gateway) -----------------------------------

/** uploadUrl is the raw multipart upload endpoint. */
export const uploadUrl = `${API_BASE}/music/upload`;

/** streamUrl returns the byte-stream path for a track (range-capable). */
export function streamUrl(id: string): string {
  return `${API_BASE}/music/${id}/content`;
}

/** downloadUrl forces a save-to-disk disposition. */
export function downloadUrl(id: string): string {
  return `${API_BASE}/music/${id}/content?download=1`;
}

/** uploadTrack posts a multipart form with optional metadata. Reports progress
 *  via the optional callback. Resolves to the created Track. */
export function uploadTrack(
  file: File,
  meta: {
    title?: string;
    artist?: string;
    album?: string;
    duration_seconds?: number;
    artwork_data_url?: string;
    /** Text of a .lrc / .txt lyrics file picked together with the track. */
    lyrics?: string;
  },
  onProgress?: (fraction: number) => void,
): Promise<Track> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("file", file);
    if (meta.title) form.append("title", meta.title);
    if (meta.artist) form.append("artist", meta.artist);
    if (meta.album) form.append("album", meta.album);
    if (meta.duration_seconds != null)
      form.append("duration_seconds", String(meta.duration_seconds));
    if (meta.artwork_data_url)
      form.append("artwork_data_url", meta.artwork_data_url);
    if (meta.lyrics) form.append("lyrics", meta.lyrics);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", uploadUrl);
    xhr.withCredentials = true;
    xhr.upload.onprogress = (e) => {
      if (onProgress && e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText) as Track);
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

// --- Extras (library with stats, history, lyrics, sharing, Drive) ---------

const X = "/music/x";

export async function getLibrary(): Promise<Track[]> {
  const r = await jsonFetch<{ tracks?: Track[] }>(`${X}/library`);
  return r.tracks ?? [];
}

export async function getSmart(kind: "most_played" | "recently_played" | "recently_added" | "liked", limit = 50): Promise<Track[]> {
  const r = await jsonFetch<{ tracks?: Track[] }>(`${X}/smart/${kind}?limit=${limit}`);
  return r.tracks ?? [];
}

export async function recordPlay(trackId: string, seconds: number): Promise<void> {
  await jsonFetch<unknown>(`${X}/plays`, { method: "POST", body: JSON.stringify({ track_id: trackId, seconds }) }).catch(() => undefined);
}

export async function getHistory(limit = 60): Promise<HistoryPlay[]> {
  const r = await jsonFetch<{ plays?: HistoryPlay[] }>(`${X}/history?limit=${limit}`);
  return r.plays ?? [];
}

export async function getLyrics(trackId: string): Promise<{ lyrics: string; timed: boolean }> {
  return jsonFetch<{ lyrics: string; timed: boolean }>(`${X}/tracks/${trackId}/lyrics`);
}

export async function setLyrics(trackId: string, lyrics: string): Promise<void> {
  await jsonFetch<unknown>(`${X}/tracks/${trackId}/lyrics`, { method: "PUT", body: JSON.stringify({ lyrics }) });
}

export function rescanTrack(trackId: string): Promise<Track> {
  return jsonFetch<Track>(`${X}/tracks/${trackId}/rescan`, { method: "POST", body: "{}" });
}

export async function listPlaylistInfos(): Promise<PlaylistInfo[]> {
  const r = await jsonFetch<{ playlists?: PlaylistInfo[] }>(`${X}/playlists`);
  return r.playlists ?? [];
}

export function getPlaylistFull(id: string): Promise<{ playlist: PlaylistInfo; tracks: Track[] }> {
  return jsonFetch<{ playlist: PlaylistInfo; tracks: Track[] }>(`${X}/playlists/${id}`);
}

export async function setPlaylistSharing(id: string, visibility: Visibility, groupId = ""): Promise<void> {
  await jsonFetch<unknown>(`${X}/playlists/${id}`, { method: "PATCH", body: JSON.stringify({ visibility, group_id: groupId }) });
}

export async function reorderPlaylist(id: string, trackIds: string[]): Promise<void> {
  await jsonFetch<unknown>(`${X}/playlists/${id}/order`, { method: "PUT", body: JSON.stringify({ track_ids: trackIds }) });
}

export async function listGroups(): Promise<GroupRef[]> {
  const r = await jsonFetch<{ groups?: GroupRef[] }>(`${X}/groups`);
  return r.groups ?? [];
}

export async function listDriveAudio(q = ""): Promise<DriveAudio[]> {
  const r = await jsonFetch<{ files?: DriveAudio[] }>(`${X}/drive-files?q=${encodeURIComponent(q)}`);
  return r.files ?? [];
}

export function importFromDrive(fileId: string): Promise<Track> {
  return jsonFetch<Track>(`${X}/import-drive`, { method: "POST", body: JSON.stringify({ file_id: fileId }) });
}
