import { useState, type ReactNode } from "react";
import { Alert, Box, Button, Checkbox, FormControl, FormLabel, Input, Option, Select, Textarea, Typography } from "@mui/joy";
import { Dialog } from "../legal/Dialogs";
import { errText } from "../know/http";
import { captable, type Class, type Pool, type Security, type Stakeholder } from "./api";
import { fmtShares, KIND_LABEL, parseMoney, parseShares, SEC_LABEL } from "./logic";

const LB = { listbox: { sx: { zIndex: 2000 } } };
const grid = { display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 } as const;
function F({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return <FormControl sx={wide ? { gridColumn: "1 / -1" } : undefined}><FormLabel>{label}</FormLabel>{children}</FormControl>;
}
function Foot({ err, busy, onSave, onClose, label = "Save" }: { err: string; busy: boolean; onSave: () => void; onClose: () => void; label?: string }) {
  return (
    <>
      {err && <Alert color="danger" sx={{ mt: 1.5 }}>{err}</Alert>}
      <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}><Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button><Button loading={busy} onClick={onSave}>{label}</Button></Box>
    </>
  );
}

export function HolderForm({ initial, onClose, onSaved }: { initial?: Stakeholder; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ name: initial?.name ?? "", kind: initial?.kind ?? "employee", email: initial?.email ?? "", notes: initial?.notes ?? "" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    try { if (initial) await captable.updateStakeholder(initial.id, f); else await captable.createStakeholder(f); onSaved(); } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title={initial ? "Edit holder" : "New holder"} onClose={onClose} width={480}>
      <Box sx={grid}>
        <F label="Name" wide><Input autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} data-testid="holder-name" /></F>
        <F label="Type"><Select value={f.kind} onChange={(_, v) => v && setF({ ...f, kind: v })} slotProps={LB}>{["founder", "employee", "investor", "advisor", "entity"].map((k) => <Option key={k} value={k}>{KIND_LABEL[k]}</Option>)}</Select></F>
        <F label="Email"><Input value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></F>
        <F label="Notes" wide><Textarea minRows={2} value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></F>
      </Box>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} />
    </Dialog>
  );
}

export function IssueForm({ holders, classes, pools, today, onClose, onSaved }: { holders: Stakeholder[]; classes: Class[]; pools: Pool[]; today: string; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ kind: "stock", holder: "", class_id: classes[0]?.id ?? "", pool_id: "", qty: "", price: "", date: today, vest_months: "0", cliff: "0", freq: "monthly", expires: "", principal: "", cap: "", discount: "", interest: "", post_money: false, notes: "" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: string, v: string | boolean) => setF((x) => ({ ...x, [k]: v }));
  const conv = f.kind === "safe" || f.kind === "note";
  const save = async () => {
    const body: Record<string, unknown> = { kind: f.kind, stakeholder_id: f.holder, issue_date: f.date, notes: f.notes };
    if (conv) {
      const p = parseMoney(f.principal), c = f.cap.trim() ? parseMoney(f.cap) : 0;
      if (p === null || p <= 0 || c === null) return setErr("Enter the principal and the cap as amounts in rupees");
      Object.assign(body, { principal: p, cap: c, post_money: f.post_money, discount_bp: Math.round(Number(f.discount || 0) * 100), interest_bp: Math.round(Number(f.interest || 0) * 100) });
    } else {
      const q = parseShares(f.qty);
      if (q === null || q <= 0) return setErr("Enter the number of shares as a whole number");
      Object.assign(body, { quantity: q, price: f.price.trim().replace(/,/g, "") || "0", class_id: f.class_id || undefined, pool_id: f.kind === "option" ? f.pool_id || undefined : undefined,
        vest_months: Number(f.vest_months) || 0, cliff_months: Number(f.cliff) || 0, vest_freq: f.freq, expires: f.expires || undefined });
    }
    setBusy(true);
    try { await captable.issue(body as never); onSaved(); } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  return (
    <Dialog title="Issue equity" onClose={onClose} width={640}>
      <Box sx={grid}>
        <F label="What"><Select value={f.kind} onChange={(_, v) => v && set("kind", v)} slotProps={LB}>{Object.entries(SEC_LABEL).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select></F>
        <F label="Holder"><Select value={f.holder} placeholder="Choose" onChange={(_, v) => set("holder", v ?? "")} slotProps={LB} data-testid="issue-holder">{holders.map((h) => <Option key={h.id} value={h.id}>{h.name}</Option>)}</Select></F>
        {!conv && <F label="Share class"><Select value={f.class_id} onChange={(_, v) => set("class_id", v ?? "")} slotProps={LB}>{classes.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select></F>}
        {f.kind === "option" && <F label="Option pool"><Select value={f.pool_id} placeholder="Outside any pool" onChange={(_, v) => set("pool_id", v ?? "")} slotProps={LB}>{pools.map((p) => <Option key={p.id} value={p.id}>{p.name}</Option>)}</Select></F>}
        {!conv && <F label="Number of shares"><Input value={f.qty} onChange={(e) => set("qty", e.target.value)} data-testid="issue-qty" /></F>}
        {!conv && <F label={f.kind === "stock" ? "Price per share (₹)" : "Strike price (₹)"}><Input value={f.price} onChange={(e) => set("price", e.target.value)} /></F>}
        {conv && <><F label="Amount invested (₹)"><Input value={f.principal} onChange={(e) => set("principal", e.target.value)} /></F><F label="Valuation cap (₹), optional"><Input value={f.cap} onChange={(e) => set("cap", e.target.value)} /></F><F label="Discount (%)"><Input value={f.discount} onChange={(e) => set("discount", e.target.value)} /></F>
          {f.kind === "note" && <F label="Interest (% a year, simple)"><Input value={f.interest} onChange={(e) => set("interest", e.target.value)} /></F>}
          {f.kind === "safe" && <Checkbox label="Post-money SAFE (the cap includes the SAFE shares)" checked={f.post_money} onChange={(e) => set("post_money", e.target.checked)} />}</>}
        <F label="Issue date"><Input type="date" value={f.date} onChange={(e) => set("date", e.target.value)} /></F>
        {(f.kind === "option" || f.kind === "stock") && <><F label="Vests over (months, 0 = none)"><Input value={f.vest_months} onChange={(e) => set("vest_months", e.target.value)} /></F><F label="Cliff (months)"><Input value={f.cliff} onChange={(e) => set("cliff", e.target.value)} /></F>
          <F label="Vesting steps"><Select value={f.freq} onChange={(_, v) => v && set("freq", v)} slotProps={LB}><Option value="monthly">Monthly</Option><Option value="quarterly">Quarterly</Option><Option value="annual">Yearly</Option></Select></F></>}
        {(f.kind === "option" || f.kind === "warrant") && <F label="Expires"><Input type="date" value={f.expires} onChange={(e) => set("expires", e.target.value)} /></F>}
        <F label="Notes" wide><Textarea minRows={2} value={f.notes} onChange={(e) => set("notes", e.target.value)} /></F>
      </Box>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} label="Issue" />
    </Dialog>
  );
}

export function ActionForm({ sec, kind, holders, classes, today, onClose, onSaved }: { sec: Security; kind: "exercise" | "cancel" | "transfer" | "convert"; holders: Stakeholder[]; classes: Class[]; today: string; onClose: () => void; onSaved: () => void }) {
  const [date, setDate] = useState(today);
  const [qty, setQty] = useState(kind === "exercise" ? String(sec.exercisable) : kind === "convert" ? "" : String(sec.outstanding));
  const [to, setTo] = useState("");
  const [price, setPrice] = useState("");
  const [cls, setCls] = useState(classes.find((c) => c.kind === "preferred")?.id ?? classes[0]?.id ?? "");
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    const q = parseShares(qty);
    if (q === null || q <= 0) return setErr("Enter a whole number of shares");
    setBusy(true);
    try { await captable.record(sec.id, kind, { date, quantity: q, to_stakeholder: to || undefined, price: price.trim() || undefined, class_id: kind === "convert" ? cls : undefined, note }); onSaved(); } catch (e) { setErr(errText(e)); setBusy(false); }
  };
  const title = { exercise: "Exercise", cancel: "Cancel", transfer: "Transfer", convert: "Convert" }[kind];
  return (
    <Dialog title={`${title} ${sec.label}`} onClose={onClose} width={480}>
      <Typography level="body-sm" sx={{ mb: 1 }}>{sec.stakeholder} · {fmtShares(sec.outstanding)} held{kind === "exercise" ? ` · ${fmtShares(sec.exercisable)} vested and not yet exercised today` : ""}</Typography>
      <Box sx={grid}>
        <F label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></F>
        <F label={kind === "convert" ? "Shares it converts into" : "Number of shares"}><Input value={qty} onChange={(e) => setQty(e.target.value)} data-testid="action-qty" /></F>
        {kind === "transfer" && <F label="To" wide><Select value={to} placeholder="Choose a holder" onChange={(_, v) => setTo(v ?? "")} slotProps={LB}>{holders.filter((h) => h.id !== sec.stakeholder_id).map((h) => <Option key={h.id} value={h.id}>{h.name}</Option>)}</Select></F>}
        {(kind === "transfer" || kind === "convert") && <F label="Price per share (₹)"><Input value={price} onChange={(e) => setPrice(e.target.value)} /></F>}
        {kind === "convert" && <F label="Into class"><Select value={cls} onChange={(_, v) => setCls(v ?? "")} slotProps={LB}>{classes.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select></F>}
        <F label="Note" wide><Input value={note} onChange={(e) => setNote(e.target.value)} /></F>
      </Box>
      <Foot err={err} busy={busy} onSave={save} onClose={onClose} label={title} />
    </Dialog>
  );
}
