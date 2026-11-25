import { jf } from "../know/http";

export type FieldType = "signature" | "initials" | "date" | "text" | "checkbox";
export interface Field { id: string; signer_id: string; type: FieldType; label?: string; page: number; x: number; y: number; w: number; h: number; required: boolean; value?: string }
export interface Signer {
  id: string; name: string; email: string; user_id?: string; contact_id?: string; role: "signer" | "cc"; order: number; status: "pending" | "viewed" | "signed" | "declined";
  viewed_at?: string; signed_at?: string; declined_at?: string; decline_reason?: string; signature_kind?: string; sign_ip?: string; link?: string;
}
export interface Request {
  id: string; title: string; message: string; status: "draft" | "sent" | "completed" | "declined" | "voided" | "expired"; sender_id: string; sender_name?: string;
  file_id: string; file_name: string; doc_hash?: string; signed_file_id?: string; signed_hash?: string; sequential: boolean; expires_at?: string; contract_id?: string;
  created_at: string; sent_at?: string; completed_at?: string; signers?: Signer[]; fields?: Field[]; progress?: string; can_manage: boolean;
}
export interface SignerInput { name: string; email: string; user_id?: string; contact_id?: string; role?: "signer" | "cc" }
export interface FieldInput { signer_index: number; type: FieldType; label?: string; page: number; x: number; y: number; w: number; h: number; required?: boolean }
export interface CreateInput { title: string; message?: string; file_id: string; signers: SignerInput[]; fields: FieldInput[]; sequential?: boolean; expire_days?: number; contract_id?: string }
export interface TrailEvent { id: number; event: string; actor: string; ip?: string; detail?: string; at: string; hash: string }
export interface PublicDoc {
  title: string; message: string; sender: string; file_name: string; status: string; signer: Signer; fields: Field[]; waiting: boolean; others: string[]; can_sign: boolean; done_signed: boolean;
}

const B = "/api/v1/signatures";
const P = "/api/v1/public/signatures";
export const signatures = {
  people: () => jf<{ people: { id: string; name: string; email: string }[]; me: string }>(`${B}/people`),
  contacts: (q: string) => jf<{ contacts: { id: string; name: string; email: string }[] }>(`${B}/contacts?q=${encodeURIComponent(q)}`).then((r) => r.contacts),
  list: (status = "") => jf<{ requests: Request[] }>(`${B}/requests?status=${status}`).then((r) => r.requests),
  get: (id: string) => jf<Request>(`${B}/requests/${id}`),
  create: (i: CreateInput) => jf<Request>(`${B}/requests`, { method: "POST", body: i }),
  send: (id: string) => jf<Request>(`${B}/requests/${id}/send`, { method: "POST" }),
  void: (id: string, reason = "") => jf<{ ok: boolean }>(`${B}/requests/${id}/void`, { method: "POST", body: { reason } }),
  remind: (id: string) => jf<{ sent: number }>(`${B}/requests/${id}/remind`, { method: "POST" }),
  remove: (id: string) => jf<{ ok: boolean }>(`${B}/requests/${id}`, { method: "DELETE" }),
  trail: (id: string) => jf<{ events: TrailEvent[]; intact: boolean }>(`${B}/requests/${id}/trail`),
  pdfUrl: (id: string, original = false) => `${B}/requests/${id}/pdf${original ? "?original=1" : ""}`,
  // signer side
  publicDoc: (tok: string) => jf<PublicDoc>(`${P}/${tok}`),
  publicPdf: (tok: string) => `${P}/${tok}/pdf`,
  publicSign: (tok: string, b: { consent: boolean; signature_kind: "typed" | "drawn"; signature_data: string; values: Record<string, string> }) => jf<PublicDoc>(`${P}/${tok}/sign`, { method: "POST", body: b }),
  publicDecline: (tok: string, reason: string) => jf<{ ok: boolean }>(`${P}/${tok}/decline`, { method: "POST", body: { reason } }),
};
