import { jf } from "../know/http";

export interface Renewal {
  phase: "none" | "evergreen" | "expired" | "notice" | "soon" | "ok";
  date?: string; days_left?: number; notice_date?: string; notice_days_left?: number;
  decision?: Decision | null; handled: boolean; attention: boolean;
}
export interface Decision { kind: string; for_term_end?: string; until?: string; note?: string; at?: string }
export interface Repeat { every: string; anchor?: string; count?: number; until?: string; contract_end?: boolean }
export interface Obligation {
  id: string; contract_id: string; title: string; owner: "us" | "them" | "mutual"; due?: string; done: boolean;
  repeat?: Repeat | null; history: { due: string; done_at: string }[]; assignee_id?: string; notes?: string;
}
export interface Doc { id: string; file_id: string; name: string; kind: string }
export interface Contract {
  id: string; title: string; type: string; status: "draft" | "active" | "terminated"; party_name: string; contact_id?: string; party_label: string;
  value: number; currency: string; billing: string; start_date: string; end_date: string; auto_renew: boolean; term_months: number; notice_days: number;
  owner_id: string; access: "org" | "restricted"; shared_with: string[]; remind: boolean; tags: string[]; notes: string; annual_value: number;
  renewal: Renewal; completed_terms?: { start: string; end: string }[]; obligations?: Obligation[]; documents?: Doc[]; can_edit: boolean; updated_at: string; decision?: Decision | null;
}
export type ContractInput = Partial<Pick<Contract, "title" | "type" | "status" | "party_name" | "contact_id" | "value" | "currency" | "billing" | "start_date" | "end_date" | "auto_renew" | "term_months" | "notice_days" | "access" | "shared_with" | "tags" | "notes" | "remind">>;
export interface ObItem extends Obligation { contract_title: string; urgency: "overdue" | "soon" | "later" | "undated"; days_left?: number }
export interface Stats { active: number; needs_attention: number; renewing_60_days: number; overdue_obligations: number; due_soon_obligations: number; annual_by_currency: Record<string, number> }

const B = "/api/v1/contracts";
export const contracts = {
  meta: () => jf<{ types: string[]; billings: string[]; today: string }>(`${B}/meta`),
  overview: () => jf<{ stats: Stats; obligations: ObItem[] }>(`${B}/overview`),
  people: () => jf<{ people: { id: string; name: string }[]; me: string }>(`${B}/people`),
  parties: (q: string) => jf<{ parties: { id: string; name: string; company: string }[] }>(`${B}/parties?q=${encodeURIComponent(q)}`).then((r) => r.parties),
  list: (q = "", status = "") => jf<{ contracts: Contract[] }>(`${B}/contracts?q=${encodeURIComponent(q)}&status=${status}`).then((r) => r.contracts),
  get: (id: string) => jf<Contract>(`${B}/contracts/${id}`),
  create: (c: ContractInput) => jf<Contract>(`${B}/contracts`, { method: "POST", body: c }),
  update: (id: string, c: ContractInput) => jf<Contract>(`${B}/contracts/${id}`, { method: "PUT", body: c }),
  remove: (id: string) => jf<{ ok: boolean }>(`${B}/contracts/${id}`, { method: "DELETE" }),
  decide: (id: string, d: { kind: string; until?: string; note?: string }) => jf<Contract>(`${B}/contracts/${id}/decision`, { method: "POST", body: d }),
  sync: (id: string) => jf<{ ok: boolean }>(`${B}/contracts/${id}/sync`, { method: "POST" }),
  addObligation: (id: string, o: Partial<Obligation> & { title: string }) => jf<Obligation>(`${B}/contracts/${id}/obligations`, { method: "POST", body: o }),
  updateObligation: (id: string, o: Partial<Obligation> & { title: string }) => jf<Obligation>(`${B}/obligations/${id}`, { method: "PUT", body: o }),
  completeObligation: (id: string) => jf<Obligation>(`${B}/obligations/${id}/complete`, { method: "POST" }),
  reopenObligation: (id: string) => jf<Obligation>(`${B}/obligations/${id}/reopen`, { method: "POST" }),
  deleteObligation: (id: string) => jf<{ ok: boolean }>(`${B}/obligations/${id}`, { method: "DELETE" }),
  addDocument: (id: string, file_id: string, kind = "agreement") => jf<Doc>(`${B}/contracts/${id}/documents`, { method: "POST", body: { file_id, kind } }),
  removeDocument: (id: string, doc: string) => jf<{ ok: boolean }>(`${B}/contracts/${id}/documents/${doc}`, { method: "DELETE" }),
  icsUrl: `${B}/calendar.ics`,
};
