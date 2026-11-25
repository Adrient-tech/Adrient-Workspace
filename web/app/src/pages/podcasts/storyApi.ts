/** Stories API (/api/v1/stories). All calls use the session cookie. */
import type { Sentence, Timing } from "./playerLogic";

const BASE = "/api/v1/stories";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`${BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(init?.body && typeof init.body === "string" ? { "Content-Type": "application/json" } : {}) },
    ...init,
  });
  if (!r.ok) {
    const t = (await r.text().catch(() => "")).trim();
    throw new ApiError(r.status, t || `HTTP ${r.status}`);
  }
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

const json = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });

export interface Person {
  id: string;
  name: string;
}
export interface Show {
  id: string;
  title: string;
  description: string;
  genre: string;
  language: string;
  cover_url?: string;
  cover_style: string;
  author: Person;
  audience: "everyone" | "group" | "department";
  audience_id?: string;
  comments_on: boolean;
  allow_download: boolean;
  provider: string;
  voice: string;
  voice_map: Record<string, string>;
  speed: number;
  pitch: number;
  episodes: number;
  published: number;
  total_ms: number;
  following: boolean;
  in_library: boolean;
  mine: boolean;
  followers: number;
  updated_at: string;
  created_at: string;
}
export type EpisodeStatus = "draft" | "scheduled" | "published" | "unpublished";
export type AudioStatus = "none" | "queued" | "generating" | "ready" | "failed";
export interface Episode {
  id: string;
  show_id: string;
  show_title: string;
  position: number;
  title: string;
  summary: string;
  status: EpisodeStatus;
  publish_at?: string;
  published_at?: string;
  audio_status: AudioStatus;
  audio_error?: string;
  has_audio: boolean;
  duration_ms: number;
  likes: number;
  liked: boolean;
  saved: boolean;
  comments: number;
  position_ms: number;
  progress_duration_ms: number;
  completed: boolean;
  author: Person;
  cover_style: string;
  cover_url?: string;
  genre: string;
  language: string;
  updated_at: string;
}
export interface Effective {
  provider: string;
  provider_name: string;
  voice: string;
  voice_map: Record<string, string>;
  speed: number;
  pitch: number;
  language: string;
  mode: "server" | "browser";
}
export interface Override {
  provider?: string | null;
  voice?: string | null;
  voice_map?: Record<string, string> | null;
  speed?: number | null;
  pitch?: number | null;
}
export interface JobInfo {
  id: string;
  status: "queued" | "running" | "done" | "failed" | "cancelled";
  progress: number;
  error?: string;
  chars: number;
  created_at: string;
  finished_at?: string;
}
export interface EpisodeDetail extends Episode {
  body_html: string;
  sentences: Sentence[];
  timings: Timing[];
  audio_url?: string;
  audio_mime?: string;
  audio_stale: boolean;
  effective: Effective;
  override: Override;
  job?: JobInfo;
  can_edit: boolean;
  comments_on: boolean;
  can_download: boolean;
  show_description: string;
  audience: string;
  chars: number;
  max_chars: number;
  next_episode_id?: string;
}
export interface Home {
  continue: Episode[];
  new: Episode[];
  following: Episode[];
  trending: Show[];
  genres: { genre: string; count: number }[];
  my_stories: number;
}
export interface Comment {
  id: string;
  episode_id: string;
  author: Person;
  body: string;
  status: "visible" | "hidden";
  mine: boolean;
  reports: number;
  reported: boolean;
  created_at: string;
  episode?: string;
}
export interface Voice {
  id: string;
  name: string;
  language?: string;
}
export interface VoiceList {
  provider_id: string;
  name: string;
  kind: "openai" | "command";
  default_voice: string;
  is_default: boolean;
  voices: Voice[];
  error?: string;
}
export interface Quota {
  used_chars: number;
  daily_chars: number;
  per_minute_jobs: number;
  max_chars: number;
}
export interface EpisodeStats {
  id: string;
  title: string;
  status: string;
  plays: number;
  listeners: number;
  completed: number;
  completion_rate: number;
  avg_percent: number;
  likes: number;
  saves: number;
  comments: number;
}
export interface ShowStats {
  plays: number;
  listeners: number;
  followers: number;
  likes: number;
  completion_rate: number;
  episodes: EpisodeStats[];
  daily: { day: string; plays: number }[];
}

export interface ShowInput {
  title?: string;
  description?: string;
  genre?: string;
  language?: string;
  cover_style?: string;
  audience?: string;
  audience_id?: string;
  comments_on?: boolean;
  allow_download?: boolean;
  provider?: string;
  voice?: string;
  voice_map?: Record<string, string>;
  speed?: number;
  pitch?: number;
}
export interface EpisodeInput {
  title?: string;
  summary?: string;
  body_html?: string;
  position?: number;
  provider?: string;
  voice?: string;
  voice_map?: Record<string, string>;
  speed?: number;
  pitch?: number;
  inherit?: string[];
}

export const stories = {
  home: () => call<Home>("/home"),
  search: (q: string) => call<{ shows: Show[]; episodes: Episode[] }>(`/search?q=${encodeURIComponent(q)}`),
  genres: () => call<{ genres: string[] }>("/genres").then((r) => r.genres),
  audiences: () => call<{ audiences: { kind: string; id: string; name: string }[] }>("/audiences").then((r) => r.audiences),
  voices: () => call<{ providers: VoiceList[] }>("/voices").then((r) => r.providers),
  quota: () => call<{ quota: Quota; can_publish: boolean; can_generate: boolean }>("/quota"),
  shows: (q: { genre?: string; q?: string; mine?: boolean; sort?: string; following?: boolean } = {}) => {
    const p = new URLSearchParams();
    if (q.genre) p.set("genre", q.genre);
    if (q.q) p.set("q", q.q);
    if (q.mine) p.set("mine", "1");
    if (q.sort) p.set("sort", q.sort);
    if (q.following) p.set("following", "1");
    return call<{ shows: Show[] }>(`/shows?${p}`).then((r) => r.shows);
  },
  show: (id: string) => call<{ show: Show; episodes: Episode[] }>(`/shows/${id}`),
  createShow: (in_: ShowInput) => call<Show>("/shows", json("POST", in_)),
  updateShow: (id: string, in_: ShowInput) => call<Show>(`/shows/${id}`, json("PATCH", in_)),
  deleteShow: (id: string) => call<{ ok: boolean }>(`/shows/${id}`, json("DELETE")),
  setCover: (id: string, blob: Blob) => call<Show>(`/shows/${id}/cover`, { method: "PUT", body: blob, headers: { "Content-Type": blob.type || "application/octet-stream" } }),
  clearCover: (id: string) => call<Show>(`/shows/${id}/cover`, json("DELETE")),
  stats: (id: string) => call<ShowStats>(`/shows/${id}/stats`),
  createEpisode: (showId: string, in_: EpisodeInput) => call<EpisodeDetail>(`/shows/${showId}/episodes`, json("POST", in_)),
  episode: (id: string) => call<EpisodeDetail>(`/episodes/${id}`),
  updateEpisode: (id: string, in_: EpisodeInput) => call<EpisodeDetail>(`/episodes/${id}`, json("PATCH", in_)),
  deleteEpisode: (id: string) => call<{ ok: boolean }>(`/episodes/${id}`, json("DELETE")),
  publish: (id: string, at?: string) => call<EpisodeDetail>(`/episodes/${id}/publish`, json("POST", at ? { at } : {})),
  unpublish: (id: string) => call<EpisodeDetail>(`/episodes/${id}/unpublish`, json("POST", {})),
  generateAudio: (id: string, force = false) => call<{ job: JobInfo }>(`/episodes/${id}/audio`, json("POST", { force })),
  saveProgress: (id: string, position_ms: number, duration_ms: number, completed = false) =>
    call<{ ok: boolean }>(`/episodes/${id}/progress`, json("PUT", { position_ms: Math.round(position_ms), duration_ms: Math.round(duration_ms), completed })),
  play: (id: string) => call<{ ok: boolean }>(`/episodes/${id}/play`, json("POST", {})),
  markPlayed: (id: string, played: boolean) => call<{ ok: boolean }>(`/episodes/${id}/played`, json(played ? "PUT" : "DELETE")),
  like: (id: string, on: boolean) => call<{ ok: boolean }>(`/episodes/${id}/like`, json(on ? "PUT" : "DELETE")),
  library: () => call<{ episodes: Episode[]; shows: Show[]; liked: Episode[]; history: Episode[] }>("/library"),
  save: (kind: "show" | "episode", id: string, on: boolean) => call<{ ok: boolean }>(`/library/${kind}/${id}`, json(on ? "PUT" : "DELETE")),
  follows: () => call<{ authors: string[] }>("/follows").then((r) => r.authors),
  follow: (kind: "show" | "author", id: string, on: boolean) => call<{ ok: boolean }>(`/follows/${kind}/${id}`, json(on ? "PUT" : "DELETE")),
  comments: (id: string) => call<{ comments: Comment[] }>(`/episodes/${id}/comments`).then((r) => r.comments),
  addComment: (id: string, body: string) => call<Comment>(`/episodes/${id}/comments`, json("POST", { body })),
  commentAction: (id: string, action: "report" | "hide" | "show" | "dismiss" | "delete", reason?: string) =>
    call<{ ok: boolean }>(`/comments/${id}/${action}`, json("POST", reason ? { reason } : {})),
  moderation: () => call<{ comments: Comment[] }>("/moderation").then((r) => r.comments),
};

// ---- admin (Voices) ------------------------------------------------------------

export interface ProviderRow {
  id: string;
  name: string;
  kind: "openai" | "command";
  base_url: string;
  key_env: string;
  key_source: "" | "stored" | "env";
  model: string;
  format: string;
  command: string;
  command_found: boolean;
  args: string[];
  voices_dir: string;
  voices: Voice[];
  default_voice: string;
  enabled: boolean;
  is_default: boolean;
}
export interface ProviderInput {
  name: string;
  kind: "openai" | "command";
  base_url?: string;
  api_key?: string;
  clear_key?: boolean;
  key_env?: string;
  model?: string;
  format?: string;
  command?: string;
  args?: string[];
  voices_dir?: string;
  voices?: Voice[];
  default_voice?: string;
  enabled?: boolean;
  is_default?: boolean;
}
export interface Preset {
  id: string;
  name: string;
  command: string;
  path: string;
  args: string[];
  found: boolean;
  note?: string;
}
export interface VoiceSettings {
  daily_chars: number;
  per_minute_jobs: number;
  max_chars: number;
  auto_generate: boolean;
}
export interface JobRow {
  id: string;
  episode: string;
  episode_id: string;
  user: string;
  status: string;
  progress: number;
  chars: number;
  error?: string;
  created_at: string;
  finished_at?: string;
}

export const voicesAdmin = {
  providers: () => call<{ providers: ProviderRow[] }>("/admin/providers").then((r) => r.providers),
  create: (in_: ProviderInput) => call<ProviderRow>("/admin/providers", json("POST", in_)),
  update: (id: string, in_: ProviderInput) => call<ProviderRow>(`/admin/providers/${id}`, json("PUT", in_)),
  remove: (id: string) => call<{ ok: boolean }>(`/admin/providers/${id}`, json("DELETE")),
  test: (id: string, voice?: string) => call<{ ok: boolean; error?: string; duration_ms: number; voices: number; latency_ms: number }>(`/admin/providers/${id}/test`, json("POST", { voice })),
  detect: () => call<{ presets: Preset[] }>("/admin/detect").then((r) => r.presets),
  settings: () => call<VoiceSettings>("/admin/settings"),
  saveSettings: (s: VoiceSettings) => call<VoiceSettings>("/admin/settings", json("PUT", s)),
  jobs: () => call<{ jobs: JobRow[] }>("/admin/jobs").then((r) => r.jobs),
};
