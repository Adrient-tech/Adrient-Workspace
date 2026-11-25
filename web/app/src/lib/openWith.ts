/**
 * openWith — the single place that decides which Adrient app opens a thing.
 *
 * Every app (Chat, Mail, Drive, Calendar, notifications, search) passes a
 * URL, a file or an object reference here and gets back where to go. That
 * keeps the "handshake" between apps identical everywhere:
 *   - same-origin workspace links open in-app (no reload, no new tab)
 *   - meeting links/codes open Meet with a way back
 *   - files open in Docs / Sheets / Slides / PDF / Whiteboard / 3D / viewer
 *   - foreign links stay external (new tab)
 */

export type AppId =
  | "docs" | "sheets" | "slides" | "pdf" | "whiteboard" | "drive" | "meet" | "mail"
  | "chat" | "calendar" | "contacts" | "tasks" | "keep" | "projects" | "crm" | "talent" | "hr" | "invoicing" | "records" | "wiki" | "contracts" | "signatures" | "legal" | "captable"
  | "inventory" | "pos" | "accounting" | "tickets" | "forms" | "photos" | "video" | "music"
  | "books" | "sites" | "3d" | "admin" | "settings" | "other";

export interface OpenTarget {
  /** In-app route (starts with "/") or an absolute external URL. */
  href: string;
  app: AppId;
  /** True when href is outside the workspace and must open in a new tab. */
  external: boolean;
  /** Human label for buttons ("Open in Sheets"). */
  label: string;
  /** Whether the target can be shown inside another app's side pane. */
  embeddable: boolean;
}

const APP_LABEL: Partial<Record<AppId, string>> = {
  docs: "Docs", sheets: "Sheets", slides: "Slides", pdf: "PDF", whiteboard: "Whiteboard",
  drive: "Drive", meet: "Meet", mail: "Mail", chat: "Chat", calendar: "Calendar",
  contacts: "Contacts", tasks: "Tasks", keep: "Keep", projects: "Projects", crm: "CRM", talent: "Talent", hr: "HR", records: "Records", wiki: "Wiki", contracts: "Contracts", signatures: "Signatures", legal: "Legal", captable: "Cap table",
  invoicing: "Invoicing", inventory: "Inventory", pos: "POS", accounting: "Accounting",
  tickets: "Tickets", forms: "Forms", photos: "Photos", video: "Video", music: "Music",
  books: "Books", sites: "Sites", "3d": "3D", admin: "Admin", settings: "Settings",
};

export function appLabel(app: AppId): string {
  return APP_LABEL[app] ?? "Workspace";
}

const MEET_CODE = /^[a-z]{3}-[a-z]{4}-[a-z]{3}$/;
const KNOWN_APPS = new Set(Object.keys(APP_LABEL));

/** Apps whose pages are safe and useful inside a side pane. */
const EMBEDDABLE = new Set<AppId>([
  "docs", "sheets", "slides", "pdf", "whiteboard", "drive", "tasks", "keep", "calendar",
  "contacts", "crm", "invoicing", "inventory", "projects", "tickets", "forms", "photos", "video",
]);

function target(href: string, app: AppId, external = false): OpenTarget {
  return {
    href,
    app,
    external,
    label: external ? "Open link" : `Open in ${appLabel(app)}`,
    embeddable: !external && EMBEDDABLE.has(app),
  };
}

/** App id for an in-app path ("/sheets/123" → "sheets"). */
export function appForPath(path: string): AppId {
  const seg = path.split(/[/?#]/).filter(Boolean)[0] ?? "";
  if (seg === "drive") return "drive";
  if (seg === "org-chart") return "contacts";
  if (seg === "invoice") return "invoicing";
  if (seg === "live") return "video";
  return (KNOWN_APPS.has(seg) ? seg : "other") as AppId;
}

/**
 * Classifies a link. `origin` defaults to the current page origin; links to
 * the same workspace host (or any host listed in `aliases`) become in-app
 * routes. Bare meeting codes ("abc-defg-hij") open Meet.
 */
export function openLink(raw: string, origin = window.location.origin, aliases: string[] = []): OpenTarget | null {
  const s = raw.trim();
  if (!s) return null;
  if (MEET_CODE.test(s)) return target(`/meet/${s}`, "meet");
  if (s.startsWith("/") && !s.startsWith("//")) return target(s, appForPath(s));
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    return null;
  }
  if (u.protocol === "mailto:") {
    const to = decodeURIComponent(u.pathname);
    return target(`/mail?compose=1&to=${encodeURIComponent(to)}`, "mail");
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return null;
  const local = new URL(origin);
  const sameHost = u.host === local.host || aliases.includes(u.host);
  if (sameHost) {
    const path = u.pathname + u.search + u.hash;
    return target(path || "/", appForPath(path));
  }
  if (isExternalMeeting(u)) {
    return { ...target(u.toString(), "meet", true), label: "Join video meeting" };
  }
  return target(u.toString(), "other", true);
}

const MEETING_HOSTS = [
  "meet.google.com", "zoom.us", "teams.microsoft.com", "teams.live.com",
  "webex.com", "whereby.com", "meet.jit.si",
];

/** True for Google Meet / Zoom / Teams / Webex style meeting links on other hosts. */
export function isExternalMeeting(u: URL): boolean {
  const h = u.hostname.toLowerCase();
  return MEETING_HOSTS.some((m) => h === m || h.endsWith(`.${m}`));
}

/**
 * Resolves an <a href> for the global click handler. Returns the in-app
 * path when the link belongs to the workspace, otherwise null.
 */
export function inAppHref(raw: string, origin = window.location.origin, aliases: string[] = []): string | null {
  const t = openLink(raw, origin, aliases);
  if (!t || t.external) return null;
  return t.href;
}

/** Minimal file shape every app can provide (Drive file, mail or chat attachment). */
export interface FileLike {
  name: string;
  mime_type?: string;
  /** Drive file id when the file already lives in Drive. */
  drive_id?: string;
}

const EXT_APP: Array<[RegExp, AppId]> = [
  [/\.(docx?|odt|rtf|wpd|md|txt)$/i, "docs"],
  [/\.(xlsx?|xlsm|ods|csv|tsv)$/i, "sheets"],
  [/\.(pptx?|odp|key)$/i, "slides"],
  [/\.pdf$/i, "pdf"],
  [/\.vsdx$/i, "whiteboard"],
  [/\.(glb|gltf|obj|stl|ply|fbx|3mf)$/i, "3d"],
  [/\.(png|jpe?g|gif|webp|heic|bmp|svg|tiff?)$/i, "photos"],
  [/\.(mp4|mov|webm|mkv|avi|m4v)$/i, "video"],
  [/\.(mp3|wav|m4a|flac|ogg|aac)$/i, "music"],
  [/\.(epub|mobi|cbz)$/i, "books"],
  [/\.(ics)$/i, "calendar"],
  [/\.(vcf)$/i, "contacts"],
];

const MIME_APP: Array<[RegExp, AppId]> = [
  [/wordprocessingml|msword|opendocument\.text|rtf/, "docs"],
  [/spreadsheetml|ms-excel|opendocument\.spreadsheet|text\/csv|application\/csv/, "sheets"],
  [/presentationml|ms-powerpoint|opendocument\.presentation/, "slides"],
  [/application\/pdf/, "pdf"],
  [/^image\//, "photos"],
  [/^video\//, "video"],
  [/^audio\//, "music"],
  [/text\/calendar/, "calendar"],
  [/text\/(x-)?vcard/, "contacts"],
];

/** Which app should open a file (by mime type first, then extension). */
export function appForFile(f: FileLike): AppId {
  const m = (f.mime_type ?? "").toLowerCase();
  for (const [re, app] of MIME_APP) if (re.test(m)) return app;
  for (const [re, app] of EXT_APP) if (re.test(f.name)) return app;
  return "drive";
}

/**
 * Route that opens a Drive file in its app. Editors take the Drive id
 * (/sheets/<id> imports and edits the file); media and other files use the
 * Drive viewer, which plays images, video, audio and text inline.
 */
export function openDriveFile(f: FileLike & { drive_id: string }): OpenTarget {
  const app = appForFile(f);
  switch (app) {
    case "docs":
    case "sheets":
    case "slides":
    case "pdf":
    case "whiteboard":
      return target(`/${app}/${f.drive_id}`, app);
    case "3d":
      return target(`/3d?file=${f.drive_id}`, "3d");
    default:
      return { ...target(`/drive/file/${f.drive_id}`, "drive"), label: `Open ${labelForKind(app)}` };
  }
}

function labelForKind(app: AppId): string {
  switch (app) {
    case "photos":
      return "image";
    case "video":
      return "video";
    case "music":
      return "audio";
    default:
      return "file";
  }
}

/** Adds ?return=<path> so the opened app can offer "Back to <app>". */
export function withReturn(href: string, returnTo: string): string {
  if (!href.startsWith("/") || !returnTo.startsWith("/") || returnTo.startsWith("//")) return href;
  const [path, hash = ""] = href.split("#");
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}return=${encodeURIComponent(returnTo)}${hash ? `#${hash}` : ""}`;
}
