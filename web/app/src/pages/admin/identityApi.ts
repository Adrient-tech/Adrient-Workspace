// Client for the identity admin APIs: verified domains and org sign-up settings
// (internal/orgdomains) and user lifecycle + invitations (internal/lifecycle).

export interface OrgDomain {
  id: string;
  domain: string;
  status: "pending" | "verified" | "failed" | "removed";
  verify_host: string;
  verify_txt: string;
  verified_at?: string | null;
  last_checked_at?: string | null;
  is_primary: boolean;
  allow_signup: boolean;
  auto_join: boolean;
  require_approval: boolean;
  user_count: number;
}

export type SignupMode = "invite_only" | "domain_auto_join" | "domain_approval" | "open";

export interface OrgSettings {
  signup_mode: SignupMode;
  allowed_signin_domains: string[];
  personal_orgs_allowed: boolean;
}

export type UserType = "member" | "external" | "guest" | "service";
export type UserStatus = "active" | "blocked" | "expired";

export interface IdentityUser {
  id: string;
  email: string;
  display_name: string;
  upn: string;
  user_type: UserType;
  status: UserStatus;
  expires_at: string | null;
  sponsor_email: string;
  blocked_reason: string;
  last_sign_in_at: string | null;
  guest_apps: string[];
}

export interface Invitation {
  id: string;
  email: string;
  user_type: UserType;
  sponsor_email: string;
  app_ids: string[];
  user_expires_at: string | null;
  expires_at: string;
  state: "pending" | "accepted" | "revoked" | "expired";
}

export interface InviteResult {
  invitation: Invitation;
  accept_url: string;
  email_sent: boolean;
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) {
    let msg = `HTTP ${r.status}`;
    try {
      const b = (await r.json()) as { error?: string; message?: string };
      msg = b.error && b.error !== "forbidden" ? b.error : (b.message ?? msg);
    } catch {
      /* keep status text */
    }
    throw new Error(msg);
  }
  return (await r.json()) as T;
}

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const identityApi = {
  domains: () => req<{ domains: OrgDomain[] }>("/api/v1/admin/domains"),
  addDomain: (domain: string) => req<OrgDomain>("/api/v1/admin/domains", json("POST", { domain })),
  verifyDomain: (id: string) =>
    req<{ verified: boolean; domain: OrgDomain }>(`/api/v1/admin/domains/${id}/verify`, json("POST")),
  patchDomain: (id: string, p: Partial<Pick<OrgDomain, "is_primary" | "allow_signup" | "auto_join">>) =>
    req<OrgDomain>(`/api/v1/admin/domains/${id}`, json("PATCH", p)),
  removeDomain: (id: string) => req<{ ok: boolean }>(`/api/v1/admin/domains/${id}`, json("DELETE")),
  settings: () => req<OrgSettings>("/api/v1/admin/org/settings"),
  putSettings: (s: OrgSettings) => req<OrgSettings>("/api/v1/admin/org/settings", json("PUT", s)),

  users: (f: { type?: string; status?: string; q?: string; page?: number }) => {
    const p = new URLSearchParams();
    if (f.page) p.set("page", String(f.page));
    if (f.type) p.set("type", f.type);
    if (f.status) p.set("status", f.status);
    if (f.q) p.set("q", f.q);
    return req<{ users: IdentityUser[]; next_cursor?: string }>(`/api/v1/admin/identity/users?${p}`);
  },
  block: (id: string, reason: string) =>
    req<{ ok: boolean }>(`/api/v1/admin/identity/users/${id}/block`, json("POST", { reason })),
  unblock: (id: string) => req<{ ok: boolean }>(`/api/v1/admin/identity/users/${id}/unblock`, json("POST")),
  patchUser: (id: string, p: { user_type?: UserType; expires_at?: string; clear_expiry?: boolean }) =>
    req<{ ok: boolean }>(`/api/v1/admin/identity/users/${id}`, json("PATCH", p)),
  runExpiry: () => req<{ blocked: number }>("/api/v1/admin/identity/run-expiry", json("POST")),

  invitations: () => req<{ invitations: Invitation[] }>("/api/v1/admin/invitations"),
  invite: (b: { email: string; user_type: UserType; expire_days?: number; app_ids?: string[] }) =>
    req<InviteResult>("/api/v1/admin/invitations", json("POST", b)),
  resend: (id: string) => req<InviteResult>(`/api/v1/admin/invitations/${id}/resend`, json("POST")),
  revoke: (id: string) => req<{ ok: boolean }>(`/api/v1/admin/invitations/${id}`, json("DELETE")),

  lookupInvite: (code: string) =>
    req<{ org: string; email: string; user_type: UserType; apps: string[]; expires_at: string }>(
      `/api/v1/invitations/lookup?code=${encodeURIComponent(code)}`,
    ),
  acceptInvite: (code: string) =>
    req<{ ok: boolean; user_type: UserType }>("/api/v1/invitations/accept", json("POST", { code })),
};
