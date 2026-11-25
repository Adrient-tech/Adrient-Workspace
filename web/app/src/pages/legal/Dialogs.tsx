import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useState, type ReactNode } from "react";
import { Alert, Box, Button, Checkbox, DialogContent, DialogTitle, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography } from "@mui/joy";
import { errText } from "../know/http";
import { legal, type Client, type Deadline, type Entry, type Matter, type Meta, type Party, type TimeCand } from "./api";
import { fmtHM, fmtMoney, KIND_LABEL, parseDuration, parseMoney, roundUp } from "./logic";

const LB = { listbox: { sx: { zIndex: 2000 } } };

export function Dialog({ title, onClose, children, width = 560 }: { title: string; onClose: () => void; children: ReactNode; width?: number }) {
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width, maxWidth: "100vw", maxHeight: "96vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }} role="dialog" aria-label={title}>
        <ModalClose />
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>{children}</DialogContent>
      </ModalDialog>
    </Modal>
  );
}

function Field({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return (
    <FormControl sx={wide ? { gridColumn: "1 / -1" } : undefined}>
      <FormLabel>{label}</FormLabel>
      {children}
    </FormControl>
  );
}
const grid = { display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 } as const;

function Foot({ err, busy, onSave, onClose, label = "Save" }: { err: string; busy: boolean; onSave: () => void; onClose: () => void; label?: string }) {
  return (
    <>
      {err && <Alert color="danger" sx={{ mt: 1.5 }}>{err}</Alert>}
      <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
        <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
        <Button loading={busy} onClick={onSave}>{label}</Button>
      </Box>
    </>
  );
}

// ---- client ----
export function ClientForm({ initial, onClose, onSaved }: { initial?: Client; onClose: () => void; onSaved: (c: Client) => void }) {
  const [f, setF] = useState({ kind: initial?.kind ?? "person", name: initial?.name ?? "", email: initial?.email ?? "", phone: initial?.phone ?? "", address: initial?.address ?? "", rate: initial ? (initial.rate / 100).toString() : "", notes: initial?.notes ?? "", archived: initial?.archived ?? false });
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<{ id: string; name: string; company: string; email: string; phone: string }[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (initial || q.length < 2) return setHits([]);
    const t = setTimeout(() => legal.contacts(q).then(setHits).catch(() => setHits([])), 200);
    return () => clearTimeout(t);
  }, [q, initial]);
  const save = async () => {
    const rate = f.rate.trim() ? parseMoney(f.rate) : 0;
    if (rate === null) return setErr("Enter the hourly rate as an amount like 4500 or 4,500.50");
    setBusy(true);
    try {
      const body = { ...f, rate, kind: f.kind as "person" };
      onSaved(initial ? await legal.updateClient(initial.id, body) : await legal.createClient(body));
    } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={initial ? "Edit client" : "New client"} onClose={onClose}>
      {!initial && (
        <Box sx={{ mb: 1.5 }}>
          <Input size="sm" placeholder="Start from a contact in Contacts…" value={q} onChange={(e) => setQ(e.target.value)} />
          {hits.map((h) => (
            <Button key={h.id} size="sm" variant="plain" sx={{ display: "block", textAlign: "left" }} onClick={async () => { try { onSaved(await legal.clientFromContact(h.id)); } catch (e) { setErr(errText(e)); } }}>
              {h.name}{h.company ? ` · ${h.company}` : ""}
            </Button>
          ))}
        </Box>
      )}
      <Box sx={grid}>
        <Field label="Name" wide><Input autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} data-testid="client-name" /></Field>
        <Field label="Type"><Select value={f.kind} onChange={(_, v) => v && setF({ ...f, kind: v })} slotProps={LB}><Option value="person">Person</Option><Option value="company">Company</Option></Select></Field>
        <Field label="Hourly rate (₹), optional"><Input value={f.rate} onChange={(e) => setF({ ...f, rate: e.target.value })} placeholder="uses the firm default" /></Field>
        <Field label="Email"><Input value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></Field>
        <Field label="Phone"><Input value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} /></Field>
        <Field label="Address" wide><Textarea minRows={2} value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} /></Field>
        <Field label="Notes" wide><Textarea minRows={2} value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></Field>
        {initial && <Checkbox label="Archived (hide from pickers)" checked={f.archived} onChange={(e) => setF({ ...f, archived: e.target.checked })} />}
      </Box>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} />
    </Dialog>
  );
}

// ---- matter ----
export function MatterForm({ initial, meta, clients, people, onClose, onSaved }: { initial?: Matter; meta: Meta; clients: Client[]; people: { id: string; name: string }[]; onClose: () => void; onSaved: (m: Matter) => void }) {
  const [f, setF] = useState({
    title: initial?.title ?? "", description: initial?.description ?? "", client_id: initial?.client_id ?? "", practice_area: initial?.practice_area ?? "", stage: initial?.stage ?? "",
    responsible_id: initial?.responsible_id ?? "", sol_date: initial?.sol_date ?? "", billing_method: initial?.billing_method ?? "hourly",
    rate: initial?.rate ? (initial.rate / 100).toString() : "", flat_fee: initial?.flat_fee ? (initial.flat_fee / 100).toString() : "", contingency: initial ? (initial.contingency_bp / 100).toString() : "0",
    budget: initial?.budget ? (initial.budget / 100).toString() : "", tags: (initial?.tags ?? []).join(", "), access: initial?.access ?? "org",
  });
  const [parties, setParties] = useState<Party[]>(initial?.parties ?? []);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: string, v: string) => setF((x) => ({ ...x, [k]: v }));
  const save = async () => {
    const money = (s: string) => (s.trim() ? parseMoney(s) : 0);
    const rate = money(f.rate), flat = money(f.flat_fee), budget = money(f.budget);
    const cont = Math.round(Number(f.contingency || 0) * 100);
    if (rate === null || flat === null || budget === null) return setErr("Amounts must look like 4500 or 4,500.50");
    if (!(cont >= 0 && cont <= 10000)) return setErr("The contingency share is a percentage from 0 to 100");
    setBusy(true);
    try {
      const body = { ...f, rate, flat_fee: flat, budget, contingency_bp: cont, billing_method: f.billing_method as "hourly", access: f.access as "org", tags: f.tags.split(",").map((s) => s.trim()).filter(Boolean), parties: parties.filter((p) => p.name.trim()) };
      onSaved(initial ? await legal.updateMatter(initial.id, body) : await legal.createMatter(body));
    } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={initial ? `Edit matter ${initial.number}` : "New matter"} onClose={onClose} width={720}>
      <Box sx={grid}>
        <Field label="Title" wide><Input autoFocus value={f.title} onChange={(e) => set("title", e.target.value)} data-testid="matter-title" /></Field>
        <Field label="Client">
          <Select value={f.client_id} placeholder="Choose a client" onChange={(_, v) => set("client_id", v ?? "")} slotProps={LB}>
            {clients.filter((c) => !c.archived || c.id === f.client_id).map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
          </Select>
        </Field>
        <Field label="Responsible lawyer">
          <PersonSelect value={f.responsible_id} known={people} onChange={(id) => set("responsible_id", id)} placeholder="Me" ariaLabel="Person" />
        </Field>
        <Field label="Practice area">
          <Select value={f.practice_area} placeholder="Choose" onChange={(_, v) => set("practice_area", v ?? "")} slotProps={LB}>{meta.settings.practice_areas.map((a) => <Option key={a} value={a}>{a}</Option>)}</Select>
        </Field>
        <Field label="Stage"><Input value={f.stage} onChange={(e) => set("stage", e.target.value)} placeholder="e.g. Pleadings, Drafting" /></Field>
        <Field label="Billing">
          <Select value={f.billing_method} onChange={(_, v) => v && set("billing_method", v)} slotProps={LB}><Option value="hourly">Hourly</Option><Option value="flat">Flat fee</Option><Option value="contingency">Contingency</Option></Select>
        </Field>
        {f.billing_method === "hourly" && <Field label="Matter rate per hour (₹), optional"><Input value={f.rate} onChange={(e) => set("rate", e.target.value)} placeholder={`firm default ${fmtMoney(meta.settings.default_rate)}`} /></Field>}
        {f.billing_method === "flat" && <Field label="Flat fee (₹)"><Input value={f.flat_fee} onChange={(e) => set("flat_fee", e.target.value)} /></Field>}
        {f.billing_method === "contingency" && <Field label="Contingency share (%)"><Input value={f.contingency} onChange={(e) => set("contingency", e.target.value)} /></Field>}
        <Field label="Budget (₹), optional"><Input value={f.budget} onChange={(e) => set("budget", e.target.value)} /></Field>
        <Field label="Limitation date, optional"><Input type="date" value={f.sol_date} onChange={(e) => set("sol_date", e.target.value)} /></Field>
        <Field label="Who can see it">
          <Select value={f.access} onChange={(_, v) => v && set("access", v)} slotProps={LB}><Option value="org">Everyone with access to Legal</Option><Option value="restricted">Restricted (ethical wall): only the people on it and admins</Option></Select>
        </Field>
        <Field label="Tags (comma separated)" wide><Input value={f.tags} onChange={(e) => set("tags", e.target.value)} /></Field>
        <Field label="Description" wide><Textarea minRows={2} value={f.description} onChange={(e) => set("description", e.target.value)} /></Field>
      </Box>
      <Typography level="title-sm" sx={{ mt: 2 }}>Related parties</Typography>
      <Typography level="body-xs" sx={{ mb: 0.5 }}>Opposing parties, counsel, judges, witnesses. They are searched by conflict checks.</Typography>
      {parties.map((p, i) => (
        <Box key={i} sx={{ display: "flex", gap: 1, mb: 0.75, flexWrap: "wrap" }}>
          <Input sx={{ flex: 2, minWidth: 140 }} placeholder="Name" value={p.name} onChange={(e) => setParties(parties.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
          <Input sx={{ flex: 1, minWidth: 110 }} placeholder="Role" value={p.role} onChange={(e) => setParties(parties.map((x, j) => (j === i ? { ...x, role: e.target.value } : x)))} />
          <Button size="sm" variant="plain" color="danger" onClick={() => setParties(parties.filter((_, j) => j !== i))} aria-label="Remove party">✕</Button>
        </Box>
      ))}
      <Button size="sm" variant="soft" onClick={() => setParties([...parties, { name: "", role: "Opposing party" }])}>+ Add a party</Button>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} />
    </Dialog>
  );
}

// ---- time / expense entry ----
export function EntryForm({ matter, meta, initial, onClose, onSaved }: { matter: Matter; meta: Meta; initial?: Entry; onClose: () => void; onSaved: () => void }) {
  const [kind, setKind] = useState<string>(initial?.kind ?? "time");
  const [date, setDate] = useState(initial?.date ?? meta.today);
  const [dur, setDur] = useState(initial ? fmtHM(initial.minutes) : "");
  const [price, setPrice] = useState(initial?.kind === "expense" ? (initial.rate / 100).toString() : "");
  const [qty, setQty] = useState(initial?.qty ?? "1");
  const [amount, setAmount] = useState(initial?.kind === "flat" ? (initial.amount / 100).toString() : "");
  const [desc, setDesc] = useState(initial?.description ?? "");
  const [billable, setBillable] = useState(initial?.billable ?? true);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const mins = parseDuration(dur);
  const save = async () => {
    const body: Record<string, unknown> = { kind, date, description: desc, billable };
    if (kind === "time") {
      if (mins === null || mins <= 0) return setErr("Enter the time as 1.5, 1:30, 90m or 1h 30m");
      body.duration = dur;
    } else if (kind === "expense") {
      const p = parseMoney(price);
      if (p === null) return setErr("Enter the price as an amount");
      body.price = p; body.qty = qty;
    } else {
      const a = parseMoney(amount);
      if (a === null || a === 0) return setErr("Enter the fee amount");
      body.amount = a;
    }
    setBusy(true);
    try {
      if (initial) await legal.updateEntry(initial.id, body as never); else await legal.addEntry(matter.id, body as never);
      onSaved();
    } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={initial ? "Edit entry" : `Record work on ${matter.number}`} onClose={onClose} width={480}>
      <Box sx={grid}>
        {!initial && <Field label="What"><Select value={kind} onChange={(_, v) => v && setKind(v)} slotProps={LB}><Option value="time">Time</Option><Option value="expense">Expense</Option><Option value="flat">Flat fee</Option></Select></Field>}
        <Field label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        {kind === "time" && (
          <Field label="Time spent" wide>
            <Input autoFocus value={dur} onChange={(e) => setDur(e.target.value)} placeholder="1.5, 1:30, 90m, 1h 30m" data-testid="entry-duration" />
            <Typography level="body-xs" sx={{ mt: 0.5 }}>{mins ? `Billed as ${fmtHM(roundUp(mins, meta.settings.increment_min))} (rounded up to ${meta.settings.increment_min} minutes)` : "\u00a0"}</Typography>
          </Field>
        )}
        {kind === "expense" && <><Field label="Price each (₹)"><Input value={price} onChange={(e) => setPrice(e.target.value)} /></Field><Field label="Quantity"><Input value={qty} onChange={(e) => setQty(e.target.value)} /></Field></>}
        {kind === "flat" && <Field label="Amount (₹)" wide><Input value={amount} onChange={(e) => setAmount(e.target.value)} /></Field>}
        <Field label="Description" wide><Textarea minRows={2} value={desc} onChange={(e) => setDesc(e.target.value)} /></Field>
        <Checkbox label="Billable" checked={billable} onChange={(e) => setBillable(e.target.checked)} />
      </Box>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} />
    </Dialog>
  );
}

// ---- deadline ----
export function DeadlineForm({ matter, initial, people, onClose, onSaved }: { matter: Matter; initial?: Deadline; people: { id: string; name: string }[]; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ title: initial?.title ?? "", kind: initial?.kind ?? "deadline", due_date: initial?.due_date ?? "", assignee_id: initial?.assignee_id ?? "", notes: initial?.notes ?? "" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    try {
      if (initial) await legal.updateDeadline(initial.id, f); else await legal.addDeadline(matter.id, f);
      onSaved();
    } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={initial ? "Edit deadline" : `New deadline on ${matter.number}`} onClose={onClose} width={480}>
      <Box sx={grid}>
        <Field label="Title" wide><Input autoFocus value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} data-testid="deadline-title" /></Field>
        <Field label="Kind"><Select value={f.kind} onChange={(_, v) => v && setF({ ...f, kind: v })} slotProps={LB}>{Object.entries(KIND_LABEL).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select></Field>
        <Field label="Due date"><Input type="date" value={f.due_date} onChange={(e) => setF({ ...f, due_date: e.target.value })} /></Field>
        <Field label="Assigned to" wide><PersonSelect value={f.assignee_id} known={people} onChange={(id) => setF({ ...f, assignee_id: id })} placeholder="Me" ariaLabel="Person" /></Field>
        <Field label="Notes" wide><Textarea minRows={2} value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></Field>
      </Box>
      <Typography level="body-xs" sx={{ mt: 1 }}>Adds a Calendar event and a Task for the assignee, and moves them if you change the date.</Typography>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} />
    </Dialog>
  );
}

// ---- status move ----
export function MoveDialog({ matter, to, onClose, onDone }: { matter: Matter; to: string; onClose: () => void; onDone: (m: Matter) => void }) {
  const [note, setNote] = useState("");
  const [waive, setWaive] = useState("");
  const [force, setForce] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const needsConflict = to === "open" && !matter.open_date && !matter.conflict_checked_at && !matter.conflict_waiver;
  const unbilled = to === "closed" && (matter.unbilled > 0 || matter.unbilled_minutes > 0);
  const go = async () => {
    setBusy(true);
    try { onDone(await legal.move(matter.id, { status: to, note, force, waive_conflict: waive })); } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={`Move to ${to.replace("_", " ")}`} onClose={onClose} width={480}>
      {needsConflict && (
        <Alert color="warning" sx={{ mb: 1.5 }}>No conflict check is on file for this matter. Run one on the Conflicts tab, or give a reason to open it without one.</Alert>
      )}
      {needsConflict && <Field label="Reason for waiving the conflict check"><Input value={waive} onChange={(e) => setWaive(e.target.value)} data-testid="waive" /></Field>}
      {unbilled && (
        <Alert color="warning" sx={{ my: 1.5 }}>
          Unbilled work of {fmtMoney(matter.unbilled)} ({fmtHM(matter.unbilled_minutes)}). Bill it first, or close anyway.
          <Checkbox sx={{ mt: 1 }} label="Close anyway" checked={force} onChange={(e) => setForce(e.target.checked)} />
        </Alert>
      )}
      <Field label="Note (kept in the status history)"><Textarea minRows={2} value={note} onChange={(e) => setNote(e.target.value)} /></Field>
      <Foot err={err} busy={busy} onSave={go} onClose={onClose} label="Move" />
    </Dialog>
  );
}

// ---- time tracking import ----
export function ImportTime({ matter, onClose, onDone }: { matter: Matter; onClose: () => void; onDone: () => void }) {
  const [list, setList] = useState<TimeCand[] | null>(null);
  const [pick, setPick] = useState<Record<string, boolean>>({});
  const [err, setErr] = useState("");
  useEffect(() => { legal.timeCandidates().then(setList).catch((e) => { setErr(errText(e)); setList([]); }); }, []);
  const ids = Object.keys(pick).filter((k) => pick[k]);
  return (
    <Dialog title="Import from Time tracking" onClose={onClose} width={620}>
      {!list && <Typography>Loading…</Typography>}
      {list && list.length === 0 && !err && <Typography>No entries waiting. Everything in Time tracking is already imported.</Typography>}
      {list?.map((c) => (
        <Checkbox key={c.id} sx={{ my: 0.5 }} checked={!!pick[c.id]} onChange={(e) => setPick({ ...pick, [c.id]: e.target.checked })}
          label={`${c.date} · ${fmtHM(Math.ceil(c.seconds / 60))} · ${c.project || "No project"}${c.note ? ` · ${c.note}` : ""}${c.billable ? "" : " (non-billable)"}`} />
      ))}
      <Foot err={err} busy={false} onClose={onClose} label={`Import ${ids.length || ""}`.trim()} onSave={async () => { try { await legal.importTime(matter.id, ids); onDone(); } catch (e) { setErr(errText(e)); } }} />
    </Dialog>
  );
}
