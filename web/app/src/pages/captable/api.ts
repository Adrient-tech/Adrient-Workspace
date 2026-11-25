import { jf } from "../know/http";

export interface Company { name: string; currency: string; incorporated: string; notes: string }
export interface Row { id: string; name: string; kind: string; stock: number; options: number; warrants: number; fully_diluted: number; pct_outstanding: number; pct_fully_diluted: number; vested_stock: number }
export interface PoolBalance { id: string; name: string; size: number; granted: number; exercised: number; unallocated: number }
export interface ClassRow { id: string; name: string; shares: number; pct_outstanding: number }
export interface Table { as_of: string; rows: Row[]; pool_unallocated: number; pools: PoolBalance[]; outstanding: number; fully_diluted: number; pool_pct_fully_diluted: number; classes: ClassRow[]; convertible_cents: number }
export interface Round { id: string; name: string; kind: string; date: string; pre_money: number; investment: number; price_per_share: string; class_id?: string; class?: string; notes: string; post_money: number }
export interface Upcoming { date: string; kind: string; label: string; stakeholder: string; shares?: number }
export interface Dashboard { company: Company; table: Table; capital_raised: number; last_round?: Round; implied_valuation: number; holders: number; upcoming: Upcoming[]; convertible_outstanding: number }
export interface Stakeholder { id: string; name: string; kind: string; email: string; contact_id?: string; user_id?: string; notes: string; securities: number }
export interface Class { id: string; name: string; kind: "common" | "preferred"; seniority: number; pref_multiple_milli: number; participating: boolean; convert_ratio_milli: number; issue_price: string; authorized: number; issued: number }
export interface Pool { id: string; name: string; size: number; adopted: string; notes: string }
export interface VestPoint { date: string; vested: number }
export interface EventRec { id: string; kind: string; security_id: string; label?: string; date: string; quantity: number; to_stakeholder?: string; to_name?: string; new_security_id?: string; price: string; note: string }
export interface Security {
  id: string; label: string; kind: "stock" | "option" | "warrant" | "safe" | "note"; stakeholder_id: string; stakeholder: string; class_id?: string; class?: string; pool_id?: string; round_id?: string; parent_id?: string;
  quantity: number; price: string; issue_date: string; vest_months: number; cliff_months: number; vest_freq: string; vest_start: string; expires: string; principal: number; cap: number; discount_bp: number; post_money: boolean; interest_bp: number; notes: string;
  outstanding: number; vested: number; exercised: number; cancelled: number; transferred: number; exercisable: number; converted: boolean; value?: number; events?: EventRec[]; schedule?: VestPoint[];
}
export interface SecurityInput { kind: string; stakeholder_id: string; class_id?: string; pool_id?: string; round_id?: string; quantity?: number; price?: string; issue_date: string; vest_months?: number; cliff_months?: number; vest_freq?: string; vest_start?: string; expires?: string; principal?: number; cap?: number; discount_bp?: number; post_money?: boolean; interest_bp?: number; notes?: string }
export interface Investor { stakeholder_id: string; name?: string; amount: number }
export interface RoundModelInput { name?: string; date?: string; pre_money: number; investment: number; pool_target_bp: number; convert?: string[]; investors?: Investor[]; class_id?: string; kind?: string }
export interface RoundHolder { id: string; name: string; before: number; added: number; after: number; pct_before: number; pct_after: number }
export interface RoundModel {
  price_per_share_cents: string; pre_fully_diluted: number; pool_top_up: number; new_investor_shares: number; post_fully_diluted: number; post_money_cents: number; new_investor_pct: number; notes?: string[];
  converted: { id: string; name: string; value_cents: number; shares: number; price_cents: string; cap_binds: boolean; ownership_pct: number }[];
  holders: RoundHolder[]; investors: { stakeholder_id: string; name: string; amount: number; shares: number; pct: string }[]; unallocated_shares: number; closing?: { kind: string; holder: string; shares: number; detail: string }[];
}
export interface Payee { id: string; name: string; kind: string; shares: number; payout_cents: number; converted?: boolean; exercised?: boolean; per_share_cents: string }
export interface HolderPayout { id: string; name: string; kind: string; payout: number; pct: string }
export interface ExitOutcome { exit: number; payees: Payee[]; holders: HolderPayout[]; common_per_share_cents: string; total: number; notes?: string[] }
export interface Statement { holder: Stakeholder; securities: Security[]; stock: number; options: number; vested_options: number; pct_outstanding: number; pct_fully_diluted: number }
export interface Scenario { id: string; kind: string; name: string; params: Record<string, unknown>; created_at: string }
export interface AccessEntry { user_id: string; name: string; email: string; level: string }

const B = "/api/v1/captable";
const post = <T>(u: string, body?: unknown) => jf<T>(u, { method: "POST", body: body ?? {} });
const put = <T>(u: string, body: unknown) => jf<T>(u, { method: "PUT", body });
const del = (u: string) => jf<{ ok: boolean }>(u, { method: "DELETE" });
const q = (o: Record<string, string | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v) p.set(k, v);
  return p.toString() ? `?${p}` : "";
};

export const captable = {
  whoami: () => jf<{ level: string; admin: boolean }>(`${B}/whoami`),
  meta: () => jf<{ company: Company; level: string; admin: boolean; today: string }>(`${B}/meta`),
  dashboard: () => jf<Dashboard>(`${B}/dashboard`),
  table: (asOf = "") => jf<Table>(`${B}/table${q({ as_of: asOf })}`),
  saveCompany: (c: Company) => put<Company>(`${B}/company`, c),
  people: () => jf<{ people: { id: string; name: string; email: string }[] }>(`${B}/people`).then((r) => r.people),
  access: () => jf<{ access: AccessEntry[] }>(`${B}/access`).then((r) => r.access),
  grant: (user_id: string, level: string) => post<{ ok: boolean }>(`${B}/access`, { user_id, level }),
  revoke: (id: string) => del(`${B}/access/${id}`),
  stakeholders: () => jf<{ stakeholders: Stakeholder[] }>(`${B}/stakeholders`).then((r) => r.stakeholders),
  createStakeholder: (s: Partial<Stakeholder>) => post<Stakeholder>(`${B}/stakeholders`, s),
  updateStakeholder: (id: string, s: Partial<Stakeholder>) => put<Stakeholder>(`${B}/stakeholders/${id}`, s),
  deleteStakeholder: (id: string) => del(`${B}/stakeholders/${id}`),
  statement: (id: string) => jf<Statement>(`${B}/stakeholders/${id}/statement`),
  classes: () => jf<{ classes: Class[] }>(`${B}/classes`).then((r) => r.classes),
  createClass: (c: Partial<Class>) => post<Class>(`${B}/classes`, c),
  updateClass: (id: string, c: Partial<Class>) => put<Class>(`${B}/classes/${id}`, c),
  deleteClass: (id: string) => del(`${B}/classes/${id}`),
  pools: () => jf<{ pools: Pool[] }>(`${B}/pools`).then((r) => r.pools),
  createPool: (p: Partial<Pool>) => post<Pool>(`${B}/pools`, p),
  updatePool: (id: string, p: Partial<Pool>) => put<Pool>(`${B}/pools/${id}`, p),
  deletePool: (id: string) => del(`${B}/pools/${id}`),
  rounds: () => jf<{ rounds: Round[] }>(`${B}/rounds`).then((r) => r.rounds),
  createRound: (r: Partial<Round> & { date: string }) => post<Round>(`${B}/rounds`, r),
  deleteRound: (id: string) => del(`${B}/rounds/${id}`),
  securities: (f: { holder?: string; kind?: string; as_of?: string } = {}) => jf<{ securities: Security[] }>(`${B}/securities${q(f)}`).then((r) => r.securities),
  security: (id: string) => jf<Security>(`${B}/securities/${id}`),
  issue: (s: SecurityInput) => post<Security>(`${B}/securities`, s),
  deleteSecurity: (id: string) => del(`${B}/securities/${id}`),
  record: (id: string, kind: "exercise" | "cancel" | "transfer" | "convert", e: { date: string; quantity: number; to_stakeholder?: string; price?: string; class_id?: string; note?: string }) => post<Security>(`${B}/securities/${id}/${kind}`, e),
  events: () => jf<{ events: EventRec[] }>(`${B}/events`).then((r) => r.events),
  undo: (id: string) => del(`${B}/events/${id}`),
  modelRound: (i: RoundModelInput) => post<RoundModel>(`${B}/model/round`, i),
  closeRound: (i: RoundModelInput) => post<Round>(`${B}/model/close-round`, i),
  modelExit: (exits: number[], unvested = "participate", as_of = "") => post<{ outcomes: ExitOutcome[] }>(`${B}/model/exit`, { exits, unvested, as_of }).then((r) => r.outcomes),
  scenarios: (kind = "") => jf<{ scenarios: Scenario[] }>(`${B}/scenarios${q({ kind })}`).then((r) => r.scenarios),
  saveScenario: (kind: string, name: string, params: unknown) => post<Scenario>(`${B}/scenarios`, { kind, name, params }),
  deleteScenario: (id: string) => del(`${B}/scenarios/${id}`),
  exportUrl: (what: string, asOf = "") => `${B}/export/${what}${q({ as_of: asOf })}`,
};
