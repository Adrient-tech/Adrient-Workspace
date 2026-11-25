// Admin → AI assistant API (internal/ai, /api/v1/ai/admin/*). Keys are write-only:
// the server never returns them, only whether one is stored or read from env.
export type ProviderKind = "openai" | "azure" | "anthropic" | "gemini";

export interface AIProvider {
  id: string;
  name: string;
  kind: ProviderKind;
  base_url: string;
  models: string[];
  key_env?: string;
  api_version?: string;
  local: boolean;
  enabled: boolean;
  price_in_per_m?: number;
  price_out_per_m?: number;
  has_key: boolean;
  key_source: "" | "stored" | "env";
}

export interface AISettings {
  kill_switch: boolean;
  local_only: boolean;
  default_model: string;
  app_models?: Record<string, string>;
  disabled_apps?: string[];
  notice: string;
  org_daily_tokens: number;
  user_daily_tokens: number;
  user_per_minute: number;
  max_output_tokens: number;
}

export interface UsageRow {
  key: string;
  label?: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
}

export interface TestResult {
  ok: boolean;
  model: string;
  latency_ms: number;
  reply?: string;
  error?: string;
}

export class AIApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/ai/admin${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) {
    let msg = `HTTP ${r.status}`;
    try {
      msg = ((await r.json()) as { message?: string }).message ?? msg;
    } catch {
      /* not json */
    }
    throw new AIApiError(r.status, msg);
  }
  return (await r.json()) as T;
}

export const aiApi = {
  settings: () => call<{ settings: AISettings; apps: string[]; server_key: boolean }>("/settings"),
  saveSettings: (s: AISettings) => call<AISettings>("/settings", { method: "PUT", body: JSON.stringify(s) }),
  providers: () => call<AIProvider[]>("/providers"),
  /** api_key: undefined keeps the stored key, "" removes it. */
  saveProvider: (p: Partial<AIProvider> & { api_key?: string }, id?: string) =>
    call<AIProvider>(id ? `/providers/${id}` : "/providers", { method: id ? "PUT" : "POST", body: JSON.stringify(p) }),
  deleteProvider: (id: string) => call<{ ok: boolean }>(`/providers/${id}`, { method: "DELETE" }),
  test: (id: string, model?: string) => call<TestResult>(`/providers/${id}/test`, { method: "POST", body: JSON.stringify({ model: model ?? "" }) }),
  usage: (by: "app" | "model" | "user", days = 30) => call<{ rows: UsageRow[] }>(`/usage?by=${by}&days=${days}`),
};

/** Base URL presets for the provider form. */
export const PRESETS: { label: string; kind: ProviderKind; base_url: string; local: boolean; models: string }[] = [
  { label: "Ollama (this server)", kind: "openai", base_url: "http://127.0.0.1:11434/v1", local: true, models: "llama3.1:8b" },
  { label: "llama.cpp server", kind: "openai", base_url: "http://127.0.0.1:8080/v1", local: true, models: "default" },
  { label: "vLLM", kind: "openai", base_url: "http://127.0.0.1:8000/v1", local: true, models: "" },
  { label: "OpenAI", kind: "openai", base_url: "https://api.openai.com/v1", local: false, models: "gpt-4o-mini" },
  { label: "OpenRouter", kind: "openai", base_url: "https://openrouter.ai/api/v1", local: false, models: "" },
  { label: "Azure OpenAI", kind: "azure", base_url: "https://<resource>.openai.azure.com", local: false, models: "" },
  { label: "Anthropic", kind: "anthropic", base_url: "https://api.anthropic.com", local: false, models: "" },
  { label: "Google Gemini", kind: "gemini", base_url: "https://generativelanguage.googleapis.com", local: false, models: "" },
];

export const APP_LABELS: Record<string, string> = {
  sheets: "Sheets", docs: "Docs", slides: "Slides", global: "Ask Adrient (search bar)", drive: "Drive", mail: "Mail",
  calendar: "Calendar", contacts: "Contacts", chat: "Chat", forms: "Forms", keep: "Keep", tasks: "Tasks", crm: "CRM",
};
