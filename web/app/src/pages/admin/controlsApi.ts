// Client for the per-app controls, effective access and external collaboration
// admin APIs (internal/authz/controls_http.go, internal/externalpolicy).

export type ControlValue = boolean | string | number;

export interface ControlDef {
  key: string;
  app: string;
  label: string;
  help: string;
  type: "bool" | "enum" | "number";
  enum?: string[];
  default: ControlValue;
  type_defaults?: Record<string, ControlValue>;
}

export interface Condition {
  ip_cidrs?: string[];
  user_types?: string[];
  departments?: string[];
}

export type CtlSubject = "everyone" | "role" | "user_type" | "department" | "group" | "user";

export interface ControlRule {
  id?: string;
  app_id?: string;
  control: string;
  value: ControlValue;
  subject_type: CtlSubject;
  subject_id: string;
  subject_name?: string;
  condition?: Condition | null;
  priority: number;
  effect_mode: "set" | "restrict_only";
  note?: string;
}

export interface RuleEval {
  rule: ControlRule;
  status: "winner" | "overridden" | "restricts" | "condition_not_met";
  why?: string;
}

export interface ControlResult {
  key: string;
  app: string;
  value: ControlValue;
  source: "default" | "rule" | "error";
  winner?: RuleEval;
  rules?: RuleEval[];
}

export interface EffectiveApp {
  id: string;
  allowed: boolean;
  reason: string;
  policies?: { service_id: string; subject_type: string; subject_id: string; effect: string }[];
  controls: ControlResult[];
}

export interface Effective {
  user: { id: string; email: string; name: string; user_type: string; status: string };
  simulated: { user_type: string; ip: string };
  roles: string[];
  apps: EffectiveApp[];
  shared: ControlResult[];
}

export interface ExternalPolicy {
  guest_invites: string;
  domain_mode: string;
  guest_visibility: string;
  guest_default_expiry_days: number;
  external_member_default_expiry_days: number;
  require_mfa_for_guests: boolean;
}

export interface Partner {
  domain: string;
  effect: "allow" | "block";
  note: string;
  created_at?: string;
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

type RuleBody = Omit<ControlRule, "id" | "subject_name" | "app_id">;

export const controlsApi = {
  list: () =>
    req<{ catalog: ControlDef[]; rules: ControlRule[]; user_types: string[] }>("/api/v1/admin/controls"),
  create: (rule: RuleBody) =>
    req<{ ok: boolean; id: string }>("/api/v1/admin/controls/rules", { method: "POST", body: JSON.stringify(rule) }),
  update: (id: string, rule: RuleBody) =>
    req<{ ok: boolean; id: string }>(`/api/v1/admin/controls/rules/${id}`, { method: "PUT", body: JSON.stringify(rule) }),
  remove: (id: string) =>
    req<{ ok: boolean }>(`/api/v1/admin/controls/rules/${id}`, { method: "DELETE" }),
  effective: (userId: string, opts: { ip?: string; userType?: string; app?: string } = {}) => {
    const q = new URLSearchParams({ user_id: userId });
    if (opts.ip) q.set("ip", opts.ip);
    if (opts.userType) q.set("user_type", opts.userType);
    if (opts.app) q.set("app", opts.app);
    return req<Effective>(`/api/v1/admin/access/effective?${q.toString()}`);
  },
};

export const externalApi = {
  policy: () => req<ExternalPolicy>("/api/v1/admin/external/policy"),
  savePolicy: (p: ExternalPolicy) =>
    req<ExternalPolicy>("/api/v1/admin/external/policy", { method: "PUT", body: JSON.stringify(p) }),
  partners: () => req<{ partners: Partner[] }>("/api/v1/admin/external/partners"),
  addPartner: (p: { domain: string; effect: "allow" | "block"; note: string }) =>
    req<{ ok: boolean }>("/api/v1/admin/external/partners", { method: "POST", body: JSON.stringify(p) }),
  removePartner: (domain: string) =>
    req<{ ok: boolean }>(`/api/v1/admin/external/partners/${encodeURIComponent(domain)}`, { method: "DELETE" }),
};

/** Human text for a control value ("Off", "On", "Domains", "25 MB"). */
export function showValue(def: ControlDef | undefined, v: ControlValue): string {
  if (typeof v === "boolean") return v ? "On" : "Off";
  if (typeof v === "number") return def?.key.endsWith("_mb") ? `${v} MB` : String(v);
  return v.charAt(0).toUpperCase() + v.slice(1).replace("_", " ");
}
