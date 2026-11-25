import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Card, Chip, CircularProgress, Divider, Dropdown, Input, Menu, MenuButton, MenuItem, Option, Select, Tab, TabList, Tabs, Textarea, Typography } from "@mui/joy";
import { DrivePicker } from "../drive/DrivePicker";
import { errText, fmtDate } from "../know/http";
import { legal, type Bill, type DocLink, type Entry, type Matter, type Meta, type Note } from "./api";
import { Dialog, DeadlineForm, EntryForm, ImportTime, MatterForm, MoveDialog } from "./Dialogs";
import { BILL_COLOR, fmtHM, fmtMoney, KIND_LABEL, relDays, STATUS_COLOR, STATUS_LABEL, URGENCY_COLOR } from "./logic";
import type { Client } from "./api";

type Tab_ = "overview" | "time" | "deadlines" | "bills" | "documents" | "notes";

export function MatterDetail({ id, meta, clients, people, onClose, onChanged }: { id: string; meta: Meta; clients: Client[]; people: { id: string; name: string }[]; onClose: () => void; onChanged: () => void }) {
  const [m, setM] = useState<Matter | null>(null);
  const [tab, setTab] = useState<Tab_>("overview");
  const [entries, setEntries] = useState<Entry[]>([]);
  const [bills, setBills] = useState<Bill[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [err, setErr] = useState("");
  const [dlg, setDlg] = useState<null | "edit" | "entry" | "deadline" | "import" | "link" | "picker" | "bill">(null);
  const [move, setMove] = useState("");
  const [editEntry, setEditEntry] = useState<Entry | undefined>();
  const [billPreview, setBillPreview] = useState<Bill | null>(null);
  const [noteText, setNoteText] = useState("");
  const [noteKind, setNoteKind] = useState("note");

  const load = useCallback(async () => {
    try {
      const [mm, ee, bb, nn] = await Promise.all([legal.matter(id), legal.entries({ matter: id }), legal.bills(id), legal.notes(id)]);
      setM(mm); setEntries(ee); setBills(bb); setNotes(nn); setErr("");
    } catch (e) { setErr(errText(e)); }
  }, [id]);
  useEffect(() => { void load(); }, [load]);
  const changed = () => { void load(); onChanged(); };
  const run = async (f: () => Promise<unknown>) => { try { await f(); changed(); } catch (e) { setErr(errText(e)); } };

  if (!m) return <Dialog title="Matter" onClose={onClose} width={900}>{err ? <Alert color="danger">{err}</Alert> : <CircularProgress />}</Dialog>;
  const unbilledEntries = entries.filter((e) => e.billable && !e.bill_id);
  const spent = bills.filter((b) => b.status !== "void").reduce((s, b) => s + b.total, 0) + m.unbilled;

  const Row = ({ k, v }: { k: string; v: React.ReactNode }) => (
    <Box sx={{ display: "flex", gap: 1, py: 0.4 }}><Typography level="body-sm" sx={{ width: 140, color: "text.tertiary", flexShrink: 0 }}>{k}</Typography><Typography level="body-sm" component="div">{v || "—"}</Typography></Box>
  );

  return (
    <Dialog title={`${m.number}  ${m.title}`} onClose={onClose} width={960}>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 1 }}>
        <Chip color={STATUS_COLOR[m.status]} variant="soft">{STATUS_LABEL[m.status]}</Chip>
        <Typography level="body-sm">{m.client_name || "No client"} · {m.practice_area || "No practice area"}{m.stage ? ` · ${m.stage}` : ""}</Typography>
        {m.access === "restricted" && <Chip size="sm" color="warning" variant="outlined">Restricted</Chip>}
        <Box sx={{ ml: "auto", display: "flex", gap: 1 }}>
          <Dropdown>
            <MenuButton size="sm" variant="outlined" data-testid="move-menu">Move to…</MenuButton>
            <Menu sx={{ zIndex: 2000 }}>{m.next_statuses.map((s) => <MenuItem key={s} onClick={() => setMove(s)}>{STATUS_LABEL[s]}</MenuItem>)}</Menu>
          </Dropdown>
          <Button size="sm" variant="outlined" onClick={() => setDlg("edit")}>Edit</Button>
        </Box>
      </Box>
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      <Tabs value={tab} onChange={(_, v) => v && setTab(v as Tab_)} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ overflowX: "auto" }}>
          <Tab value="overview">Overview</Tab><Tab value="time">Time & expenses ({entries.length})</Tab><Tab value="deadlines">Deadlines ({m.deadlines?.length ?? 0})</Tab>
          <Tab value="bills">Bills ({bills.length})</Tab><Tab value="documents">Documents ({m.documents?.length ?? 0})</Tab><Tab value="notes">Notes ({notes.length})</Tab>
        </TabList>
      </Tabs>

      {tab === "overview" && (
        <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
          <Box>
            <Row k="Responsible" v={m.responsible} />
            <Row k="Opened" v={m.open_date ? fmtDate(m.open_date) : ""} />
            <Row k="Closed" v={m.closed_date ? fmtDate(m.closed_date) : ""} />
            <Row k="Limitation date" v={m.sol_date ? <>{fmtDate(m.sol_date)} {m.sol_days !== undefined && <Chip size="sm" color={m.sol_days < 30 ? "danger" : "neutral"} variant="soft">{relDays(m.sol_days)}</Chip>}</> : ""} />
            <Row k="Billing" v={m.billing_method === "hourly" ? `Hourly${m.rate ? ` at ${fmtMoney(m.rate)}` : " at the client or firm rate"}` : m.billing_method === "flat" ? `Flat fee ${fmtMoney(m.flat_fee)}${m.flat_fee_billed ? " (billed)" : ""}` : `Contingency ${(m.contingency_bp / 100).toFixed(2)}%`} />
            <Row k="Budget" v={m.budget ? `${fmtMoney(m.budget)}, ${Math.round((spent / m.budget) * 100)}% used` : ""} />
            <Row k="Conflict check" v={<>{m.conflict_checked_at ? `Run ${m.conflict_checked_at}` : m.conflict_waiver ? `Waived: ${m.conflict_waiver}` : "Not run yet"}{" "}
              <Button size="sm" variant="plain" data-testid="run-conflict" onClick={() => run(async () => { for (const n of [m.client_name, ...(m.parties ?? []).map((p) => p.name)].filter(Boolean)) await legal.recordConflict(m.id, n); })}>Run and save conflict check</Button></>} />
            <Row k="Tags" v={m.tags.join(", ")} />
            {m.description && <Typography level="body-sm" sx={{ mt: 1, whiteSpace: "pre-wrap" }}>{m.description}</Typography>}
            <Divider sx={{ my: 1 }} />
            <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
              <Card variant="soft" size="sm" sx={{ flex: 1, minWidth: 130 }}><Typography level="body-xs">Unbilled work</Typography><Typography level="title-lg">{fmtMoney(m.unbilled)}</Typography><Typography level="body-xs">{fmtHM(m.unbilled_minutes)} of time</Typography></Card>
              <Card variant="soft" size="sm" sx={{ flex: 1, minWidth: 130 }}><Typography level="body-xs">Billed</Typography><Typography level="title-lg">{fmtMoney(m.billed)}</Typography><Typography level="body-xs">{fmtHM(m.minutes)} recorded in total</Typography></Card>
            </Box>
          </Box>
          <Box>
            <Typography level="title-sm">Related parties</Typography>
            {(m.parties ?? []).length === 0 ? <Typography level="body-sm">None recorded.</Typography> : (m.parties ?? []).map((p) => <Typography key={p.id} level="body-sm">{p.name} <Chip size="sm" variant="soft">{p.role}</Chip></Typography>)}
            <Typography level="title-sm" sx={{ mt: 2 }}>Status history</Typography>
            {(m.status_log ?? []).map((l, i) => (
              <Typography key={i} level="body-xs" sx={{ py: 0.3 }}>{new Date(l.at).toLocaleDateString()} · {l.from ? `${STATUS_LABEL[l.from]} → ` : ""}{STATUS_LABEL[l.to]} · {l.by}{l.note ? ` · ${l.note}` : ""}</Typography>
            ))}
          </Box>
        </Box>
      )}

      {tab === "time" && (
        <Box sx={{ mt: 1.5 }}>
          <Box sx={{ display: "flex", gap: 1, mb: 1, flexWrap: "wrap" }}>
            <Button size="sm" onClick={() => { setEditEntry(undefined); setDlg("entry"); }} disabled={m.status === "closed"} data-testid="add-entry">＋ Record time or expense</Button>
            {meta.time_tracking && <Button size="sm" variant="outlined" onClick={() => setDlg("import")}>Import from Time tracking</Button>}
          </Box>
          {entries.length === 0 ? <Typography level="body-sm">No work recorded yet.</Typography> : entries.map((e) => (
            <Box key={e.id} sx={{ display: "flex", gap: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
              <Typography level="body-sm" sx={{ width: 92 }}>{fmtDate(e.date)}</Typography>
              <Chip size="sm" variant="soft">{e.kind === "time" ? fmtHM(e.minutes) : e.kind === "expense" ? `Expense ×${e.qty}` : "Flat fee"}</Chip>
              <Typography level="body-sm" sx={{ flex: 1, minWidth: 160 }}>{e.description || "—"} <Typography level="body-xs" component="span">· {e.user}{e.source ? " · from Time tracking" : ""}{e.billable ? "" : " · non-billable"}</Typography></Typography>
              <Typography level="body-sm" sx={{ width: 100, textAlign: "right" }}>{e.billable ? fmtMoney(e.amount) : "—"}</Typography>
              {e.bill_id ? <Chip size="sm" color="success" variant="soft">Billed</Chip> : (
                <Box sx={{ display: "flex" }}>
                  <Button size="sm" variant="plain" onClick={() => { setEditEntry(e); setDlg("entry"); }}>Edit</Button>
                  <Button size="sm" variant="plain" color="danger" onClick={() => run(() => legal.deleteEntry(e.id))}>Delete</Button>
                </Box>
              )}
            </Box>
          ))}
        </Box>
      )}

      {tab === "deadlines" && (
        <Box sx={{ mt: 1.5 }}>
          <Button size="sm" onClick={() => setDlg("deadline")} data-testid="add-deadline">＋ Add a deadline</Button>
          {(m.deadlines ?? []).map((d) => (
            <Box key={d.id} sx={{ display: "flex", gap: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
              <Chip size="sm" variant="soft" color={URGENCY_COLOR[d.urgency]}>{d.urgency === "done" ? "Done" : relDays(d.days_left)}</Chip>
              <Typography level="body-sm" sx={{ flex: 1, minWidth: 160, textDecoration: d.done ? "line-through" : "none" }}>{d.title} <Typography level="body-xs" component="span">· {KIND_LABEL[d.kind]} · {fmtDate(d.due_date)}{d.assignee ? ` · ${d.assignee}` : ""}</Typography></Typography>
              <Button size="sm" variant="plain" onClick={() => run(() => legal.deadlineDone(d.id, !d.done))}>{d.done ? "Reopen" : "Done"}</Button>
              <Button size="sm" variant="plain" color="danger" onClick={() => run(() => legal.deleteDeadline(d.id))}>Delete</Button>
            </Box>
          ))}
        </Box>
      )}

      {tab === "bills" && (
        <Box sx={{ mt: 1.5 }}>
          <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
            <Button size="sm" disabled={unbilledEntries.length === 0 && !(m.billing_method === "flat" && !m.flat_fee_billed && m.flat_fee > 0)} onClick={() => run(async () => { setBillPreview(await legal.draftBill(m.id, { preview: true })); setDlg("bill"); }).catch(() => undefined)} data-testid="draft-bill">Prepare a bill from unbilled work</Button>
          </Box>
          {bills.length === 0 ? <Typography level="body-sm">No bills yet.</Typography> : bills.map((b) => (
            <Box key={b.id} sx={{ display: "flex", gap: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
              <Typography level="body-sm" sx={{ width: 130 }}>{b.number}</Typography>
              <Chip size="sm" variant="soft" color={BILL_COLOR[b.status]}>{b.status}</Chip>
              <Typography level="body-sm" sx={{ flex: 1 }}>{fmtDate(b.issue_date)}{b.invoice_number ? ` · Invoice ${b.invoice_number}` : ""}</Typography>
              <Typography level="body-sm">{fmtMoney(b.total)}</Typography>
              {b.status === "draft" && <Button size="sm" disabled={!meta.invoicing} onClick={() => run(() => legal.issueBill(b.id))} data-testid="issue-bill">Issue as invoice</Button>}
              {b.invoice_id && b.status === "issued" && <Button size="sm" variant="plain" component="a" href={`/invoicing?doc=${b.invoice_id}`}>Open invoice</Button>}
              {b.status !== "void" && <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { const v = await legal.voidBill(b.id); if (v.note) setErr(v.note); })}>Void</Button>}
            </Box>
          ))}
          {!meta.invoicing && <Typography level="body-xs" sx={{ mt: 1 }}>Invoicing is not connected, so bills cannot be issued as invoices.</Typography>}
        </Box>
      )}

      {tab === "documents" && (
        <Box sx={{ mt: 1.5 }}>
          <Box sx={{ display: "flex", gap: 1, mb: 1, flexWrap: "wrap" }}>
            <Button size="sm" onClick={() => setDlg("picker")} disabled={!meta.drive}>＋ From Drive</Button>
            <Button size="sm" variant="outlined" onClick={() => setDlg("link")}>＋ Contract or signature request</Button>
          </Box>
          {(m.documents ?? []).length === 0 ? <Typography level="body-sm">No documents linked.</Typography> : (m.documents ?? []).map((d) => <DocRow key={d.id} d={d} onRemove={() => run(() => legal.removeDocument(m.id, d.id))} />)}
        </Box>
      )}

      {tab === "notes" && (
        <Box sx={{ mt: 1.5 }}>
          <Box sx={{ display: "flex", gap: 1, mb: 1, flexWrap: "wrap" }}>
            <Select size="sm" value={noteKind} onChange={(_, v) => v && setNoteKind(v)} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
              <Option value="note">Note</Option><Option value="call">Call</Option><Option value="email">Email</Option><Option value="meeting">Meeting</Option>
            </Select>
            <Textarea sx={{ flex: 1, minWidth: 200 }} minRows={2} value={noteText} onChange={(e) => setNoteText(e.target.value)} placeholder="Privileged notes stay on the matter" />
            <Button size="sm" disabled={!noteText.trim()} onClick={() => run(async () => { await legal.addNote(m.id, noteKind, noteText); setNoteText(""); })}>Add</Button>
          </Box>
          {notes.map((n) => (
            <Box key={n.id} sx={{ py: 0.8, borderBottom: "1px solid", borderColor: "divider" }}>
              <Typography level="body-xs"><Chip size="sm" variant="soft">{n.kind}</Chip> {n.author} · {new Date(n.created_at).toLocaleString()}</Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{n.body}</Typography>
            </Box>
          ))}
        </Box>
      )}

      {dlg === "edit" && <MatterForm initial={m} meta={meta} clients={clients} people={people} onClose={() => setDlg(null)} onSaved={() => { setDlg(null); changed(); }} />}
      {dlg === "entry" && <EntryForm matter={m} meta={meta} initial={editEntry} onClose={() => setDlg(null)} onSaved={() => { setDlg(null); changed(); }} />}
      {dlg === "deadline" && <DeadlineForm matter={m} people={people} onClose={() => setDlg(null)} onSaved={() => { setDlg(null); changed(); }} />}
      {dlg === "import" && <ImportTime matter={m} onClose={() => setDlg(null)} onDone={() => { setDlg(null); changed(); }} />}
      {dlg === "picker" && <DrivePicker mode="file" title="Link a Drive file" confirmLabel="Link" onClose={() => setDlg(null)} onPick={(f) => { setDlg(null); void run(() => legal.addDocument(m.id, { kind: "drive", ref_id: (f as { id: string }).id })); }} />}
      {dlg === "link" && <LinkDialog matterId={m.id} onClose={() => setDlg(null)} onDone={() => { setDlg(null); changed(); }} />}
      {move && <MoveDialog matter={m} to={move} onClose={() => setMove("")} onDone={() => { setMove(""); changed(); }} />}
      {dlg === "bill" && billPreview && (
        <Dialog title="Bill preview" onClose={() => setDlg(null)} width={680}>
          {(billPreview.lines ?? []).map((l, i) => (
            <Box key={i} sx={{ display: "flex", gap: 1, py: 0.4 }}><Typography level="body-sm" sx={{ flex: 1 }}>{l.date} · {l.description || l.kind}{l.minutes ? ` · ${fmtHM(l.minutes)}` : ""}</Typography><Typography level="body-sm">{fmtMoney(l.amount)}</Typography></Box>
          ))}
          <Divider sx={{ my: 1 }} />
          <Typography level="body-sm">Fees {fmtMoney(billPreview.fees)} · Expenses {fmtMoney(billPreview.expenses)} · {meta.settings.tax_name} {fmtMoney(billPreview.tax)}</Typography>
          <Typography level="title-md">Total {fmtMoney(billPreview.total)}</Typography>
          <Typography level="body-xs" sx={{ mt: 0.5 }}>The final amount is set by the invoice in Invoicing when you issue the bill.</Typography>
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={() => setDlg(null)}>Cancel</Button>
            <Button data-testid="confirm-draft" onClick={() => { setDlg(null); void run(() => legal.draftBill(m.id, {})); }}>Create draft bill</Button>
          </Box>
        </Dialog>
      )}
    </Dialog>
  );
}

function DocRow({ d, onRemove }: { d: DocLink; onRemove: () => void }) {
  const href = d.kind === "drive" ? `/api/v1/drive/files/${d.ref_id}/content` : d.kind === "contract" ? `/contracts?contract=${d.ref_id}` : `/signatures?request=${d.ref_id}`;
  return (
    <Box sx={{ display: "flex", gap: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", alignItems: "center" }}>
      <Chip size="sm" variant="soft">{d.kind === "drive" ? "Drive" : d.kind === "contract" ? "Contract" : "Signature"}</Chip>
      <Typography level="body-sm" sx={{ flex: 1 }} component="a" {...{ href, target: "_blank", rel: "noreferrer" }}>{d.name}</Typography>
      {d.status && <Chip size="sm" variant="outlined">{d.status}</Chip>}
      <Button size="sm" variant="plain" color="danger" onClick={onRemove}>Unlink</Button>
    </Box>
  );
}

function LinkDialog({ matterId, onClose, onDone }: { matterId: string; onClose: () => void; onDone: () => void }) {
  const [kind, setKind] = useState("contract");
  const [q, setQ] = useState("");
  const [items, setItems] = useState<{ id: string; name: string; status?: string }[]>([]);
  const [err, setErr] = useState("");
  useEffect(() => { const t = setTimeout(() => legal.linkable(kind, q).then(setItems).catch((e) => setErr(errText(e))), 150); return () => clearTimeout(t); }, [kind, q]);
  return (
    <Dialog title="Link a contract or signature request" onClose={onClose} width={520}>
      <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
        <Select value={kind} onChange={(_, v) => v && setKind(v)} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="contract">Contract</Option><Option value="signature">Signature request</Option></Select>
        <Input sx={{ flex: 1 }} placeholder="Search by title" value={q} onChange={(e) => setQ(e.target.value)} />
      </Box>
      {err && <Alert color="danger">{err}</Alert>}
      {items.length === 0 && <Typography level="body-sm">Nothing found.</Typography>}
      {items.map((i) => (
        <Button key={i.id} variant="plain" sx={{ display: "flex", justifyContent: "space-between", width: "100%" }} onClick={async () => { try { await legal.addDocument(matterId, { kind, ref_id: i.id }); onDone(); } catch (e) { setErr(errText(e)); } }}>
          <span>{i.name}</span><span>{i.status}</span>
        </Button>
      ))}
    </Dialog>
  );
}
