import { useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, DialogContent, DialogTitle, Input, List, ListItem, ListItemButton, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography } from "@mui/joy";
import { DrivePicker } from "../drive/DrivePicker";
import { records, type Field, type LinkVal, type Rec } from "./api";
import { cellText, choiceColor, choiceName, editText } from "./logic";

export const ACCENT = "#126C5A";

export function Pill({ text, color }: { text: string; color: string }) {
  return (
    <Box component="span" sx={{ display: "inline-block", px: 0.9, py: 0.1, borderRadius: "10px", fontSize: 12, lineHeight: "18px", bgcolor: color + "33", color: "text.primary", border: `1px solid ${color}66`, mr: 0.5, whiteSpace: "nowrap" }}>
      {text}
    </Box>
  );
}

export function Stars({ value, max, onPick }: { value: number; max: number; onPick?: (n: number) => void }) {
  return (
    <Box sx={{ display: "inline-flex", color: "#f5a524" }} role={onPick ? "radiogroup" : undefined} aria-label="Rating">
      {Array.from({ length: max }, (_, i) => (
        <Box
          key={i}
          component={onPick ? "button" : "span"}
          type={onPick ? "button" : undefined}
          aria-label={`${i + 1} star${i ? "s" : ""}`}
          onClick={onPick ? (e: React.MouseEvent) => { e.stopPropagation(); onPick(value === i + 1 ? 0 : i + 1); } : undefined}
          sx={{ all: onPick ? "unset" : undefined, cursor: onPick ? "pointer" : "default", fontSize: 16, lineHeight: 1, opacity: i < value ? 1 : 0.25 }}
        >
          ★
        </Box>
      ))}
    </Box>
  );
}

/** Read-only rendering of a cell. */
export function CellView({ field, value }: { field: Field; value: unknown }) {
  if (value === null || value === undefined || value === "") return null;
  switch (field.type) {
    case "checkbox":
      return <Box sx={{ color: ACCENT, fontWeight: 700 }}>✓</Box>;
    case "single_select":
      return <Pill text={choiceName(field, value)} color={choiceColor(field, value)} />;
    case "multi_select":
      return <>{(value as string[]).map((id) => <Pill key={id} text={choiceName(field, id)} color={choiceColor(field, id)} />)}</>;
    case "rating":
      return <Stars value={Number(value)} max={field.options.max ?? 5} />;
    case "link":
      return <>{(value as LinkVal[]).map((l) => <Pill key={l.id} text={l.name || "Untitled"} color="#4263eb" />)}</>;
    case "lookup":
    case "formula":
    case "rollup":
      if (typeof value === "object" && !Array.isArray(value) && "error" in (value as object)) return <Box sx={{ color: "danger.600" }} title={(value as { error: string }).error}>#ERROR</Box>;
      if (Array.isArray(value)) return <>{value.map((v, i) => <Pill key={i} text={typeof v === "number" ? String(v) : String(v ?? "")} color="#8a94a6" />)}</>;
      break;
    case "attachment":
      return <>{(value as { file_id: string; name: string }[]).map((a) => (
        <Box key={a.file_id} component="a" href={`/api/v1/drive/files/${a.file_id}/content`} target="_blank" rel="noreferrer" onClick={(e: React.MouseEvent) => e.stopPropagation()} sx={{ mr: 1, fontSize: 13 }}>📎 {a.name}</Box>
      ))}</>;
    case "url":
      return <Box component="a" href={String(value)} target="_blank" rel="noreferrer noopener" onClick={(e: React.MouseEvent) => e.stopPropagation()} sx={{ color: "primary.600" }}>{String(value)}</Box>;
    case "email":
      return <Box component="a" href={`mailto:${String(value)}`} onClick={(e: React.MouseEvent) => e.stopPropagation()}>{String(value)}</Box>;
    case "button": {
      const href = String(value);
      return /^https?:\/\//.test(href) ? <Button size="sm" variant="soft" component="a" href={href} target="_blank" rel="noreferrer noopener" onClick={(e: React.MouseEvent) => e.stopPropagation()}>{String(field.options.label ?? "Open")}</Button> : null;
    }
  }
  return <>{cellTextShort(field, value)}</>;
}

const cellTextShort = cellText;

/** Picks records of the linked table. primaryId is the id of that table's first field (it names the records). */
export function LinkPicker({ field, primaryId, current, onDone, onClose }: { field: Field; primaryId: string; current: LinkVal[]; onDone: (ids: string[]) => void; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<Rec[]>([]);
  const [sel, setSel] = useState<LinkVal[]>(current);
  const [err, setErr] = useState("");
  const multi = field.options.allow_multiple !== false;
  const target = field.options.table_id ?? "";
  useEffect(() => {
    let live = true;
    const t = setTimeout(() => {
      records.query(target, { search: q, limit: 50 }).then((r) => live && setRows(r.records)).catch((e: Error) => live && setErr(e.message));
    }, 200);
    return () => { live = false; clearTimeout(t); };
  }, [q, target]);
  const label = (r: Rec): string => String(r.fields[primaryId] ?? "") || `Record ${r.seq}`;
  const has = (id: string) => sel.some((s) => s.id === id);
  const toggle = (r: Rec) => {
    if (has(r.id)) setSel(sel.filter((s) => s.id !== r.id));
    else setSel(multi ? [...sel, { id: r.id, name: label(r) }] : [{ id: r.id, name: label(r) }]);
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw" }}>
        <ModalClose />
        <DialogTitle>Link records</DialogTitle>
        <DialogContent>
          <Input autoFocus placeholder="Search records" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1 }} />
          {err && <Typography color="danger" level="body-sm">{err}</Typography>}
          <Box sx={{ mb: 1 }}>{sel.map((s) => <Chip key={s.id} size="sm" variant="soft" onClick={() => setSel(sel.filter((x) => x.id !== s.id))} sx={{ mr: 0.5 }}>{s.name} ✕</Chip>)}</Box>
          <List sx={{ maxHeight: 280, overflow: "auto" }}>
            {rows.map((r) => (
              <ListItem key={r.id}>
                <ListItemButton selected={has(r.id)} onClick={() => toggle(r)}>{label(r)}</ListItemButton>
              </ListItem>
            ))}
            {rows.length === 0 && <Typography level="body-sm" sx={{ p: 1 }}>No records found.</Typography>}
          </List>
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={() => onDone(sel.map((s) => s.id))}>Done</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

/** Attach files from Drive. */
export function AttachPicker({ current, onDone, onClose }: { current: { file_id: string; name: string; size: number; mime: string }[]; onDone: (v: unknown[]) => void; onClose: () => void }) {
  return (
    <DrivePicker
      mode="file"
      title="Attach from Drive"
      confirmLabel="Attach"
      onClose={onClose}
      onPick={(f) => {
        const d = f as { id: string; name: string; mime_type?: string; size_bytes?: string };
        if (current.some((c) => c.file_id === d.id)) return onClose();
        onDone([...current, { file_id: d.id, name: d.name, size: Number(d.size_bytes ?? 0), mime: d.mime_type ?? "" }]);
      }}
    />
  );
}

/** Inline editor for one cell. onCommit gets the value to send (a string for typed fields). */
export function InlineEditor({ field, value, onCommit, onCancel }: { field: Field; value: unknown; onCommit: (v: unknown) => void; onCancel: () => void }) {
  const [text, setText] = useState(editText(field, value));
  const finish = (t = text) => {
    let v: unknown = t.trim();
    if (v === "") v = null;
    else if (field.type === "percent" && typeof v === "string" && !v.includes("%")) v = v + "%";
    else if (field.type === "multi_select" && typeof v === "string") v = (v as string).split(",").map((s) => s.trim()).filter(Boolean);
    onCommit(v);
  };
  if (field.type === "single_select") {
    return (
      <Select
        size="sm"
        listboxOpen
        value={(value as string) ?? null}
        onChange={(_, v) => onCommit(v ? choiceName(field, v) : null)}
        onClose={onCancel}
        slotProps={{ listbox: { sx: { zIndex: 2000 } } }}
        sx={{ width: "100%" }}
      >
        <Option value={null as unknown as string}>(none)</Option>
        {(field.options.choices ?? []).map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
      </Select>
    );
  }
  if (field.type === "multi_select") {
    const cur = new Set((value as string[] | undefined) ?? []);
    return (
      <Box sx={{ p: 0.5, bgcolor: "background.surface", border: "1px solid", borderColor: "primary.400", borderRadius: "sm", position: "absolute", zIndex: 30, minWidth: 200, boxShadow: "md" }}>
        {(field.options.choices ?? []).map((c) => (
          <Box key={c.id}><Checkbox size="sm" label={c.name} checked={cur.has(c.id)} onChange={(e) => { if (e.target.checked) cur.add(c.id); else cur.delete(c.id); onCommit([...cur].map((id) => choiceName(field, id))); }} /></Box>
        ))}
        <Button size="sm" variant="plain" onClick={onCancel}>Close</Button>
      </Box>
    );
  }
  if (field.type === "long_text") {
    return <Textarea autoFocus minRows={3} value={text} onChange={(e) => setText(e.target.value)} onBlur={() => finish()} onKeyDown={(e) => { if (e.key === "Escape") onCancel(); if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) finish(); }} sx={{ position: "absolute", zIndex: 30, minWidth: 260, boxShadow: "md" }} />;
  }
  return (
    <Input
      autoFocus
      size="sm"
      type={field.type === "date" ? (field.options.include_time ? "datetime-local" : "date") : "text"}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => finish()}
      onKeyDown={(e) => {
        if (e.key === "Escape") onCancel();
        if (e.key === "Enter") finish();
      }}
      sx={{ width: "100%", "--Input-minHeight": "26px" }}
      slotProps={{ input: { "aria-label": field.name } }}
    />
  );
}
