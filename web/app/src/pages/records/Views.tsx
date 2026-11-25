import { useMemo, useState } from "react";
import { Box, Button, Card, Input, Textarea, Typography } from "@mui/joy";
import type { Field, Rec, Table, View } from "./api";
import { CellView } from "./Editors";
import { isEditable, isoDay, kanbanColumns, monthGrid, recordsByDay, visibleFields } from "./logic";
import { toSend } from "./Grid";

interface Common {
  table: Table;
  view: View;
  recs: Rec[];
  canEdit: boolean;
  onOpen: (r: Rec) => void;
  onEdit: (r: Rec, f: Field, v: unknown) => Promise<void>;
  onAdd: (initial?: Record<string, unknown>) => Promise<void>;
}

const title = (t: Table, r: Rec) => String(r.fields[t.fields[0].id] ?? "") || "Untitled";

function CardBody({ table, view, rec }: { table: Table; view: View; rec: Rec }) {
  const shown = visibleFields(table, view).slice(1, 5);
  return (
    <>
      <Typography level="title-sm" sx={{ mb: 0.5 }}>{title(table, rec)}</Typography>
      {shown.map((f) => {
        const v = rec.fields[f.id];
        if (v === undefined || v === null || v === "") return null;
        return <Box key={f.id} sx={{ fontSize: 12, color: "text.secondary", mb: 0.2 }}><CellView field={f} value={v} /></Box>;
      })}
    </>
  );
}

export function Kanban(p: Common) {
  const f = p.table.fields.find((x) => x.id === p.view.config.stack_by);
  const cols = useMemo(() => kanbanColumns(f, p.recs), [f, p.recs]);
  const [drag, setDrag] = useState("");
  const [over, setOver] = useState("");
  if (!f) return <Box sx={{ p: 3 }}>Choose a select, checkbox or single-link field to stack by (view settings, "Stack by").</Box>;
  const drop = async (colId: string) => {
    const rec = p.recs.find((r) => r.id === drag);
    setDrag("");
    setOver("");
    if (!rec || !p.canEdit) return;
    const col = cols.find((c) => c.id === colId);
    let v: unknown = colId ? col?.name : null;
    if (f.type === "checkbox") v = colId === "true";
    if (f.type === "link") v = colId ? [colId] : [];
    await p.onEdit(rec, f, v);
  };
  return (
    <Box sx={{ display: "flex", gap: 1.5, p: 1.5, overflow: "auto", alignItems: "flex-start", flex: 1 }} data-testid="kanban">
      {cols.filter((c) => c.id || c.recs.length || true).map((c) => (
        <Box key={c.id || "none"} onDragOver={(e) => { e.preventDefault(); setOver(c.id); }} onDrop={() => void drop(c.id)}
          sx={{ width: 270, flexShrink: 0, bgcolor: over === c.id && drag ? "primary.softBg" : "background.level1", borderRadius: "md", p: 1, maxHeight: "100%", display: "flex", flexDirection: "column" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: c.color }} />
            <Typography level="title-sm">{c.name}</Typography>
            <Typography level="body-xs">{c.recs.length}</Typography>
          </Box>
          <Box sx={{ overflow: "auto", display: "flex", flexDirection: "column", gap: 1 }}>
            {c.recs.map((r) => (
              <Card key={r.id} size="sm" variant="outlined" draggable={p.canEdit} onDragStart={() => setDrag(r.id)} onClick={() => p.onOpen(r)} sx={{ cursor: "pointer", p: 1 }} data-testid="kanban-card">
                <CardBody table={p.table} view={p.view} rec={r} />
              </Card>
            ))}
            {p.canEdit && <Button size="sm" variant="plain" onClick={() => p.onAdd(f.type === "link" ? (c.id ? { [f.id]: [c.id] } : {}) : f.type === "checkbox" ? { [f.id]: c.id === "true" } : c.id ? { [f.id]: c.name } : {})}>＋ Add</Button>}
          </Box>
        </Box>
      ))}
    </Box>
  );
}

export function Gallery(p: Common) {
  return (
    <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(230px, 1fr))", gap: 1.5, p: 1.5, overflow: "auto", flex: 1, alignContent: "start" }} data-testid="gallery">
      {p.recs.map((r) => {
        const att = p.table.fields.find((f) => f.id === p.view.config.cover_field) ?? p.table.fields.find((f) => f.type === "attachment");
        const first = att ? (r.fields[att.id] as { file_id: string; mime: string }[] | undefined)?.find((a) => a.mime.startsWith("image/")) : undefined;
        return (
          <Card key={r.id} variant="outlined" onClick={() => p.onOpen(r)} sx={{ cursor: "pointer", p: 0, overflow: "hidden" }}>
            {first && <Box component="img" src={`/api/v1/drive/files/${first.file_id}/content`} alt="" sx={{ width: "100%", height: 130, objectFit: "cover" }} />}
            <Box sx={{ p: 1.2 }}><CardBody table={p.table} view={p.view} rec={r} /></Box>
          </Card>
        );
      })}
      {p.canEdit && <Button variant="outlined" color="neutral" onClick={() => void p.onAdd()} sx={{ minHeight: 90 }}>＋ Add record</Button>}
    </Box>
  );
}

export function List({ table, view, recs, onOpen, canEdit, onAdd }: Common) {
  const fields = visibleFields(table, view).slice(1, 4);
  return (
    <Box sx={{ overflow: "auto", flex: 1 }} data-testid="list-view">
      {recs.map((r) => (
        <Box key={r.id} onClick={() => onOpen(r)} sx={{ display: "flex", gap: 2, px: 2, py: 1, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", alignItems: "center", flexWrap: "wrap", "&:hover": { bgcolor: "background.level1" } }}>
          <Typography level="title-sm" sx={{ minWidth: 160 }}>{title(table, r)}</Typography>
          {fields.map((f) => <Box key={f.id} sx={{ fontSize: 13, color: "text.secondary" }}><CellView field={f} value={r.fields[f.id]} /></Box>)}
        </Box>
      ))}
      {canEdit && <Button variant="plain" onClick={() => void onAdd()} sx={{ m: 1 }}>＋ Add record</Button>}
    </Box>
  );
}

export function Calendar(p: Common) {
  const now = new Date();
  const [ym, setYm] = useState({ y: now.getFullYear(), m: now.getMonth() });
  const df = p.table.fields.find((f) => f.id === p.view.config.date_field);
  const byDay = useMemo(() => (df ? recordsByDay(p.recs, df.id, p.view.config.end_date_field) : new Map<string, Rec[]>()), [p.recs, df, p.view.config.end_date_field]);
  const grid = monthGrid(ym.y, ym.m);
  const [drag, setDrag] = useState("");
  if (!df) return <Box sx={{ p: 3 }}>Choose a date field for this calendar (view settings, "Date field").</Box>;
  const today = isoDay(new Date());
  const label = new Date(ym.y, ym.m, 1).toLocaleDateString(undefined, { month: "long", year: "numeric" });
  const step = (d: number) => setYm(({ y, m }) => { const x = new Date(y, m + d, 1); return { y: x.getFullYear(), m: x.getMonth() }; });
  return (
    <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }} data-testid="calendar-view">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 1 }}>
        <Button size="sm" variant="outlined" color="neutral" onClick={() => step(-1)} aria-label="Previous month">‹</Button>
        <Button size="sm" variant="outlined" color="neutral" onClick={() => setYm({ y: now.getFullYear(), m: now.getMonth() })}>Today</Button>
        <Button size="sm" variant="outlined" color="neutral" onClick={() => step(1)} aria-label="Next month">›</Button>
        <Typography level="title-md" sx={{ ml: 1 }}>{label}</Typography>
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(7, minmax(0,1fr))", flex: 1, overflow: "auto", borderTop: "1px solid", borderColor: "divider" }}>
        {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((d) => <Box key={d} sx={{ p: 0.5, fontSize: 12, fontWeight: 600, textAlign: "center", borderBottom: "1px solid", borderColor: "divider" }}>{d}</Box>)}
        {grid.map((g) => (
          <Box key={g.date} onDragOver={(e) => e.preventDefault()} onDrop={async () => { const r = p.recs.find((x) => x.id === drag); setDrag(""); if (r && p.canEdit) await p.onEdit(r, df, g.date); }}
            sx={{ minHeight: 84, borderRight: "1px solid", borderBottom: "1px solid", borderColor: "divider", p: 0.4, bgcolor: g.inMonth ? "background.surface" : "background.level1", overflow: "hidden" }}>
            <Box sx={{ fontSize: 12, color: g.date === today ? "#fff" : "text.tertiary", bgcolor: g.date === today ? "primary.600" : undefined, borderRadius: "50%", width: 20, height: 20, textAlign: "center", lineHeight: "20px", mb: 0.3 }}>{Number(g.date.slice(8))}</Box>
            {(byDay.get(g.date) ?? []).slice(0, 4).map((r) => (
              <Box key={r.id + g.date} draggable={p.canEdit} onDragStart={() => setDrag(r.id)} onClick={() => p.onOpen(r)} sx={{ fontSize: 11.5, bgcolor: "primary.softBg", borderRadius: "xs", px: 0.5, mb: 0.3, cursor: "pointer", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{title(p.table, r)}</Box>
            ))}
            {(byDay.get(g.date)?.length ?? 0) > 4 && <Typography level="body-xs">+{(byDay.get(g.date)?.length ?? 0) - 4} more</Typography>}
          </Box>
        ))}
      </Box>
    </Box>
  );
}

/** A form that adds records; the fields come from the view's form_fields (all editable fields when unset). */
export function FormView(p: Common) {
  const ids = p.view.config.form_fields;
  const fields = p.table.fields.filter((f) => isEditable(f) && f.type !== "link" && (!ids || ids.includes(f.id)));
  const [vals, setVals] = useState<Record<string, string>>({});
  const [done, setDone] = useState(false);
  const [err, setErr] = useState("");
  const submit = async () => {
    setErr("");
    const body: Record<string, unknown> = {};
    for (const f of fields) {
      let v: unknown = (vals[f.id] ?? "").trim();
      if (v === "") continue;
      if (f.type === "checkbox") v = v === "true";
      else if (f.type === "percent" && !String(v).includes("%")) v = v + "%";
      else if (f.type === "multi_select") v = String(v).split(",").map((s) => s.trim()).filter(Boolean);
      body[f.id] = v;
    }
    try {
      await p.onAdd(body);
      setVals({});
      setDone(true);
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  if (!p.canEdit) return <Box sx={{ p: 3 }}>You can view this form but not submit it.</Box>;
  return (
    <Box sx={{ p: 2, maxWidth: 560, mx: "auto", width: "100%", overflow: "auto" }} data-testid="form-view">
      <Typography level="h3" sx={{ mb: 2 }}>{p.view.config.form_title || p.view.name}</Typography>
      {done && <Box sx={{ p: 1, mb: 2, bgcolor: "success.softBg", borderRadius: "sm" }} role="status">Thanks, your record was added. <Button size="sm" variant="plain" onClick={() => setDone(false)}>Add another</Button></Box>}
      {err && <Box sx={{ p: 1, mb: 2, bgcolor: "danger.softBg", borderRadius: "sm" }} role="alert">{err}</Box>}
      {!done && fields.map((f) => (
        <Box key={f.id} sx={{ mb: 1.5 }}>
          <Typography level="title-sm" component="label" htmlFor={`ff-${f.id}`}>{f.name}</Typography>
          {f.description && <Typography level="body-xs">{f.description}</Typography>}
          {f.type === "long_text" ? <Textarea id={`ff-${f.id}`} minRows={3} value={vals[f.id] ?? ""} onChange={(e) => setVals({ ...vals, [f.id]: e.target.value })} /> :
            f.type === "checkbox" ? <Box><input id={`ff-${f.id}`} type="checkbox" checked={vals[f.id] === "true"} onChange={(e) => setVals({ ...vals, [f.id]: String(e.target.checked) })} /></Box> :
              <Input id={`ff-${f.id}`} type={f.type === "date" ? "date" : "text"} placeholder={f.type === "single_select" ? (f.options.choices ?? []).map((c) => c.name).join(" / ") : ""} value={vals[f.id] ?? ""} onChange={(e) => setVals({ ...vals, [f.id]: e.target.value })} />}
        </Box>
      ))}
      {!done && <Button onClick={() => void submit()}>Submit</Button>}
    </Box>
  );
}

export { toSend };
