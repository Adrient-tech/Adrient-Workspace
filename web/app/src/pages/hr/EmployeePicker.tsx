import { useCallback, useEffect, useState } from "react";
import { AsyncPicker, type PickerItem, type PickerPage } from "../../components/AsyncPicker";
import type { Employee } from "./api";

interface EmpItem extends PickerItem {
  emp: Employee;
}

const toItem = (e: Employee): EmpItem => ({
  id: e.id,
  label: e.name,
  secondary: [e.emp_code, e.work_email].filter(Boolean).join(" · "),
  tertiary: [e.designation_name, e.department_name].filter(Boolean).join(" · ") || undefined,
  emp: e,
});

/** One page of HR employees from the server (search by name, code, email, designation, department). */
export async function fetchEmployeePage(q: string, cursor: string | undefined, signal?: AbortSignal, extra = ""): Promise<PickerPage<Employee>> {
  const p = new URLSearchParams({ limit: "30", page: cursor ?? "1" });
  if (q) p.set("q", q);
  const r = await fetch(`/api/v1/hr/employees?${p}${extra}`, { credentials: "same-origin", signal });
  if (!r.ok) throw new Error(`Could not search employees (HTTP ${r.status})`);
  const d = (await r.json()) as { items?: Employee[]; employees?: Employee[]; next_cursor?: string };
  return { items: d.items ?? d.employees ?? [], next_cursor: d.next_cursor || undefined };
}

/**
 * Server-searched picker over HR employees (ids are hr_employees ids, not user
 * ids). `known` supplies the selected employee when the caller has it.
 */
export function EmployeePicker({
  value,
  onChange,
  known,
  placeholder = "Search employees",
  ariaLabel = "Employee",
  excludeId,
  testId,
}: {
  value: string;
  onChange: (id: string, emp?: Employee) => void;
  known?: { id: string; name: string }[];
  placeholder?: string;
  ariaLabel?: string;
  excludeId?: string;
  testId?: string;
}) {
  const [sel, setSel] = useState<EmpItem | null>(null);
  const [loaded, setLoaded] = useState<{ id: string; name: string } | null>(null);
  useEffect(() => {
    if (!value || known?.some((k) => k.id === value) || sel?.id === value) return;
    let live = true;
    fetch(`/api/v1/hr/employees/${encodeURIComponent(value)}`, { credentials: "same-origin" })
      .then((r) => (r.ok ? r.json() : null))
      .then((e: Employee | null) => live && e && setLoaded({ id: e.id, name: e.name }))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [value, known, sel]);
  const fetchPage = useCallback(
    async (q: string, cursor: string | undefined, signal: AbortSignal): Promise<PickerPage<EmpItem>> => {
      const p = await fetchEmployeePage(q, cursor, signal);
      return { items: p.items.filter((e) => e.id !== excludeId).map(toItem), next_cursor: p.next_cursor };
    },
    [excludeId],
  );
  const k = known?.find((x) => x.id === value) ?? (loaded?.id === value ? loaded : null);
  const shown: EmpItem[] =
    !value ? [] : sel?.id === value ? [sel] : [{ id: value, label: k?.name ?? "…", emp: { id: value, name: k?.name ?? "" } as Employee }];
  return (
    <AsyncPicker<EmpItem>
      fetchPage={fetchPage}
      value={shown}
      onChange={(n) => {
        setSel(n[0] ?? null);
        onChange(n[0]?.id ?? "", n[0]?.emp);
      }}
      placeholder={placeholder}
      ariaLabel={ariaLabel}
      testId={testId}
      recentKey="hr-employees"
      emptyText="No employees match."
    />
  );
}

/** Multi-select over HR employees; keeps the picked rows so chips show names. */
export function EmployeeMultiPicker({
  value,
  onChange,
  ariaLabel = "Employees",
  placeholder = "Add employees",
  testId,
}: {
  value: string[];
  onChange: (ids: string[]) => void;
  ariaLabel?: string;
  placeholder?: string;
  testId?: string;
}) {
  const [items, setItems] = useState<EmpItem[]>([]);
  const fetchPage = useCallback(async (q: string, cursor: string | undefined, signal: AbortSignal): Promise<PickerPage<EmpItem>> => {
    const p = await fetchEmployeePage(q, cursor, signal);
    return { items: p.items.map(toItem), next_cursor: p.next_cursor };
  }, []);
  const shown = value.map((id) => items.find((i) => i.id === id) ?? { id, label: "…", emp: { id } as Employee });
  return (
    <AsyncPicker<EmpItem>
      multiple
      fetchPage={fetchPage}
      value={shown}
      onChange={(n) => {
        setItems(n);
        onChange(n.map((i) => i.id));
      }}
      placeholder={placeholder}
      ariaLabel={ariaLabel}
      testId={testId}
      recentKey="hr-employees"
      emptyText="No employees match."
    />
  );
}
