import { DataTable, type Column, type TablePage, type TableQuery } from "../../components/DataTable";
import { EmployeePicker } from "./EmployeePicker";
import { useCallback, useEffect, useMemo, useState } from "react";
import { deepParam } from "../../lib/deepLink";
import { Avatar, Box, Button, Input, Option, Select, Tab, TabList, TabPanel, Tabs, Typography } from "@mui/joy";
import { hr } from "./api";
import type { Balance, Document, Employee, EmployeeSalary, Lookup, ImportResult } from "./api";
import { EMPLOYMENT_TYPES, formatINR, initials } from "./logic";
import { Card, Dialog, DownloadLink, ErrorAlert, Field, FileButton, Row, StatusChip, useErr } from "./ui";
import { SalaryEditor } from "./Payroll";

const blank = (): Partial<Employee> => ({ first_name: "", last_name: "", work_email: "", phone: "", join_date: "", employment_type: "full_time", status: "active", gender: "" });

function EmployeeForm({ initial, lookups, people, onSaved, onClose }: {
  initial: Partial<Employee>; lookups: { dep: Lookup[]; des: Lookup[]; loc: Lookup[] }; people: Employee[]; onSaved: (e: Employee) => void; onClose: () => void;
}) {
  const { err, run } = useErr();
  const [f, setF] = useState<Partial<Employee>>(initial);
  const [users, setUsers] = useState<{ id: string; name: string; email: string }[]>([]);
  useEffect(() => { void hr.availableUsers().then(setUsers).catch(() => undefined); }, []);
  const set = (k: keyof Employee, v: string) => setF((p) => ({ ...p, [k]: v }));
  const text = (k: keyof Employee, label: string, type = "text") => (
    <Field label={label}><Input type={type} value={(f[k] as string) ?? ""} onChange={(e) => set(k, e.target.value)} slotProps={{ input: { "aria-label": label } }} /></Field>
  );
  const pick = (k: keyof Employee, label: string, opts: { v: string; l: string }[]) => (
    <Field label={label}>
      <Select value={(f[k] as string) ?? ""} onChange={(_, v) => set(k, v ?? "")} slotProps={{ button: { "aria-label": label } }}>
        <Option value="">None</Option>
        {opts.map((o) => <Option key={o.v} value={o.v}>{o.l}</Option>)}
      </Select>
    </Field>
  );
  const save = async () => {
    const saved = await run(() => hr.saveEmployee(initial.id ?? "", f));
    if (saved) onSaved(saved);
  };
  return (
    <>
      <ErrorAlert err={err} />
      <Row>{text("first_name", "First name")}{text("last_name", "Last name")}{text("emp_code", "Employee code (auto if blank)")}</Row>
      <Row sx={{ mt: 1 }}>{text("work_email", "Work email")}{text("phone", "Phone")}{text("join_date", "Join date", "date")}</Row>
      <Row sx={{ mt: 1 }}>
        {pick("department_id", "Department", lookups.dep.map((d) => ({ v: d.id, l: d.name })))}
        {pick("designation_id", "Designation", lookups.des.map((d) => ({ v: d.id, l: d.name })))}
        {pick("location_id", "Work location", lookups.loc.map((d) => ({ v: d.id, l: d.name })))}
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Manager">
          <EmployeePicker value={(f.manager_id as string) ?? ""} known={people} excludeId={initial.id} onChange={(id) => set("manager_id", id)} placeholder="None" ariaLabel="Manager" />
        </Field>
        {pick("employment_type", "Employment type", EMPLOYMENT_TYPES.map((t) => ({ v: t.v, l: t.l })))}
        {pick("status", "Status", [{ v: "active", l: "Active" }, { v: "notice", l: "On notice" }, { v: "exited", l: "Exited" }])}
      </Row>
      <Row sx={{ mt: 1 }}>
        {pick("user_id", "Linked account", [...(initial.user_id ? [{ v: initial.user_id, l: "Current account" }] : []), ...users.map((u) => ({ v: u.id, l: `${u.name} (${u.email})` }))])}
        {text("dob", "Date of birth", "date")}{pick("gender", "Gender", [{ v: "female", l: "Female" }, { v: "male", l: "Male" }, { v: "other", l: "Other" }])}
      </Row>
      <Row sx={{ mt: 1 }}>{text("pan", "PAN")}{text("uan", "UAN (PF)")}{text("esi_number", "ESI number")}{text("aadhaar_last4", "Aadhaar (last 4)")}</Row>
      <Row sx={{ mt: 1 }}>{text("bank_name", "Bank")}{text("bank_account", "Account number")}{text("bank_ifsc", "IFSC")}</Row>
      {f.status === "exited" && <Row sx={{ mt: 1 }}>{text("exit_date", "Exit date", "date")}{text("exit_reason", "Exit reason")}</Row>}
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
        <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
        <Button onClick={() => void save()} data-testid="employee-save">Save employee</Button>
      </Row>
    </>
  );
}

function EmployeeDetail({ id, isHR, lookups, people, onChanged, onClose }: {
  id: string; isHR: boolean; lookups: { dep: Lookup[]; des: Lookup[]; loc: Lookup[] }; people: Employee[]; onChanged: () => void; onClose: () => void;
}) {
  const { err, run } = useErr();
  const [e, setE] = useState<Employee | null>(null);
  const [docs, setDocs] = useState<Document[]>([]);
  const [sal, setSal] = useState<EmployeeSalary[]>([]);
  const [bal, setBal] = useState<Balance[]>([]);
  const [cat, setCat] = useState("offer_letter");
  const [edit, setEdit] = useState(false);
  const [links, setLinks] = useState<{ app: string; label: string; href: string }[]>([]);
  const load = useCallback(() => {
    void run(async () => {
      const emp = await hr.employee(id);
      setE(emp);
      if (emp.full) {
        setDocs(await hr.documents(id));
        setSal(await hr.salaries(id));
      }
      setBal(await hr.balances(id).catch(() => []));
      fetch(`/api/v1/bridge/links?app=hr&type=employee&id=${encodeURIComponent(id)}`, { credentials: "same-origin" })
        .then((r) => (r.ok ? r.json() : { links: [] }))
        .then((j: { links?: { app: string; label: string; href: string }[] }) => setLinks(j.links ?? []))
        .catch(() => setLinks([]));
    });
  }, [id, run]);
  useEffect(load, [load]);
  if (!e) return <ErrorAlert err={err} />;
  return (
    <Dialog open onClose={onClose} title={e.name} wide>
      <ErrorAlert err={err} />
      <Typography level="body-sm">{e.emp_code} · {e.designation_name || "—"} · {e.department_name || "—"} · {e.location_name || "—"} · <StatusChip status={e.status} /></Typography>
      {(links.length > 0 || (e.full && (e.contact_id || e.user_id))) && (
        <Row sx={{ mt: 0.5, flexWrap: "wrap" }} data-testid="employee-links">
          {e.full && e.contact_id && <Button size="sm" variant="soft" component="a" href="/contacts">Open in Contacts</Button>}
          {e.user_id && <Button size="sm" variant="soft" component="a" href={`/org-chart?user=${e.user_id}`}>Open in directory</Button>}
          {links.map((l) => <Button key={l.href} size="sm" variant="soft" component="a" href={l.href}>{l.label}</Button>)}
        </Row>
      )}
      <Tabs defaultValue="profile" sx={{ mt: 1 }}>
        <TabList>
          <Tab value="profile">Profile</Tab>
          {e.full && <Tab value="docs">Documents</Tab>}
          {e.full && <Tab value="salary">Salary</Tab>}
          {bal.length > 0 && <Tab value="leave">Leave</Tab>}
        </TabList>
        <TabPanel value="profile">
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill,minmax(240px,1fr))", gap: 1 }}>
            {([["Work email", e.work_email], ["Phone", e.phone], ["Manager", e.manager_name], ["Employment", e.employment_type], ["Joined", e.join_date],
              ...(e.full ? [["Date of birth", e.dob], ["Personal email", e.personal_email], ["PAN", e.pan], ["UAN", e.uan], ["Bank", `${e.bank_name} ${e.bank_account} ${e.bank_ifsc}`.trim()], ["Address", e.address]] : [])] as string[][]).map(([k, v]) => (
              <Box key={k}><Typography level="body-xs">{k}</Typography><Typography level="body-sm">{v || "—"}</Typography></Box>
            ))}
          </Box>
          {isHR && (
            <Row sx={{ mt: 2 }}>
              <Button size="sm" onClick={() => setEdit(true)} data-testid="employee-edit">Edit</Button>
              <Button size="sm" color="danger" variant="outlined" onClick={() => { if (window.confirm(`Delete ${e.name}? This only works for employees without payroll or leave history.`)) void run(async () => { await hr.deleteEmployee(e.id); onChanged(); onClose(); }); }}>Delete</Button>
            </Row>
          )}
        </TabPanel>
        <TabPanel value="docs">
          <Row sx={{ mb: 1, alignItems: "center" }}>
            <Select size="sm" value={cat} onChange={(_, v) => setCat(v ?? "other")} slotProps={{ button: { "aria-label": "Document category" } }}>
              {["offer_letter", "id_proof", "address_proof", "contract", "education", "other"].map((c) => <Option key={c} value={c}>{c.replace("_", " ")}</Option>)}
            </Select>
            <FileButton label="Upload document" onFile={(f) => void run(async () => { await hr.uploadDocument(e.id, cat, f); load(); })} />
          </Row>
          <Typography level="body-xs">Stored in Drive under HR Documents; only HR and this employee can open them.</Typography>
          {docs.map((d) => (
            <Row key={d.id} sx={{ alignItems: "center", justifyContent: "space-between", mt: 0.5 }}>
              <Typography level="body-sm">{d.name} · {d.category.replace("_", " ")} · {d.created_at}</Typography>
              <Row>
                <DownloadLink href={hr.documentUrl(d.id)}>Download</DownloadLink>
                {isHR && <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteDocument(d.id); load(); })}>Delete</Button>}
              </Row>
            </Row>
          ))}
        </TabPanel>
        <TabPanel value="salary">
          {isHR ? <SalaryEditor employeeId={e.id} history={sal} onSaved={load} /> : sal.map((s) => <Typography key={s.id} level="body-sm">From {s.effective_from}: {formatINR(s.monthly_gross)} / month</Typography>)}
        </TabPanel>
        <TabPanel value="leave">
          {bal.filter((b) => b.tracked).map((b) => <Typography key={b.type_id} level="body-sm">{b.name}: {b.balance} left (used {b.used})</Typography>)}
          {isHR && <AdjustBalance employeeId={e.id} balances={bal} onDone={load} />}
        </TabPanel>
      </Tabs>
      {edit && (
        <Dialog open onClose={() => setEdit(false)} title={`Edit ${e.name}`} wide>
          <EmployeeForm initial={e} lookups={lookups} people={people} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load(); onChanged(); }} />
        </Dialog>
      )}
    </Dialog>
  );
}

function AdjustBalance({ employeeId, balances, onDone }: { employeeId: string; balances: Balance[]; onDone: () => void }) {
  const { err, run } = useErr();
  const [t, setT] = useState(balances.find((b) => b.tracked)?.type_id ?? "");
  const [days, setDays] = useState("1");
  const [note, setNote] = useState("");
  return (
    <Box sx={{ mt: 2 }}>
      <Typography level="title-sm">Adjust balance (HR)</Typography>
      <ErrorAlert err={err} />
      <Row>
        <Select size="sm" value={t} onChange={(_, v) => setT(v ?? "")} slotProps={{ button: { "aria-label": "Adjust leave type" } }}>
          {balances.filter((b) => b.tracked).map((b) => <Option key={b.type_id} value={b.type_id}>{b.name}</Option>)}
        </Select>
        <Input size="sm" type="number" value={days} onChange={(e) => setDays(e.target.value)} slotProps={{ input: { "aria-label": "Days" } }} sx={{ width: 90 }} />
        <Input size="sm" placeholder="Reason (required)" value={note} onChange={(e) => setNote(e.target.value)} />
        <Button size="sm" onClick={() => void run(async () => { await hr.adjustBalance(employeeId, t, Number(days), note); setNote(""); onDone(); })}>Apply</Button>
      </Row>
    </Box>
  );
}

function LookupEditor({ kind, title, items, withState, onChanged }: { kind: "departments" | "designations" | "locations"; title: string; items: Lookup[]; withState?: boolean; onChanged: () => void }) {
  const { err, run } = useErr();
  const [name, setName] = useState("");
  const [state, setState] = useState("");
  return (
    <Card title={title} sx={{ flex: 1, minWidth: 260 }}>
      <ErrorAlert err={err} />
      {items.map((i) => (
        <Row key={i.id} sx={{ alignItems: "center", justifyContent: "space-between" }}>
          <Typography level="body-sm">{i.name}{i.state ? ` (${i.state})` : ""} · {i.count}</Typography>
          <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteLookup(kind, i.id); onChanged(); })}>Remove</Button>
        </Row>
      ))}
      <Row sx={{ mt: 1 }}>
        <Input size="sm" placeholder="New name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": `New ${kind}` } }} />
        {withState && <Input size="sm" placeholder="State (for professional tax)" value={state} onChange={(e) => setState(e.target.value)} slotProps={{ input: { "aria-label": "State" } }} />}
        <Button size="sm" onClick={() => void run(async () => { await hr.saveLookup(kind, "", { name, state }); setName(""); setState(""); onChanged(); })}>Add</Button>
      </Row>
    </Card>
  );
}

export function People({ isHR }: { isHR: boolean }) {
  const { err, run } = useErr();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [dep, setDep] = useState("");
  const [lk, setLk] = useState<{ dep: Lookup[]; des: Lookup[]; loc: Lookup[] }>({ dep: [], des: [], loc: [] });
  const [open, setOpen] = useState(() => deepParam("employee"));
  const [adding, setAdding] = useState(false);
  const [imp, setImp] = useState<ImportResult | null>(null);
  const loadLookups = useCallback(() => void Promise.all([hr.lookups("departments"), hr.lookups("designations"), hr.lookups("locations")]).then(([a, b, c]) => setLk({ dep: a, des: b, loc: c })).catch(() => undefined), []);
  const [reload, setReload] = useState(0);
  const [qd, setQd] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setQd(q), 250);
    return () => clearTimeout(t);
  }, [q]);
  const load = useCallback(() => setReload((n) => n + 1), []);
  // Server-paged employee list; the filter inputs above feed the query.
  const loadEmployees = useCallback(
    async (t: TableQuery, signal: AbortSignal): Promise<TablePage<Employee>> => {
      const p = new URLSearchParams({ limit: String(t.limit), page: String(t.page), sort: t.sort || "name", order: t.order });
      if (qd.trim()) p.set("q", qd.trim());
      if (status) p.set("status", status);
      if (dep) p.set("department_id", dep);
      const r = await fetch(`/api/v1/hr/employees?${p}`, { credentials: "same-origin", signal });
      if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
      const d = (await r.json()) as { items: Employee[]; next_cursor?: string };
      return { items: d.items ?? [], next_cursor: d.next_cursor };
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [qd, status, dep, reload],
  );
  const empColumns: Column<Employee>[] = useMemo(
    () => [
      { key: "name", header: "Employee", sortable: true, width: "30%", render: (e) => <Box sx={{ display: "flex", alignItems: "center", gap: 1 }} data-testid={`employee-row-${e.emp_code}`}><Avatar size="sm">{initials(e.name)}</Avatar><Box sx={{ minWidth: 0 }}><Typography level="title-sm" noWrap>{e.name}</Typography><Typography level="body-xs" noWrap>{e.work_email}</Typography></Box></Box> },
      { key: "code", header: "Code", sortable: true, width: 100, render: (e) => e.emp_code },
      { key: "designation", header: "Designation", render: (e) => e.designation_name || "—" },
      { key: "department", header: "Department", sortable: true, render: (e) => e.department_name || "—" },
      { key: "location", header: "Location", render: (e) => e.location_name || "—" },
      { key: "manager", header: "Manager", render: (e) => e.manager_name || "—" },
      { key: "status", header: "Status", sortable: true, width: 100, render: (e) => <StatusChip status={e.status} /> },
    ],
    [],
  );
  useEffect(loadLookups, [loadLookups]);
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Row>
        <Field label="Search" sx={{ maxWidth: 320 }}><Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Name, code, email, designation" slotProps={{ input: { "aria-label": "Search employees" } }} /></Field>
        <Field label="Department" sx={{ maxWidth: 220 }}>
          <Select value={dep} onChange={(_, v) => setDep(v ?? "")} slotProps={{ button: { "aria-label": "Filter department" } }}>
            <Option value="">All</Option>
            {lk.dep.map((d) => <Option key={d.id} value={d.id}>{d.name}</Option>)}
          </Select>
        </Field>
        {isHR && (
          <Field label="Status" sx={{ maxWidth: 160 }}>
            <Select value={status} onChange={(_, v) => setStatus(v ?? "")} slotProps={{ button: { "aria-label": "Filter status" } }}>
              <Option value="">Current</Option><Option value="exited">Exited</Option><Option value="all">All</Option>
            </Select>
          </Field>
        )}
        <Box sx={{ flex: 1 }} />
        {isHR && (
          <>
            <FileButton label="Import CSV" accept=".csv,text/csv" onFile={(f) => void run(async () => { setImp(await hr.importEmployees(f)); load(); loadLookups(); })} />
            <DownloadLink href="/api/v1/hr/employees/export.csv" testId="employees-export">Export CSV</DownloadLink>
            <Button onClick={() => setAdding(true)} data-testid="employee-add">Add employee</Button>
          </>
        )}
      </Row>
      {imp && (
        <Card title="Import result" action={<Button size="sm" variant="plain" onClick={() => setImp(null)}>Dismiss</Button>}>
          <Typography level="body-sm" data-testid="import-result">{imp.created ?? 0} created, {imp.updated ?? 0} updated, {imp.errors.length} problem row(s).</Typography>
          {imp.errors.slice(0, 20).map((x) => <Typography key={x.row} level="body-xs" color="danger">Row {x.row}: {x.error}</Typography>)}
          <Typography level="body-xs" sx={{ mt: 1 }}>Columns: emp_code, first_name, last_name, work_email, phone, join_date, dob, gender, employment_type, status, department, designation, location, manager_code, pan, uan, bank_name, bank_account, bank_ifsc.</Typography>
        </Card>
      )}
      <DataTable<Employee>
        testId="employee-table"
        columns={empColumns}
        rowKey={(e) => e.id}
        load={loadEmployees}
        mode="page"
        defaultSort="name"
        defaultOrder="asc"
        search={false}
        filters={[]}
        urlPrefix="emp_"
        reloadKey={reload}
        onRowClick={(e) => setOpen(e.id)}
        emptyText="No employees found."
      />
      {isHR && (
        <Row sx={{ alignItems: "stretch" }}>
          <LookupEditor kind="departments" title="Departments" items={lk.dep} onChanged={loadLookups} />
          <LookupEditor kind="designations" title="Designations" items={lk.des} onChanged={loadLookups} />
          <LookupEditor kind="locations" title="Work locations" items={lk.loc} withState onChanged={loadLookups} />
        </Row>
      )}
      {open && <EmployeeDetail id={open} isHR={isHR} lookups={lk} people={[]} onChanged={load} onClose={() => setOpen("")} />}
      {adding && (
        <Dialog open onClose={() => setAdding(false)} title="Add employee" wide>
          <EmployeeForm initial={blank()} lookups={lk} people={[]} onClose={() => setAdding(false)} onSaved={(e) => { setAdding(false); load(); setOpen(e.id); }} />
        </Dialog>
      )}
    </Box>
  );
}
