import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Button, CircularProgress, Dropdown, IconButton, Input, Menu, MenuButton, MenuItem, Typography } from "@mui/joy";
import FilterListIcon from "@mui/icons-material/FilterList";
import SortIcon from "@mui/icons-material/Sort";
import ViewColumnIcon from "@mui/icons-material/ViewColumn";
import GroupWorkIcon from "@mui/icons-material/GroupWork";
import TuneIcon from "@mui/icons-material/Tune";
import SearchIcon from "@mui/icons-material/Search";
import { records, type Base, type Field, type Rec, type Table, type View, type ViewConfig, type ViewType } from "./api";
import { Grid } from "./Grid";
import { Calendar, FormView, Gallery, Kanban, List } from "./Views";
import { FieldDialog, FilterPanel, GroupPanel, HidePanel, ImportDialog, RecordDialog, ShareDialog, SortPanel, ViewSettings } from "./Panels";
import { filterCount, isComputed, viewQuery, visibleFields } from "./logic";

const PAGE = 200;
const VIEW_ICON: Record<ViewType, string> = { grid: "▦", kanban: "☷", gallery: "▣", calendar: "▤", list: "☰", form: "✎" };

export interface WorkspaceProps {
  baseId: string;
  embedded?: boolean;
  initialTable?: string;
  initialRecord?: string;
  onBaseChange?: (b: Base) => void;
  onBack?: () => void;
}

/** One base: tables, views, toolbar and the records of the open view. Also used for databases inside Wiki pages. */
export function Workspace({ baseId, embedded, initialTable, initialRecord, onBaseChange, onBack }: WorkspaceProps) {
  const [base, setBase] = useState<Base | null>(null);
  const [tableId, setTableId] = useState(initialTable ?? "");
  const [viewId, setViewId] = useState("");
  const [recs, setRecs] = useState<Rec[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [err, setErr] = useState("");
  const [panel, setPanel] = useState<"" | "filter" | "sort" | "group" | "hide" | "settings">("");
  const [fieldDlg, setFieldDlg] = useState<{ field?: Field } | null>(null);
  const [openRec, setOpenRec] = useState<string>(initialRecord ?? "");
  const [importOpen, setImportOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const gen = useRef(0);
  const loadedRef = useRef(0);
  const saveTimer = useRef<ReturnType<typeof setTimeout>>();

  const table: Table | undefined = base?.tables?.find((t) => t.id === tableId) ?? base?.tables?.[0];
  const view: View | undefined = table?.views.find((v) => v.id === viewId) ?? table?.views[0];
  const canEdit = (base?.role ?? 0) >= 2;
  const tableById = useMemo(() => Object.fromEntries((base?.tables ?? []).map((t) => [t.id, t])), [base]);

  const loadBase = useCallback(async () => {
    try {
      const b = await records.base(baseId);
      setBase(b);
      onBaseChange?.(b);
      return b;
    } catch (e) {
      setErr((e as Error).message);
      return null;
    }
  }, [baseId, onBaseChange]);
  useEffect(() => { void loadBase(); }, [loadBase]);
  useEffect(() => { if (table && table.id !== tableId) setTableId(table.id); }, [table, tableId]);
  useEffect(() => { if (view && view.id !== viewId) setViewId(view.id); }, [view, viewId]);

  // Load the first page whenever the table, view query or search change.
  const queryKey = JSON.stringify([table?.id, view?.id, viewQuery(view, search)]);
  const loadFirst = useCallback(async (keep = false) => {
    if (!table || !view) return;
    const g = ++gen.current;
    setLoading(true);
    try {
      const limit = keep ? Math.min(2000, Math.max(PAGE, loadedRef.current)) : PAGE;
      const r = await records.query(table.id, viewQuery(view, search, limit, 0));
      if (g !== gen.current) return;
      setRecs(r.records);
      loadedRef.current = r.records.length;
      setTotal(r.total);
      setErr("");
    } catch (e) {
      if (g === gen.current) setErr((e as Error).message);
    } finally {
      if (g === gen.current) setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey]);
  useEffect(() => {
    const t = setTimeout(() => void loadFirst(), search ? 250 : 0);
    return () => clearTimeout(t);
  }, [loadFirst, search]);
  const loadMore = useCallback(async () => {
    if (!table || !view || loading) return;
    const g = gen.current;
    setLoading(true);
    try {
      const r = await records.query(table.id, viewQuery(view, search, PAGE, loadedRef.current));
      if (g !== gen.current) return;
      setRecs((cur) => { const seen = new Set(cur.map((x) => x.id)); const out = [...cur, ...r.records.filter((x) => !seen.has(x.id))]; loadedRef.current = out.length; return out; });
      setTotal(r.total);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [table, view, search, loading]);

  // ---- view config ----
  const patchView = (patch: Partial<ViewConfig>, name?: string) => {
    if (!table || !view || !base) return;
    const next: View = { ...view, name: name ?? view.name, config: { ...view.config, ...patch } };
    setBase({ ...base, tables: base.tables!.map((t) => (t.id === table.id ? { ...t, views: t.views.map((v) => (v.id === view.id ? next : v)) } : t)) });
    if (canEdit) {
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(() => void records.saveView(table.id, { id: view.id, name: next.name, type: next.type, config: next.config }).catch((e: Error) => setErr(e.message)), 500);
    }
  };
  const addView = async (type: ViewType) => {
    if (!table) return;
    const f = table.fields;
    const cfg: ViewConfig = {};
    if (type === "kanban") cfg.stack_by = f.find((x) => x.type === "single_select")?.id;
    if (type === "calendar") cfg.date_field = f.find((x) => x.type === "date")?.id;
    const v = await records.saveView(table.id, { name: `${type[0].toUpperCase()}${type.slice(1)} view`, type, config: cfg });
    await loadBase();
    setViewId(v.id);
  };
  const deleteView = async () => {
    if (!table || !view || !window.confirm(`Delete the view "${view.name}"?`)) return;
    try { await records.deleteView(table.id, view.id); setViewId(""); await loadBase(); } catch (e) { setErr((e as Error).message); }
  };
  const renameView = async () => {
    const n = window.prompt("View name", view?.name);
    if (n && n.trim()) patchView({}, n.trim());
  };

  // ---- records ----
  const replaceRec = (r: Rec) => setRecs((cur) => cur.map((x) => (x.id === r.id ? r : x)));
  const onEdit = useCallback(async (rec: Rec, f: Field, value: unknown) => {
    if (!table) return;
    const res = await records.update(table.id, [{ id: rec.id, fields: { [f.id]: value } }]);
    if (res[0]) replaceRec(res[0]);
    // Calculated fields in other records can depend on this edit; refresh them quietly.
    if (f.type === "link" || table.fields.some((x) => isComputed(x.type))) void loadFirst(true);
  }, [table, loadFirst]);
  const onAdd = useCallback(async (initial?: Record<string, unknown>) => {
    if (!table) return;
    const created = await records.create(table.id, [{ fields: initial ?? {} }]);
    void loadFirst(true).then(() => { if (!initial) setTimeout(() => { const el = document.querySelector('[data-testid="grid-scroller"]'); if (el) el.scrollTop = el.scrollHeight; }, 100); });
    void created;
  }, [table, loadFirst]);
  const onDelete = async (ids: string[]) => {
    if (!table) return;
    try { await records.remove(table.id, ids); setOpenRec(""); await loadFirst(true); } catch (e) { setErr((e as Error).message); }
  };
  const fields = table ? visibleFields(table, view) : [];
  const onPasteRows = async (startRow: number, startCol: number, rows: string[][]) => {
    if (!table || !view) return;
    // Unknown options in select fields are added first.
    for (let j = 0; ; j++) {
      const f = fields[startCol + j];
      if (!f) break;
      if (f.type !== "single_select" && f.type !== "multi_select") continue;
      const have = new Set((f.options.choices ?? []).map((c) => c.name.toLowerCase()));
      const add: string[] = [];
      for (const r of rows) for (const part of f.type === "multi_select" ? (r[j] ?? "").split(",") : [r[j] ?? ""]) { const t = part.trim(); if (t && !have.has(t.toLowerCase())) { have.add(t.toLowerCase()); add.push(t); } }
      if (add.length) {
        await records.updateField(f.id, { options: { ...f.options, choices: [...(f.options.choices ?? []), ...add.map((name) => ({ id: "", name }))] } });
      }
    }
    const toVal = (f: Field, t: string): unknown => {
      t = t.trim();
      if (t === "") return null;
      if (f.type === "percent" && !t.includes("%")) return t + "%";
      if (f.type === "multi_select") return t.split(",").map((s) => s.trim()).filter(Boolean);
      return t;
    };
    const editable = (f: Field) => f && !isComputed(f.type) && f.type !== "link" && f.type !== "attachment";
    const updates: { id: string; fields: Record<string, unknown> }[] = [];
    const creates: { fields: Record<string, unknown> }[] = [];
    rows.forEach((cells, i) => {
      const patch: Record<string, unknown> = {};
      cells.forEach((c, j) => { const f = fields[startCol + j]; if (f && editable(f)) patch[f.id] = toVal(f, c); });
      if (Object.keys(patch).length === 0) return;
      const rec = recs[startRow + i];
      if (rec) updates.push({ id: rec.id, fields: patch });
      else creates.push({ fields: patch });
    });
    if (updates.length) await records.update(table.id, updates);
    if (creates.length) await records.create(table.id, creates);
    await loadBase();
    await loadFirst(true);
  };

  const download = async () => {
    if (!table || !view) return;
    const r = await fetch(records.exportUrl(table.id), { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ query: viewQuery(view, search), fields: fields.map((f) => f.id) }) });
    if (!r.ok) { setErr(await r.text()); return; }
    const blob = await r.blob();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${table.name}.csv`;
    a.click();
    URL.revokeObjectURL(a.href);
  };
  const saveToDrive = async () => {
    if (!table || !view) return;
    try { const r = await records.exportToDrive(table.id, viewQuery(view, search)); setErr(""); window.alert(`Saved "${r.name}" to your Drive.`); } catch (e) { setErr((e as Error).message); }
  };

  if (err && !base) return <Typography color="danger" sx={{ p: 2 }} role="alert">{err}</Typography>;
  if (!base || !table || !view) return <Box sx={{ p: 3 }}><CircularProgress /></Box>;
  const sortOf = (fid: string) => { const s = view.config.sort?.find((x) => x.field === fid); return s ? (s.desc ? "desc" : "asc") : "" as const; };
  const nFilter = filterCount(view.config.filter);
  const common = { table, view, recs, canEdit, onOpen: (r: Rec) => setOpenRec(r.id), onEdit, onAdd };
  const openIdx = recs.findIndex((r) => r.id === openRec);
  const current = recs[openIdx];

  return (
    <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, height: embedded ? 560 : "calc(100vh - 150px)", border: embedded ? "1px solid" : undefined, borderColor: "divider", borderRadius: embedded ? "md" : 0 }} data-testid="records-workspace">
      {!embedded && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }}>
          {onBack && <Button size="sm" variant="plain" onClick={onBack}>‹ Bases</Button>}
          <Box sx={{ width: 12, height: 12, borderRadius: "3px", bgcolor: base.color }} />
          <Typography level="title-md">{base.name}</Typography>
          <Box sx={{ flex: 1 }} />
          <Dropdown>
            <MenuButton size="sm" variant="outlined" color="neutral">Base ▾</MenuButton>
            <Menu size="sm" placement="bottom-end" sx={{ zIndex: 2000 }}>
              {base.role >= 3 && <MenuItem onClick={() => setShareOpen(true)}>Share…</MenuItem>}
              {canEdit && <MenuItem onClick={() => setImportOpen(true)}>Import CSV…</MenuItem>}
              <MenuItem component="a" href={records.backupUrl(base.id)} download="records-backup.json">Download backup (JSON)</MenuItem>
              {base.role >= 3 && <MenuItem color="danger" onClick={() => { if (window.confirm(`Move "${base.name}" to the trash?`)) void records.trashBase(base.id).then(() => onBack?.()); }}>Move to trash</MenuItem>}
            </Menu>
          </Dropdown>
        </Box>
      )}
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, px: 1, borderBottom: "1px solid", borderColor: "divider", overflowX: "auto" }} role="tablist" aria-label="Tables">
        {base.tables!.map((t) => (
          <Box key={t.id} role="tab" aria-selected={t.id === table.id} tabIndex={0} onClick={() => { setTableId(t.id); setViewId(""); setSearch(""); }}
            onDoubleClick={() => { if (!canEdit) return; const n = window.prompt("Rename table", t.name); if (n?.trim()) void records.updateTable(t.id, n.trim(), t.description).then(loadBase); }}
            sx={{ px: 1.5, py: 0.8, cursor: "pointer", fontSize: 14, whiteSpace: "nowrap", borderBottom: "2px solid", borderColor: t.id === table.id ? "primary.600" : "transparent", fontWeight: t.id === table.id ? 600 : 400 }}>
            {t.name}
          </Box>
        ))}
        {canEdit && <Button size="sm" variant="plain" onClick={() => { const n = window.prompt("Table name", "New table"); if (n?.trim()) void records.createTable(base.id, n.trim()).then(async (t) => { await loadBase(); setTableId(t.id); setViewId(""); }).catch((e: Error) => setErr(e.message)); }} aria-label="Add table">＋ Table</Button>}
        {canEdit && base.tables!.length > 1 && <Button size="sm" variant="plain" color="danger" onClick={() => { if (window.confirm(`Delete the table "${table.name}" and all its records?`)) void records.deleteTable(table.id).then(async () => { setTableId(""); setViewId(""); await loadBase(); }).catch((e: Error) => setErr(e.message)); }} aria-label="Delete table">🗑</Button>}
      </Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.7, px: 1, py: 0.6, flexWrap: "wrap", position: "relative", borderBottom: "1px solid", borderColor: "divider" }}>
        <Dropdown>
          <MenuButton size="sm" variant="soft" color="neutral" data-testid="view-menu">{VIEW_ICON[view.type]} {view.name} ▾</MenuButton>
          <Menu size="sm" sx={{ zIndex: 2000 }}>
            {table.views.map((v) => <MenuItem key={v.id} selected={v.id === view.id} onClick={() => { setViewId(v.id); setPanel(""); }}>{VIEW_ICON[v.type]} {v.name}</MenuItem>)}
            {canEdit && <>
              <MenuItem disabled>Add a view</MenuItem>
              {(["grid", "kanban", "gallery", "calendar", "list", "form"] as ViewType[]).map((t) => <MenuItem key={t} onClick={() => void addView(t)}>{VIEW_ICON[t]} {t[0].toUpperCase() + t.slice(1)}</MenuItem>)}
              <MenuItem onClick={() => void renameView()}>Rename this view</MenuItem>
              {table.views.length > 1 && <MenuItem color="danger" onClick={() => void deleteView()}>Delete this view</MenuItem>}
            </>}
          </Menu>
        </Dropdown>
        {view.type !== "form" && (
          <>
            {(view.type === "grid" || view.type === "list" || view.type === "gallery" || view.type === "kanban" || view.type === "calendar") && (
              <>
                <Button size="sm" variant={nFilter ? "soft" : "plain"} color={nFilter ? "primary" : "neutral"} startDecorator={<FilterListIcon fontSize="small" />} onClick={() => setPanel(panel === "filter" ? "" : "filter")} data-testid="filter-btn">{nFilter ? `${nFilter} filter${nFilter > 1 ? "s" : ""}` : "Filter"}</Button>
                <Button size="sm" variant={view.config.sort?.length ? "soft" : "plain"} color={view.config.sort?.length ? "primary" : "neutral"} startDecorator={<SortIcon fontSize="small" />} onClick={() => setPanel(panel === "sort" ? "" : "sort")} data-testid="sort-btn">{view.config.sort?.length ? `Sorted by ${view.config.sort.length}` : "Sort"}</Button>
                {view.type === "grid" && <Button size="sm" variant={view.config.group_by?.length ? "soft" : "plain"} color={view.config.group_by?.length ? "primary" : "neutral"} startDecorator={<GroupWorkIcon fontSize="small" />} onClick={() => setPanel(panel === "group" ? "" : "group")}>Group</Button>}
                <Button size="sm" variant="plain" color="neutral" startDecorator={<ViewColumnIcon fontSize="small" />} onClick={() => setPanel(panel === "hide" ? "" : "hide")}>Fields</Button>
              </>
            )}
            <Input size="sm" startDecorator={<SearchIcon fontSize="small" />} placeholder="Search" value={search} onChange={(e) => setSearch(e.target.value)} sx={{ width: { xs: 120, sm: 180 } }} slotProps={{ input: { "aria-label": "Search records" } }} />
          </>
        )}
        <Box sx={{ flex: 1 }} />
        {canEdit && <IconButton size="sm" variant="plain" aria-label="View settings" onClick={() => setPanel(panel === "settings" ? "" : "settings")}><TuneIcon fontSize="small" /></IconButton>}
        <Dropdown>
          <MenuButton size="sm" variant="plain" color="neutral" aria-label="Table actions">⋯</MenuButton>
          <Menu size="sm" placement="bottom-end" sx={{ zIndex: 2000 }}>
            <MenuItem onClick={() => void download()}>Download CSV</MenuItem>
            <MenuItem onClick={() => void saveToDrive()}>Save CSV to Drive</MenuItem>
            {canEdit && <MenuItem onClick={() => setImportOpen(true)}>Import CSV…</MenuItem>}
            {canEdit && <MenuItem onClick={() => setFieldDlg({})}>Add field…</MenuItem>}
          </Menu>
        </Dropdown>
        {panel === "filter" && <FilterPanel fields={table.fields} value={view.config.filter} onChange={(n) => patchView({ filter: n })} onClose={() => setPanel("")} />}
        {panel === "sort" && <SortPanel fields={table.fields} value={view.config.sort ?? []} onChange={(s) => patchView({ sort: s })} onClose={() => setPanel("")} />}
        {panel === "group" && <GroupPanel fields={table.fields} value={view.config.group_by ?? []} onChange={(g) => patchView({ group_by: g })} onClose={() => setPanel("")} />}
        {panel === "hide" && <HidePanel fields={table.fields} hidden={view.config.hidden ?? []} onChange={(h) => patchView({ hidden: h })} onClose={() => setPanel("")} />}
        {panel === "settings" && <ViewSettings table={table} type={view.type} config={view.config} onChange={(c) => patchView(c)} onClose={() => setPanel("")} />}
      </Box>
      {err && <Box sx={{ px: 1.5, py: 0.5, bgcolor: "danger.softBg", color: "danger.700", fontSize: 13 }} role="alert" onClick={() => setErr("")}>{err}</Box>}
      {view.type === "grid" && (
        <Grid
          table={table} view={view} fields={fields} recs={recs} total={total} loading={loading} canEdit={canEdit} tableById={tableById}
          onNeedMore={() => void loadMore()} onEdit={onEdit} onOpen={(r) => setOpenRec(r.id)} onDelete={(ids) => void onDelete(ids)} onAdd={() => void onAdd()}
          onPasteRows={onPasteRows} onEditField={(f) => setFieldDlg({ field: f })} onHideField={(f) => patchView({ hidden: [...(view.config.hidden ?? []), f.id] })}
          onDeleteField={(f) => { if (window.confirm(`Delete the field "${f.name}"? Its values, and any fields built on it, are removed.`)) void records.deleteField(f.id).then(loadBase).then(() => loadFirst(true)).catch((e: Error) => setErr(e.message)); }}
          onSort={(f, desc) => patchView({ sort: [{ field: f.id, desc }] })} onResize={(id, w) => patchView({ widths: { ...(view.config.widths ?? {}), [id]: Math.round(w) } })}
          onAddField={() => setFieldDlg({})} sortOf={sortOf}
        />
      )}
      {view.type === "kanban" && <Kanban {...common} />}
      {view.type === "gallery" && <Gallery {...common} />}
      {view.type === "list" && <List {...common} />}
      {view.type === "calendar" && <Calendar {...common} />}
      {view.type === "form" && <FormView {...common} />}
      {fieldDlg && <FieldDialog table={table} tables={base.tables!} field={fieldDlg.field} onClose={() => setFieldDlg(null)} onSaved={() => { setFieldDlg(null); void loadBase().then(() => loadFirst(true)); }} />}
      {current && (
        <RecordDialog table={table} tables={base.tables!} rec={current} canEdit={canEdit} onClose={() => setOpenRec("")} onEdit={onEdit} onDelete={(id) => void onDelete([id])}
          onNav={(d) => { const n = recs[openIdx + d]; if (n) setOpenRec(n.id); }} />
      )}
      {importOpen && <ImportDialog base={base} table={table} onClose={() => setImportOpen(false)} onDone={async (tid) => { setImportOpen(false); await loadBase(); setTableId(tid); setViewId(""); }} />}
      {shareOpen && <ShareDialog base={base} onClose={() => setShareOpen(false)} onSaved={() => { setShareOpen(false); void loadBase(); }} />}
    </Box>
  );
}
