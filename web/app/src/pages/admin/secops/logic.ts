// Pure helpers of the identity operations console (tested in logic.test.ts).
import type { Alert } from "./api";

export const SEVERITY_COLOR: Record<string, "neutral" | "warning" | "danger" | "primary"> = {
  low: "neutral",
  medium: "warning",
  high: "danger",
  critical: "danger",
};

export const STATUS_LABEL: Record<string, string> = {
  open: "Open",
  investigating: "Investigating",
  false_positive: "False positive",
  compromised: "Compromised",
  policy: "Policy or expected",
};

/** Time left (or overdue) against an SLA due time, in words. */
export function slaText(due: string | null, status: string, now: Date = new Date()): { text: string; late: boolean } {
  if (status !== "open" && status !== "investigating") return { text: "Closed", late: false };
  if (!due) return { text: "No SLA", late: false };
  const ms = new Date(due).getTime() - now.getTime();
  const abs = Math.abs(ms);
  const mins = Math.round(abs / 60000);
  const span = mins >= 1440 ? `${Math.round(mins / 1440)} d` : mins >= 60 ? `${Math.floor(mins / 60)} h ${mins % 60} m` : `${mins} m`;
  return ms < 0 ? { text: `${span} overdue`, late: true } : { text: `${span} left`, late: false };
}

export interface ActionDef {
  action: string;
  label: string;
  cap: string;
  group: "Password" | "Second step" | "Sessions and tokens" | "Account";
  help: string;
  fields?: { key: string; label: string; type: "text" | "number"; placeholder?: string; required?: boolean }[];
  danger?: boolean;
  stepUp?: boolean;
}

export const CONTAIN: ActionDef = {
  action: "contain",
  label: "Contain account",
  cap: "contain",
  group: "Account",
  stepUp: true,
  danger: true,
  help: "Signs the person out everywhere, resets the password, removes second steps and notifies them. Safe to run twice. Use Preview first.",
};

export const ACTIONS: ActionDef[] = [
  CONTAIN,
  { action: "password.reset_link", label: "Send reset link", cap: "reset_password", group: "Password", help: "E-mails the person a link to choose a new password." },
  { action: "password.temp", label: "Set temporary password", cap: "temp_password", group: "Password", stepUp: true, help: "Shown once. The person must change it at the next sign-in and is signed out everywhere." },
  { action: "password.expire", label: "Expire password", cap: "reset_password", group: "Password", help: "Invalidates the current password, signs the person out and e-mails a reset link." },
  { action: "account.unlock", label: "Unlock after failed attempts", cap: "unlock", group: "Password", help: "Clears a lock caused by too many wrong passwords." },
  { action: "mfa.reset_all", label: "Reset all second steps", cap: "reset_mfa", group: "Second step", stepUp: true, danger: true, help: "Removes every authenticator, security key and code. The person enrols again." },
  { action: "mfa.bypass", label: "Time-limited bypass", cap: "bypass_mfa", group: "Second step", stepUp: true, help: "Removes the second steps for a short window and gives you a one-time code to confirm the person's identity.", fields: [{ key: "hours", label: "Hours (1-24)", type: "number", placeholder: "4", required: true }] },
  { action: "sessions.revoke_all", label: "Sign out everywhere", cap: "sessions", group: "Sessions and tokens", help: "Ends every session in the apps and at the identity provider." },
  { action: "tokens.revoke_all", label: "Revoke all API tokens", cap: "tokens", group: "Sessions and tokens", stepUp: true, help: "Personal API tokens stop working at once." },
  { action: "account.suspend", label: "Suspend (block sign-in)", cap: "suspend", group: "Account", stepUp: true, danger: true, help: "Blocks sign-in and ends every session. Data stays." },
  { action: "account.unsuspend", label: "Unsuspend", cap: "suspend", group: "Account", help: "Lets the person sign in again." },
  { action: "account.quarantine", label: "Quarantine (read only)", cap: "quarantine", group: "Account", stepUp: true, help: "The person can read but not create, change or delete anything.", fields: [{ key: "hours", label: "Hours (blank = until released)", type: "number" }] },
  { action: "account.unquarantine", label: "Release quarantine", cap: "quarantine", group: "Account", help: "Restores normal access." },
  { action: "account.expiry", label: "Set account expiry", cap: "account_edit", group: "Account", stepUp: true, help: "Access ends after this many days. Enter 0 to clear.", fields: [{ key: "days", label: "Days", type: "number", required: true }] },
  { action: "account.email", label: "Change e-mail", cap: "account_edit", group: "Account", stepUp: true, help: "Sends a verification code to the new address; it applies once confirmed.", fields: [{ key: "email", label: "New e-mail", type: "text", required: true }] },
  { action: "account.phone", label: "Change phone", cap: "account_edit", group: "Account", stepUp: true, help: "Sends a verification code by text.", fields: [{ key: "phone", label: "Phone with country code", type: "text", placeholder: "+91...", required: true }] },
];

/** Validate the reason, ticket and required fields before sending. Returns an error message or "". */
export function validateAction(def: ActionDef, reason: string, params: Record<string, string>): string {
  if (reason.trim().length < 3) return "Write a reason (at least 3 characters).";
  for (const f of def.fields ?? []) {
    if (f.required && !(params[f.key] ?? "").trim()) return `${f.label} is required.`;
  }
  return "";
}

/** Convert the form strings into the typed params the server expects. */
export function paramsFor(def: ActionDef, raw: Record<string, string>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of def.fields ?? []) {
    const v = (raw[f.key] ?? "").trim();
    if (v === "") continue;
    out[f.key] = f.type === "number" ? Number(v) : v;
  }
  return out;
}

export function can(caps: string[] | undefined, cap: string): boolean {
  return !!caps && caps.includes(cap);
}

/** Order alerts for display: active first, then severity, then newest. */
export function sortAlerts(list: Alert[]): Alert[] {
  const rank = { critical: 4, high: 3, medium: 2, low: 1 } as Record<string, number>;
  const active = (a: Alert) => (a.status === "open" || a.status === "investigating" ? 1 : 0);
  return [...list].sort((a, b) => active(b) - active(a) || rank[b.severity] - rank[a.severity] || b.created_at.localeCompare(a.created_at));
}

export function ago(iso: string | null, now: Date = new Date()): string {
  if (!iso) return "never";
  const s = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return `${Math.floor(s / 86400)} d ago`;
}
