export interface Stage {
  id: string;
  name: string;
}
export interface Settings {
  stages: Stage[];
  currency: string;
  monthly_goal: number;
  stale_days: number;
  cold_days: number;
  weekly_target: number;
  sla_lead_hours?: number;
  sla_stage_days?: Record<string, number>;
  default_scope?: string;
}
export interface Company {
  id: string;
  name: string;
  industry: string;
  website: string;
  phone: string;
  address: string;
  notes: string;
  tags: string[];
  created_at: string;
  owner_id?: string | null;
  custom?: Record<string, unknown>;
}
export interface Person {
  id: string;
  contact_id: string;
  company_id: string | null;
  status: "lead" | "customer" | "inactive";
  source: string;
  tags: string[];
  notes: string;
  created_at: string;
  display_name: string;
  emails: string[] | null;
  phones: unknown;
  job_title: string;
  owner_id?: string | null;
  score?: number;
  custom?: Record<string, unknown>;
  sla_due_at?: string | null;
  first_response_at?: string | null;
}
export interface Member {
  contact_id: string;
  role: string;
}
export type Billing = "one_time" | "monthly" | "quarterly" | "yearly";
export interface Deal {
  id: string;
  title: string;
  company_id: string | null;
  contact_id: string | null;
  stage: string;
  value: number;
  currency: string;
  billing: Billing;
  expected_close: string | null;
  source: string;
  notes: string;
  close_reason: string;
  closed_at: string | null;
  stage_entered_at: string;
  created_at: string;
  updated_at: string;
  committee: Member[];
  owner_id?: string | null;
  custom?: Record<string, unknown>;
  items?: DealItem[];
}
export interface DealItem {
  product_id: string;
  name: string;
  qty: number;
  unit_price: number;
  discount_pct: number;
}
export interface DealInput {
  title: string;
  company_id: string;
  contact_id: string;
  stage: string;
  value: number;
  currency: string;
  billing: Billing;
  expected_close: string;
  source: string;
  notes: string;
  committee: Member[];
  owner_id?: string;
  custom?: Record<string, unknown>;
  items?: DealItem[];
}
export interface HistoryRow {
  id: string;
  deal_id: string;
  stage: string;
  entered_at: string;
}
export interface Followup {
  id: string;
  title: string;
  due_date: string;
  due_time: string;
  deal_id: string | null;
  contact_id: string | null;
  done: boolean;
  calendar_event_id: string;
  created_at: string;
}
export type ActivityKind = "call" | "meeting" | "email" | "note";
export interface Activity {
  id: string;
  kind: ActivityKind;
  summary: string;
  occurred_at: string;
  deal_id: string | null;
  contact_id: string | null;
  company_id: string | null;
  calendar_event_id: string;
  mail_thread_id: string;
  created_at: string;
  direction?: string;
  duration_sec?: number;
  meet_url?: string;
  user_id?: string | null;
}
export interface ContactLite {
  id: string;
  display_name: string;
  company: string;
  emails: string[];
}
export interface LinkedDoc {
  id: string;
  number: string;
  kind: "invoice" | "estimate";
  display_status: string;
  total: number;
  currency: string;
}

export type FieldType = "text" | "number" | "date" | "select" | "multiselect" | "url" | "user";
export type FieldEntity = "lead" | "contact" | "company" | "deal";
export interface FieldDef {
  id: string;
  entity: FieldEntity;
  key: string;
  label: string;
  type: FieldType;
  options: string[];
  required: boolean;
  position: number;
}
export interface Me {
  user_id: string;
  is_admin: boolean;
  scope: string;
  auto_log_mail: boolean;
}
export interface UserLite {
  id: string;
  name: string;
  email: string;
}
export interface SavedViewRow {
  id: string;
  entity: string;
  name: string;
  config: Record<string, unknown>;
  shared: boolean;
  owner_id: string;
  mine: boolean;
}
export interface Report {
  name: string;
  title: string;
  columns: string[];
  rows: (string | number | null)[][];
}
