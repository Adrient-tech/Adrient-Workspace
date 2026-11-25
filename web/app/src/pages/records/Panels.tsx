import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useMemo, useState } from "react";
import { Box, Button, Checkbox, DialogContent, DialogTitle, FormControl, FormLabel, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography, Tab, TabList, TabPanel, Tabs } from "@mui/joy";
import { records, type Base, type Choice, type Field, type FieldType, type Member, type Node, type Person, type Rec, type Table, type ViewConfig } from "./api";
import { CellView, Pill } from "./Editors";
import { DATE_MODES, kindOf, newCondition, NO_VALUE, OPERATORS, TYPE_LABEL, TYPE_ORDER, WITHIN_MODES, isComputed } from "./logic";

// ---- field dialog -------------------------------------------------------------

const COLORS = ["#2D7FF9", "#18BFFF", "#20D9D2", "#20C933", "#FFD66E", "#FF9F40", "#F82B60", "#FF08C2", "#8B46FF", "#666666"];

export function FieldDialog({ table, tables, field, onClose, onSaved }: { table: Table; tables: Table[]; field?: Field; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(field?.name ?? "");
  const [type, setType] = useState<FieldType>(field?.type ?? "text");
  const [desc, setDesc] = useState(field?.description ?? "");
  const [opts, setOpts] = useState<Record<string, unknown>>(field?.options ?? {});
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [warn, setWarn] = useState("");
  const set = (k: string, v: unknown) => setOpts((o) => ({ ...o, [k]: v }));
  const choices = (opts.choices as Choice[] | undefined) ?? [];
  const links = table.fields.filter((f) => f.type === "link");
  const linkField = table.fields.find((f) => f.id === opts.link_field_id);
  const targetTable = linkField ? tables.find((t) => t.id === linkField.options.table_id) : undefined;
  const isFirst = field && table.fields[0]?.id === field.id;
  const types = isFirst ? (["text", "long_text", "number", "formula", "autonumber"] as FieldType[]) : TYPE_ORDER;
  const save = async () => {
    setBusy(true);
    setErr("");
    try {
      if (field) {
        const r = await records.updateField(field.id, { name, type, options: opts, description: desc });
        if (r.cleared > 0) setWarn(`${r.cleared} value(s) did not fit the new type and were cleared.`);
        else { onSaved(); return; }
      } else {
        await records.createField(table.id, { name, type, options: opts, description: desc, two_way: opts.two_way !== false });
        onSaved();
        return;
      }
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 480, maxWidth: "95vw", maxHeight: "92vh", overflow: "auto" }} role="dialog" aria-label={field ? "Edit field" : "Add field"}>
        <ModalClose />
        <DialogTitle>{field ? "Edit field" : "Add field"}</DialogTitle>
        <DialogContent>
          <FormControl sx={{ mb: 1.5 }}><FormLabel>Name</FormLabel><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} data-testid="field-name" /></FormControl>
          <FormControl sx={{ mb: 1.5 }}>
            <FormLabel>Type</FormLabel>
            <Select value={type} onChange={(_, v) => v && setType(v)} slotProps={{ listbox: { sx: { zIndex: 2000 } } }} data-testid="field-type">
              {types.map((t) => <Option key={t} value={t}>{TYPE_LABEL[t]}</Option>)}
            </Select>
          </FormControl>
          {field && field.type !== type && !(isComputed(field.type) && isComputed(type)) && (
            <Typography level="body-sm" color="warning" sx={{ mb: 1 }}>Changing the type converts the values. Values that do not fit are cleared.</Typography>
          )}
          {(type === "single_select" || type === "multi_select") && (
            <Box sx={{ mb: 1.5 }}>
              <FormLabel>Options</FormLabel>
              {choices.map((c, i) => (
                <Box key={c.id ?? i} sx={{ display: "flex", gap: 0.5, mb: 0.5, alignItems: "center" }}>
                  <Box component="input" type="color" value={c.color ?? COLORS[i % COLORS.length]} onChange={(e: React.ChangeEvent<HTMLInputElement>) => set("choices", choices.map((x, j) => (j === i ? { ...x, color: e.target.value } : x)))} sx={{ width: 28, height: 28, border: 0, p: 0, background: "none" }} aria-label="Option colour" />
                  <Input size="sm" value={c.name} onChange={(e) => set("choices", choices.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} sx={{ flex: 1 }} aria-label={`Option ${i + 1}`} />
                  <IconButton size="sm" variant="plain" aria-label="Remove option" onClick={() => set("choices", choices.filter((_, j) => j !== i))}>✕</IconButton>
                </Box>
              ))}
              <Button size="sm" variant="soft" onClick={() => set("choices", [...choices, { id: "", name: "", color: COLORS[choices.length % COLORS.length] }])}>＋ Add option</Button>
            </Box>
          )}
          {(type === "number" || type === "currency" || type === "percent") && (
            <FormControl sx={{ mb: 1.5 }}><FormLabel>Decimal places</FormLabel>
              <Select value={Number(opts.precision ?? (type === "currency" ? 2 : 0))} onChange={(_, v) => set("precision", v)} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                {[0, 1, 2, 3, 4, 5, 6, 7, 8].map((n) => <Option key={n} value={n}>{n}</Option>)}
              </Select>
            </FormControl>
          )}
          {type === "currency" && <FormControl sx={{ mb: 1.5 }}><FormLabel>Symbol</FormLabel><Input value={String(opts.symbol ?? "₹")} onChange={(e) => set("symbol", e.target.value.slice(0, 3))} /></FormControl>}
          {type === "rating" && <FormControl sx={{ mb: 1.5 }}><FormLabel>Maximum</FormLabel><Select value={Number(opts.max ?? 5)} onChange={(_, v) => set("max", v)} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => <Option key={n} value={n}>{n}</Option>)}</Select></FormControl>}
          {type === "date" && <Checkbox label="Include time" checked={!!opts.include_time} onChange={(e) => set("include_time", e.target.checked)} sx={{ mb: 1.5 }} />}
          {type === "link" && !field && (
            <>
              <FormControl sx={{ mb: 1.5 }}><FormLabel>Link to table</FormLabel>
                <Select value={String(opts.table_id ?? "")} onChange={(_, v) => set("table_id", v)} placeholder="Choose a table" slotProps={{ listbox: { sx: { zIndex: 2000 } } }} data-testid="link-table">
                  {tables.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
                </Select>
              </FormControl>
              <Checkbox label="Allow linking to multiple records" checked={opts.allow_multiple !== false} onChange={(e) => set("allow_multiple", e.target.checked)} sx={{ mb: 1 }} />
              <Checkbox label="Also add a field to the other table (two-way link)" checked={opts.two_way !== false} onChange={(e) => set("two_way", e.target.checked)} sx={{ mb: 1.5 }} />
            </>
          )}
          {(type === "lookup" || type === "rollup" || type === "count") && (
            <>
              <FormControl sx={{ mb: 1.5 }}><FormLabel>Through link field</FormLabel>
                <Select value={String(opts.link_field_id ?? "")} onChange={(_, v) => { set("link_field_id", v); set("target_field_id", ""); }} placeholder="Choose a link field" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                  {links.map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}
                </Select>
              </FormControl>
              {type !== "count" && (
                <FormControl sx={{ mb: 1.5 }}><FormLabel>Field to read</FormLabel>
                  <Select value={String(opts.target_field_id ?? "")} onChange={(_, v) => set("target_field_id", v)} placeholder={targetTable ? "Choose a field" : "Choose a link field first"} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                    {(targetTable?.fields ?? []).map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}
                  </Select>
                </FormControl>
              )}
              {type === "rollup" && (
                <FormControl sx={{ mb: 1.5 }}><FormLabel>Function</FormLabel>
                  <Select value={String(opts.fn ?? "")} onChange={(_, v) => set("fn", v)} placeholder="Choose" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                    {["sum", "avg", "min", "max", "count", "count_all", "count_unique", "concat", "unique", "and", "or", "earliest", "latest"].map((f) => <Option key={f} value={f}>{f.replace("_", " ")}</Option>)}
                  </Select>
                </FormControl>
              )}
            </>
          )}
          {(type === "formula" || type === "button") && (
            <FormControl sx={{ mb: 1.5 }}>
              <FormLabel>{type === "button" ? "Link (a formula that returns a URL)" : "Formula"}</FormLabel>
              <Textarea minRows={3} value={String(opts[type === "button" ? "url" : "formula"] ?? "")} onChange={(e) => set(type === "button" ? "url" : "formula", e.target.value)} placeholder={'IF({Qty} > 10, "Bulk", "Single")'} slotProps={{ textarea: { "aria-label": "Formula" } as never }} data-testid="formula-input" />
              <Typography level="body-xs">Use {"{Field name}"} to read a field. Functions: IF, SWITCH, SUM, ROUND, CONCATENATE, DATEADD, DATETIME_DIFF, REGEX_MATCH, WORKDAY and about 70 more.</Typography>
              {type === "button" && <Input sx={{ mt: 1 }} placeholder="Button label" value={String(opts.label ?? "")} onChange={(e) => set("label", e.target.value)} />}
            </FormControl>
          )}
          <FormControl sx={{ mb: 1 }}><FormLabel>Description (optional)</FormLabel><Input value={desc} onChange={(e) => setDesc(e.target.value)} /></FormControl>
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          {warn && <Typography color="warning" level="body-sm">{warn}</Typography>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button loading={busy} disabled={!name.trim()} onClick={() => void save()} data-testid="field-save">{field ? "Save" : "Create field"}</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---- filters ------------------------------------------------------------------

function ValueInput({ field, cond, onChange }: { field: Field; cond: Node; onChange: (v: unknown) => void }) {
  const kind = kindOf(field);
  const op = cond.op ?? "";
  if (NO_VALUE.has(op)) return null;
  if (kind === "checkbox") return <Select size="sm" value={cond.value === false ? "false" : "true"} onChange={(_, v) => onChange(v === "true")} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="true">checked</Option><Option value="false">unchecked</Option></Select>;
  if (kind === "select" || kind === "multi") {
    const many = op === "isAnyOf" || op === "isNoneOf" || kind === "multi";
    const cur = many ? (Array.isArray(cond.value) ? (cond.value as string[]) : []) : (cond.value as string | undefined);
    return (
      <Select size="sm" multiple={many} value={(cur ?? (many ? [] : null)) as never} onChange={(_, v) => onChange(v)} placeholder="Choose" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
        {(field.options.choices ?? []).map((c) => <Option key={c.id} value={c.name}>{c.name}</Option>)}
      </Select>
    );
  }
  if (kind === "date") {
    if (op === "isWithin") {
      const m = (cond.value as { mode?: string; n?: number } | undefined) ?? {};
      return (
        <Box sx={{ display: "flex", gap: 0.5 }}>
          <Select size="sm" value={m.mode ?? null} placeholder="Choose" onChange={(_, v) => onChange({ mode: v, n: m.n })} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{WITHIN_MODES.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select>
          {(m.mode === "pastDays" || m.mode === "nextDays") && <Input size="sm" type="number" sx={{ width: 70 }} value={m.n ?? ""} onChange={(e) => onChange({ mode: m.mode, n: Number(e.target.value) })} />}
        </Box>
      );
    }
    const m = (cond.value as { mode?: string; n?: number; date?: string } | undefined) ?? {};
    return (
      <Box sx={{ display: "flex", gap: 0.5 }}>
        <Select size="sm" value={m.mode ?? null} placeholder="Choose" onChange={(_, v) => onChange({ ...m, mode: v })} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{DATE_MODES.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select>
        {(m.mode === "daysAgo" || m.mode === "daysFromNow") && <Input size="sm" type="number" sx={{ width: 70 }} value={m.n ?? ""} onChange={(e) => onChange({ ...m, n: Number(e.target.value) })} />}
        {m.mode === "exactDate" && <Input size="sm" type="date" value={m.date ?? ""} onChange={(e) => onChange({ ...m, date: e.target.value })} />}
      </Box>
    );
  }
  if (kind === "attachment" && op === "hasFileType") {
    return <Select size="sm" value={String(cond.value ?? "")} onChange={(_, v) => onChange(v)} placeholder="Type" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{["image", "text", "pdf", "video", "audio", "spreadsheet"].map((t) => <Option key={t} value={t}>{t}</Option>)}</Select>;
  }
  return <Input size="sm" value={String(cond.value ?? "")} onChange={(e) => onChange(kind === "number" ? e.target.value : e.target.value)} placeholder="Value" slotProps={{ input: { "aria-label": "Filter value" } }} />;
}

export function FilterPanel({ fields, value, onChange, onClose }: { fields: Field[]; value: Node | undefined; onChange: (n: Node | undefined) => void; onClose: () => void }) {
  const root: Node = value && !value.field ? value : value ? { logic: "and", conds: [value] } : { logic: "and", conds: [] };
  const byId = new Map(fields.map((f) => [f.id, f]));
  const upd = (path: number[], fn: (n: Node) => Node | null) => {
    const clone = JSON.parse(JSON.stringify(root)) as Node;
    const walk = (n: Node, p: number[]): void => {
      if (p.length === 1) {
        const r = fn(n.conds![p[0]]);
        if (r === null) n.conds!.splice(p[0], 1);
        else n.conds![p[0]] = r;
        return;
      }
      walk(n.conds![p[0]], p.slice(1));
    };
    if (path.length === 0) { onChange(fn(clone) ?? undefined); return; }
    walk(clone, path);
    onChange(clone.conds?.length ? clone : undefined);
  };
  const addAt = (path: number[], group = false) => {
    const clone = JSON.parse(JSON.stringify(root)) as Node;
    let n = clone;
    for (const i of path) n = n.conds![i];
    n.conds = n.conds ?? [];
    n.conds.push(group ? { logic: "or", conds: [newCondition(fields[0])] } : newCondition(fields[0]));
    onChange(clone);
  };
  const setLogic = (path: number[], l: "and" | "or") => {
    const clone = JSON.parse(JSON.stringify(root)) as Node;
    let n = clone;
    for (const i of path) n = n.conds![i];
    n.logic = l;
    onChange(clone);
  };
  const renderNode = (n: Node, path: number[], depth: number): React.ReactNode => (
    <Box sx={{ pl: depth ? 1.5 : 0, borderLeft: depth ? "2px solid" : "none", borderColor: "divider", my: 0.5 }}>
      {(n.conds ?? []).map((c, i) => {
        const p = [...path, i];
        if (!c.field) {
          return (
            <Box key={i} sx={{ my: 0.5 }}>
              <Box sx={{ display: "flex", gap: 0.5, alignItems: "center", fontSize: 12 }}>
                Match <Select size="sm" value={c.logic ?? "and"} onChange={(_, v) => v && setLogic(p, v as "and" | "or")} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="and">all</Option><Option value="or">any</Option></Select> of
                <IconButton size="sm" variant="plain" onClick={() => upd(p, () => null)} aria-label="Remove group">✕</IconButton>
              </Box>
              {renderNode(c, p, depth + 1)}
            </Box>
          );
        }
        const f = byId.get(c.field);
        return (
          <Box key={i} sx={{ display: "flex", gap: 0.5, alignItems: "center", flexWrap: "wrap", my: 0.5 }} data-testid="filter-row">
            <Box sx={{ width: 42, fontSize: 12, color: "text.tertiary" }}>{i === 0 ? "Where" : (n.logic ?? "and")}</Box>
            <Select size="sm" value={c.field} onChange={(_, v) => { const nf = byId.get(v ?? ""); if (nf) upd(p, () => newCondition(nf)); }} sx={{ width: 130 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{fields.map((x) => <Option key={x.id} value={x.id}>{x.name}</Option>)}</Select>
            <Select size="sm" value={c.op ?? ""} onChange={(_, v) => upd(p, (x) => ({ ...x, op: v ?? "", value: undefined }))} sx={{ width: 140 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{(f ? OPERATORS[kindOf(f)] : []).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select>
            {f && <ValueInput field={f} cond={c} onChange={(v) => upd(p, (x) => ({ ...x, value: v }))} />}
            <IconButton size="sm" variant="plain" onClick={() => upd(p, () => null)} aria-label="Remove condition">✕</IconButton>
          </Box>
        );
      })}
      {depth < 2 && (
        <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
          <Button size="sm" variant="plain" onClick={() => addAt(path)} data-testid="add-condition">＋ Add condition</Button>
          {depth === 0 && <Button size="sm" variant="plain" onClick={() => addAt(path, true)}>＋ Add condition group</Button>}
        </Box>
      )}
    </Box>
  );
  return (
    <Box sx={{ p: 1.5, minWidth: 320, maxWidth: "95vw", bgcolor: "background.popup", border: "1px solid", borderColor: "divider", borderRadius: "md", boxShadow: "lg", position: "absolute", zIndex: 40, mt: 0.5 }} role="dialog" aria-label="Filter records">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
        <Typography level="title-sm">Filter</Typography>
        {(root.conds?.length ?? 0) > 1 && <>Match <Select size="sm" value={root.logic ?? "and"} onChange={(_, v) => v && setLogic([], v as "and" | "or")} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="and">all</Option><Option value="or">any</Option></Select></>}
        <Box sx={{ flex: 1 }} />
        <Button size="sm" variant="plain" color="neutral" onClick={() => onChange(undefined)}>Clear</Button>
        <Button size="sm" variant="plain" onClick={onClose}>Done</Button>
      </Box>
      {(root.conds?.length ?? 0) === 0 && <Typography level="body-sm" sx={{ my: 1 }}>No filters. Add a condition to narrow this view.</Typography>}
      {renderNode(root, [], 0)}
    </Box>
  );
}

export function SortPanel({ fields, value, onChange, onClose }: { fields: Field[]; value: { field: string; desc?: boolean }[]; onChange: (v: { field: string; desc?: boolean }[]) => void; onClose: () => void }) {
  return (
    <Box sx={{ p: 1.5, minWidth: 300, bgcolor: "background.popup", border: "1px solid", borderColor: "divider", borderRadius: "md", boxShadow: "lg", position: "absolute", zIndex: 40, mt: 0.5 }} role="dialog" aria-label="Sort records">
      <Box sx={{ display: "flex", mb: 1 }}><Typography level="title-sm" sx={{ flex: 1 }}>Sort</Typography><Button size="sm" variant="plain" onClick={onClose}>Done</Button></Box>
      {value.map((s, i) => (
        <Box key={i} sx={{ display: "flex", gap: 0.5, mb: 0.5 }}>
          <Select size="sm" value={s.field} onChange={(_, v) => v && onChange(value.map((x, j) => (j === i ? { ...x, field: v } : x)))} sx={{ flex: 1 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{fields.map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}</Select>
          <Select size="sm" value={s.desc ? "desc" : "asc"} onChange={(_, v) => onChange(value.map((x, j) => (j === i ? { ...x, desc: v === "desc" } : x)))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="asc">A → Z</Option><Option value="desc">Z → A</Option></Select>
          <IconButton size="sm" variant="plain" aria-label="Remove sort" onClick={() => onChange(value.filter((_, j) => j !== i))}>✕</IconButton>
        </Box>
      ))}
      <Button size="sm" variant="plain" onClick={() => onChange([...value, { field: fields.find((f) => !value.some((v) => v.field === f.id))?.id ?? fields[0].id }])} data-testid="add-sort">＋ Add sort</Button>
    </Box>
  );
}

export function GroupPanel({ fields, value, onChange, onClose }: { fields: Field[]; value: string[]; onChange: (v: string[]) => void; onClose: () => void }) {
  return (
    <Box sx={{ p: 1.5, minWidth: 260, bgcolor: "background.popup", border: "1px solid", borderColor: "divider", borderRadius: "md", boxShadow: "lg", position: "absolute", zIndex: 40, mt: 0.5 }} role="dialog" aria-label="Group records">
      <Box sx={{ display: "flex", mb: 1 }}><Typography level="title-sm" sx={{ flex: 1 }}>Group by</Typography><Button size="sm" variant="plain" onClick={onClose}>Done</Button></Box>
      <Select size="sm" value={value[0] ?? ""} onChange={(_, v) => onChange(v ? [v] : [])} placeholder="No grouping" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
        <Option value="">No grouping</Option>
        {fields.filter((f) => !["attachment", "long_text"].includes(f.type)).map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}
      </Select>
    </Box>
  );
}

export function HidePanel({ fields, hidden, onChange, onClose }: { fields: Field[]; hidden: string[]; onChange: (v: string[]) => void; onClose: () => void }) {
  return (
    <Box sx={{ p: 1.5, minWidth: 240, maxHeight: 360, overflow: "auto", bgcolor: "background.popup", border: "1px solid", borderColor: "divider", borderRadius: "md", boxShadow: "lg", position: "absolute", zIndex: 40, mt: 0.5 }} role="dialog" aria-label="Show fields">
      <Box sx={{ display: "flex", mb: 1 }}><Typography level="title-sm" sx={{ flex: 1 }}>Fields</Typography><Button size="sm" variant="plain" onClick={onClose}>Done</Button></Box>
      {fields.slice(1).map((f) => <Box key={f.id} sx={{ mb: 0.5 }}><Checkbox size="sm" label={f.name} checked={!hidden.includes(f.id)} onChange={(e) => onChange(e.target.checked ? hidden.filter((h) => h !== f.id) : [...hidden, f.id])} /></Box>)}
    </Box>
  );
}

export function ViewSettings({ table, type, config, onChange, onClose }: { table: Table; type: string; config: ViewConfig; onChange: (c: ViewConfig) => void; onClose: () => void }) {
  const fieldSel = (label: string, key: keyof ViewConfig, accept: (f: Field) => boolean) => (
    <FormControl sx={{ mb: 1 }}><FormLabel>{label}</FormLabel>
      <Select size="sm" value={String(config[key] ?? "")} onChange={(_, v) => onChange({ ...config, [key]: v || undefined })} placeholder="None" slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
        <Option value="">None</Option>
        {table.fields.filter(accept).map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}
      </Select>
    </FormControl>
  );
  return (
    <Box sx={{ p: 1.5, minWidth: 280, bgcolor: "background.popup", border: "1px solid", borderColor: "divider", borderRadius: "md", boxShadow: "lg", position: "absolute", zIndex: 40, mt: 0.5, right: 0 }} role="dialog" aria-label="View settings">
      <Box sx={{ display: "flex", mb: 1 }}><Typography level="title-sm" sx={{ flex: 1 }}>View settings</Typography><Button size="sm" variant="plain" onClick={onClose}>Done</Button></Box>
      {type === "grid" && (
        <FormControl sx={{ mb: 1 }}><FormLabel>Row height</FormLabel>
          <Select size="sm" value={config.row_height ?? "short"} onChange={(_, v) => onChange({ ...config, row_height: (v ?? "short") as ViewConfig["row_height"] })} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="short">Short</Option><Option value="medium">Medium</Option><Option value="tall">Tall</Option></Select>
        </FormControl>
      )}
      {type === "kanban" && fieldSel("Stack by", "stack_by", (f) => ["single_select", "checkbox"].includes(f.type) || (f.type === "link" && f.options.allow_multiple === false))}
      {type === "calendar" && <>{fieldSel("Date field", "date_field", (f) => ["date", "created_time", "modified_time"].includes(f.type))}{fieldSel("End date field (optional)", "end_date_field", (f) => f.type === "date")}</>}
      {type === "gallery" && fieldSel("Cover (attachment field)", "cover_field", (f) => f.type === "attachment")}
      {type === "form" && (
        <>
          <FormControl sx={{ mb: 1 }}><FormLabel>Form title</FormLabel><Input size="sm" value={config.form_title ?? ""} onChange={(e) => onChange({ ...config, form_title: e.target.value })} /></FormControl>
          <FormLabel>Fields on the form</FormLabel>
          {table.fields.filter((f) => !isComputed(f.type) && f.type !== "link" && f.type !== "attachment").map((f) => {
            const ids = config.form_fields ?? table.fields.filter((x) => !isComputed(x.type) && x.type !== "link" && x.type !== "attachment").map((x) => x.id);
            return <Box key={f.id}><Checkbox size="sm" label={f.name} checked={ids.includes(f.id)} onChange={(e) => onChange({ ...config, form_fields: e.target.checked ? [...ids, f.id] : ids.filter((i) => i !== f.id) })} /></Box>;
          })}
        </>
      )}
      {type !== "grid" && type !== "kanban" && type !== "calendar" && type !== "gallery" && type !== "form" && <Typography level="body-sm">Use Filter, Sort and Fields to shape this view.</Typography>}
    </Box>
  );
}

// ---- record dialog ------------------------------------------------------------

export function RecordDialog({ table, tables, rec, canEdit, onClose, onEdit, onDelete, onNav }: {
  table: Table; tables: Table[]; rec: Rec; canEdit: boolean; onClose: () => void;
  onEdit: (r: Rec, f: Field, v: unknown) => Promise<void>; onDelete: (id: string) => void; onNav: (d: number) => void;
}) {
  void tables;
  const [tab, setTab] = useState<string | number>("fields");
  const [comments, setComments] = useState<{ id: string; author: string; body: string; created_at: string }[]>([]);
  const [history, setHistory] = useState<{ at: string; user: string; changes: { field: string; old: string; new: string }[] }[]>([]);
  const [text, setText] = useState("");
  const [err, setErr] = useState("");
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  useEffect(() => {
    records.comments(rec.id).then(setComments).catch(() => undefined);
    records.history(rec.id).then(setHistory).catch(() => undefined);
  }, [rec.id, rec.updated_at]);
  const save = async (f: Field, raw: string) => {
    let v: unknown = raw.trim() === "" ? null : raw.trim();
    if (f.type === "percent" && typeof v === "string" && !v.includes("%")) v = v + "%";
    if (f.type === "multi_select" && typeof v === "string") v = v.split(",").map((s) => s.trim()).filter(Boolean);
    if (f.type === "checkbox") v = raw === "true";
    setErr("");
    try {
      await onEdit(rec, f, v);
      setDrafts((d) => { const n = { ...d }; delete n[f.id]; return n; });
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog layout="center" sx={{ width: 640, maxWidth: "100vw", maxHeight: "95vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }} role="dialog" aria-label="Record">
        <ModalClose />
        <DialogTitle>
          {String(rec.fields[table.fields[0].id] ?? "") || "Untitled"}
          <Box sx={{ ml: 2 }}><Button size="sm" variant="plain" onClick={() => onNav(-1)} aria-label="Previous record">↑</Button><Button size="sm" variant="plain" onClick={() => onNav(1)} aria-label="Next record">↓</Button></Box>
        </DialogTitle>
        <DialogContent>
          <Tabs value={tab} onChange={(_, v) => v !== null && setTab(v)}>
            <TabList><Tab value="fields">Fields</Tab><Tab value="comments">Comments ({comments.length})</Tab><Tab value="history">History</Tab></TabList>
            <TabPanel value="fields" sx={{ px: 0 }}>
              {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
              {table.fields.map((f) => {
                const computed = isComputed(f.type) || f.type === "link" || f.type === "attachment";
                const v = rec.fields[f.id];
                const simple = !computed && !["single_select", "multi_select", "checkbox", "rating"].includes(f.type);
                return (
                  <Box key={f.id} sx={{ mb: 1.5 }}>
                    <Typography level="body-xs" sx={{ fontWeight: 600 }} component="label" htmlFor={`rf-${f.id}`}>{f.name}{computed && f.type !== "link" && f.type !== "attachment" ? " (calculated)" : ""}</Typography>
                    {simple && canEdit ? (
                      f.type === "long_text" ? <Textarea id={`rf-${f.id}`} minRows={2} value={drafts[f.id] ?? String(v ?? "")} onChange={(e) => setDrafts({ ...drafts, [f.id]: e.target.value })} onBlur={() => drafts[f.id] !== undefined && void save(f, drafts[f.id])} />
                        : <Input id={`rf-${f.id}`} type={f.type === "date" ? "date" : "text"} value={drafts[f.id] ?? (f.type === "date" ? String(v ?? "").slice(0, 10) : f.type === "percent" && typeof v === "number" ? `${Math.round(v * 1e6) / 1e4}%` : String(v ?? ""))} onChange={(e) => setDrafts({ ...drafts, [f.id]: e.target.value })} onBlur={() => drafts[f.id] !== undefined && void save(f, drafts[f.id])} onKeyDown={(e) => e.key === "Enter" && drafts[f.id] !== undefined && void save(f, drafts[f.id])} />
                    ) : f.type === "single_select" && canEdit ? (
                      <Select value={(v as string) ?? null} placeholder="Choose" onChange={(_, nv) => void onEdit(rec, f, nv ? (f.options.choices ?? []).find((c) => c.id === nv)?.name : null).catch((e: Error) => setErr(e.message))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                        <Option value={null as unknown as string}>(none)</Option>{(f.options.choices ?? []).map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
                      </Select>
                    ) : f.type === "multi_select" && canEdit ? (
                      <Select multiple value={(v as string[]) ?? []} onChange={(_, nv) => void onEdit(rec, f, (nv as string[]).map((id) => (f.options.choices ?? []).find((c) => c.id === id)?.name)).catch((e: Error) => setErr(e.message))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                        {(f.options.choices ?? []).map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
                      </Select>
                    ) : f.type === "checkbox" && canEdit ? (
                      <Box><Checkbox checked={!!v} onChange={(e) => void save(f, String(e.target.checked))} slotProps={{ input: { "aria-label": f.name } }} /></Box>
                    ) : f.type === "rating" && canEdit ? (
                      <Input type="number" value={String(v ?? "")} onChange={(e) => setDrafts({ ...drafts, [f.id]: e.target.value })} onBlur={() => drafts[f.id] !== undefined && void save(f, drafts[f.id])} />
                    ) : (
                      <Box sx={{ minHeight: 28, display: "flex", alignItems: "center" }}><CellView field={f} value={v} /></Box>
                    )}
                    {f.description && <Typography level="body-xs">{f.description}</Typography>}
                  </Box>
                );
              })}
              <Typography level="body-xs">Created {new Date(rec.created_at).toLocaleString()}{rec.created_by ? ` by ${rec.created_by}` : ""}. Link fields and attachments are edited in the grid.</Typography>
              {canEdit && <Button color="danger" variant="soft" size="sm" sx={{ mt: 1 }} onClick={() => { if (window.confirm("Delete this record?")) onDelete(rec.id); }}>Delete record</Button>}
            </TabPanel>
            <TabPanel value="comments">
              {comments.map((c) => <Box key={c.id} sx={{ mb: 1 }}><Typography level="body-xs"><b>{c.author}</b> · {new Date(c.created_at).toLocaleString()}</Typography><Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{c.body}</Typography></Box>)}
              {comments.length === 0 && <Typography level="body-sm">No comments yet.</Typography>}
              {canEdit && (
                <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                  <Input sx={{ flex: 1 }} value={text} onChange={(e) => setText(e.target.value)} placeholder="Write a comment" onKeyDown={(e) => { if (e.key === "Enter" && text.trim()) void records.addComment(rec.id, text).then(() => { setText(""); return records.comments(rec.id).then(setComments); }); }} />
                  <Button disabled={!text.trim()} onClick={() => void records.addComment(rec.id, text).then(() => { setText(""); return records.comments(rec.id).then(setComments); })}>Send</Button>
                </Box>
              )}
            </TabPanel>
            <TabPanel value="history">
              {history.map((h, i) => <Box key={i} sx={{ mb: 1 }}><Typography level="body-xs">{h.user} · {new Date(h.at).toLocaleString()}</Typography>{h.changes.map((c, j) => <Typography key={j} level="body-sm"><b>{c.field}</b>: {c.old || "(empty)"} → {c.new || "(empty)"}</Typography>)}</Box>)}
              {history.length === 0 && <Typography level="body-sm">No edits recorded.</Typography>}
            </TabPanel>
          </Tabs>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---- import / share / new base ------------------------------------------------

export function ImportDialog({ base, table, onClose, onDone }: { base: Base; table?: Table; onClose: () => void; onDone: (tableId: string) => void }) {
  const [data, setData] = useState("");
  const [fileName, setFileName] = useState("");
  const [header, setHeader] = useState(true);
  const [pv, setPv] = useState<Awaited<ReturnType<typeof records.importPreview>> | null>(null);
  const [cols, setCols] = useState<{ index: number; name: string; type: FieldType; field_id?: string; skip?: boolean }[]>([]);
  const [append, setAppend] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!data) return;
    records.importPreview(base.id, data, header).then((p) => {
      setPv(p);
      setCols(p.columns.map((c) => {
        const match = append ? table?.fields.find((f) => f.name.toLowerCase() === c.name.toLowerCase() && !isComputed(f.type) && f.type !== "link" && f.type !== "attachment") : undefined;
        return { ...c, field_id: match?.id, skip: append && !match };
      }));
      setErr("");
    }).catch((e: Error) => { setPv(null); setErr(e.message); });
  }, [data, header, base.id, append, table]);
  const run = async () => {
    setBusy(true);
    try {
      const r = await records.importCsv(base.id, { table_name: fileName.replace(/\.[^.]+$/, "") || "Imported table", table_id: append ? table?.id : undefined, data, has_header: header, columns: cols });
      onDone(r.table_id);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 720, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }} role="dialog" aria-label="Import CSV">
        <ModalClose />
        <DialogTitle>Import CSV or TSV</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 1 }}>
            <Button component="label" variant="outlined">Choose file<input hidden type="file" accept=".csv,.tsv,.txt,text/csv" data-testid="import-file" onChange={(e) => { const f = e.target.files?.[0]; if (!f) return; setFileName(f.name); void f.text().then(setData); }} /></Button>
            <Typography level="body-sm">{fileName || "No file chosen"}</Typography>
            <Checkbox label="First row is a header" checked={header} onChange={(e) => setHeader(e.target.checked)} />
            {table && <Checkbox label={`Add to "${table.name}" instead of a new table`} checked={append} onChange={(e) => setAppend(e.target.checked)} />}
          </Box>
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          {pv && (
            <>
              <Typography level="body-sm" sx={{ mb: 1 }}>{pv.rows.toLocaleString()} rows. Check what each column becomes.</Typography>
              <Box sx={{ overflow: "auto", maxHeight: 300 }}>
                {cols.map((c, i) => (
                  <Box key={c.index} sx={{ display: "flex", gap: 1, alignItems: "center", mb: 0.5 }}>
                    <Checkbox checked={!c.skip} onChange={(e) => setCols(cols.map((x, j) => (j === i ? { ...x, skip: !e.target.checked } : x)))} slotProps={{ input: { "aria-label": `Import column ${c.name}` } }} />
                    {append ? (
                      <Select size="sm" sx={{ width: 220 }} value={c.field_id ?? null} placeholder={c.name} onChange={(_, v) => setCols(cols.map((x, j) => (j === i ? { ...x, field_id: v ?? undefined, skip: !v } : x)))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                        {(table?.fields ?? []).filter((f) => !isComputed(f.type) && f.type !== "link" && f.type !== "attachment").map((f) => <Option key={f.id} value={f.id}>{f.name}</Option>)}
                      </Select>
                    ) : (
                      <>
                        <Input size="sm" sx={{ width: 200 }} value={c.name} onChange={(e) => setCols(cols.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
                        <Select size="sm" sx={{ width: 180 }} value={c.type} onChange={(_, v) => v && setCols(cols.map((x, j) => (j === i ? { ...x, type: v } : x)))} disabled={i === 0} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                          {(["text", "long_text", "number", "currency", "percent", "date", "checkbox", "single_select", "multi_select", "email", "url", "phone", "rating", "duration", "barcode"] as FieldType[]).map((t) => <Option key={t} value={t}>{TYPE_LABEL[t]}</Option>)}
                        </Select>
                      </>
                    )}
                    <Typography level="body-xs" noWrap sx={{ flex: 1 }}>{pv.sample.map((r) => r[c.index]).filter(Boolean).slice(0, 3).join(" · ")}</Typography>
                  </Box>
                ))}
              </Box>
            </>
          )}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1.5 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button disabled={!pv} loading={busy} onClick={() => void run()} data-testid="import-run">Import {pv ? pv.rows.toLocaleString() + " records" : ""}</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

export function ShareDialog({ base, onClose, onSaved }: { base: Base; onClose: () => void; onSaved: () => void }) {
  const [people, setPeople] = useState<Person[]>([]);
  const [members, setMembers] = useState<Member[]>(base.members ?? []);
  const [access, setAccess] = useState(base.org_access);
  const [add, setAdd] = useState<string | null>(null);
  const [addName, setAddName] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => { records.people().then(setPeople).catch(() => undefined); }, []);
  const name = useMemo(() => new Map(people.map((p) => [p.id, p.name])), [people]);
  const save = async () => {
    try {
      await records.setMembers(base.id, members);
      await records.updateBase(base.id, { name: base.name, description: base.description, color: base.color, org_access: access });
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "95vw" }} role="dialog" aria-label="Share base">
        <ModalClose />
        <DialogTitle>Share “{base.name}”</DialogTitle>
        <DialogContent>
          <FormControl sx={{ mb: 1.5 }}><FormLabel>Everyone in the organisation</FormLabel>
            <Select value={access} onChange={(_, v) => v && setAccess(v as Base["org_access"])} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="none">No access</Option><Option value="view">Can view</Option><Option value="edit">Can edit</Option></Select>
          </FormControl>
          <FormLabel>People</FormLabel>
          {members.map((m) => (
            <Box key={m.user_id} sx={{ display: "flex", gap: 1, alignItems: "center", mb: 0.5 }}>
              <Typography sx={{ flex: 1 }}>{m.name ?? name.get(m.user_id) ?? m.user_id}</Typography>
              <Select size="sm" value={m.role} onChange={(_, v) => v && setMembers(members.map((x) => (x.user_id === m.user_id ? { ...x, role: v as Member["role"] } : x)))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="viewer">Viewer</Option><Option value="editor">Editor</Option><Option value="admin">Admin</Option></Select>
              <IconButton size="sm" variant="plain" aria-label="Remove person" onClick={() => setMembers(members.filter((x) => x.user_id !== m.user_id))}>✕</IconButton>
            </Box>
          ))}
          <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
            <PersonSelect value={add ?? ""} known={people} onChange={(id, item) => { setAdd(id || null); setAddName(item?.label ?? ""); }} placeholder="Add a person" ariaLabel="Add a person" />
            <Button size="sm" disabled={!add || add === base.owner_id || members.some((m) => m.user_id === add)} onClick={() => { if (add) { setMembers([...members, { user_id: add, role: "viewer", name: name.get(add) ?? addName }]); setAdd(null); } }}>Add</Button>
          </Box>
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}><Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button><Button onClick={() => void save()}>Save</Button></Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

export { Pill };
