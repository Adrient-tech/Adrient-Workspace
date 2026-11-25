import { SalesSettingsPanel } from "./SalesSettingsPanel";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  Chip,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import {
  createItem,
  createRecurring,
  deleteItem,
  deleteRecurring,
  getReport,
  listClients,
  listDocuments,
  listRecurring,
  runRecurring,
  saveSettings,
  updateItem,
  updateRecurring,
} from "./api";
import { ACCENT, inputFromDoc, money, parseItemsCSV, todayISO } from "./logic";
import type { ClientRow, InvDocument, Item, Recurring, Report, Settings } from "./types";
import { Empty, ErrorAlert, Stat, rightCell, useErr } from "./ui";

// ---- Price list ----

export function ItemsView({ items, settings, onChanged }: { items: Item[]; settings: Settings; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<Item> | null>(null);
  const [note, setNote] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const { err, run } = useErr();
  const save = async () => {
    if (!edit) return;
    const ok = await run(() => (edit.id ? updateItem(edit.id, edit) : createItem(edit)));
    if (ok) {
      setEdit(null);
      onChanged();
    }
  };
  const importCSV = async (f: File) => {
    const rows = parseItemsCSV(await f.text());
    let n = 0;
    for (const r of rows) if (await run(() => createItem(r))) n++;
    setNote(rows.length ? `Imported ${n} of ${rows.length} items.` : "No items found. Expected a header row with name, unit, price, tax.");
    onChanged();
  };
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
        <Typography level="body-sm" sx={{ flex: 1, alignSelf: "center" }}>
          Saved items you can add to any invoice. Products from Inventory are picked directly in the editor.
        </Typography>
        <Button variant="outlined" onClick={() => fileRef.current?.click()}>
          Import CSV
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept=".csv,text/csv"
          hidden
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) importCSV(f);
          }}
        />
        <Button onClick={() => setEdit({ name: "", description: "", unit: "", price: 0, tax_rate: 0 })} data-testid="invc-item-new" sx={{ bgcolor: ACCENT }}>
          New item
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {note && <Typography level="body-sm">{note}</Typography>}
      {items.length === 0 ? (
        <Empty>No saved items yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow>
            <thead>
              <tr>
                <th>Name</th>
                <th>Unit</th>
                <th style={rightCell}>Price</th>
                <th style={rightCell}>Tax %</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((i) => (
                <tr key={i.id} data-testid="invc-item-row">
                  <td>
                    {i.name}
                    <Typography level="body-xs">{i.description}</Typography>
                  </td>
                  <td>{i.unit}</td>
                  <td style={rightCell}>{money(i.price, settings.default_currency)}</td>
                  <td style={rightCell}>{i.tax_rate}</td>
                  <td style={rightCell}>
                    <Button size="sm" variant="plain" onClick={() => setEdit(i)}>
                      Edit
                    </Button>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteItem(i.id)); onChanged(); }}>
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: 420 }}>
            <ModalClose />
            <Typography level="title-lg">{edit.id ? "Edit item" : "New item"}</Typography>
            <ErrorAlert err={err} />
            <FormControl>
              <FormLabel>Name</FormLabel>
              <Input value={edit.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "invc-item-name" } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Description</FormLabel>
              <Input value={edit.description ?? ""} onChange={(e) => setEdit({ ...edit, description: e.target.value })} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Unit</FormLabel>
                <Select value={edit.unit || "-"} onChange={(_, v) => setEdit({ ...edit, unit: v === "-" ? "" : (v as string) })}>
                  <Option value="-">-</Option>
                  {settings.units.map((u) => (
                    <Option key={u} value={u}>
                      {u}
                    </Option>
                  ))}
                </Select>
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Price</FormLabel>
                <Input type="number" value={edit.price ?? 0} onChange={(e) => setEdit({ ...edit, price: Number(e.target.value) })} slotProps={{ input: { "data-testid": "invc-item-price" } }} />
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Tax %</FormLabel>
                <Input type="number" value={edit.tax_rate ?? 0} onChange={(e) => setEdit({ ...edit, tax_rate: Number(e.target.value) })} />
              </FormControl>
            </Box>
            <Button onClick={save} data-testid="invc-item-save">
              Save
            </Button>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---- Recurring ----

export function RecurringView({ settings, version, onChanged }: { settings: Settings; version: number; onChanged: () => void }) {
  const [list, setList] = useState<Recurring[] | null>(null);
  const [invoices, setInvoices] = useState<InvDocument[]>([]);
  const [draft, setDraft] = useState<null | { name: string; source: string; frequency: Recurring["frequency"]; next_run: string; end_date: string; auto_send: boolean }>(null);
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const load = useCallback(() => {
    listRecurring().then(setList).catch(() => setList([]));
  }, []);
  useEffect(load, [load, version]);

  const create = async () => {
    if (!draft) return;
    const src = invoices.find((d) => d.id === draft.source);
    if (!src) return;
    const r = await run(() =>
      createRecurring({ name: draft.name || src.client_name, frequency: draft.frequency, next_run: draft.next_run, end_date: draft.end_date, auto_send: draft.auto_send, active: true, template: inputFromDoc(src) }),
    );
    if (r) {
      setDraft(null);
      load();
    }
  };
  const runNow = async () => {
    const made = await run(() => runRecurring());
    if (made) {
      setNote(made.length ? `Generated ${made.map((d) => d.number).join(", ")}.` : "Nothing is due.");
      load();
      onChanged();
    }
  };
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
        <Typography level="body-sm" sx={{ flex: 1, alignSelf: "center" }}>
          Templates that create an invoice every week, month, quarter or year. Due ones are created when you open this module or press Run now.
        </Typography>
        <Button variant="outlined" onClick={runNow} data-testid="invc-recurring-run">
          Run now
        </Button>
        <Button
          onClick={async () => {
            setInvoices(await listDocuments({ kind: "invoice", limit: 500 }));
            setDraft({ name: "", source: "", frequency: "monthly", next_run: todayISO(), end_date: "", auto_send: false });
          }}
          data-testid="invc-recurring-new"
          sx={{ bgcolor: ACCENT }}
        >
          New recurring
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {note && <Typography level="body-sm" data-testid="invc-note">{note}</Typography>}
      {list && list.length === 0 ? (
        <Empty>No recurring invoices. Create one from an existing invoice.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Every</th>
                <th>Next run</th>
                <th>Ends</th>
                <th>Created</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(list ?? []).map((r) => (
                <tr key={r.id} data-testid="invc-recurring-row">
                  <td>
                    {r.name} {!r.active && <Chip size="sm">paused</Chip>} {r.auto_send && <Chip size="sm" color="primary">auto-send</Chip>}
                  </td>
                  <td>{r.frequency}</td>
                  <td>{r.next_run}</td>
                  <td>{r.end_date || "-"}</td>
                  <td>{r.generated}</td>
                  <td style={rightCell}>
                    <Button size="sm" variant="plain" onClick={async () => { await run(() => updateRecurring(r.id, { ...r, active: !r.active })); load(); }}>
                      {r.active ? "Pause" : "Resume"}
                    </Button>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteRecurring(r.id)); load(); }}>
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {draft && (
        <Modal open onClose={() => setDraft(null)}>
          <ModalDialog sx={{ width: 440 }}>
            <ModalClose />
            <Typography level="title-lg">New recurring invoice</Typography>
            <ErrorAlert err={err} />
            <FormControl>
              <FormLabel>Copy lines and client from</FormLabel>
              <Select placeholder="Choose an invoice" value={draft.source || null} onChange={(_, v) => setDraft({ ...draft, source: (v as string) ?? "" })} slotProps={{ button: { "data-testid": "invc-recurring-source" } }}>
                {invoices.map((d) => (
                  <Option key={d.id} value={d.id}>
                    {d.number} · {d.client_name} · {money(d.total, d.currency)}
                  </Option>
                ))}
              </Select>
            </FormControl>
            <FormControl>
              <FormLabel>Name</FormLabel>
              <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Repeat</FormLabel>
                <Select value={draft.frequency} onChange={(_, v) => v && setDraft({ ...draft, frequency: v as Recurring["frequency"] })}>
                  {["weekly", "monthly", "quarterly", "yearly"].map((f) => (
                    <Option key={f} value={f}>
                      {f}
                    </Option>
                  ))}
                </Select>
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>First run</FormLabel>
                <Input type="date" value={draft.next_run} onChange={(e) => setDraft({ ...draft, next_run: e.target.value })} />
              </FormControl>
            </Box>
            <FormControl>
              <FormLabel>End date (optional)</FormLabel>
              <Input type="date" value={draft.end_date} onChange={(e) => setDraft({ ...draft, end_date: e.target.value })} />
            </FormControl>
            <Checkbox label="Mark as sent and email the client automatically" checked={draft.auto_send} onChange={(e) => setDraft({ ...draft, auto_send: e.target.checked })} />
            <Typography level="body-xs">Invoices are created due in {settings.due_days} days.</Typography>
            <Button onClick={create} disabled={!draft.source} data-testid="invc-recurring-save">
              Create
            </Button>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---- Clients ----

export function ClientsView({ settings, version, onOpenClient }: { settings: Settings; version: number; onOpenClient: (c: ClientRow) => void }) {
  const [rows, setRows] = useState<ClientRow[] | null>(null);
  useEffect(() => {
    listClients().then(setRows).catch(() => setRows([]));
  }, [version]);
  const m = (n: number) => money(n, settings.default_currency);
  return rows && rows.length === 0 ? (
    <Empty>Clients appear here once you send an invoice. Clients are your Contacts.</Empty>
  ) : (
    <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
      <Table hoverRow data-testid="invc-clients">
        <thead>
          <tr>
            <th>Client</th>
            <th style={rightCell}>Invoices</th>
            <th style={rightCell}>Billed</th>
            <th style={rightCell}>Paid</th>
            <th style={rightCell}>Outstanding</th>
          </tr>
        </thead>
        <tbody>
          {(rows ?? []).map((c, i) => (
            <tr key={i} style={{ cursor: "pointer" }} onClick={() => onOpenClient(c)}>
              <td>
                {c.client_name || "(no name)"}
                <Typography level="body-xs">{c.client_email}</Typography>
              </td>
              <td style={rightCell}>{c.invoices}</td>
              <td style={rightCell}>{m(c.billed)}</td>
              <td style={rightCell}>{m(c.paid)}</td>
              <td style={{ ...rightCell, color: c.outstanding > 0 ? "#B91C1C" : undefined }}>{m(c.outstanding)}</td>
            </tr>
          ))}
        </tbody>
      </Table>
    </Sheet>
  );
}

// ---- Reports ----

export function ReportsView({ version }: { version: number }) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [rep, setRep] = useState<Report | null>(null);
  useEffect(() => {
    getReport(from, to).then(setRep).catch(() => setRep(null));
  }, [from, to, version]);
  if (!rep) return <Empty>Loading report…</Empty>;
  const m = (n: number) => money(n, rep.currency);
  const Block = ({ title, rows, native }: { title: string; rows: Report["by_month"]; native?: boolean }) => (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", flex: "1 1 280px" }}>
      <Typography level="title-sm" sx={{ mb: 1 }}>
        {title}
      </Typography>
      {rows.length === 0 && <Typography level="body-sm">No data.</Typography>}
      {rows.map((r) => (
        <Box key={r.label} sx={{ display: "flex", justifyContent: "space-between", py: 0.25, fontSize: 14 }}>
          <span>
            {r.label} <Typography level="body-xs" component="span">({r.count})</Typography>
          </span>
          <span>{native ? money(r.amount, r.label) : m(r.amount)}</span>
        </Box>
      ))}
    </Sheet>
  );
  return (
    <Box data-testid="invc-report">
      <Box sx={{ display: "flex", gap: 1, mb: 2, alignItems: "center", flexWrap: "wrap" }}>
        <Typography level="body-sm">Invoiced between</Typography>
        <Input type="date" size="sm" value={from} onChange={(e) => setFrom(e.target.value)} />
        <Input type="date" size="sm" value={to} onChange={(e) => setTo(e.target.value)} />
        <Typography level="body-xs">All amounts converted to {rep.currency} using the rates in Settings.</Typography>
      </Box>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Stat label="Invoiced" value={m(rep.invoiced)} testid="invc-rep-invoiced" />
        <Stat label="Collected" value={m(rep.collected)} color="#15803D" testid="invc-rep-collected" />
        <Stat label="Outstanding (today)" value={m(rep.outstanding)} />
        <Stat label="Overdue" value={m(rep.overdue)} color={rep.overdue > 0 ? "#B91C1C" : undefined} />
        <Stat label="Written off" value={m(rep.written_off)} />
      </Box>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
        <Block title="Aging of open balances" rows={rep.aging} />
        <Block title="By month" rows={rep.by_month} />
        <Block title="Top clients" rows={rep.by_client} />
        <Block title="By status" rows={rep.by_status} />
        <Block title="By currency (native)" rows={rep.by_currency} native />
      </Box>
    </Box>
  );
}

// ---- Settings ----

export function SettingsView({ settings, onSaved }: { settings: Settings; onSaved: () => void }) {
  const [s, setS] = useState(settings);
  const [rates, setRates] = useState(Object.entries(settings.rates).map(([k, v]) => ({ code: k, rate: v })));
  const [units, setUnits] = useState(settings.units.join(", "));
  const [saved, setSaved] = useState(false);
  const { err, run } = useErr();
  const set = (p: Partial<Settings>) => setS({ ...s, ...p });
  const save = async () => {
    const r: Record<string, number> = {};
    rates.forEach((x) => x.code.trim() && x.rate > 0 && (r[x.code.trim().toUpperCase()] = x.rate));
    const out = await run(() => saveSettings({ ...s, rates: r, units: units.split(",").map((u) => u.trim()).filter(Boolean) }));
    if (out) {
      setSaved(true);
      onSaved();
      setTimeout(() => setSaved(false), 2500);
    }
  };
  const field = (label: string, key: keyof Settings, props: object = {}) => (
    <FormControl>
      <FormLabel>{label}</FormLabel>
      <Input value={String(s[key] ?? "")} onChange={(e) => set({ [key]: e.target.value } as Partial<Settings>)} {...props} />
    </FormControl>
  );
  const area = (label: string, key: keyof Settings) => (
    <FormControl>
      <FormLabel>{label}</FormLabel>
      <Textarea minRows={2} value={String(s[key] ?? "")} onChange={(e) => set({ [key]: e.target.value } as Partial<Settings>)} />
    </FormControl>
  );
  return (
    <Box sx={{ maxWidth: 760, display: "grid", gap: 1.5 }} data-testid="invc-settings">
      <ErrorAlert err={err} />
      <Typography level="title-md">Business details (shown on documents)</Typography>
      {field("Business name", "business_name", { slotProps: { input: { "data-testid": "invc-set-name" } } })}
      {area("Address", "address")}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
        {field("Tax ID (e.g. GSTIN)", "tax_id")}
        {field("Email", "email")}
        {field("Phone", "phone")}
        {field("Logo URL", "logo_url")}
      </Box>
      {area("Payment details (bank account, UPI id)", "payment_info")}
      <Typography level="title-md" sx={{ mt: 1 }}>
        Numbering and defaults
      </Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(4, 1fr)" }, gap: 1.5 }}>
        {field("Invoice prefix", "invoice_prefix")}
        <FormControl>
          <FormLabel>Next invoice no.</FormLabel>
          <Input type="number" value={s.next_invoice} onChange={(e) => set({ next_invoice: Number(e.target.value) })} />
        </FormControl>
        {field("Estimate prefix", "estimate_prefix")}
        <FormControl>
          <FormLabel>Next estimate no.</FormLabel>
          <Input type="number" value={s.next_estimate} onChange={(e) => set({ next_estimate: Number(e.target.value) })} />
        </FormControl>
        {field("Default currency", "default_currency")}
        <FormControl>
          <FormLabel>Payment terms (days)</FormLabel>
          <Input type="number" value={s.due_days} onChange={(e) => set({ due_days: Number(e.target.value) })} />
        </FormControl>
      </Box>
      <FormControl>
        <FormLabel>Units (comma separated)</FormLabel>
        <Input value={units} onChange={(e) => setUnits(e.target.value)} />
      </FormControl>
      {area("Default notes", "default_notes")}
      {area("Default terms", "default_terms")}
      {area("Default tax note", "tax_note")}
      <Typography level="title-md" sx={{ mt: 1 }}>
        Exchange rates
      </Typography>
      <Typography level="body-xs">How many {s.default_currency || "INR"} one unit of another currency is worth. Used for totals in Reports and Clients.</Typography>
      {rates.map((r, i) => (
        <Box key={i} sx={{ display: "flex", gap: 1 }}>
          <Input placeholder="USD" value={r.code} sx={{ width: 100 }} onChange={(e) => setRates(rates.map((x, j) => (j === i ? { ...x, code: e.target.value.toUpperCase().slice(0, 3) } : x)))} />
          <Input type="number" placeholder="83" value={r.rate} sx={{ width: 140 }} onChange={(e) => setRates(rates.map((x, j) => (j === i ? { ...x, rate: Number(e.target.value) } : x)))} />
          <Button variant="plain" color="danger" onClick={() => setRates(rates.filter((_, j) => j !== i))}>
            Remove
          </Button>
        </Box>
      ))}
      <Box>
        <Button variant="outlined" size="sm" onClick={() => setRates([...rates, { code: "", rate: 0 }])}>
          Add rate
        </Button>
      </Box>
      <SalesSettingsPanel sales={s.sales} onChange={(sales) => setS((x) => ({ ...x, sales }))} />
      <Typography level="title-md" sx={{ mt: 1 }}>
        Inventory
      </Typography>
      <Checkbox label="Deduct stock from Inventory when an invoice is first sent (lines picked from Inventory only)" checked={s.deduct_stock} onChange={(e) => set({ deduct_stock: e.target.checked })} />
      <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
        <Button onClick={save} data-testid="invc-set-save" sx={{ bgcolor: ACCENT, "&:hover": { bgcolor: "#9A3412" } }}>
          Save settings
        </Button>
        {saved && <Typography level="body-sm" color="success">Saved</Typography>}
      </Box>
    </Box>
  );
}
