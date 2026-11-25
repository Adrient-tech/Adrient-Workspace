import { useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, DialogContent, DialogTitle, FormControl, FormLabel, IconButton, Input, Modal, ModalDialog, Option, Select, Sheet, Textarea, Typography } from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import { svc, TICKET_TYPES, type FieldDef, type Row, type StatusDef } from "./svcApi";
import { EmptyState } from "../../components/ui/EmptyState";
import { SettingsSection } from "./SettingsSection";

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));
const KINDS: [FieldDef["kind"], string][] = [["text", "Short text"], ["longtext", "Long text"], ["number", "Number"], ["select", "Choice list"], ["date", "Date"], ["checkbox", "Yes or no"]];

/** Settings page: custom ticket fields, which types they apply to and the statuses that require them. */
export function FieldsSection({ pid }: { pid: string }) {
  const [defs, setDefs] = useState<FieldDef[] | null>(null);
  const [statuses, setStatuses] = useState<StatusDef[]>([]);
  const [edit, setEdit] = useState<FieldDef | null>(null);
  const [err, setErr] = useState("");
  const load = () => svc.fieldDefs(pid).then(setDefs).catch((e) => setErr(errText(e)));
  useEffect(() => { void load(); svc.statuses(pid).then((r) => setStatuses(r.statuses)).catch(() => undefined); }, [pid]); // eslint-disable-line react-hooks/exhaustive-deps
  const blank: FieldDef = { id: "", key: "", label: "", kind: "text", options: [], required_on: [], ticket_type: "", position: 0 };
  return (
    <SettingsSection title="Custom fields" hint="Extra questions on every ticket of this project, for example an asset tag or a site. A field can apply to one ticket type only and can be required before a ticket reaches a status."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit(blank)} data-testid="new-field">New field</Button>}>
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      {defs?.length === 0 && <EmptyState illustration="generic" title="No custom fields" hint="Fields you add show in the properties panel of every ticket." />}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
        {defs?.map((d) => (
          <Sheet key={d.id} variant="outlined" sx={{ borderRadius: "lg", p: 1.5, display: "flex", alignItems: "center", gap: 1.5 }} data-testid={`field-${d.key}`}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm">{d.label} <span style={{ opacity: 0.5, fontWeight: 400 }}>({d.key})</span></Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary" }}>
                {KINDS.find((k) => k[0] === d.kind)?.[1]}{d.kind === "select" ? `: ${d.options.join(", ")}` : ""} · {d.ticket_type ? `only ${d.ticket_type} tickets` : "all ticket types"}
                {d.required_on.length ? ` · required for ${d.required_on.join(", ")}` : ""}
              </Typography>
            </Box>
            <Button size="sm" variant="outlined" onClick={() => setEdit(d)}>Edit</Button>
            <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${d.label}`} onClick={async () => { if (window.confirm(`Delete the field "${d.label}"? Values already entered are kept but no longer shown.`)) { await svc.deleteFieldDef(d.id).catch((e) => setErr(errText(e))); void load(); } }}><DeleteIcon fontSize="small" /></IconButton>
          </Sheet>
        ))}
      </Box>
      {edit && <FieldDialog pid={pid} d={edit} statuses={statuses} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); void load(); }} />}
    </SettingsSection>
  );
}

function FieldDialog({ pid, d, statuses, onClose, onSaved }: { pid: string; d: FieldDef; statuses: StatusDef[]; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState(d);
  const [err, setErr] = useState("");
  const keyOf = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 32);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(560px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="field-dialog">
        <DialogTitle>{d.id ? "Edit field" : "New field"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Label</FormLabel><Input value={v.label} onChange={(e) => setV({ ...v, label: e.target.value, key: d.id ? v.key : keyOf(e.target.value) })} slotProps={{ input: { "data-testid": "field-label" } as never }} /></FormControl>
            <FormControl size="sm"><FormLabel>Key</FormLabel><Input value={v.key} disabled={!!d.id} onChange={(e) => setV({ ...v, key: keyOf(e.target.value) })} /></FormControl>
            <FormControl size="sm"><FormLabel>Kind</FormLabel><Select value={v.kind} onChange={(_, k) => k && setV({ ...v, kind: k })}>{KINDS.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select></FormControl>
            <FormControl size="sm"><FormLabel>Applies to</FormLabel><Select value={v.ticket_type} onChange={(_, k) => setV({ ...v, ticket_type: k ?? "" })}><Option value="">All ticket types</Option>{TICKET_TYPES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
          </Box>
          {v.kind === "select" && (
            <FormControl size="sm" sx={{ mt: 1.5 }}><FormLabel>Choices (one per line)</FormLabel>
              <Textarea minRows={3} value={v.options.join("\n")} onChange={(e) => setV({ ...v, options: e.target.value.split("\n") })} onBlur={() => setV({ ...v, options: v.options.map((o) => o.trim()).filter(Boolean) })} /></FormControl>
          )}
          <Typography level="title-sm" sx={{ mt: 2, mb: 0.5 }}>Required before a ticket moves to</Typography>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            {statuses.map((s) => (
              <Checkbox key={s.key} size="sm" label={s.label} checked={v.required_on.includes(s.key)} onChange={(e) => setV({ ...v, required_on: e.target.checked ? [...v.required_on, s.key] : v.required_on.filter((x) => x !== s.key) })} />
            ))}
          </Box>
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={async () => { try { await svc.saveFieldDef(pid, { ...v, options: v.options.map((o) => o.trim()).filter(Boolean) }); onSaved(); } catch (e) { setErr(errText(e)); } }} data-testid="field-save">Save field</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

/** The custom fields of one ticket in the properties panel. Saves a field when it loses focus or changes. */
export function TicketCustomFields({ row, projectId, onSave }: { row: Row; projectId: string; onSave: (custom: Record<string, string>) => Promise<void> }) {
  const [defs, setDefs] = useState<FieldDef[]>([]);
  const [local, setLocal] = useState<Record<string, string>>(row.custom ?? {});
  const [err, setErr] = useState("");
  useEffect(() => { svc.fieldDefs(projectId).then(setDefs).catch(() => setDefs([])); }, [projectId]);
  useEffect(() => setLocal(row.custom ?? {}), [row.id, row.custom]);
  const shown = defs.filter((d) => !d.ticket_type || d.ticket_type === row.type);
  if (!shown.length) return null;
  const save = async (key: string, value: string) => {
    if ((row.custom?.[key] ?? "") === value) return;
    setErr("");
    try { await onSave({ ...(row.custom ?? {}), [key]: value }); } catch (e) { setErr(errText(e)); }
  };
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }} data-testid="custom-fields">
      <Typography level="title-sm">Details</Typography>
      {shown.map((d) => {
        const need = d.required_on.includes(row.status);
        const val = local[d.key] ?? "";
        const set = (s: string) => setLocal({ ...local, [d.key]: s });
        return (
          <FormControl key={d.id} size="sm" required={need}>
            <FormLabel sx={{ fontSize: 11, textTransform: "uppercase", letterSpacing: 0.4, color: "text.tertiary" }}>{d.label}</FormLabel>
            {d.kind === "select" ? (
              <Select size="sm" value={val || null} placeholder="Choose…" onChange={(_, s) => { set(s ?? ""); void save(d.key, s ?? ""); }}>{d.options.map((o) => <Option key={o} value={o}>{o}</Option>)}</Select>
            ) : d.kind === "checkbox" ? (
              <Checkbox size="sm" label="Yes" checked={val === "true"} onChange={(e) => { set(e.target.checked ? "true" : "false"); void save(d.key, e.target.checked ? "true" : "false"); }} />
            ) : d.kind === "longtext" ? (
              <Textarea size="sm" minRows={2} value={val} onChange={(e) => set(e.target.value)} onBlur={() => void save(d.key, val)} />
            ) : (
              <Input size="sm" type={d.kind === "number" ? "number" : d.kind === "date" ? "date" : "text"} value={val} onChange={(e) => set(e.target.value)} onBlur={() => void save(d.key, val)} slotProps={{ input: { "data-testid": `cf-${d.key}` } as never }} />
            )}
          </FormControl>
        );
      })}
      {err && <Alert color="danger" size="sm">{err}</Alert>}
      {shown.some((d) => d.required_on.length > 0) && <Chip size="sm" variant="plain" sx={{ alignSelf: "flex-start" }}>A star marks fields this status needs.</Chip>}
    </Box>
  );
}
