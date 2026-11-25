// Client for the role assignment and app access admin APIs
// (internal/authz/adminhttp.go).

export type SubjectType = "everyone" | "user" | "group" | "department" | "role" | "user_type";
export type Effect = "allow" | "deny";

export interface AccessPolicy {
  id?: string;
  service_id: string;
  subject_type: SubjectType;
  subject_id: string;
  effect: Effect;
  subject_name?: string;
}

export interface RoleAssignment {
  id: string;
  subject_type: "user" | "group";
  subject_id: string;
  subject_name: string;
  subject_detail: string;
  role_key: string;
  role_name: string;
  scope_app: string;
}

export interface Subject {
  id: string;
  name: string;
  detail?: string;
}

export interface AppDecision {
  id: string;
  allowed: boolean;
  reason: string;
}

export interface Preview {
  apps: AppDecision[];
  allowed: string[];
  denied: string[];
  roles: string[];
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

export interface RolePreset {
  key: string;
  name: string;
  description: string;
  apps: string[];
  app_admin?: string[];
}

export const accessApi = {
  presets: () => req<{ presets: RolePreset[]; note: string }>("/api/v1/admin/app-access/presets"),
  applyPreset: (key: string, subject: { subject_type: "group" | "user"; subject_id: string }) =>
    req<{ ok: boolean; applied: string[]; newly_restricted: string[] }>(
      `/api/v1/admin/app-access/presets/${encodeURIComponent(key)}/apply`,
      { method: "POST", body: JSON.stringify(subject) },
    ),
  roles: () =>
    req<{
      roles: { key: string; name: string }[];
      assignments: RoleAssignment[];
      apps: string[];
    }>("/api/v1/admin/roles"),
  assign: (a: {
    subject_type: "user" | "group";
    subject_id: string;
    role_key: string;
    scope_app?: string;
  }) =>
    req<{ id: string }>("/api/v1/admin/roles/assignments", {
      method: "POST",
      body: JSON.stringify(a),
    }),
  revoke: (id: string) =>
    req<{ ok: boolean }>(`/api/v1/admin/roles/assignments/${id}`, {
      method: "DELETE",
    }),
  policies: () =>
    req<{ policies: AccessPolicy[]; apps: string[] }>(
      "/api/v1/admin/app-access",
    ),
  savePolicies: (app: string, policies: AccessPolicy[]) =>
    req<{ ok: boolean }>(`/api/v1/admin/app-access/${encodeURIComponent(app)}`, {
      method: "PUT",
      body: JSON.stringify({
        policies: policies.map((p) => ({
          subject_type: p.subject_type,
          subject_id: p.subject_id,
          effect: p.effect,
        })),
      }),
    }),
  preview: (userId: string) =>
    req<Preview>(
      `/api/v1/admin/app-access/preview?user_id=${encodeURIComponent(userId)}`,
    ),
  subjects: (type: SubjectType, q: string) =>
    req<{ subjects: Subject[] }>(
      `/api/v1/admin/subjects?type=${type}&q=${encodeURIComponent(q)}`,
    ).then((r) => r.subjects ?? []),
};
