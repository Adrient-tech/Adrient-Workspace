import type { DriveFile } from "./types";
import { isFolder } from "./types";

export type DriveSort = "name" | "modified" | "size" | "type";
export type DriveGroup = "none" | "type" | "modified";

/** Coarse file category used for grouping and the type sort. */
export function typeCategory(f: DriveFile): string {
  if (isFolder(f)) return "Folders";
  const m = f.mime_type || "";
  if (m.startsWith("image/")) return "Images";
  if (m.startsWith("video/")) return "Video";
  if (m.startsWith("audio/")) return "Audio";
  if (m === "application/pdf") return "PDF";
  if (/word|document|opendocument\.text/.test(m)) return "Documents";
  if (/sheet|excel|csv/.test(m)) return "Spreadsheets";
  if (/presentation|powerpoint/.test(m)) return "Presentations";
  if (m.startsWith("text/")) return "Text";
  return "Other";
}

/** updated_at / created_at are unix seconds (as strings). */
function fileTime(f: DriveFile): number {
  const v = Number(f.updated_at || f.created_at || 0);
  return Number.isFinite(v) ? v * 1000 : 0;
}

function modifiedBucket(f: DriveFile, now = new Date()): string {
  const t = fileTime(f);
  const day = 86400_000;
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (t >= startToday) return "Today";
  if (t >= startToday - 7 * day) return "Earlier this week";
  if (t >= startToday - 30 * day) return "Earlier this month";
  return "Older";
}

/** Folders first, then the chosen key. */
export function sortFiles(files: DriveFile[], key: DriveSort, desc: boolean): DriveFile[] {
  const dir = desc ? -1 : 1;
  const time = fileTime;
  return [...files].sort((a, b) => {
    const fa = isFolder(a) ? 0 : 1;
    const fb = isFolder(b) ? 0 : 1;
    if (fa !== fb) return fa - fb;
    let c = 0;
    switch (key) {
      case "modified": c = time(a) - time(b); break;
      case "size": c = Number(a.size_bytes) - Number(b.size_bytes); break;
      case "type": c = typeCategory(a).localeCompare(typeCategory(b)); break;
      default: c = 0;
    }
    return (c || a.name.localeCompare(b.name, undefined, { numeric: true })) * dir;
  });
}

export function groupLabel(f: DriveFile, group: DriveGroup, now = new Date()): string {
  if (group === "type") return typeCategory(f);
  if (group === "modified") return modifiedBucket(f, now);
  return "";
}
