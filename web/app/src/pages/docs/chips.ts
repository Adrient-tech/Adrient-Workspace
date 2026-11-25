// Smart chips: pill-style inline links to people, dates, Drive files and
// other Adrient apps. A chip is an ordinary hyperlink whose title starts
// with "chip:" (so it exports to DOCX/HTML as a normal link and the
// workspace link handler opens app links in the right app).
import { openDriveFile, openLink } from "../../lib/openWith";

export type ChipKind = "person" | "date" | "file" | "app";
const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

export function chipHtml(kind: ChipKind, label: string, href: string): string {
  return `<a href="${esc(href)}" title="chip:${kind}">${esc(label)}</a>&nbsp;`;
}

export const personChip = (name: string, email: string) => chipHtml("person", `@${name || email}`, `mailto:${email}`);
/** fileChip links a Drive file to the app that edits/views it. */
export function fileChip(f: { name: string; mime_type?: string; id: string }): string {
  return chipHtml("file", f.name, openDriveFile({ name: f.name, mime_type: f.mime_type, drive_id: f.id }).href);
}

/** dateChip links to the calendar for that day. */
export function dateChip(d: Date): string {
  const iso = d.toISOString().slice(0, 10);
  return chipHtml("date", d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" }), `/calendar?date=${iso}`);
}

export const APP_CHIPS: { id: string; label: string; href: string }[] = [
  { id: "docs", label: "Docs", href: "/docs" },
  { id: "sheets", label: "Sheets", href: "/sheets" },
  { id: "slides", label: "Slides", href: "/slides" },
  { id: "calendar", label: "Calendar", href: "/calendar" },
  { id: "contacts", label: "Contacts", href: "/contacts" },
  { id: "crm", label: "CRM", href: "/crm" },
  { id: "tasks", label: "Tasks", href: "/tasks" },
  { id: "projects", label: "Projects", href: "/projects" },
  { id: "mail", label: "Mail", href: "/mail" },
  { id: "chat", label: "Chat", href: "/chat" },
  { id: "drive", label: "Drive", href: "/drive" },
  { id: "invoicing", label: "Invoicing", href: "/invoicing" },
];

export const appChip = (label: string, href: string) => chipHtml("app", label, href);

/** Which app a chip/link opens (for tooltips and tests). */
export function chipTarget(href: string) {
  return openLink(href);
}
