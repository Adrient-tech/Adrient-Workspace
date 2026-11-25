// Client for the org directory: /api/v1/people and /api/v1/departments
// (internal/people). Same-origin fetch; error bodies surface as Error messages.

export interface Person {
  id: string;
  name: string;
  email: string;
  job_title: string;
  phone: string;
  location: string;
  employee_id: string;
  start_date: string;
  department_id: string;
  department_name: string;
  manager_id: string;
  manager_name: string;
}

export interface Department {
  id: string;
  parent_id: string;
  name: string;
  head_user_id: string;
  head_name: string;
  member_count: number;
}

export interface PersonProfile {
  person: Person;
  manager?: Person;
  manager_chain: Person[];
  direct_reports: Person[];
  department?: Department;
  avatar_url: string;
}

/** Fields an admin may edit; "" clears, undefined leaves unchanged. */
export interface PersonPatch {
  job_title?: string;
  phone?: string;
  location?: string;
  employee_id?: string;
  start_date?: string;
  department_id?: string;
  manager_id?: string;
}

async function call<T>(method: string, url: string, body?: unknown): Promise<T> {
  const r = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!r.ok) {
    const text = (await r.text()).trim();
    throw new Error(text || `HTTP ${r.status}`);
  }
  return (await r.json()) as T;
}

export const peopleApi = {
  search: (query: string, limit = 50) =>
    call<{ people: Person[] }>(
      "GET",
      `/api/v1/people?query=${encodeURIComponent(query)}&limit=${limit}`,
    ).then((r) => r.people ?? []),
  profile: (id: string) =>
    call<PersonProfile>("GET", `/api/v1/people/${encodeURIComponent(id)}`),
  update: (id: string, patch: PersonPatch) =>
    call<{ person: Person }>(
      "PATCH",
      `/api/v1/people/${encodeURIComponent(id)}`,
      patch,
    ).then((r) => r.person),
  orgChart: () =>
    call<{ people: Person[]; departments: Department[] }>(
      "GET",
      "/api/v1/people/orgchart",
    ),
  departments: () =>
    call<{ departments: Department[] }>("GET", "/api/v1/departments").then(
      (r) => r.departments ?? [],
    ),
  createDepartment: (d: { name: string; parent_id?: string; head_user_id?: string }) =>
    call<{ department: Department }>("POST", "/api/v1/departments", d).then(
      (r) => r.department,
    ),
  updateDepartment: (
    id: string,
    d: { name?: string; parent_id?: string; head_user_id?: string },
  ) =>
    call<{ department: Department }>(
      "PATCH",
      `/api/v1/departments/${encodeURIComponent(id)}`,
      d,
    ).then((r) => r.department),
  deleteDepartment: (id: string) =>
    call<{ ok: boolean }>("DELETE", `/api/v1/departments/${encodeURIComponent(id)}`),
};

/** Resolve a person by exact email (case-insensitive) via the search API. */
export async function findPersonByEmail(email: string): Promise<Person | null> {
  const e = email.trim().toLowerCase();
  if (!e) return null;
  const list = await peopleApi.search(e, 10);
  return list.find((p) => p.email.toLowerCase() === e) ?? null;
}
