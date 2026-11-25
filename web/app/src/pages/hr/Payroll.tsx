import { LoadOlder } from "../../components/LoadOlder";
import { EmployeePicker } from "./EmployeePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Input, Option, Select, Tab, TabList, TabPanel, Tabs, Table, Typography } from "@mui/joy";
import { hr } from "./api";
import type { Adjustment, EmployeeSalary, Payslip, PTSlab, Run, SalaryLine, Structure } from "./api";
import { currentPeriod, formatINR, periodLabel, shiftPeriod } from "./logic";
import { Card, Dialog, DownloadLink, ErrorAlert, Field, Row, Stat, StatusChip, useErr } from "./ui";

const CALCS = [
  { v: "fixed", l: "Fixed amount" }, { v: "pct_gross", l: "% of gross" }, { v: "pct_basic", l: "% of basic" }, { v: "balance", l: "Balancing amount" },
];

function LinesTable({ lines, setLines, monthly }: { lines: SalaryLine[]; setLines: (l: SalaryLine[]) => void; monthly: boolean }) {
  const upd = (i: number, p: Partial<SalaryLine>) => setLines(lines.map((l, j) => (j === i ? { ...l, ...p } : l)));
  return (
    <Table size="sm" aria-label="Salary components">
      <thead><tr><th>Code</th><th>Name</th><th>Kind</th>{!monthly && <th>Calc</th>}<th>{monthly ? "Monthly amount" : "Value"}</th><th>PF</th><th>ESI</th><th /></tr></thead>
      <tbody>
        {lines.map((l, i) => (
          <tr key={i}>
            <td><Input size="sm" value={l.code} onChange={(e) => upd(i, { code: e.target.value.toUpperCase() })} sx={{ width: 90 }} slotProps={{ input: { "aria-label": `Code ${i + 1}` } }} /></td>
            <td><Input size="sm" value={l.name} onChange={(e) => upd(i, { name: e.target.value })} slotProps={{ input: { "aria-label": `Name ${i + 1}` } }} /></td>
            <td>
              <Select size="sm" value={l.kind} onChange={(_, v) => upd(i, { kind: v ?? "earning" })} slotProps={{ button: { "aria-label": `Kind ${i + 1}` } }}>
                <Option value="earning">Earning</Option><Option value="deduction">Deduction</Option>
              </Select>
            </td>
            {!monthly && (
              <td>
                <Select size="sm" value={l.calc} onChange={(_, v) => upd(i, { calc: v ?? "fixed" })} slotProps={{ button: { "aria-label": `Calc ${i + 1}` } }}>
                  {CALCS.map((c) => <Option key={c.v} value={c.v}>{c.l}</Option>)}
                </Select>
              </td>
            )}
            <td>
              {!monthly && l.calc === "balance" ? <Typography level="body-xs">remainder</Typography> : (
                <Input size="sm" type="number" value={monthly ? l.amount : l.value} sx={{ width: 120 }}
                  onChange={(e) => upd(i, monthly ? { amount: Number(e.target.value) } : { value: Number(e.target.value) })} slotProps={{ input: { "aria-label": `Value ${i + 1}` } }} />
              )}
            </td>
            <td><Checkbox checked={l.pf_wage} onChange={(e) => upd(i, { pf_wage: e.target.checked })} slotProps={{ input: { "aria-label": `PF wage ${i + 1}` } }} /></td>
            <td><Checkbox checked={l.esi_wage} onChange={(e) => upd(i, { esi_wage: e.target.checked })} slotProps={{ input: { "aria-label": `ESI wage ${i + 1}` } }} /></td>
            <td><Button size="sm" variant="plain" color="danger" onClick={() => setLines(lines.filter((_, j) => j !== i))}>×</Button></td>
          </tr>
        ))}
      </tbody>
      <tfoot><tr><td colSpan={8}>
        <Button size="sm" variant="outlined" onClick={() => setLines([...lines, { code: "", name: "", kind: "earning", calc: "fixed", value: 0, amount: 0, pf_wage: false, esi_wage: true, taxable: true }])}>Add component</Button>
      </td></tr></tfoot>
    </Table>
  );
}

export function SalaryEditor({ employeeId, history, onSaved }: { employeeId: string; history: EmployeeSalary[]; onSaved: () => void }) {
  const { err, run } = useErr();
  const [structs, setStructs] = useState<Structure[]>([]);
  const [sid, setSid] = useState("");
  const [annual, setAnnual] = useState("");
  const [from, setFrom] = useState(new Date().toISOString().slice(0, 8) + "01");
  const [lines, setLines] = useState<SalaryLine[]>(history[0]?.lines ?? []);
  const [flags, setFlags] = useState({ pf_enabled: history[0]?.pf_enabled ?? true, esi_enabled: history[0]?.esi_enabled ?? true, pt_enabled: history[0]?.pt_enabled ?? true, tds_enabled: history[0]?.tds_enabled ?? false });
  useEffect(() => { void hr.structures().then((s) => { setStructs(s); setSid((c) => c || s[0]?.id || ""); }).catch(() => undefined); }, []);
  const preview = () => run(async () => {
    const s = structs.find((x) => x.id === sid);
    if (!s) throw new Error("Choose a salary structure");
    setLines(await hr.previewStructure(Number(annual), s.lines));
  });
  const monthly = lines.filter((l) => l.kind === "earning").reduce((a, l) => a + l.amount, 0);
  return (
    <Box>
      <ErrorAlert err={err} />
      {history.length > 0 && <Typography level="body-sm" data-testid="salary-current">Current: {formatINR(history[0].monthly_gross)} / month ({formatINR(history[0].annual_gross)} a year) from {history[0].effective_from}</Typography>}
      <Row sx={{ mt: 1 }}>
        <Field label="Structure"><Select value={sid} onChange={(_, v) => setSid(v ?? "")} slotProps={{ button: { "aria-label": "Structure" } }}>{structs.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}</Select></Field>
        <Field label="Annual gross (₹)"><Input type="number" value={annual} onChange={(e) => setAnnual(e.target.value)} slotProps={{ input: { "aria-label": "Annual gross" } }} /></Field>
        <Button variant="outlined" onClick={() => void preview()} data-testid="salary-preview">Preview split</Button>
      </Row>
      {lines.length > 0 && (
        <>
          <LinesTable lines={lines} setLines={setLines} monthly />
          <Typography level="body-sm" data-testid="salary-monthly">Monthly gross: {formatINR(monthly)}</Typography>
        </>
      )}
      <Row sx={{ mt: 1 }}>
        <Checkbox label="Provident Fund" checked={flags.pf_enabled} onChange={(e) => setFlags({ ...flags, pf_enabled: e.target.checked })} />
        <Checkbox label="ESI" checked={flags.esi_enabled} onChange={(e) => setFlags({ ...flags, esi_enabled: e.target.checked })} />
        <Checkbox label="Professional tax" checked={flags.pt_enabled} onChange={(e) => setFlags({ ...flags, pt_enabled: e.target.checked })} />
        <Checkbox label="TDS estimate" checked={flags.tds_enabled} onChange={(e) => setFlags({ ...flags, tds_enabled: e.target.checked })} />
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Effective from" sx={{ maxWidth: 200 }}><Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "Effective from" } }} /></Field>
        <Button disabled={lines.length === 0} onClick={() => void run(async () => { await hr.saveSalary(employeeId, { effective_from: from, structure_id: sid, lines, ...flags }); onSaved(); })} data-testid="salary-save">Save salary</Button>
      </Row>
    </Box>
  );
}

function Structures() {
  const { err, run } = useErr();
  const [list, setList] = useState<Structure[]>([]);
  const [edit, setEdit] = useState<Structure | null>(null);
  const load = useCallback(() => void run(async () => setList(await hr.structures())), [run]);
  useEffect(load, [load]);
  return (
    <Card title="Salary structures" action={<Button size="sm" onClick={() => setEdit({ id: "", name: "", lines: [{ code: "BASIC", name: "Basic", kind: "earning", calc: "pct_gross", value: 40, amount: 0, pf_wage: true, esi_wage: true, taxable: true }, { code: "SPECIAL", name: "Special Allowance", kind: "earning", calc: "balance", value: 0, amount: 0, pf_wage: false, esi_wage: true, taxable: true }] })}>New structure</Button>}>
      <ErrorAlert err={err} />
      {list.map((s) => (
        <Row key={s.id} sx={{ alignItems: "center", justifyContent: "space-between" }}>
          <Typography level="body-sm">{s.name} · {s.lines.map((l) => l.code).join(", ")}</Typography>
          <Row><Button size="sm" variant="outlined" onClick={() => setEdit(s)}>Edit</Button><Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteStructure(s.id); load(); })}>Delete</Button></Row>
        </Row>
      ))}
      <Typography level="body-xs" sx={{ mt: 1 }}>BASIC is required. PF is computed on components marked PF; ESI on those marked ESI. One balancing component absorbs the rest of the gross.</Typography>
      {edit && (
        <Dialog open onClose={() => setEdit(null)} title={edit.id ? "Edit structure" : "New structure"} wide>
          <Field label="Name"><Input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "aria-label": "Structure name" } }} /></Field>
          <LinesTable lines={edit.lines} setLines={(l) => setEdit({ ...edit, lines: l })} monthly={false} />
          <Button sx={{ mt: 1 }} onClick={() => void run(async () => { await hr.saveStructure(edit.id, edit); setEdit(null); load(); })}>Save structure</Button>
        </Dialog>
      )}
    </Card>
  );
}

function PTSlabs() {
  const { err, run } = useErr();
  const [slabs, setSlabs] = useState<PTSlab[]>([]);
  const [msg, setMsg] = useState("");
  useEffect(() => { void run(async () => setSlabs(await hr.ptSlabs())); }, [run]);
  const upd = (i: number, p: Partial<PTSlab>) => setSlabs(slabs.map((s, j) => (j === i ? { ...s, ...p } : s)));
  return (
    <Card title="Professional tax slabs (editable)" action={<Button size="sm" onClick={() => void run(async () => { await hr.savePtSlabs(slabs); setMsg("Saved"); })}>Save slabs</Button>}>
      <ErrorAlert err={err} />
      {msg && <Typography level="body-xs">{msg}</Typography>}
      <Typography level="body-xs" sx={{ mb: 1 }}>Starter values for a few states. State rules change, so verify them before running payroll. Month 0 applies every month; a specific month overrides it.</Typography>
      <Table size="sm" aria-label="PT slabs">
        <thead><tr><th>State</th><th>Salary from</th><th>Salary to</th><th>Monthly tax</th><th>Month</th><th /></tr></thead>
        <tbody>
          {slabs.map((s, i) => (
            <tr key={i}>
              <td><Input size="sm" value={s.state} onChange={(e) => upd(i, { state: e.target.value })} slotProps={{ input: { "aria-label": `PT state ${i + 1}` } }} /></td>
              <td><Input size="sm" type="number" value={s.min_salary} onChange={(e) => upd(i, { min_salary: Number(e.target.value) })} sx={{ width: 110 }} /></td>
              <td><Input size="sm" type="number" value={s.max_salary ?? ""} placeholder="no limit" onChange={(e) => upd(i, { max_salary: e.target.value === "" ? null : Number(e.target.value) })} sx={{ width: 110 }} /></td>
              <td><Input size="sm" type="number" value={s.amount} onChange={(e) => upd(i, { amount: Number(e.target.value) })} sx={{ width: 90 }} /></td>
              <td><Input size="sm" type="number" value={s.month} onChange={(e) => upd(i, { month: Number(e.target.value) })} sx={{ width: 70 }} /></td>
              <td><Button size="sm" variant="plain" color="danger" onClick={() => setSlabs(slabs.filter((_, j) => j !== i))}>×</Button></td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Button size="sm" variant="outlined" sx={{ mt: 1 }} onClick={() => setSlabs([...slabs, { state: "", min_salary: 0, max_salary: null, amount: 0, month: 0 }])}>Add slab</Button>
    </Card>
  );
}

function AdjustDialog({ slip, onClose, onDone }: { slip: Payslip; onClose: () => void; onDone: () => void }) {
  const { err, run } = useErr();
  const [ads, setAds] = useState<Adjustment[]>(slip.adjustments ?? []);
  const [claims, setClaims] = useState<{ id: string; title: string; amount: number }[]>([]);
  useEffect(() => { void hr.reimbursements(slip.id).then(setClaims).catch(() => undefined); }, [slip.id]);
  return (
    <Dialog open onClose={onClose} title={`Adjustments: ${slip.emp_name}`}>
      <ErrorAlert err={err} />
      {ads.map((a, i) => (
        <Row key={i} sx={{ mt: 0.5, alignItems: "center" }}>
          <Select size="sm" value={a.kind} onChange={(_, v) => setAds(ads.map((x, j) => (j === i ? { ...x, kind: v ?? "earning" } : x)))} slotProps={{ button: { "aria-label": `Adjustment kind ${i + 1}` } }}>
            <Option value="earning">Extra earning</Option><Option value="deduction">Deduction</Option><Option value="reimbursement">Reimbursement</Option>
          </Select>
          <Input size="sm" placeholder="Description" value={a.name} onChange={(e) => setAds(ads.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} slotProps={{ input: { "aria-label": `Adjustment name ${i + 1}` } }} />
          <Input size="sm" type="number" value={a.amount} sx={{ width: 110 }} onChange={(e) => setAds(ads.map((x, j) => (j === i ? { ...x, amount: Number(e.target.value) } : x)))} slotProps={{ input: { "aria-label": `Adjustment amount ${i + 1}` } }} />
          <Button size="sm" variant="plain" color="danger" onClick={() => setAds(ads.filter((_, j) => j !== i))}>×</Button>
        </Row>
      ))}
      <Row sx={{ mt: 1 }}>
        <Button size="sm" variant="outlined" onClick={() => setAds([...ads, { kind: "earning", name: "", amount: 0 }])}>Add adjustment</Button>
      </Row>
      {claims.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography level="title-sm">Approved expense reports (Accounting)</Typography>
          {claims.map((c) => (
            <Row key={c.id} sx={{ alignItems: "center", justifyContent: "space-between" }}>
              <Typography level="body-sm">{c.title} · {formatINR(c.amount)}</Typography>
              <Button size="sm" variant="outlined" disabled={ads.some((a) => a.expense_report_id === c.id)} onClick={() => setAds([...ads, { kind: "reimbursement", name: c.title, amount: c.amount, expense_report_id: c.id }])}>Reimburse in this payslip</Button>
            </Row>
          ))}
        </Box>
      )}
      <Button sx={{ mt: 2 }} onClick={() => void run(async () => { await hr.setAdjustments(slip.id, ads); onDone(); onClose(); })} data-testid="adjust-save">Save and recompute</Button>
    </Dialog>
  );
}

function RunView({ run0, onChanged }: { run0: Run; onChanged: () => void }) {
  const { err, run } = useErr();
  const [r, setR] = useState(run0);
  const [slips, setSlips] = useState<Payslip[]>([]);
  const [nextSlips, setNextSlips] = useState("");
  const [slipQ, setSlipQ] = useState("");
  const [adj, setAdj] = useState<Payslip | null>(null);
  // A run can hold thousands of payslips: 100 per page, searchable on the server.
  const load = useCallback(() => void run(async () => {
    const pg = await hr.runPayslipsPage(r.id, 1, slipQ);
    setSlips(pg.items);
    setNextSlips(pg.next);
  }), [r.id, run, slipQ]);
  useEffect(load, [load]);
  const act = (a: "recompute" | "review" | "reopen" | "lock") => void run(async () => { setR(await hr.runAction(r.id, a)); load(); onChanged(); });
  return (
    <Card title={`Payroll ${periodLabel(r.period)}`} action={<StatusChip status={r.status} />} sx={{ mt: 2 }}>
      <ErrorAlert err={err} />
      {r.skipped && r.skipped.length > 0 && <Typography level="body-xs" color="warning" data-testid="run-skipped">Not included: {r.skipped.join("; ")}</Typography>}
      <Row sx={{ mb: 1 }}>
        <Stat label="Employees" value={r.employees} /><Stat label="Gross" value={formatINR(r.gross)} testId="run-gross" /><Stat label="Net pay" value={formatINR(r.net_pay)} testId="run-net" />
        <Stat label="PF (EE+ER)" value={formatINR(r.pf)} /><Stat label="ESI (EE+ER)" value={formatINR(r.esi)} /><Stat label="Prof. tax" value={formatINR(r.prof_tax)} /><Stat label="TDS" value={formatINR(r.tds)} />
      </Row>
      <Input size="sm" placeholder="Find an employee in this run" value={slipQ} onChange={(e) => setSlipQ(e.target.value)} sx={{ maxWidth: 320, mb: 1 }} slotProps={{ input: { "aria-label": "Find payslip" } }} />
      <Table size="sm" aria-label="Payslips in run">
        <thead><tr><th>Employee</th><th>Paid days</th><th>Gross</th><th>PF</th><th>ESI</th><th>PT</th><th>TDS</th><th>Other</th><th>Net</th><th /></tr></thead>
        <tbody>
          {slips.map((p) => (
            <tr key={p.id} data-testid={`slip-${p.emp_code}`}>
              <td>{p.emp_name}<Typography level="body-xs">{p.emp_code}</Typography></td>
              <td>{p.paid_days}{p.lop_days > 0 ? ` (LOP ${p.lop_days})` : ""}</td><td>{formatINR(p.gross)}</td><td>{formatINR(p.pf_employee)}</td><td>{formatINR(p.esi_employee)}</td>
              <td>{formatINR(p.prof_tax)}</td><td>{formatINR(p.tds)}</td><td>{formatINR(p.other_deductions)}</td><td><b>{formatINR(p.net_pay)}</b></td>
              <td><Row>
                {r.status === "draft" && <Button size="sm" variant="outlined" onClick={() => setAdj(p)}>Adjust</Button>}
                <DownloadLink href={hr.payslipPdfUrl(p.id)} testId={`slip-pdf-${p.emp_code}`}>PDF</DownloadLink>
              </Row></td>
            </tr>
          ))}
        </tbody>
      </Table>
      <LoadOlder next={nextSlips} shown={slips.length} label="Load more payslips" testId="slips-more" loadMore={async (c) => {
        const pg = await hr.runPayslipsPage(r.id, Number(c), slipQ);
        setSlips((s) => [...s, ...pg.items]);
        setNextSlips(pg.next);
      }} />
      <Row sx={{ mt: 2 }}>
        {r.status === "draft" && <><Button variant="outlined" onClick={() => act("recompute")}>Recompute</Button><Button onClick={() => act("review")} data-testid="run-review">Mark reviewed</Button>
          <Button color="danger" variant="plain" onClick={() => { if (window.confirm("Delete this draft run?")) void run(async () => { await hr.deleteRun(r.id); onChanged(); }); }}>Delete draft</Button></>}
        {r.status === "reviewed" && <><Button variant="outlined" onClick={() => act("reopen")}>Reopen as draft</Button>
          <Button color="success" onClick={() => { if (window.confirm("Lock this run? It posts the payroll journal to Accounting, publishes payslips to employees and cannot be undone.")) act("lock"); }} data-testid="run-lock">Lock and post to Accounting</Button></>}
        {r.status !== "draft" && <><DownloadLink href={hr.bankAdviceUrl(r.id)} testId="bank-advice">Bank advice CSV</DownloadLink><DownloadLink href={hr.statutoryUrl(r.id)}>PF / ESI / PT register</DownloadLink></>}
        {r.status === "locked" && <Typography level="body-sm" data-testid="run-journal">Journal {r.journal_id ? "posted to Accounting" : "not posted"}{r.journal_id && <> · <a href="/accounting" data-testid="run-journal-link">open Accounting</a></>}</Typography>}
      </Row>
      {adj && <AdjustDialog slip={adj} onClose={() => setAdj(null)} onDone={() => { load(); onChanged(); }} />}
    </Card>
  );
}

function Runs() {
  const { err, run } = useErr();
  const [runs, setRuns] = useState<Run[]>([]);
  const [period, setPeriod] = useState(shiftPeriod(currentPeriod(), 0));
  const [open, setOpen] = useState("");
  const load = useCallback(() => void run(async () => setRuns(await hr.runs())), [run]);
  useEffect(load, [load]);
  const sel = runs.find((r) => r.id === open);
  return (
    <Box>
      <ErrorAlert err={err} />
      <Card title="Payroll runs">
        <Row>
          <Field label="Month" sx={{ maxWidth: 200 }}><Input type="month" value={period} onChange={(e) => setPeriod(e.target.value)} slotProps={{ input: { "aria-label": "Payroll month" } }} /></Field>
          <Button onClick={() => void run(async () => { const r = await hr.createRun(period); await load(); setOpen(r.id); })} data-testid="run-create">Start payroll for {periodLabel(period)}</Button>
        </Row>
        <Table size="sm" hoverRow sx={{ mt: 1 }} aria-label="Payroll runs">
          <thead><tr><th>Month</th><th>Status</th><th>Employees</th><th>Gross</th><th>Net</th></tr></thead>
          <tbody>
            {runs.map((r) => <tr key={r.id} onClick={() => setOpen(r.id)} style={{ cursor: "pointer" }} data-testid={`run-row-${r.period}`}><td>{periodLabel(r.period)}</td><td><StatusChip status={r.status} /></td><td>{r.employees}</td><td>{formatINR(r.gross)}</td><td>{formatINR(r.net_pay)}</td></tr>)}
            {runs.length === 0 && <tr><td colSpan={5}>No payroll runs yet.</td></tr>}
          </tbody>
        </Table>
      </Card>
      {sel && <RunView key={sel.id + sel.status} run0={sel} onChanged={load} />}
    </Box>
  );
}

function SalaryTab() {
  const [id, setId] = useState("");
  const [hist, setHist] = useState<EmployeeSalary[] | null>(null);
  const { err, run } = useErr();
  const load = useCallback((e: string) => void run(async () => setHist(await hr.salaries(e))), [run]);
  return (
    <Card title="Employee salary">
      <ErrorAlert err={err} />
      <Field label="Employee" sx={{ maxWidth: 360 }}>
        <EmployeePicker value={id} onChange={(v) => { setId(v); setHist(null); if (v) load(v); }} ariaLabel="Salary employee" placeholder="Choose an employee" />
      </Field>
      {id && hist && <Box sx={{ mt: 2 }}><SalaryEditor key={id + hist.length} employeeId={id} history={hist} onSaved={() => load(id)} /></Box>}
    </Card>
  );
}

export function Payroll() {
  return (
    <Tabs defaultValue="runs" sx={{ bgcolor: "transparent" }}>
      <TabList><Tab value="runs">Runs</Tab><Tab value="salary">Employee salaries</Tab><Tab value="structures">Structures</Tab><Tab value="pt">Professional tax</Tab></TabList>
      <TabPanel value="runs"><Runs /></TabPanel>
      <TabPanel value="salary"><SalaryTab /></TabPanel>
      <TabPanel value="structures"><Structures /></TabPanel>
      <TabPanel value="pt"><PTSlabs /></TabPanel>
    </Tabs>
  );
}
