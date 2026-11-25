// Client for the mail DNS APIs (internal/dnsmail): record plan, live check,
// DKIM keys and per-domain mail settings.

export interface DnsRecord {
  kind: string;
  name: string;
  type: string;
  value: string;
  ttl: number;
  required: boolean;
  note?: string;
}

export type CheckStatus = "pass" | "warn" | "fail";

export interface DnsResult extends DnsRecord {
  status: CheckStatus;
  expected: string;
  found: string[] | null;
  message: string;
}

export interface MailSettings {
  dmarc_policy: "none" | "quarantine" | "reject";
  dmarc_rua: string;
  mta_sts_mode: "none" | "testing" | "enforce";
  mta_sts_id: string;
  tls_rpt_rua: string;
  mail_host: string;
  spf_ips: string[];
}

export interface DkimKey {
  id: string;
  selector: string;
  algo: string;
  state: "active" | "next" | "retired";
  public_key: string;
  created_at: string;
  activated_at?: string | null;
}

export interface DnsPlan {
  domain: string;
  records: DnsRecord[];
  settings: MailSettings;
  keys: DkimKey[];
  clean_days: number;
  ramp_hint: string;
  mail_host: string;
  web_host: string;
  mail_backend: string;
}

export interface DnsCheck {
  results: DnsResult[];
  pass: number;
  warn: number;
  fail: number;
  clean: boolean;
  clean_days: number;
  ramp_hint: string;
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

const base = (id: string) => `/api/v1/admin/domains/${id}`;
const post = (body?: unknown): RequestInit => ({ method: "POST", body: body === undefined ? undefined : JSON.stringify(body) });

export const dnsApi = {
  plan: (id: string) => req<DnsPlan>(`${base(id)}/dns/plan`),
  check: (id: string) => req<DnsCheck>(`${base(id)}/dns/check`, post()),
  generateKey: (id: string, algo?: string) => req<DkimKey>(`${base(id)}/dkim/generate`, post({ algo })),
  rotateKey: (id: string, algo?: string) => req<DkimKey>(`${base(id)}/dkim/rotate`, post({ algo })),
  activateKey: (id: string) => req<DkimKey>(`${base(id)}/dkim/activate`, post()),
  putSettings: (id: string, s: MailSettings) =>
    req<MailSettings>(`${base(id)}/mail-settings`, { method: "PUT", body: JSON.stringify(s) }),
};
