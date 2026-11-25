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
  Switch,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import { api } from "./stockApi";
import type { Bill, Grn, Price, ReorderRun, Request, Rule } from "./stockApi";
import { money, qtyText } from "./logic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, dateOnly, dateTime, right, useAction, useLoad } from "./stockUi";
import type { InvSupplier } from "./types";

export interface TabProps {
  suppliers: InvSupplier[];
  version: number;
  onChanged: () => void;
  goto: (t: "orders" | "requests" | "receipts" | "bills" | "returns" | "prices" | "reorder" | "performance" | "settings") => void;
}

const REQ_TONE = { draft: "neutral", submitted: "primary", approved: "success", rejected: "danger", converted: "neutral" } as const;

// ---------------------------------------------------------------- requests

export function RequestsTab({ version, onChanged, goto }: TabProps) {
  const list = useLoad(() => api.requests(), [version]);
  const [creating, setCreating] = useState(false);
  const act = useAction();
  const done = () => { onChanged(); void list.reload(); };
  return (
    <Box>
      <Title right={<Button size="sm" onClick={() => setCreating(true)} data-testid="req-new">New request</Button>}>Purchase requests</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Optional step: staff ask for goods, a manager approves, and approving can turn the request into draft purchase orders (one per supplier).
      </Typography>
      <ErrorBar error={list.error || act.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No requests.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="req-table">
            <thead>
              <tr><th>Number</th><th>Items</th><th>Needed by</th><th>Status</th><th /></tr>
            </thead>
            <tbody>
              {list.data.map((r: Request) => (
                <tr key={r.id}>
                  <td>{r.number}</td>
                  <td>{r.lines.map((l) => `${qtyText(l.qty)} × ${l.product_name}`).join(", ")}</td>
                  <td>{dateOnly(r.needed_by)}</td>
                  <td><Chip size="sm" variant="soft" color={REQ_TONE[r.state]}>{r.state}</Chip></td>
                  <td style={{ ...right, whiteSpace: "nowrap" }}>
                    {r.state === "draft" && <Button size="sm" variant="plain" onClick={async () => { if (await act.run(() => api.decideRequest(r.id, "submit"))) done(); }}>Submit</Button>}
                    {r.state === "submitted" && (
                      <>
                        <Button size="sm" variant="plain" onClick={async () => { if (await act.run(() => api.decideRequest(r.id, "approve"))) done(); }} data-testid={`req-approve-${r.number}`}>Approve</Button>
                        <Button size="sm" variant="plain" color="danger" onClick={async () => { if (await act.run(() => api.decideRequest(r.id, "reject"))) done(); }}>Reject</Button>
                      </>
                    )}
                    {r.state === "approved" && (
                      <Button size="sm" variant="plain" onClick={async () => { if (await act.run(() => api.convertRequest(r.id))) { done(); goto("orders"); } }} data-testid={`req-convert-${r.number}`}>Create purchase orders</Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {creating && <NewRequest onClose={() => setCreating(false)} onDone={() => { setCreating(false); done(); }} />}
    </Box>
  );
}

function NewRequest({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [note, setNote] = useState("");
  const [needed, setNeeded] = useState("");
  const [lines, setLines] = useState([{ product_id: "", qty: "1" }]);
  const act = useAction();
  const save = async (submit: boolean) => {
    const r = await act.run(() => api.createRequest({ note, needed_by: needed, submit, lines: lines.filter((l) => l.product_id).map((l) => ({ product_id: l.product_id, qty: Number(l.qty) })) }));
    if (r) onDone();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, sm: 520 } }}>
        <ModalClose />
        <Typography level="title-md">New purchase request</Typography>
        <ErrorBar error={act.error} />
        {lines.map((l, i) => (
          <Box key={i} sx={{ display: "flex", gap: 1 }}>
            <Box sx={{ flex: 1 }}><ProductPicker value={l.product_id} testId={`req-product-${i}`} onChange={(id) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, product_id: id } : x)))} /></Box>
            <Input size="sm" type="number" sx={{ width: 90 }} value={l.qty} onChange={(e) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, qty: e.target.value } : x)))} slotProps={{ input: { min: 0, step: "any", "data-testid": `req-qty-${i}` } }} />
          </Box>
        ))}
        <Button size="sm" variant="soft" onClick={() => setLines((ls) => [...ls, { product_id: "", qty: "1" }])}>Add item</Button>
        <FormControl><FormLabel>Needed by</FormLabel><Input type="date" value={needed} onChange={(e) => setNeeded(e.target.value)} /></FormControl>
        <FormControl><FormLabel>Why</FormLabel><Input value={note} onChange={(e) => setNote(e.target.value)} /></FormControl>
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="outlined" loading={act.busy} onClick={() => save(false)}>Save draft</Button>
          <Button loading={act.busy} onClick={() => save(true)} data-testid="req-submit">Submit for approval</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

// ---------------------------------------------------------------- receipts

export function ReceiptsTab({ version, onChanged }: TabProps) {
  const list = useLoad(() => api.receipts(), [version]);
  const [ret, setRet] = useState<Grn | null>(null);
  return (
    <Box>
      <Title>Goods receipts</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>Every delivery booked against a purchase order, with the cost each line carries into stock (order price plus its share of freight).</Typography>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>Nothing received yet. Receive goods from a purchase order.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="grn-table">
            <thead>
              <tr><th>Receipt</th><th>Order</th><th>Supplier</th><th>Received</th><th>Lines</th><th /></tr>
            </thead>
            <tbody>
              {list.data.map((g) => (
                <tr key={g.id}>
                  <td>{g.number}</td>
                  <td>{g.po_number}</td>
                  <td>{g.supplier_name}</td>
                  <td>{dateTime(g.received_at)}<br /><Typography level="body-xs">{g.location_name}</Typography></td>
                  <td>
                    {g.lines.map((l) => (
                      <div key={l.id}>
                        {qtyText(l.qty)} × {l.product_name} @ {money(l.unit_cost + l.landed_alloc)}
                        {l.lot_no && <Typography level="body-xs" component="span"> lot {l.lot_no}{l.expiry ? `, exp ${l.expiry}` : ""}</Typography>}
                        {l.qty_returned > 0 && <Typography level="body-xs" component="span"> ({qtyText(l.qty_returned)} returned)</Typography>}
                      </div>
                    ))}
                  </td>
                  <td style={right}><Button size="sm" variant="plain" onClick={() => setRet(g)} data-testid={`grn-return-${g.number}`}>Return to vendor</Button></td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {ret && <ReturnDialog grn={ret} onClose={() => setRet(null)} onDone={() => { setRet(null); onChanged(); void list.reload(); }} />}
    </Box>
  );
}

function ReturnDialog({ grn, onClose, onDone }: { grn: Grn; onClose: () => void; onDone: () => void }) {
  const [qty, setQty] = useState<Record<string, string>>({});
  const [reason, setReason] = useState("");
  const [cn, setCn] = useState("");
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340, maxWidth: 560 }}>
        <ModalClose />
        <Typography level="title-md">Return to vendor from {grn.number}</Typography>
        <Typography level="body-sm">Stock leaves at the cost it came in at, and the order&apos;s received quantity drops.</Typography>
        <ErrorBar error={act.error} />
        {grn.lines.map((l) => (
          <FormControl key={l.id}>
            <FormLabel>{l.product_name}{l.lot_no ? ` (lot ${l.lot_no})` : ""}: up to {qtyText(l.qty - l.qty_returned)}</FormLabel>
            <Input type="number" value={qty[l.id] ?? ""} placeholder="0" onChange={(e) => setQty((q) => ({ ...q, [l.id]: e.target.value }))} slotProps={{ input: { min: 0, step: "any", "data-testid": `ret-qty-${l.id}` } }} />
          </FormControl>
        ))}
        <FormControl><FormLabel>Reason</FormLabel><Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Damaged, wrong item…" data-testid="ret-reason" /></FormControl>
        <FormControl><FormLabel>Supplier credit note no. (optional)</FormLabel><Input value={cn} onChange={(e) => setCn(e.target.value)} /></FormControl>
        <Button
          loading={act.busy}
          onClick={async () => {
            const lines = grn.lines.map((l) => ({ grn_line_id: l.id, qty: Number(qty[l.id] ?? 0), serials: l.serials })).filter((l) => l.qty > 0);
            if (lines.length === 0) { act.setError("Enter a quantity to return"); return; }
            if (await act.run(() => api.createReturn({ grn_id: grn.id, reason, credit_note: cn, lines }))) onDone();
          }}
          data-testid="ret-save"
        >
          Return goods
        </Button>
      </ModalDialog>
    </Modal>
  );
}

export function ReturnsTab({ version }: TabProps) {
  const list = useLoad(() => api.returns(), [version]);
  return (
    <Box>
      <Title>Returns to vendor</Title>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No returns. Start one from a goods receipt.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="rtv-table">
            <thead><tr><th>Number</th><th>Supplier</th><th>Items</th><th>Reason</th><th>Date</th><th style={right}>Value</th></tr></thead>
            <tbody>
              {list.data.map((r) => (
                <tr key={r.id}>
                  <td>{r.number}</td>
                  <td>{r.supplier_name}</td>
                  <td>{r.lines.map((l) => `${qtyText(l.qty)} × ${l.product_name}`).join(", ")}</td>
                  <td>{r.reason}{r.credit_note ? ` (credit note ${r.credit_note})` : ""}</td>
                  <td>{dateTime(r.returned_at)}</td>
                  <td style={right}>{money(r.total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

// ------------------------------------------------------------------- bills

const MATCH_TONE: Record<string, "success" | "warning" | "neutral"> = { matched: "success", exceptions: "warning", unmatched: "neutral" };

export function BillsTab({ version, onChanged }: TabProps) {
  const list = useLoad(() => api.bills(), [version]);
  const [sel, setSel] = useState<Bill | null>(null);
  return (
    <Box>
      <Title>Supplier bills</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Bills are matched three ways: what was ordered, what was received, and what the supplier charges. A matching bill posts to Accounting as a vendor bill.
        Create one from a purchase order&apos;s receipts.
      </Typography>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No supplier bills yet.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="bill-table">
            <thead><tr><th>Number</th><th>Supplier</th><th>Their invoice</th><th>Order</th><th>Match</th><th>Status</th><th style={right}>Total</th></tr></thead>
            <tbody>
              {list.data.map((b) => (
                <tr key={b.id} onClick={() => setSel(b)} style={{ cursor: "pointer" }} data-testid={`bill-row-${b.number}`}>
                  <td>{b.number}</td>
                  <td>{b.supplier_name}</td>
                  <td>{b.bill_ref}</td>
                  <td>{b.po_number}</td>
                  <td><Chip size="sm" variant="soft" color={MATCH_TONE[b.match_status] ?? "neutral"}>{b.match_status === "matched" ? "3-way match" : b.match_status === "exceptions" ? "Mismatch" : "No order"}</Chip></td>
                  <td><Chip size="sm" variant="soft" color={b.state === "posted" ? "success" : "neutral"}>{b.state}</Chip></td>
                  <td style={right}>{money(b.total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {sel && <BillDialog bill={sel} onClose={() => setSel(null)} onChanged={() => { onChanged(); void list.reload(); }} />}
    </Box>
  );
}

function BillDialog({ bill, onClose, onChanged }: { bill: Bill; onClose: () => void; onChanged: () => void }) {
  const [b, setB] = useState<Bill>(bill);
  const [ref, setRef] = useState(bill.bill_ref);
  const [date, setDate] = useState(bill.bill_date);
  const [due, setDue] = useState(bill.due_date);
  const [qty, setQty] = useState<Record<string, string>>({});
  const [cost, setCost] = useState<Record<string, string>>({});
  const [override, setOverride] = useState("");
  const act = useAction();
  const draft = b.state === "draft";
  const lines = () => b.lines.map((l) => ({ po_line_id: l.po_line_id || undefined, product_id: l.product_id || undefined, description: l.description, qty: Number(qty[l.id] ?? l.qty), unit_cost: Number(cost[l.id] ?? l.unit_cost), tax_pct: l.tax_pct }));
  const save = async () => {
    const r = await act.run(() => api.updateBill(b.id, { bill_ref: ref, bill_date: date, due_date: due, lines: lines() }));
    if (r) { setB(r); onChanged(); }
    return r;
  };
  const post = async () => {
    const saved = await save();
    if (!saved) return;
    const r = await act.run(() => api.postBill(b.id, override));
    if (r) { setB(r); onChanged(); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 760 }, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <ModalClose />
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          <Typography level="title-lg">{b.number}</Typography>
          <Chip variant="soft" color={b.state === "posted" ? "success" : "neutral"} data-testid="bill-state">{b.state}</Chip>
          <Chip variant="soft" color={MATCH_TONE[b.match_status] ?? "neutral"} data-testid="bill-match">{b.match_status}</Chip>
          <Typography level="body-sm">{b.supplier_name}{b.po_number ? ` · ${b.po_number}` : ""}</Typography>
        </Box>
        <ErrorBar error={act.error} />
        {b.match_detail.length > 0 && (
          <Alert color="warning" variant="soft" data-testid="bill-issues">
            <Box>
              {b.match_detail.map((m, i) => <div key={i}><b>{m.name}</b>: {m.detail}</div>)}
            </Box>
          </Alert>
        )}
        {b.override_reason && <Alert color="neutral">Posted with override: {b.override_reason}</Alert>}
        {b.state === "posted" && (
          <Alert color={b.accounting_state === "posted" ? "success" : "warning"} variant="soft">
            Accounting: {b.accounting_state === "posted" ? "posted as a vendor bill" : b.accounting_state === "failed" ? `not posted (${b.accounting_error}); it retries automatically` : "waiting to be posted"}
          </Alert>
        )}
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 1 }}>
          <FormControl><FormLabel>Supplier invoice no.</FormLabel><Input disabled={!draft} value={ref} onChange={(e) => setRef(e.target.value)} data-testid="bill-ref" /></FormControl>
          <FormControl><FormLabel>Bill date</FormLabel><Input type="date" disabled={!draft} value={date} onChange={(e) => setDate(e.target.value)} /></FormControl>
          <FormControl><FormLabel>Due date</FormLabel><Input type="date" disabled={!draft} value={due} onChange={(e) => setDue(e.target.value)} /></FormControl>
        </Box>
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm">
            <thead><tr><th>Item</th><th style={right}>Qty</th><th style={right}>Cost</th><th style={right}>Tax %</th><th style={right}>Amount</th></tr></thead>
            <tbody>
              {b.lines.map((l) => (
                <tr key={l.id}>
                  <td>{l.product_name || l.description}</td>
                  <td style={right}>{draft ? <Input size="sm" type="number" sx={{ width: 90, ml: "auto" }} value={qty[l.id] ?? String(l.qty)} onChange={(e) => setQty((q) => ({ ...q, [l.id]: e.target.value }))} slotProps={{ input: { min: 0, step: "any", "data-testid": `bill-qty-${l.id}` } }} /> : qtyText(l.qty)}</td>
                  <td style={right}>{draft ? <Input size="sm" type="number" sx={{ width: 100, ml: "auto" }} value={cost[l.id] ?? String(l.unit_cost)} onChange={(e) => setCost((q) => ({ ...q, [l.id]: e.target.value }))} slotProps={{ input: { min: 0, step: "any", "data-testid": `bill-cost-${l.id}` } }} /> : money(l.unit_cost)}</td>
                  <td style={right}>{qtyText(l.tax_pct)}</td>
                  <td style={right}>{money(l.line_total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
        <Typography level="body-sm" sx={{ textAlign: "right" }}>Subtotal {money(b.subtotal)} · Tax {money(b.tax_total)} · <b>Total {money(b.total)}</b></Typography>
        {draft && (
          <>
            {b.match_status === "exceptions" && (
              <FormControl>
                <FormLabel>Override reason (only if you accept the mismatch)</FormLabel>
                <Input value={override} onChange={(e) => setOverride(e.target.value)} placeholder="Why this bill is posted anyway" data-testid="bill-override" />
              </FormControl>
            )}
            <Box sx={{ display: "flex", gap: 1, justifyContent: "space-between", flexWrap: "wrap" }}>
              <Button variant="plain" color="danger" onClick={async () => { if (window.confirm("Delete this draft bill?") && (await act.run(() => api.deleteBill(b.id)))) { onChanged(); onClose(); } }}>Delete draft</Button>
              <Box sx={{ display: "flex", gap: 1 }}>
                <Button variant="outlined" loading={act.busy} onClick={save} data-testid="bill-save">Save and re-check</Button>
                <Button loading={act.busy} onClick={post} data-testid="bill-post">Post bill</Button>
              </Box>
            </Box>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

// ------------------------------------------------------------------ prices

export function PricesTab({ suppliers, version }: TabProps) {
  const [sup, setSup] = useState("");
  const list = useLoad(() => api.prices({ supplier_id: sup }), [sup, version]);
  const [edit, setEdit] = useState<Partial<Price> | null>(null);
  const act = useAction();
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1 }}>
            <Select size="sm" value={sup} onChange={(_, v) => setSup(v ?? "")} sx={{ minWidth: 180 }} placeholder="All suppliers">
              <Option value="">All suppliers</Option>
              {suppliers.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}
            </Select>
            <Button size="sm" onClick={() => setEdit({ min_qty: 1, supplier_id: sup })} data-testid="price-new">Add price</Button>
          </Box>
        }
      >
        Supplier price lists
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>Quantity breaks, validity dates and lead times. New purchase orders use the best valid price for the quantity.</Typography>
      <ErrorBar error={list.error || act.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No prices. Without one, the product&apos;s supplier cost or cost price is used.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="price-table">
            <thead><tr><th>Supplier</th><th>Product</th><th style={right}>From qty</th><th style={right}>Unit cost</th><th style={right}>Lead (days)</th><th>Valid</th><th /></tr></thead>
            <tbody>
              {list.data.map((p) => (
                <tr key={p.id}>
                  <td>{p.supplier_name}</td>
                  <td>{p.product_name}</td>
                  <td style={right}>{qtyText(p.min_qty)}</td>
                  <td style={right}>{money(p.unit_cost)}</td>
                  <td style={right}>{p.lead_days}</td>
                  <td>{p.valid_from || p.valid_to ? `${p.valid_from || "…"} to ${p.valid_to || "…"}` : "Always"}</td>
                  <td style={right}>
                    <Button size="sm" variant="plain" onClick={() => setEdit(p)}>Edit</Button>
                    <IconButton size="sm" variant="plain" color="danger" aria-label="Delete price" onClick={async () => { if (await act.run(() => api.deletePrice(p.id))) void list.reload(); }}>✕</IconButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && <PriceDialog value={edit} suppliers={suppliers} onClose={() => setEdit(null)} onDone={() => { setEdit(null); void list.reload(); }} />}
    </Box>
  );
}

function PriceDialog({ value, suppliers, onClose, onDone }: { value: Partial<Price>; suppliers: InvSupplier[]; onClose: () => void; onDone: () => void }) {
  const [v, setV] = useState<Partial<Price>>(value);
  const act = useAction();
  const set = (p: Partial<Price>) => setV((x) => ({ ...x, ...p }));
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">{v.id ? "Edit price" : "New price"}</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>Supplier</FormLabel>
          <Select value={v.supplier_id ?? ""} onChange={(_, s) => set({ supplier_id: s ?? "" })} data-testid="price-supplier">
            {suppliers.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}
          </Select>
        </FormControl>
        <FormControl><FormLabel>Product</FormLabel><ProductPicker value={v.product_id ?? ""} testId="price-product" onChange={(id) => set({ product_id: id })} /></FormControl>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1 }}>
          <FormControl><FormLabel>From qty</FormLabel><Input type="number" value={v.min_qty ?? 1} onChange={(e) => set({ min_qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "price-minqty" } }} /></FormControl>
          <FormControl><FormLabel>Unit cost</FormLabel><Input type="number" value={v.unit_cost ?? ""} onChange={(e) => set({ unit_cost: Number(e.target.value) })} slotProps={{ input: { step: "any", "data-testid": "price-cost" } }} /></FormControl>
          <FormControl><FormLabel>Lead days</FormLabel><Input type="number" value={v.lead_days ?? 0} onChange={(e) => set({ lead_days: Number(e.target.value) })} /></FormControl>
        </Box>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl><FormLabel>Valid from</FormLabel><Input type="date" value={v.valid_from ?? ""} onChange={(e) => set({ valid_from: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Valid to</FormLabel><Input type="date" value={v.valid_to ?? ""} onChange={(e) => set({ valid_to: e.target.value })} /></FormControl>
        </Box>
        <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.savePrice(v.id ?? "", v))) onDone(); }} data-testid="price-save">Save</Button>
      </ModalDialog>
    </Modal>
  );
}

// ----------------------------------------------------------------- reorder

export function ReorderTab({ suppliers, version, onChanged, goto }: TabProps) {
  const rules = useLoad(() => api.rules(), [version]);
  const [run, setRun] = useState<ReorderRun | null>(null);
  const [edit, setEdit] = useState<Partial<Rule> | null>(null);
  const act = useAction();
  const preview = async () => { const r = await act.run(() => api.runReorder(false)); if (r) setRun(r); };
  const create = async () => { const r = await act.run(() => api.runReorder(true)); if (r) { setRun(r); onChanged(); } };
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1 }}>
            <Button size="sm" variant="outlined" onClick={() => setEdit({ min_qty: 0, max_qty: 0, multiple: 1, active: true })} data-testid="rule-new">Add rule</Button>
            <Button size="sm" variant="outlined" loading={act.busy} onClick={preview} data-testid="reorder-preview">Check what to buy</Button>
            <Button size="sm" loading={act.busy} onClick={create} data-testid="reorder-create">Create draft orders</Button>
          </Box>
        }
      >
        Reorder rules
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        A rule says: when stock on hand plus incoming minus reserved falls to the minimum, buy up to the maximum, in multiples. Products without a rule use their
        low-stock level. Draft orders are grouped by supplier and never duplicated: running again skips what is already on a draft.
      </Typography>
      <ErrorBar error={rules.error || act.error} />
      {run && (
        <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", mb: 2 }} data-testid="reorder-result">
          {run.created.length > 0 && (
            <Alert color="success" sx={{ mb: 1 }} endDecorator={<Button size="sm" variant="outlined" onClick={() => goto("orders")}>Open orders</Button>}>
              Created or extended {run.created.length} draft order{run.created.length === 1 ? "" : "s"}: {run.created.map((o) => `${o.number} (${o.supplier_name})`).join(", ")}
            </Alert>
          )}
          {run.proposals.length === 0 && run.unassigned.length === 0 && <Typography level="body-sm">Nothing needs buying right now.</Typography>}
          {run.proposals.length > 0 && (
            <Table size="sm">
              <thead><tr><th>Supplier</th><th>Product</th><th style={right}>Position</th><th style={right}>Min</th><th style={right}>Order</th><th style={right}>Cost</th></tr></thead>
              <tbody>
                {run.proposals.map((p) => (
                  <tr key={p.product_id}>
                    <td>{p.supplier_name}</td><td>{p.product_name}</td><td style={right}>{qtyText(p.position)}</td><td style={right}>{qtyText(p.min_qty)}</td>
                    <td style={right}><b>{qtyText(p.qty)}</b></td><td style={right}>{money(p.unit_cost)}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
          {run.unassigned.length > 0 && <Alert color="warning" sx={{ mt: 1 }}>No supplier known for: {run.unassigned.map((p) => p.product_name).join(", ")}. Add a supplier to these products.</Alert>}
        </Sheet>
      )}
      <Loading show={rules.loading && !rules.data} />
      {rules.data && rules.data.length === 0 && <Empty>No rules yet.</Empty>}
      {rules.data && rules.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="rule-table">
            <thead><tr><th>Product</th><th style={right}>Minimum</th><th style={right}>Maximum</th><th style={right}>Multiple</th><th>Supplier</th><th /></tr></thead>
            <tbody>
              {rules.data.map((r) => (
                <tr key={r.id} style={{ opacity: r.active ? 1 : 0.5 }}>
                  <td>{r.product_name}</td><td style={right}>{qtyText(r.min_qty)}</td><td style={right}>{qtyText(r.max_qty)}</td><td style={right}>{qtyText(r.multiple)}</td>
                  <td>{suppliers.find((s) => s.id === r.supplier_id)?.name ?? "Preferred supplier"}</td>
                  <td style={right}>
                    <Button size="sm" variant="plain" onClick={() => setEdit(r)}>Edit</Button>
                    <IconButton size="sm" variant="plain" color="danger" aria-label="Delete rule" onClick={async () => { if (await act.run(() => api.deleteRule(r.id))) void rules.reload(); }}>✕</IconButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && <RuleDialog value={edit} suppliers={suppliers} onClose={() => setEdit(null)} onDone={() => { setEdit(null); void rules.reload(); }} />}
    </Box>
  );
}

function RuleDialog({ value, suppliers, onClose, onDone }: { value: Partial<Rule>; suppliers: InvSupplier[]; onClose: () => void; onDone: () => void }) {
  const [v, setV] = useState<Partial<Rule>>(value);
  const act = useAction();
  const set = (p: Partial<Rule>) => setV((x) => ({ ...x, ...p }));
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Reorder rule</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>Product</FormLabel><ProductPicker value={v.product_id ?? ""} testId="rule-product" onChange={(id) => set({ product_id: id })} /></FormControl>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1 }}>
          <FormControl><FormLabel>Minimum</FormLabel><Input type="number" value={v.min_qty ?? 0} onChange={(e) => set({ min_qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "rule-min" } }} /></FormControl>
          <FormControl><FormLabel>Maximum</FormLabel><Input type="number" value={v.max_qty ?? 0} onChange={(e) => set({ max_qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "rule-max" } }} /></FormControl>
          <FormControl><FormLabel>Multiple</FormLabel><Input type="number" value={v.multiple ?? 1} onChange={(e) => set({ multiple: Number(e.target.value) })} /></FormControl>
        </Box>
        <FormControl><FormLabel>Supplier</FormLabel>
          <Select value={v.supplier_id ?? ""} onChange={(_, s) => set({ supplier_id: s ?? "" })}>
            <Option value="">Preferred / cheapest</Option>
            {suppliers.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}
          </Select>
        </FormControl>
        <FormControl orientation="horizontal" sx={{ gap: 1 }}><Switch checked={v.active !== false} onChange={(e) => set({ active: e.target.checked })} /><FormLabel>Active</FormLabel></FormControl>
        <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.saveRule(v))) onDone(); }} data-testid="rule-save">Save</Button>
      </ModalDialog>
    </Modal>
  );
}

// ------------------------------------------------------------- performance

export function PerformanceTab({ version }: TabProps) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const rows = useLoad(() => api.performance(from, to), [from, to, version]);
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "From date" } }} />
            <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "aria-label": "To date" } }} />
          </Box>
        }
      >
        Supplier performance
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        On time = receipts on or before the promised (else expected) date. Fill rate = received over ordered. Score weighs on time 50%, fill rate 30% and the share not returned 20%.
      </Typography>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No orders or receipts in this period.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="perf-table">
            <thead>
              <tr><th>Supplier</th><th style={right}>Orders</th><th style={right}>On time</th><th style={right}>Fill rate</th><th style={right}>Avg lead (days)</th><th style={right}>Returned</th><th style={right}>Price variance</th><th style={right}>Spend</th><th style={right}>Score</th></tr>
            </thead>
            <tbody>
              {rows.data.map((s) => (
                <tr key={s.supplier_id}>
                  <td>{s.supplier_name}</td>
                  <td style={right}>{s.orders}</td>
                  <td style={right}>{s.on_time_pct < 0 ? "n/a" : `${s.on_time_pct}%`}</td>
                  <td style={right}>{s.fill_rate_pct}%</td>
                  <td style={right}>{s.avg_lead_days}</td>
                  <td style={right}>{s.return_rate_pct}%</td>
                  <td style={{ ...right, color: s.price_variance > 0 ? "var(--joy-palette-danger-600)" : undefined }}>{money(s.price_variance)}</td>
                  <td style={right}>{money(s.spend)}</td>
                  <td style={right}><Chip size="sm" variant="soft" color={s.score >= 85 ? "success" : s.score >= 60 ? "warning" : "danger"}>{Math.round(s.score)}</Chip></td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

// ---------------------------------------------------------------- settings

export function SettingsTab({ onChanged }: TabProps) {
  const s = useLoad(() => api.settings(), []);
  const [v, setV] = useState<import("./stockApi").PurchaseSettings | null>(null);
  const [saved, setSaved] = useState(false);
  const act = useAction();
  const cur = v ?? s.data;
  if (!cur) return <><ErrorBar error={s.error} /><Loading show /></>;
  const set = (p: Partial<typeof cur>) => { setSaved(false); setV({ ...cur, ...p }); };
  const num = (x: string) => (x === "" ? 0 : Number(x));
  return (
    <Box sx={{ maxWidth: 640, display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Title>Purchasing settings</Title>
      <ErrorBar error={act.error} />
      <FormControl><FormLabel>Company name on purchase orders</FormLabel><Input value={cur.company_name} onChange={(e) => set({ company_name: e.target.value })} data-testid="set-company" /></FormControl>
      <FormControl><FormLabel>Address</FormLabel><Textarea minRows={2} value={cur.company_address} onChange={(e) => set({ company_address: e.target.value })} /></FormControl>
      <FormControl><FormLabel>GSTIN</FormLabel><Input value={cur.company_gstin} onChange={(e) => set({ company_gstin: e.target.value })} /></FormControl>
      <FormControl><FormLabel>Default terms on orders</FormLabel><Textarea minRows={2} value={cur.default_terms} onChange={(e) => set({ default_terms: e.target.value })} /></FormControl>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
        <FormControl><FormLabel>Over-receipt allowed (%)</FormLabel><Input type="number" value={cur.over_receipt_pct} onChange={(e) => set({ over_receipt_pct: num(e.target.value) })} slotProps={{ input: { min: 0, step: "any", "data-testid": "set-over" } }} /></FormControl>
        <FormControl><FormLabel>When a delivery is short</FormLabel>
          <Select value={cur.under_receipt_policy} onChange={(_, p) => set({ under_receipt_policy: (p ?? "backorder") as "backorder" | "close" })}>
            <Option value="backorder">Keep the rest on back order</Option>
            <Option value="close">Close the order</Option>
          </Select>
        </FormControl>
        <FormControl><FormLabel>Bill price tolerance (%)</FormLabel><Input type="number" value={cur.price_tolerance_pct} onChange={(e) => set({ price_tolerance_pct: num(e.target.value) })} slotProps={{ input: { min: 0, step: "any", "data-testid": "set-price-tol" } }} /></FormControl>
        <FormControl><FormLabel>Bill quantity tolerance (%)</FormLabel><Input type="number" value={cur.qty_tolerance_pct} onChange={(e) => set({ qty_tolerance_pct: num(e.target.value) })} slotProps={{ input: { min: 0, step: "any" } }} /></FormControl>
        <FormControl><FormLabel>Input tax split on bills</FormLabel>
          <Select value={cur.tax_split} onChange={(_, p) => set({ tax_split: (p ?? "cgst_sgst") as "cgst_sgst" | "igst" })}>
            <Option value="cgst_sgst">CGST + SGST (same state)</Option>
            <Option value="igst">IGST (other state)</Option>
          </Select>
        </FormControl>
        <FormControl><FormLabel>Currency</FormLabel><Input value={cur.currency} onChange={(e) => set({ currency: e.target.value.toUpperCase().slice(0, 3) })} /></FormControl>
      </Box>
      <Checkbox label="Orders need an approval step before they can be sent" checked={cur.require_approval} onChange={(e) => set({ require_approval: e.target.checked })} />
      {cur.require_approval && (
        <FormControl sx={{ maxWidth: 300 }}>
          <FormLabel>Only orders of at least this total need approval</FormLabel>
          <Input type="number" value={cur.approval_threshold} onChange={(e) => set({ approval_threshold: num(e.target.value) })} slotProps={{ input: { min: 0, step: "any", "data-testid": "set-threshold" } }} />
          <Typography level="body-xs">0 means every order. Smaller orders are approved automatically when saved.</Typography>
        </FormControl>
      )}
      <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
        <Button loading={act.busy} onClick={async () => { const r = await act.run(() => api.saveSettings(cur)); if (r) { setV(r); setSaved(true); onChanged(); } }} data-testid="set-save">Save settings</Button>
        {saved && <Typography level="body-sm" color="success">Saved</Typography>}
      </Box>
    </Box>
  );
}
