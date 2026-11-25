import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Tab,
  TabList,
  Tabs,
  Textarea,
  Typography,
} from "@mui/joy";
import { api } from "./stockApi";
import type { Loc, Po, PoLine, PurchaseSettings } from "./stockApi";
import { money, qtyText } from "./logic";
import { PO_LABEL, PO_TONE, lineTotals, poActions, receiptLimit } from "./stockLogic";
import type { PoState } from "./stockLogic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, dateOnly, right, useAction, useLoad } from "./stockUi";
import type { InvSupplier } from "./types";
import type { TabProps } from "./PurchasingMore";
import { BillsTab, PerformanceTab, PricesTab, RequestsTab, ReceiptsTab, ReorderTab, ReturnsTab, SettingsTab } from "./PurchasingMore";

type Tabname = "orders" | "requests" | "receipts" | "bills" | "returns" | "prices" | "reorder" | "performance" | "settings";

const TABS: { id: Tabname; label: string }[] = [
  { id: "orders", label: "Purchase orders" },
  { id: "requests", label: "Requests" },
  { id: "receipts", label: "Goods receipts" },
  { id: "bills", label: "Supplier bills" },
  { id: "returns", label: "Returns" },
  { id: "prices", label: "Price lists" },
  { id: "reorder", label: "Reorder rules" },
  { id: "performance", label: "Supplier performance" },
  { id: "settings", label: "Settings" },
];

export function PurchasingView({ suppliers, version, onChanged, initialTab }: { suppliers: InvSupplier[]; version: number; onChanged: () => void; initialTab?: Tabname }) {
  const [tab, setTab] = useState<Tabname>(initialTab ?? "orders");
  const props = { suppliers, version, onChanged, goto: (t: Tabname) => setTab(t) };
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(v as Tabname)} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2, flexWrap: "wrap" }}>
          {TABS.map((t) => (
            <Tab key={t.id} value={t.id} data-testid={`buy-tab-${t.id}`}>{t.label}</Tab>
          ))}
        </TabList>
      </Tabs>
      {tab === "orders" && <Orders {...props} />}
      {tab === "requests" && <RequestsTab {...props} />}
      {tab === "receipts" && <ReceiptsTab {...props} />}
      {tab === "bills" && <BillsTab {...props} />}
      {tab === "returns" && <ReturnsTab {...props} />}
      {tab === "prices" && <PricesTab {...props} />}
      {tab === "reorder" && <ReorderTab {...props} />}
      {tab === "performance" && <PerformanceTab {...props} />}
      {tab === "settings" && <SettingsTab {...props} />}
    </Box>
  );
}

// ------------------------------------------------------------------ orders

function Orders({ suppliers, version, onChanged, goto }: TabProps) {
  const [state, setState] = useState("");
  const [q, setQ] = useState("");
  const list = useLoad(() => api.orders({ state, q }), [state, q, version]);
  const [editing, setEditing] = useState<Po | "new" | null>(null);
  const [open, setOpen] = useState<string>("");
  const reload = () => { onChanged(); void list.reload(); };
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
            <Input size="sm" placeholder="Search number, supplier" value={q} onChange={(e) => setQ(e.target.value)} data-testid="po-search" />
            <Select size="sm" value={state} onChange={(_, v) => setState(v ?? "")} sx={{ minWidth: 150 }} data-testid="po-filter">
              <Option value="">All states</Option>
              {(Object.keys(PO_LABEL) as PoState[]).map((s) => <Option key={s} value={s}>{PO_LABEL[s]}</Option>)}
            </Select>
            <Button size="sm" onClick={() => setEditing("new")} data-testid="po-new">New purchase order</Button>
          </Box>
        }
      >
        Purchase orders
      </Title>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No purchase orders yet. Create one, or let reorder rules draft them.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="po-table">
            <thead>
              <tr>
                <th>Number</th>
                <th>Supplier</th>
                <th>Date</th>
                <th>Expected</th>
                <th>Status</th>
                <th style={right}>Received</th>
                <th style={right}>Total</th>
              </tr>
            </thead>
            <tbody>
              {list.data.map((o) => {
                const ordered = o.lines.reduce((s, l) => s + l.qty, 0);
                const got = o.lines.reduce((s, l) => s + l.qty_received, 0);
                return (
                  <tr key={o.id} onClick={() => setOpen(o.id)} style={{ cursor: "pointer" }} data-testid={`po-row-${o.number}`}>
                    <td>{o.number}</td>
                    <td>{o.supplier_name}</td>
                    <td>{dateOnly(o.order_date)}</td>
                    <td>{o.promised_date ? `${dateOnly(o.promised_date)} (promised)` : dateOnly(o.expected_date)}</td>
                    <td><Chip size="sm" variant="soft" color={PO_TONE[o.state]}>{PO_LABEL[o.state]}</Chip></td>
                    <td style={right}>{qtyText(got)} / {qtyText(ordered)}</td>
                    <td style={right}>{money(o.total)}</td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Sheet>
      )}
      {editing && <OrderEditor suppliers={suppliers} value={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSaved={(id) => { setEditing(null); reload(); setOpen(id); }} />}
      {open && <OrderDetail id={open} onClose={() => setOpen("")} onEdit={(p) => { setOpen(""); setEditing(p); }} onChanged={reload} goto={goto} />}
    </Box>
  );
}

interface EditLine { product_id: string; qty: string; unit_cost: string; tax_pct: string }

function OrderEditor({ suppliers, value, onClose, onSaved }: { suppliers: InvSupplier[]; value: Po | null; onClose: () => void; onSaved: (id: string) => void }) {
  const locs = useLoad(() => api.locations(), []);
  const [supplier, setSupplier] = useState(value?.supplier_id ?? "");
  const [expected, setExpected] = useState(value?.expected_date ?? "");
  const [shipTo, setShipTo] = useState(value?.ship_to_loc_id ?? "");
  const [ref, setRef] = useState(value?.supplier_ref ?? "");
  const [notes, setNotes] = useState(value?.notes ?? "");
  const [lines, setLines] = useState<EditLine[]>(
    value && value.lines.length > 0
      ? value.lines.map((l) => ({ product_id: l.product_id, qty: String(l.qty), unit_cost: String(l.unit_cost), tax_pct: String(l.tax_pct) }))
      : [{ product_id: "", qty: "1", unit_cost: "", tax_pct: "0" }],
  );
  const act = useAction();
  const set = (i: number, p: Partial<EditLine>) => setLines((ls) => ls.map((l, j) => (j === i ? { ...l, ...p } : l)));
  const totals = lineTotals(lines.map((l) => ({ qty: Number(l.qty) || 0, unit_cost: Number(l.unit_cost) || 0, tax_pct: Number(l.tax_pct) || 0 })));
  const save = async () => {
    const body = {
      supplier_id: supplier,
      expected_date: expected,
      ship_to_loc_id: shipTo,
      supplier_ref: ref,
      notes,
      lines: lines.filter((l) => l.product_id).map((l) => ({ product_id: l.product_id, qty: Number(l.qty), unit_cost: l.unit_cost === "" ? null : Number(l.unit_cost), tax_pct: Number(l.tax_pct) || 0 })),
    };
    const r = await act.run(() => (value ? api.updateOrder(value.id, body) : api.createOrder(body)));
    if (r) onSaved(r.id);
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 780 }, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">{value ? `Edit ${value.number}` : "New purchase order"}</Typography>
        <ErrorBar error={act.error} />
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
          <FormControl>
            <FormLabel>Supplier</FormLabel>
            <Select value={supplier} onChange={(_, v) => setSupplier(v ?? "")} placeholder="Choose supplier" data-testid="po-supplier">
              {suppliers.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Deliver to</FormLabel>
            <Select value={shipTo} onChange={(_, v) => setShipTo(v ?? "")} placeholder="Default location" data-testid="po-shipto">
              <Option value="">Default location</Option>
              {(locs.data ?? []).map((l: Loc) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Expected delivery</FormLabel>
            <Input type="date" value={expected} onChange={(e) => setExpected(e.target.value)} data-testid="po-expected" />
          </FormControl>
          <FormControl>
            <FormLabel>Supplier reference</FormLabel>
            <Input value={ref} onChange={(e) => setRef(e.target.value)} />
          </FormControl>
        </Box>
        <Typography level="title-sm" sx={{ mt: 1 }}>Items</Typography>
        <Typography level="body-xs">Leave the cost empty to use the supplier&apos;s price list for that quantity.</Typography>
        {lines.map((l, i) => (
          <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
            <Box sx={{ flex: 1, minWidth: 200 }}>
              <ProductPicker value={l.product_id} testId={`po-line-product-${i}`} onChange={(id) => set(i, { product_id: id })} />
            </Box>
            <Input size="sm" type="number" sx={{ width: 90 }} placeholder="Qty" value={l.qty} onChange={(e) => set(i, { qty: e.target.value })} slotProps={{ input: { min: 0, step: "any", "data-testid": `po-line-qty-${i}` } }} />
            <Input size="sm" type="number" sx={{ width: 110 }} placeholder="Cost (auto)" value={l.unit_cost} onChange={(e) => set(i, { unit_cost: e.target.value })} slotProps={{ input: { min: 0, step: "any", "data-testid": `po-line-cost-${i}` } }} />
            <Input size="sm" type="number" sx={{ width: 80 }} placeholder="Tax %" value={l.tax_pct} onChange={(e) => set(i, { tax_pct: e.target.value })} slotProps={{ input: { min: 0, max: 100, step: "any", "data-testid": `po-line-tax-${i}` } }} />
            <IconButton size="sm" variant="plain" aria-label="Remove line" disabled={lines.length === 1} onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}>✕</IconButton>
          </Box>
        ))}
        <Button size="sm" variant="soft" sx={{ alignSelf: "flex-start" }} onClick={() => setLines((ls) => [...ls, { product_id: "", qty: "1", unit_cost: "", tax_pct: "0" }])} data-testid="po-add-line">Add item</Button>
        <FormControl>
          <FormLabel>Notes to the supplier</FormLabel>
          <Textarea minRows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </FormControl>
        <Typography level="body-sm" sx={{ textAlign: "right" }}>
          Entered prices total {money(totals.subtotal)} + tax {money(totals.tax)} = {money(totals.total)}
        </Typography>
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" onClick={onClose}>Cancel</Button>
          <Button loading={act.busy} onClick={save} data-testid="po-save">Save draft</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

// ------------------------------------------------------------------ detail

function OrderDetail({ id, onClose, onEdit, onChanged, goto }: { id: string; onClose: () => void; onEdit: (p: Po) => void; onChanged: () => void; goto: (t: Tabname) => void }) {
  const po = useLoad(() => api.order(id), [id]);
  const settings = useLoad(() => api.settings(), []);
  const act = useAction();
  const [dlg, setDlg] = useState<"" | "send" | "ack" | "receive">("");
  const o = po.data;
  const refresh = () => { onChanged(); void po.reload(); };
  const doAction = async (a: "approve" | "cancel" | "close") => {
    if (a === "cancel" && !window.confirm("Cancel this order?")) return;
    if (await act.run(() => api.orderAction(id, a))) refresh();
  };
  const toBill = async () => {
    const r = await act.run(async () => {
      const d = await api.billDraft(id);
      if (d.lines.length === 0) throw new Error("Everything received so far is already billed");
      return api.createBill(d);
    });
    if (r) { onChanged(); onClose(); goto("bills"); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 820 }, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <ModalClose />
        <Loading show={po.loading && !o} />
        <ErrorBar error={po.error || act.error} />
        {o && (
          <>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
              <Typography level="title-lg" data-testid="po-detail-number">{o.number}</Typography>
              <Chip variant="soft" color={PO_TONE[o.state]} data-testid="po-detail-state">{PO_LABEL[o.state]}</Chip>
              <Typography level="body-sm">{o.supplier_name}</Typography>
            </Box>
            <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
              <Typography level="body-sm">Ordered {dateOnly(o.order_date)}</Typography>
              {o.expected_date && <Typography level="body-sm">Expected {dateOnly(o.expected_date)}</Typography>}
              {o.ship_to_name && <Typography level="body-sm">Deliver to {o.ship_to_name}</Typography>}
              {o.sent_at && <Typography level="body-sm">Sent {dateOnly(o.sent_at)}{o.sent_to ? ` to ${o.sent_to}` : ""}</Typography>}
            </Box>
            {o.ack_at && (
              <Alert color="success" variant="soft">
                Supplier acknowledged{o.ack_ref ? ` (${o.ack_ref})` : ""}{o.promised_date ? `, promised ${dateOnly(o.promised_date)}` : ""}.{o.ack_note ? ` ${o.ack_note}` : ""}
              </Alert>
            )}
            <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
              <Table size="sm" data-testid="po-detail-lines">
                <thead>
                  <tr>
                    <th>Item</th>
                    <th style={right}>Ordered</th>
                    <th style={right}>Received</th>
                    <th style={right}>Billed</th>
                    <th style={right}>Cost</th>
                    <th style={right}>Tax %</th>
                    <th style={right}>Amount</th>
                  </tr>
                </thead>
                <tbody>
                  {o.lines.map((l) => (
                    <tr key={l.id}>
                      <td>{l.product_name} <Typography level="body-xs" component="span">{l.sku}</Typography></td>
                      <td style={right}>{qtyText(l.qty)} {l.unit}</td>
                      <td style={right}>{qtyText(l.qty_received)}{l.qty_returned > 0 ? ` (−${qtyText(l.qty_returned)} returned)` : ""}</td>
                      <td style={right}>{qtyText(l.qty_billed)}</td>
                      <td style={right}>{money(l.unit_cost)}</td>
                      <td style={right}>{qtyText(l.tax_pct)}</td>
                      <td style={right}>{money(l.line_total)}</td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Sheet>
            <Typography level="body-sm" sx={{ textAlign: "right" }}>
              Subtotal {money(o.subtotal)} · Tax {money(o.tax_total)} · <b data-testid="po-detail-total">Total {o.currency} {money(o.total)}</b>
            </Typography>
            {o.notes && <Typography level="body-sm">Notes: {o.notes}</Typography>}
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", justifyContent: "flex-end" }}>
              <Button size="sm" variant="outlined" component="a" href={api.pdfUrl(o.id)} target="_blank" rel="noreferrer" data-testid="po-pdf">PDF</Button>
              {poActions(o.state, o.lines.reduce((s, l) => s + l.qty_received, 0)).map((a) => {
                switch (a) {
                  case "edit":
                    return <Button key={a} size="sm" variant="outlined" onClick={() => onEdit(o)} data-testid="po-edit">Edit</Button>;
                  case "approve":
                    return <Button key={a} size="sm" loading={act.busy} onClick={() => doAction("approve")} data-testid="po-approve">Approve</Button>;
                  case "send":
                    return <Button key={a} size="sm" onClick={() => setDlg("send")} data-testid="po-send">{o.state === "sent" ? "Send again" : "Send to supplier"}</Button>;
                  case "acknowledge":
                    return <Button key={a} size="sm" variant="outlined" onClick={() => setDlg("ack")} data-testid="po-ack">Record acknowledgement</Button>;
                  case "receive":
                    return <Button key={a} size="sm" onClick={() => setDlg("receive")} data-testid="po-receive">Receive goods</Button>;
                  case "bill":
                    return <Button key={a} size="sm" variant="outlined" loading={act.busy} onClick={toBill} data-testid="po-bill">Create bill from receipts</Button>;
                  case "close":
                    return <Button key={a} size="sm" variant="outlined" onClick={() => doAction("close")} data-testid="po-close">Close order</Button>;
                  case "cancel":
                    return <Button key={a} size="sm" variant="plain" color="danger" onClick={() => doAction("cancel")} data-testid="po-cancel">Cancel</Button>;
                }
              })}
            </Box>
            {dlg === "send" && <SendDialog po={o} onClose={() => setDlg("")} onDone={() => { setDlg(""); refresh(); }} />}
            {dlg === "ack" && <AckDialog po={o} onClose={() => setDlg("")} onDone={() => { setDlg(""); refresh(); }} />}
            {dlg === "receive" && <ReceiveDialog po={o} settings={settings.data} onClose={() => setDlg("")} onDone={() => { setDlg(""); refresh(); }} />}
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

function SendDialog({ po, onClose, onDone }: { po: Po; onClose: () => void; onDone: () => void }) {
  const [to, setTo] = useState(po.supplier_email);
  const [msg, setMsg] = useState("");
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340, maxWidth: 520 }}>
        <ModalClose />
        <Typography level="title-md">Send {po.number}</Typography>
        <Typography level="body-sm">The order goes as a PDF attachment.</Typography>
        <ErrorBar error={act.error} />
        <FormControl>
          <FormLabel>To</FormLabel>
          <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="supplier@example.com" data-testid="po-send-to" />
        </FormControl>
        <FormControl>
          <FormLabel>Message (optional)</FormLabel>
          <Textarea minRows={3} value={msg} onChange={(e) => setMsg(e.target.value)} placeholder="A short standard message is used when empty." />
        </FormControl>
        <Box sx={{ display: "flex", gap: 1, justifyContent: "space-between", flexWrap: "wrap" }}>
          <Button variant="outlined" loading={act.busy} onClick={async () => { if (await act.run(() => api.sendOrder(po.id, { to, mark_only: true }))) onDone(); }} data-testid="po-mark-sent">Mark as sent (I sent it myself)</Button>
          <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.sendOrder(po.id, { to, message: msg }))) onDone(); }} data-testid="po-send-mail">Email PDF</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function AckDialog({ po, onClose, onDone }: { po: Po; onClose: () => void; onDone: () => void }) {
  const [ref, setRef] = useState("");
  const [note, setNote] = useState("");
  const [date, setDate] = useState(po.expected_date);
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Supplier acknowledgement</Typography>
        <ErrorBar error={act.error} />
        <FormControl>
          <FormLabel>Their reference</FormLabel>
          <Input value={ref} onChange={(e) => setRef(e.target.value)} data-testid="ack-ref" />
        </FormControl>
        <FormControl>
          <FormLabel>Promised delivery date</FormLabel>
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} data-testid="ack-date" />
        </FormControl>
        <FormControl>
          <FormLabel>Note</FormLabel>
          <Input value={note} onChange={(e) => setNote(e.target.value)} />
        </FormControl>
        <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.acknowledge(po.id, { ref, note, promised_date: date }))) onDone(); }} data-testid="ack-save">Save</Button>
      </ModalDialog>
    </Modal>
  );
}

interface RecvRow { line: PoLine; qty: string; lot: string; expiry: string; serials: string }

function ReceiveDialog({ po, settings, onClose, onDone }: { po: Po; settings: PurchaseSettings | null; onClose: () => void; onDone: () => void }) {
  const locs = useLoad(() => api.locations(), []);
  const over = settings?.over_receipt_pct ?? 0;
  const [rows, setRows] = useState<RecvRow[]>(
    po.lines.filter((l) => l.qty_received - l.qty < 0 || l.qty_received < l.qty).map((l) => ({ line: l, qty: String(Math.max(0, Math.round((l.qty - l.qty_received) * 1000) / 1000)), lot: "", expiry: "", serials: "" })),
  );
  const [loc, setLoc] = useState(po.ship_to_loc_id);
  const [deliveryRef, setDeliveryRef] = useState("");
  const [landed, setLanded] = useState("");
  const [method, setMethod] = useState<"value" | "qty">("value");
  const [short, setShort] = useState(false);
  const act = useAction();
  const set = (i: number, p: Partial<RecvRow>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...p } : r)));
  const save = async () => {
    const lines = rows.filter((r) => Number(r.qty) > 0).map((r) => ({
      po_line_id: r.line.id,
      qty: Number(r.qty),
      lot_no: r.lot || undefined,
      expiry: r.expiry || undefined,
      serials: r.line.tracking === "serial" ? r.serials.split(/[\s,;]+/).filter(Boolean) : undefined,
    }));
    if (lines.length === 0) { act.setError("Enter the quantity received for at least one line"); return; }
    const r = await act.run(() => api.receive({ po_id: po.id, location_id: loc || undefined, delivery_ref: deliveryRef, landed_cost: Number(landed) || 0, landed_method: method, close_short: short, lines }));
    if (r) onDone();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 760 }, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-md">Receive goods on {po.number}</Typography>
        <Typography level="body-sm">
          Enter what actually arrived. Receiving less keeps the order open for the rest.{over > 0 ? ` Up to ${over}% over the ordered quantity is accepted.` : " More than ordered is refused."}
        </Typography>
        <ErrorBar error={act.error} />
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 1 }}>
          <FormControl>
            <FormLabel>Into location</FormLabel>
            <Select value={loc} onChange={(_, v) => setLoc(v ?? "")} data-testid="recv-location">
              <Option value="">Default</Option>
              {(locs.data ?? []).map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Delivery note no.</FormLabel>
            <Input value={deliveryRef} onChange={(e) => setDeliveryRef(e.target.value)} />
          </FormControl>
          <FormControl>
            <FormLabel>Freight / duty to add to cost</FormLabel>
            <Box sx={{ display: "flex", gap: 0.5 }}>
              <Input type="number" value={landed} onChange={(e) => setLanded(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": "recv-landed" } }} />
              <Select value={method} onChange={(_, v) => setMethod((v ?? "value") as "value" | "qty")} sx={{ minWidth: 90 }}>
                <Option value="value">by value</Option>
                <Option value="qty">by qty</Option>
              </Select>
            </Box>
          </FormControl>
        </Box>
        {rows.length === 0 && <Alert color="neutral">Every line is fully received.</Alert>}
        {rows.map((r, i) => {
          const limit = receiptLimit(r.line.qty, r.line.qty_received, over);
          return (
            <Sheet key={r.line.id} variant="soft" sx={{ p: 1, borderRadius: "md", display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
              <Box sx={{ flex: 1, minWidth: 160 }}>
                <Typography level="title-sm">{r.line.product_name}</Typography>
                <Typography level="body-xs">Ordered {qtyText(r.line.qty)}, received {qtyText(r.line.qty_received)}, up to {qtyText(limit)} more</Typography>
              </Box>
              <Input size="sm" type="number" sx={{ width: 100 }} value={r.qty} onChange={(e) => set(i, { qty: e.target.value })} slotProps={{ input: { min: 0, step: "any", "data-testid": `recv-qty-${i}` } }} />
              {r.line.tracking === "lot" && (
                <>
                  <Input size="sm" sx={{ width: 140 }} placeholder="Lot number" value={r.lot} onChange={(e) => set(i, { lot: e.target.value })} slotProps={{ input: { "data-testid": `recv-lot-${i}` } }} />
                  <Input size="sm" type="date" sx={{ width: 150 }} value={r.expiry} onChange={(e) => set(i, { expiry: e.target.value })} slotProps={{ input: { "aria-label": "Expiry date", "data-testid": `recv-expiry-${i}` } }} />
                </>
              )}
              {r.line.tracking === "serial" && (
                <Textarea size="sm" minRows={1} sx={{ minWidth: 220 }} placeholder="Serial numbers, separated by spaces or commas" value={r.serials} onChange={(e) => set(i, { serials: e.target.value })} />
              )}
            </Sheet>
          );
        })}
        <Checkbox label="Close the order even if some lines are short (no back order)" checked={short} onChange={(e) => setShort(e.target.checked)} />
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" onClick={onClose}>Cancel</Button>
          <Button loading={act.busy} onClick={save} data-testid="recv-save">Book receipt</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
