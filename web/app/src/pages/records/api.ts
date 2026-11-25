import { jf } from "../know/http";

export type FieldType =
  | "text" | "long_text" | "attachment" | "checkbox" | "single_select" | "multi_select" | "date" | "phone" | "email" | "url"
  | "number" | "currency" | "percent" | "duration" | "rating" | "barcode" | "link" | "lookup" | "rollup" | "count" | "formula"
  | "autonumber" | "created_time" | "modified_time" | "created_by" | "modified_by" | "button";

export interface Choice { id: string; name: string; color?: string }
export interface Field {
  id: string; table_id: string; name: string; type: FieldType; description?: string;
  options: Record<string, unknown> & { choices?: Choice[]; precision?: number; max?: number; table_id?: string; formula?: string; link_field_id?: string; target_field_id?: string; fn?: string; include_time?: boolean; allow_multiple?: boolean; reverse?: boolean; symbol?: string };
  position: number;
}
export type ViewType = "grid" | "kanban" | "gallery" | "calendar" | "list" | "form";
export interface Node { logic?: "and" | "or"; conds?: Node[]; field?: string; op?: string; value?: unknown }
export interface SortKey { field: string; desc?: boolean }
export interface ViewConfig {
  filter?: Node; sort?: SortKey[]; group_by?: string[]; hidden?: string[]; field_order?: string[]; widths?: Record<string, number>;
  stack_by?: string; date_field?: string; end_date_field?: string; cover_field?: string; row_height?: "short" | "medium" | "tall"; form_fields?: string[]; form_title?: string;
}
export interface View { id: string; table_id: string; name: string; type: ViewType; config: ViewConfig; position: number }
export interface Table { id: string; base_id: string; name: string; description: string; position: number; fields: Field[]; views: View[]; record_count: number }
export interface Member { user_id: string; name?: string; role: "viewer" | "editor" | "admin" }
export interface Base {
  id: string; name: string; description: string; color: string; owner_id: string; org_access: "none" | "view" | "edit"; role: number; updated_at: string;
  trashed_at?: string | null; tables?: Table[]; members?: Member[];
}
export type Cell = unknown;
export interface Rec { id: string; seq: number; fields: Record<string, Cell>; created_at: string; updated_at: string; created_by?: string; updated_by?: string }
export interface Query { filter?: Node; sort?: SortKey[]; group_by?: string[]; search?: string; offset?: number; limit?: number }
export interface QueryResult { records: Rec[]; total: number; offset: number }
export interface LinkVal { id: string; name: string }
export interface Person { id: string; name: string; email: string }

const B = "/api/v1/records";
export const records = {
  meta: () => jf<{ templates: { id: string; name: string; blurb: string }[]; types: FieldType[]; rollup_fns: string[] }>(`${B}/templates`),
  people: () => jf<{ people: Person[] }>(`${B}/people`).then((r) => r.people),
  bases: (trashed = false) => jf<{ bases: Base[] }>(`${B}/bases${trashed ? "?trashed=1" : ""}`).then((r) => r.bases),
  base: (id: string) => jf<Base>(`${B}/bases/${id}`),
  createBase: (b: { name: string; description?: string; color?: string; template?: string }) => jf<Base>(`${B}/bases`, { method: "POST", body: b }),
  updateBase: (id: string, b: { name: string; description?: string; color?: string; org_access?: string }) => jf<Base>(`${B}/bases/${id}`, { method: "PUT", body: b }),
  trashBase: (id: string) => jf<{ ok: boolean }>(`${B}/bases/${id}`, { method: "DELETE" }),
  deleteBase: (id: string) => jf<{ ok: boolean }>(`${B}/bases/${id}?permanent=1`, { method: "DELETE" }),
  restoreBase: (id: string) => jf<{ ok: boolean }>(`${B}/bases/${id}/restore`, { method: "POST" }),
  setMembers: (id: string, members: Member[]) => jf<{ ok: boolean }>(`${B}/bases/${id}/members`, { method: "PUT", body: { members } }),
  createTable: (baseId: string, name: string) => jf<Table>(`${B}/bases/${baseId}/tables`, { method: "POST", body: { name } }),
  updateTable: (id: string, name: string, description = "", position?: number) => jf<{ ok: boolean }>(`${B}/tables/${id}`, { method: "PUT", body: { name, description, position } }),
  deleteTable: (id: string) => jf<{ ok: boolean }>(`${B}/tables/${id}`, { method: "DELETE" }),
  createField: (tableId: string, f: { name: string; type: FieldType; options?: Record<string, unknown>; description?: string; two_way?: boolean }) => jf<Field>(`${B}/tables/${tableId}/fields`, { method: "POST", body: f }),
  updateField: (id: string, f: { name?: string; type?: FieldType; options?: Record<string, unknown>; description?: string }) => jf<{ field: Field; cleared: number }>(`${B}/fields/${id}`, { method: "PUT", body: f }),
  deleteField: (id: string) => jf<{ ok: boolean }>(`${B}/fields/${id}`, { method: "DELETE" }),
  saveView: (tableId: string, v: { id?: string; name: string; type: ViewType; config: ViewConfig }) =>
    v.id ? jf<View>(`${B}/tables/${tableId}/views/${v.id}`, { method: "PUT", body: v }) : jf<View>(`${B}/tables/${tableId}/views`, { method: "POST", body: v }),
  deleteView: (tableId: string, id: string) => jf<{ ok: boolean }>(`${B}/tables/${tableId}/views/${id}`, { method: "DELETE" }),
  query: (tableId: string, q: Query) => jf<QueryResult>(`${B}/tables/${tableId}/query`, { method: "POST", body: q }),
  create: (tableId: string, recs: { fields: Record<string, unknown> }[]) => jf<{ records: Rec[] }>(`${B}/tables/${tableId}/records`, { method: "POST", body: { records: recs } }).then((r) => r.records),
  update: (tableId: string, recs: { id: string; fields: Record<string, unknown> }[]) => jf<{ records: Rec[] }>(`${B}/tables/${tableId}/records`, { method: "PATCH", body: { records: recs } }).then((r) => r.records),
  remove: (tableId: string, ids: string[]) => jf<{ deleted: number }>(`${B}/tables/${tableId}/records`, { method: "DELETE", body: { ids } }),
  record: (id: string) => jf<{ record: Rec; table_id: string }>(`${B}/records/${id}`),
  comments: (id: string) => jf<{ comments: { id: string; author: string; body: string; created_at: string }[] }>(`${B}/records/${id}/comments`).then((r) => r.comments),
  addComment: (id: string, body: string) => jf<{ id: string }>(`${B}/records/${id}/comments`, { method: "POST", body: { body } }),
  history: (id: string) => jf<{ history: { at: string; user: string; changes: { field: string; old: string; new: string }[] }[] }>(`${B}/records/${id}/history`).then((r) => r.history),
  importPreview: (baseId: string, data: string, hasHeader: boolean) => jf<{ rows: number; columns: { index: number; name: string; type: FieldType }[]; sample: string[][] }>(`${B}/bases/${baseId}/import-preview`, { method: "POST", body: { data, has_header: hasHeader } }),
  importCsv: (baseId: string, req: { table_name?: string; table_id?: string; data: string; has_header: boolean; columns: { index: number; name: string; type: FieldType; field_id?: string; skip?: boolean }[] }) =>
    jf<{ table_id: string; imported: number; skipped: number; notes?: string[] }>(`${B}/bases/${baseId}/import`, { method: "POST", body: req }),
  restoreBackup: (json: string, name: string) => jf<Base>(`${B}/bases/restore-backup?name=${encodeURIComponent(name)}`, { method: "POST", raw: json }),
  exportUrl: (tableId: string) => `${B}/tables/${tableId}/export`,
  backupUrl: (baseId: string) => `${B}/bases/${baseId}/backup`,
  exportToDrive: (tableId: string, q: Query) => jf<{ file_id: string; name: string }>(`${B}/tables/${tableId}/export`, { method: "POST", body: { query: q, to_drive: true } }),
};
