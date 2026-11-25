import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Checkbox, Dropdown, IconButton, Menu, MenuButton, MenuItem, Typography } from "@mui/joy";
import MoreVert from "@mui/icons-material/MoreVert";
import ArrowUpward from "@mui/icons-material/ArrowUpward";
import ArrowDownward from "@mui/icons-material/ArrowDownward";
import type { Field, LinkVal, Rec, Table, View } from "./api";
import { AttachPicker, CellView, InlineEditor, LinkPicker, Stars } from "./Editors";
import { cellText, choiceName, isComputed, parsePasted, summarize, type Summary, TYPE_LABEL } from "./logic";

const ROW_H = { short: 34, medium: 56, tall: 92 } as const;
const GROUP_H = 34;
const HEAD_H = 36;
const OVERSCAN = 8;

type Item = { kind: "rec"; rec: Rec; index: number } | { kind: "group"; key: string; label: string; count: number; depth: number };

export interface GridProps {
  table: Table;
  view: View;
  fields: Field[];
  recs: Rec[];
  total: number;
  loading: boolean;
  canEdit: boolean;
  tableById: Record<string, Table>;
  onNeedMore: () => void;
  onEdit: (rec: Rec, field: Field, value: unknown) => Promise<void>;
  onOpen: (rec: Rec) => void;
  onDelete: (ids: string[]) => void;
  onAdd: () => void;
  onPasteRows: (startRow: number, startCol: number, rows: string[][]) => Promise<void>;
  onEditField: (f: Field) => void;
  onHideField: (f: Field) => void;
  onDeleteField: (f: Field) => void;
  onSort: (f: Field, desc: boolean) => void;
  onResize: (fieldId: string, w: number) => void;
  onAddField: () => void;
  sortOf: (fid: string) => "asc" | "desc" | "";
}

export function Grid(p: GridProps) {
  const { table, view, fields, recs, total } = p;
  const cfg = view.config;
  const rowH = ROW_H[cfg.row_height ?? "short"];
  const scroller = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewH, setViewH] = useState(600);
  const [active, setActive] = useState<{ r: number; c: number } | null>(null);
  const [editing, setEditing] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [summaries, setSummaries] = useState<Record<string, Summary>>({});
  const [picker, setPicker] = useState<null | { rec: Rec; field: Field }>(null);
  const [msg, setMsg] = useState("");

  const widths = cfg.widths ?? {};
  const colW = (f: Field, i: number) => widths[f.id] ?? (i === 0 ? 220 : f.type === "long_text" ? 260 : f.type === "checkbox" ? 90 : 170);
  const CHECK_W = 44;
  const totalW = CHECK_W + fields.reduce((a, f, i) => a + colW(f, i), 0) + 60;

  // Rows, with group headers when the view groups by a field.
  const groupField = useMemo(() => fields.find((f) => f.id === cfg.group_by?.[0]) ?? p.table.fields.find((f) => f.id === cfg.group_by?.[0]), [fields, cfg.group_by, p.table.fields]);
  const items: Item[] = useMemo(() => {
    if (!groupField) return recs.map((rec, index) => ({ kind: "rec", rec, index }));
    const out: Item[] = [];
    const counts = new Map<string, number>();
    const label = (r: Rec) => cellText(groupField, r.fields[groupField.id]) || "Empty";
    for (const r of recs) counts.set(label(r), (counts.get(label(r)) ?? 0) + 1);
    let last = "\u0000";
    recs.forEach((rec, index) => {
      const l = label(rec);
      if (l !== last) {
        last = l;
        out.push({ kind: "group", key: l, label: l, count: counts.get(l) ?? 0, depth: 0 });
      }
      if (!collapsed.has(l)) out.push({ kind: "rec", rec, index });
    });
    return out;
  }, [recs, groupField, collapsed]);
  const hs = items.map((it) => (it.kind === "group" ? GROUP_H : rowH));
  const offsets = useMemo(() => {
    const o = new Array<number>(items.length + 1);
    o[0] = 0;
    for (let i = 0; i < items.length; i++) o[i + 1] = o[i] + (items[i].kind === "group" ? GROUP_H : rowH);
    return o;
  }, [items, rowH]);
  void hs;
  const bodyH = offsets[items.length] + (groupField ? 0 : 0);
  const unseen = Math.max(0, total - recs.length);
  const virtualH = bodyH + unseen * rowH;

  const first = useMemo(() => {
    let lo = 0, hi = items.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (offsets[mid + 1] <= scrollTop) lo = mid + 1; else hi = mid;
    }
    return lo;
  }, [offsets, scrollTop, items.length]);
  const from = Math.max(0, first - OVERSCAN);
  let to = from;
  while (to < items.length && offsets[to] < scrollTop + viewH + OVERSCAN * rowH) to++;
  to = Math.min(items.length, to + OVERSCAN);

  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setViewH(el.clientHeight));
    ro.observe(el);
    setViewH(el.clientHeight);
    return () => ro.disconnect();
  }, []);
  // Ask for the next page when the end of what is loaded comes into view.
  useEffect(() => {
    if (!p.loading && recs.length < total && scrollTop + viewH + 600 > bodyH) p.onNeedMore();
  }, [scrollTop, viewH, bodyH, recs.length, total, p]);

  const recAt = useCallback((r: number) => recs[r], [recs]);
  const editable = (f: Field) => p.canEdit && !isComputed(f.type);
  const commit = async (rec: Rec, f: Field, v: unknown) => {
    setEditing(false);
    try {
      await p.onEdit(rec, f, v);
    } catch (e) {
      setMsg((e as Error).message);
    }
  };
  const move = (dr: number, dc: number) => {
    setActive((a) => {
      if (!a) return { r: 0, c: 0 };
      return { r: Math.min(Math.max(0, a.r + dr), recs.length - 1), c: Math.min(Math.max(0, a.c + dc), fields.length - 1) };
    });
  };
  useEffect(() => {
    if (!active) return;
    const idx = items.findIndex((it) => it.kind === "rec" && it.index === active.r);
    const el = scroller.current;
    if (idx < 0 || !el) return;
    const top = offsets[idx];
    if (top < el.scrollTop) el.scrollTop = top;
    else if (top + rowH > el.scrollTop + el.clientHeight - HEAD_H) el.scrollTop = top + rowH - el.clientHeight + HEAD_H + 4;
  }, [active, items, offsets, rowH]);

  const onKeyDown = async (e: React.KeyboardEvent) => {
    if (editing || !active) return;
    const rec = recAt(active.r);
    const f = fields[active.c];
    if (!rec || !f) return;
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === "ArrowDown") { e.preventDefault(); move(1, 0); }
    else if (e.key === "ArrowUp") { e.preventDefault(); move(-1, 0); }
    else if (e.key === "ArrowLeft") { e.preventDefault(); move(0, -1); }
    else if (e.key === "ArrowRight" || e.key === "Tab") { e.preventDefault(); move(0, e.shiftKey && e.key === "Tab" ? -1 : 1); }
    else if (e.key === "Enter" || e.key === "F2") { e.preventDefault(); openEditor(rec, f); }
    else if (e.key === "Delete" || e.key === "Backspace") { if (editable(f) && f.type !== "link") { e.preventDefault(); void commit(rec, f, null); } else if (editable(f)) { e.preventDefault(); void commit(rec, f, []); } }
    else if (e.key === " " && f.type === "checkbox" && editable(f)) { e.preventDefault(); void commit(rec, f, !rec.fields[f.id]); }
    else if (e.key === "Escape") setActive(null);
    else if (mod && e.key.toLowerCase() === "c") { e.preventDefault(); void navigator.clipboard?.writeText(cellText(f, rec.fields[f.id])); }
    else if (mod && e.key.toLowerCase() === "d" && active.r > 0 && editable(f) && f.type !== "link") {
      e.preventDefault();
      const above = recAt(active.r - 1);
      if (above) void commit(rec, f, toSend(f, above.fields[f.id]));
    } else if (!mod && e.key.length === 1 && editable(f) && f.type !== "checkbox" && f.type !== "rating" && f.type !== "link") {
      setEditing(true);
    }
  };
  const openEditor = (rec: Rec, f: Field) => {
    if (!p.canEdit) return;
    if (f.type === "link" || f.type === "attachment") setPicker({ rec, field: f });
    else if (f.type === "checkbox") void commit(rec, f, !rec.fields[f.id]);
    else if (editable(f)) setEditing(true);
  };
  const onPaste = async (e: React.ClipboardEvent) => {
    if (editing || !active || !p.canEdit) return;
    const text = e.clipboardData.getData("text");
    if (!text) return;
    e.preventDefault();
    try {
      await p.onPasteRows(active.r, active.c, parsePasted(text));
    } catch (er) {
      setMsg((er as Error).message);
    }
  };

  const startResize = (f: Field, i: number, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const x0 = e.clientX, w0 = colW(f, i);
    let w = w0;
    const mv = (ev: MouseEvent) => { w = Math.max(70, w0 + ev.clientX - x0); const el = document.getElementById(`col-${f.id}`); if (el) el.style.width = `${w}px`; };
    const up = () => { window.removeEventListener("mousemove", mv); window.removeEventListener("mouseup", up); p.onResize(f.id, w); };
    window.addEventListener("mousemove", mv);
    window.addEventListener("mouseup", up);
  };

  const allChecked = recs.length > 0 && recs.every((r) => checked.has(r.id));
  const sel = [...checked];

  const rowsOut = [];
  for (let i = from; i < to; i++) {
    const it = items[i];
    const top = offsets[i];
    if (it.kind === "group") {
      rowsOut.push(
        <Box key={`g-${it.key}`} onClick={() => setCollapsed((s) => { const n = new Set(s); if (n.has(it.key)) n.delete(it.key); else n.add(it.key); return n; })}
          sx={{ position: "absolute", top, left: 0, height: GROUP_H, width: totalW, display: "flex", alignItems: "center", px: 1.5, bgcolor: "background.level1", borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", fontWeight: 600, fontSize: 13 }}>
          <Box sx={{ width: 20 }}>{collapsed.has(it.key) ? "▸" : "▾"}</Box>{groupField?.name}: {it.label} <Box sx={{ ml: 1, color: "text.tertiary", fontWeight: 400 }}>{it.count}</Box>
        </Box>,
      );
      continue;
    }
    const rec = it.rec;
    const r = it.index;
    rowsOut.push(
      <Box key={rec.id} role="row" aria-rowindex={r + 2} data-testid="grid-row" sx={{ position: "absolute", top, left: 0, height: rowH, width: totalW, display: "flex", borderBottom: "1px solid", borderColor: "divider", bgcolor: checked.has(rec.id) ? "primary.softBg" : "background.surface", "&:hover": { bgcolor: checked.has(rec.id) ? "primary.softBg" : "background.level1" } }}>
        <Box sx={{ width: CHECK_W, flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center", gap: 0.3, position: "sticky", left: 0, zIndex: 3, bgcolor: "inherit", borderRight: "1px solid", borderColor: "divider" }}>
          {hoverNum(r, rec.seq)}
          <Checkbox size="sm" checked={checked.has(rec.id)} onChange={(e) => setChecked((s) => { const n = new Set(s); if (e.target.checked) n.add(rec.id); else n.delete(rec.id); return n; })} slotProps={{ input: { "aria-label": `Select row ${r + 1}` } }} />
        </Box>
        {fields.map((f, c) => {
          const isActive = active?.r === r && active.c === c;
          const w = colW(f, c);
          return (
            <Box key={f.id} role="gridcell" aria-selected={isActive} onClick={() => { setActive({ r, c }); if (f.type === "rating" || f.type === "checkbox") return; }}
              onDoubleClick={() => openEditor(rec, f)}
              sx={{ width: w, flexShrink: 0, px: 1, display: "flex", alignItems: rowH > 40 ? "flex-start" : "center", pt: rowH > 40 ? 0.7 : 0, overflow: "hidden", whiteSpace: rowH > 40 ? "normal" : "nowrap", fontSize: 13, position: c === 0 ? "sticky" : "relative", left: c === 0 ? CHECK_W : undefined, zIndex: c === 0 ? 2 : 1, bgcolor: c === 0 ? "inherit" : undefined, borderRight: "1px solid", borderColor: "divider", outline: isActive ? "2px solid" : "none", outlineColor: "primary.500", outlineOffset: -2, textOverflow: "ellipsis", cursor: "cell" }}>
              {isActive && editing ? (
                <InlineEditor field={f} value={rec.fields[f.id]} onCommit={(v) => commit(rec, f, v)} onCancel={() => setEditing(false)} />
              ) : f.type === "rating" && editable(f) ? (
                <Stars value={Number(rec.fields[f.id] ?? 0)} max={f.options.max ?? 5} onPick={(n) => void commit(rec, f, n)} />
              ) : f.type === "checkbox" && editable(f) ? (
                <Checkbox size="sm" checked={!!rec.fields[f.id]} onChange={(e) => void commit(rec, f, e.target.checked)} slotProps={{ input: { "aria-label": f.name } }} />
              ) : (
                <CellView field={f} value={rec.fields[f.id]} />
              )}
            </Box>
          );
        })}
        <Box sx={{ width: 60, flexShrink: 0 }}>
          <IconButton size="sm" variant="plain" aria-label={`Expand record ${r + 1}`} onClick={() => p.onOpen(rec)}>⤢</IconButton>
        </Box>
      </Box>,
    );
  }
  function hoverNum(r: number, seq: number) {
    return <Typography level="body-xs" sx={{ color: "text.tertiary", minWidth: 14, textAlign: "right", mr: 0.3 }}>{groupField ? "" : r + 1}{groupField ? seq : ""}</Typography>;
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", minHeight: 0, flex: 1 }} onKeyDown={onKeyDown} onPaste={onPaste} tabIndex={0} role="grid" aria-label={`${table.name} records`} aria-rowcount={total + 1} aria-colcount={fields.length}>
      {msg && <Box sx={{ px: 1.5, py: 0.5, bgcolor: "danger.softBg", color: "danger.700", fontSize: 13 }} role="alert" onClick={() => setMsg("")}>{msg} (click to dismiss)</Box>}
      {sel.length > 0 && p.canEdit && (
        <Box sx={{ px: 1.5, py: 0.5, display: "flex", gap: 1.5, alignItems: "center", bgcolor: "primary.softBg", fontSize: 13 }}>
          {sel.length} selected
          <Box component="button" onClick={() => { if (window.confirm(`Delete ${sel.length} record(s)? This cannot be undone.`)) { p.onDelete(sel); setChecked(new Set()); } }} sx={{ all: "unset", cursor: "pointer", color: "danger.600", fontWeight: 600 }}>Delete</Box>
          <Box component="button" onClick={() => setChecked(new Set())} sx={{ all: "unset", cursor: "pointer" }}>Clear</Box>
        </Box>
      )}
      <Box ref={scroller} onScroll={(e) => setScrollTop((e.target as HTMLDivElement).scrollTop)} sx={{ flex: 1, overflow: "auto", position: "relative", minHeight: 240 }} data-testid="grid-scroller">
        <Box sx={{ width: totalW, position: "sticky", top: 0, zIndex: 5, height: HEAD_H, display: "flex", bgcolor: "background.level1", borderBottom: "1px solid", borderColor: "divider" }} role="row">
          <Box sx={{ width: CHECK_W, flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center", position: "sticky", left: 0, zIndex: 6, bgcolor: "background.level1", borderRight: "1px solid", borderColor: "divider" }}>
            <Checkbox size="sm" checked={allChecked} indeterminate={!allChecked && checked.size > 0} onChange={(e) => setChecked(e.target.checked ? new Set(recs.map((r) => r.id)) : new Set())} slotProps={{ input: { "aria-label": "Select all loaded rows" } }} />
          </Box>
          {fields.map((f, i) => (
            <Box key={f.id} id={`col-${f.id}`} role="columnheader" sx={{ width: colW(f, i), flexShrink: 0, display: "flex", alignItems: "center", px: 1, fontWeight: 600, fontSize: 13, borderRight: "1px solid", borderColor: "divider", position: i === 0 ? "sticky" : "relative", left: i === 0 ? CHECK_W : undefined, zIndex: i === 0 ? 6 : undefined, bgcolor: "background.level1" }}>
              <Box sx={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={`${f.name} (${TYPE_LABEL[f.type]})`}>
                {f.name}
                {p.sortOf(f.id) === "asc" && <ArrowUpward sx={{ fontSize: 14, ml: 0.5 }} />}
                {p.sortOf(f.id) === "desc" && <ArrowDownward sx={{ fontSize: 14, ml: 0.5 }} />}
              </Box>
              {p.canEdit && (
                <Dropdown>
                  <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": `${f.name} menu` } }}><MoreVert fontSize="small" /></MenuButton>
                  <Menu size="sm" sx={{ zIndex: 2000 }}>
                    <MenuItem onClick={() => p.onEditField(f)}>Edit field</MenuItem>
                    <MenuItem onClick={() => p.onSort(f, false)}>Sort A → Z</MenuItem>
                    <MenuItem onClick={() => p.onSort(f, true)}>Sort Z → A</MenuItem>
                    {i > 0 && <MenuItem onClick={() => p.onHideField(f)}>Hide field</MenuItem>}
                    {i > 0 && <MenuItem color="danger" onClick={() => p.onDeleteField(f)}>Delete field</MenuItem>}
                  </Menu>
                </Dropdown>
              )}
              <Box onMouseDown={(e) => startResize(f, i, e)} sx={{ position: "absolute", right: -3, top: 0, bottom: 0, width: 7, cursor: "col-resize", zIndex: 7 }} />
            </Box>
          ))}
          <Box sx={{ width: 60, flexShrink: 0, display: "flex", alignItems: "center" }}>
            {p.canEdit && <IconButton size="sm" variant="plain" aria-label="Add field" onClick={p.onAddField}>＋</IconButton>}
          </Box>
        </Box>
        <Box sx={{ position: "relative", width: totalW, height: virtualH }} data-testid="grid-body">
          {rowsOut}
          {unseen > 0 && recs.length > 0 && (
            <Box sx={{ position: "absolute", top: bodyH, left: 0, width: totalW, height: unseen * rowH, background: "repeating-linear-gradient(transparent, transparent " + (rowH - 1) + "px, rgba(128,128,128,.15) " + (rowH - 1) + "px, rgba(128,128,128,.15) " + rowH + "px)" }} aria-hidden />
          )}
        </Box>
        {p.canEdit && (
          <Box sx={{ position: "sticky", bottom: 0, left: 0, width: totalW, bgcolor: "background.surface", borderTop: "1px solid", borderColor: "divider", zIndex: 4 }}>
            <Box component="button" onClick={p.onAdd} sx={{ all: "unset", cursor: "pointer", px: 1.5, py: 0.7, fontSize: 13, color: "primary.700", display: "block" }} data-testid="grid-add-row">＋ Add record</Box>
          </Box>
        )}
      </Box>
      <Box sx={{ display: "flex", borderTop: "1px solid", borderColor: "divider", bgcolor: "background.level1", overflowX: "hidden", fontSize: 12, minHeight: 30 }} aria-label="Summary bar">
        <Box sx={{ width: CHECK_W, flexShrink: 0 }} />
        {fields.map((f, i) => (
          <Box key={f.id} sx={{ width: colW(f, i), flexShrink: 0, px: 1, display: "flex", alignItems: "center", gap: 0.5, overflow: "hidden" }}>
            <Dropdown>
              <MenuButton slots={{ root: Box }} slotProps={{ root: { component: "button", "aria-label": `Summary for ${f.name}`, sx: { all: "unset", cursor: "pointer", color: "text.tertiary", "&:hover": { color: "text.primary" } } } }}>
                {summaries[f.id] && summaries[f.id] !== "none" ? `${summaries[f.id]}: ${summarize(f, recs, summaries[f.id])}` : "Summarize"}
              </MenuButton>
              <Menu size="sm" sx={{ zIndex: 2000 }}>
                {(["none", "count", "filled", "empty", ...(["number", "currency", "percent", "duration", "rating", "count", "rollup", "formula"].includes(f.type) ? ["sum", "avg", "min", "max"] : [])] as Summary[]).map((s) => (
                  <MenuItem key={s} onClick={() => setSummaries((m) => ({ ...m, [f.id]: s }))}>{s === "none" ? "None" : s}</MenuItem>
                ))}
              </Menu>
            </Dropdown>
          </Box>
        ))}
        <Box sx={{ flex: 1, textAlign: "right", px: 1.5, alignSelf: "center" }}>{total.toLocaleString()} record{total === 1 ? "" : "s"}{recs.length < total ? ` (${recs.length.toLocaleString()} loaded)` : ""}</Box>
      </Box>
      {picker && picker.field.type === "link" && (
        <LinkPicker
          field={picker.field}
          primaryId={p.tableById[picker.field.options.table_id ?? ""]?.fields[0]?.id ?? ""}
          current={(picker.rec.fields[picker.field.id] as LinkVal[]) ?? []}
          onClose={() => setPicker(null)}
          onDone={(ids) => { const { rec, field } = picker; setPicker(null); void commit(rec, field, ids); }}
        />
      )}
      {picker && picker.field.type === "attachment" && (
        <AttachPicker
          current={(picker.rec.fields[picker.field.id] as { file_id: string; name: string; size: number; mime: string }[]) ?? []}
          onClose={() => setPicker(null)}
          onDone={(v) => { const { rec, field } = picker; setPicker(null); void commit(rec, field, v); }}
        />
      )}
    </Box>
  );
}

/** Turns a displayed value back into something the API accepts (used by fill-down). */
export function toSend(f: Field, v: unknown): unknown {
  if (v === null || v === undefined) return null;
  switch (f.type) {
    case "single_select": return choiceName(f, v);
    case "multi_select": return (v as string[]).map((id) => choiceName(f, id));
    case "percent": return `${Math.round((v as number) * 1e6) / 1e4}%`;
    case "link": return (v as LinkVal[]).map((l) => l.id);
    default: return v;
  }
}
