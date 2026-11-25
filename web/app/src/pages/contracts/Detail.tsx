import { useState } from "react";
import { Box, Button, Checkbox, Chip, DialogContent, DialogTitle, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Typography } from "@mui/joy";
import { DrivePicker } from "../drive/DrivePicker";
import { fmtDate, money } from "../know/http";
import { contracts, type Contract, type Obligation } from "./api";
import { clockText, PHASE_COLOR, PHASE_LABEL, relativeDays, repeatLabel, TYPE_LABEL } from "./logic";

const LB = { listbox: { sx: { zIndex: 2000 } } };

function ObligationRow({ o, canEdit, onChange }: { o: Obligation; canEdit: boolean; onChange: () => void }) {
  const [busy, setBusy] = useState(false);
  const run = async (fn: () => Promise<unknown>) => { setBusy(true); try { await fn(); onChange(); } catch (e) { window.alert((e as Error).message); } finally { setBusy(false); } };
  const today = new Date().toISOString().slice(0, 10);
  const overdue = !o.done && o.due && o.due < today;
  return (
    <Box sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.6, borderBottom: "1px solid", borderColor: "divider", opacity: o.done ? 0.55 : 1 }} data-testid="obligation-row">
      <Checkbox disabled={!canEdit || busy} checked={o.done} onChange={() => void run(() => (o.done ? contracts.reopenObligation(o.id) : contracts.completeObligation(o.id)))} slotProps={{ input: { "aria-label": `Mark "${o.title}" done` } }} />
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography level="body-sm" sx={{ textDecoration: o.done ? "line-through" : "none" }}>{o.title}</Typography>
        <Typography level="body-xs" sx={{ color: overdue ? "danger.600" : "text.tertiary" }}>
          {o.owner === "us" ? "We owe" : o.owner === "them" ? "They owe" : "Both"}{o.due ? ` · due ${fmtDate(o.due)} (${relativeDays(Math.round((new Date(o.due).getTime() - new Date(today).getTime()) / 86400000))})` : ""}{o.repeat ? ` · repeats ${repeatLabel(o.repeat.every)}` : ""}{o.history.length ? ` · ${o.history.length} done before` : ""}
        </Typography>
      </Box>
      {canEdit && <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${o.title}`} onClick={() => { if (window.confirm(`Delete "${o.title}"?`)) void run(() => contracts.deleteObligation(o.id)); }}>✕</IconButton>}
    </Box>
  );
}

export function ContractDetail({ id, initial, onClose, onEdit, onChanged }: { id: string; initial: Contract; onClose: () => void; onEdit: (c: Contract) => void; onChanged: () => void }) {
  const [c, setC] = useState<Contract>(initial);
  const [loaded, setLoaded] = useState(false);
  const [err, setErr] = useState("");
  const [title, setTitle] = useState("");
  const [due, setDue] = useState("");
  const [owner, setOwner] = useState("us");
  const [every, setEvery] = useState("none");
  const [picker, setPicker] = useState(false);
  const [snooze, setSnooze] = useState("");
  const [note, setNote] = useState("");
  const reload = async () => { try { setC(await contracts.get(id)); onChanged(); } catch (e) { setErr((e as Error).message); } };
  if (!loaded) { setLoaded(true); void reload(); }
  const r = c.renewal;
  const decide = async (kind: string) => {
    try { setC(await contracts.decide(id, { kind, until: kind === "snoozed" ? snooze : undefined, note })); setNote(""); onChanged(); } catch (e) { setErr((e as Error).message); }
  };
  const add = async () => {
    try {
      await contracts.addObligation(id, { title, due: due || undefined, owner: owner as Obligation["owner"], repeat: every === "none" ? undefined : { every } });
      setTitle(""); setDue(""); setEvery("none");
      await reload();
    } catch (e) { setErr((e as Error).message); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 760, maxWidth: "100vw", maxHeight: "96vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }} role="dialog" aria-label={`Contract ${c.title}`}>
        <ModalClose />
        <DialogTitle sx={{ pr: 5 }}>{c.title}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1, alignItems: "center" }}>
            <Chip size="sm" variant="soft">{TYPE_LABEL[c.type] ?? c.type}</Chip>
            <Chip size="sm" variant="soft" color={c.status === "active" ? "success" : "neutral"}>{c.status}</Chip>
            {r.phase !== "none" && <Chip size="sm" variant="soft" color={PHASE_COLOR[r.phase]}>{PHASE_LABEL[r.phase]}</Chip>}
            {c.access === "restricted" && <Chip size="sm" variant="outlined">Restricted</Chip>}
            <Box sx={{ flex: 1 }} />
            {c.can_edit && <Button size="sm" variant="outlined" onClick={() => onEdit(c)}>Edit</Button>}
          </Box>
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(4, 1fr)" }, gap: 1.5, mb: 2 }}>
            <Box><Typography level="body-xs">Counterparty</Typography><Typography level="title-sm">{c.party_label || "—"}</Typography></Box>
            <Box><Typography level="body-xs">Value</Typography><Typography level="title-sm">{c.value ? `${money(c.value, c.currency)} ${c.billing === "one-time" ? "" : c.billing}` : "—"}</Typography></Box>
            <Box><Typography level="body-xs">Term</Typography><Typography level="title-sm">{c.start_date ? fmtDate(c.start_date) : "?"} → {c.end_date ? fmtDate(c.end_date) : "no end"}</Typography></Box>
            <Box><Typography level="body-xs">Renewal</Typography><Typography level="title-sm">{c.auto_renew ? `Auto, ${c.term_months} months` : "Fixed term"}</Typography></Box>
          </Box>
          {r.date && c.status === "active" && (
            <Box sx={{ p: 1.5, mb: 2, borderRadius: "md", bgcolor: r.attention ? "warning.softBg" : "background.level1" }} data-testid="renewal-box">
              <Typography level="title-sm">{clockText(r)}</Typography>
              <Typography level="body-xs">{c.auto_renew ? "Renews" : "Ends"} on {fmtDate(r.date)}{r.notice_date ? ` · notice deadline ${fmtDate(r.notice_date)}` : ""}.</Typography>
              {r.decision && <Typography level="body-sm" sx={{ mt: 0.5 }}>Your decision: <b>{r.decision.kind.replace("-", " ")}</b>{r.decision.until ? ` until ${fmtDate(r.decision.until)}` : ""}{r.decision.note ? ` — ${r.decision.note}` : ""}</Typography>}
              {c.can_edit && (
                <Box sx={{ mt: 1 }}>
                  <Input size="sm" placeholder="Note (optional)" value={note} onChange={(e) => setNote(e.target.value)} sx={{ mb: 0.7, maxWidth: 360 }} />
                  <Box sx={{ display: "flex", gap: 0.7, flexWrap: "wrap", alignItems: "center" }}>
                    <Button size="sm" variant="soft" onClick={() => void decide("renewing")}>Renewing</Button>
                    <Button size="sm" variant="soft" onClick={() => void decide("notice-given")}>Notice given</Button>
                    <Button size="sm" variant="soft" onClick={() => void decide("not-renewing")}>Not renewing</Button>
                    <Input size="sm" type="date" value={snooze} onChange={(e) => setSnooze(e.target.value)} sx={{ width: 150 }} slotProps={{ input: { "aria-label": "Snooze until" } }} />
                    <Button size="sm" variant="soft" disabled={!snooze} onClick={() => void decide("snoozed")}>Snooze</Button>
                    {r.decision && <Button size="sm" variant="plain" color="neutral" onClick={() => void decide("clear")}>Clear decision</Button>}
                  </Box>
                </Box>
              )}
            </Box>
          )}
          <Typography level="title-md" sx={{ mb: 0.5 }}>Obligations</Typography>
          {(c.obligations ?? []).length === 0 && <Typography level="body-sm">None yet.</Typography>}
          {(c.obligations ?? []).map((o) => <ObligationRow key={o.id} o={o} canEdit={c.can_edit} onChange={() => void reload()} />)}
          {c.can_edit && (
            <Box sx={{ display: "flex", gap: 0.7, mt: 1, flexWrap: "wrap" }}>
              <Input size="sm" sx={{ flex: 1, minWidth: 160 }} placeholder="Add an obligation, e.g. Pay quarterly licence fee" value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "aria-label": "New obligation" } }} />
              <Input size="sm" type="date" value={due} onChange={(e) => setDue(e.target.value)} slotProps={{ input: { "aria-label": "Due date" } }} />
              <Select size="sm" value={owner} onChange={(_, v) => v && setOwner(v)} slotProps={LB}><Option value="us">We owe</Option><Option value="them">They owe</Option><Option value="mutual">Both</Option></Select>
              <Select size="sm" value={every} onChange={(_, v) => v && setEvery(v)} slotProps={LB}><Option value="none">Once</Option><Option value="monthly">Monthly</Option><Option value="quarterly">Quarterly</Option><Option value="annually">Annually</Option></Select>
              <Button size="sm" disabled={!title.trim()} onClick={() => void add()} data-testid="add-obligation">Add</Button>
            </Box>
          )}
          <Typography level="title-md" sx={{ mt: 2, mb: 0.5 }}>Documents</Typography>
          {(c.documents ?? []).map((d) => (
            <Box key={d.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.4 }}>
              <Box component="a" href={`/api/v1/drive/files/${d.file_id}/content`} target="_blank" rel="noreferrer" sx={{ flex: 1 }}>📎 {d.name}</Box>
              <Chip size="sm" variant="outlined">{d.kind}</Chip>
              {c.can_edit && <IconButton size="sm" variant="plain" aria-label={`Remove ${d.name}`} onClick={() => void contracts.removeDocument(id, d.id).then(reload)}>✕</IconButton>}
            </Box>
          ))}
          {(c.documents ?? []).length === 0 && <Typography level="body-sm">No documents attached.</Typography>}
          {c.can_edit && <Button size="sm" variant="soft" sx={{ mt: 0.7 }} onClick={() => setPicker(true)}>Attach from Drive</Button>}
          {(c.completed_terms ?? []).length > 0 && (
            <>
              <Typography level="title-md" sx={{ mt: 2, mb: 0.5 }}>Past terms</Typography>
              {c.completed_terms!.map((t, i) => <Typography key={i} level="body-sm">{t.start ? fmtDate(t.start) : "?"} → {fmtDate(t.end)}</Typography>)}
            </>
          )}
          {c.notes && <><Typography level="title-md" sx={{ mt: 2 }}>Notes</Typography><Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{c.notes}</Typography></>}
          {c.can_edit && c.remind && <Button size="sm" variant="plain" sx={{ mt: 1 }} onClick={() => void contracts.sync(id).then(() => window.alert("Calendar and Tasks are up to date.")).catch((e: Error) => setErr(e.message))}>Refresh Calendar and Tasks reminders</Button>}
        </DialogContent>
        {picker && <DrivePicker mode="file" title="Attach a document" confirmLabel="Attach" onClose={() => setPicker(false)} onPick={(f) => { setPicker(false); void contracts.addDocument(id, (f as { id: string }).id).then(reload).catch((e: Error) => setErr(e.message)); }} />}
      </ModalDialog>
    </Modal>
  );
}
