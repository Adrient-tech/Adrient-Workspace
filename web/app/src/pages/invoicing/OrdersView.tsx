import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Textarea, Typography } from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import {
  cancelDelivery,
  createDelivery,
  dispatchDelivery,
  getOrder,
  invoiceOrder,
  listContacts,
  listDeliveries,
  listOrders,
  saveOrder,
  setOrderStatus,
} from "./api";
import type { ContactLite, Delivery, SalesOrder } from "./api";
import { GST_STATES, ACCENT, emptyLine, money, todayISO } from "./logic";
import type { Business, Line, Settings } from "./types";
import { Empty, ErrorAlert, rightCell, useErr } from "./ui";

const FULFIL: Record<string, string> = { not_delivered: "Not delivered", partially_delivered: "Partly delivered", delivered: "Delivered" };
const BILL: Record<string, string> = { not_invoiced: "Not invoiced", partially_invoiced: "Partly invoiced", invoiced: "Invoiced" };

function StateChips({ o }: { o: SalesOrder }) {
  if (o.status === "cancelled") return <Chip size="sm" color="danger" variant="soft">Cancelled</Chip>;
  if (o.status === "draft") return <Chip size="sm" variant="soft">Draft</Chip>;
  return (
    <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
      <Chip size="sm" variant="soft" color={o.fulfilment === "delivered" ? "success" : o.fulfilment === "partially_delivered" ? "warning" : "neutral"} data-testid="so-fulfil">
        {FULFIL[o.fulfilment]}
      </Chip>
      <Chip size="sm" variant="soft" color={o.billing === "invoiced" ? "success" : o.billing === "partially_invoiced" ? "warning" : "neutral"} data-testid="so-billing">
        {BILL[o.billing]}
      </Chip>
    </Box>
  );
}

export function OrdersView({
  settings,
  version,
  onChanged,
  openId,
  clearOpen,
  onOpenInvoice,
}: {
  settings: Settings;
  version: number;
  onChanged: () => void;
  openId?: string;
  clearOpen?: () => void;
  onOpenInvoice: (id: string) => void;
}) {
  const [orders, setOrders] = useState<SalesOrder[] | null>(null);
  const [q, setQ] = useState("");
  const [detail, setDetail] = useState<SalesOrder | null>(null);
  const [editor, setEditor] = useState(false);
  const { err } = useErr();

  const load = useCallback(() => {
    listOrders("", q)
      .then(setOrders)
      .catch(() => setOrders([]));
  }, [q]);
  useEffect(load, [load, version]);
  useEffect(() => {
    if (!openId) return;
    getOrder(openId)
      .then((o) => {
        setDetail(o);
        clearOpen?.();
      })
      .catch(() => clearOpen?.());
  }, [openId, clearOpen]);

  const changed = () => {
    onChanged();
    load();
  };

  return (
    <Box data-testid="so-view">
      <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
        <Input placeholder="Search order number or customer" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1 }} />
        <Button onClick={() => setEditor(true)} data-testid="so-new" sx={{ bgcolor: ACCENT, "&:hover": { bgcolor: "#9A3412" } }}>
          New sales order
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {orders && orders.length === 0 ? (
        <Empty>No sales orders yet. Accept an estimate and choose Create sales order, or add one here.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="so-table">
            <thead>
              <tr>
                <th>Order</th>
                <th>Customer</th>
                <th>Date</th>
                <th>Progress</th>
                <th style={rightCell}>Total</th>
              </tr>
            </thead>
            <tbody>
              {(orders ?? []).map((o) => (
                <tr key={o.id} style={{ cursor: "pointer" }} onClick={() => setDetail(o)} data-testid="so-row">
                  <td>{o.number}</td>
                  <td>{o.client_name}</td>
                  <td>{o.order_date}</td>
                  <td>
                    <StateChips o={o} />
                  </td>
                  <td style={rightCell}>{money(o.total, o.currency)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {detail && <OrderDetail key={detail.id} order={detail} settings={settings} onClose={() => setDetail(null)} onChanged={changed} onOpenInvoice={onOpenInvoice} />}
      {editor && (
        <OrderEditor
          settings={settings}
          onClose={() => setEditor(false)}
          onSaved={(o) => {
            setEditor(false);
            changed();
            setDetail(o);
          }}
        />
      )}
    </Box>
  );
}

function OrderEditor({ settings, onClose, onSaved }: { settings: Settings; onClose: () => void; onSaved: (o: SalesOrder) => void }) {
  const [o, setO] = useState<Partial<SalesOrder>>({
    client_name: "",
    client_email: "",
    client_address: "",
    client_gstin: "",
    place_of_supply: "",
    currency: settings.default_currency,
    order_date: todayISO(),
    lines: [emptyLine()],
    discount_kind: "percent",
    discount_value: 0,
    notes: "",
    terms: settings.default_terms,
  });
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const { err, run } = useErr();
  useEffect(() => {
    listContacts().then(setContacts);
  }, []);
  const lines = (o.lines ?? []) as Line[];
  const setLine = (i: number, p: Partial<Line>) => setO({ ...o, lines: lines.map((l, j) => (j === i ? { ...l, ...p } : l)) });
  const gstOn = !!settings.sales?.state_code;
  const save = async () => {
    const out = await run(() => saveOrder("", { ...o, lines: lines.filter((l) => l.description.trim()) }));
    if (out) onSaved(out);
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(860px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="so-editor">
        <ModalClose />
        <Typography level="h4">New sales order</Typography>
        <ErrorAlert err={err} />
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1 }}>
          <FormControl>
            <FormLabel>Customer (from Contacts)</FormLabel>
            <Select
              placeholder="Choose a contact"
              value={o.contact_id || null}
              onChange={(_, v) => {
                const c = contacts.find((x) => x.id === v);
                if (c) setO({ ...o, contact_id: c.id, client_name: c.company || c.display_name, client_email: c.emails?.[0] ?? o.client_email });
              }}
            >
              {contacts.map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.company || c.display_name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Customer name</FormLabel>
            <Input value={o.client_name} onChange={(e) => setO({ ...o, client_name: e.target.value })} slotProps={{ input: { "data-testid": "so-client" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Email</FormLabel>
            <Input value={o.client_email} onChange={(e) => setO({ ...o, client_email: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Expected delivery</FormLabel>
            <Input type="date" value={o.expected_date ?? ""} onChange={(e) => setO({ ...o, expected_date: e.target.value })} />
          </FormControl>
          {gstOn && (
            <>
              <FormControl>
                <FormLabel>Customer GSTIN</FormLabel>
                <Input value={o.client_gstin} onChange={(e) => setO({ ...o, client_gstin: e.target.value.toUpperCase() })} />
              </FormControl>
              <FormControl>
                <FormLabel>Place of supply</FormLabel>
                <Select value={o.place_of_supply || ""} onChange={(_, v) => setO({ ...o, place_of_supply: (v as string) || "" })}>
                  <Option value="">Same as my state</Option>
                  {GST_STATES.map(([c, n]) => (
                    <Option key={c} value={c}>
                      {c} {n}
                    </Option>
                  ))}
                </Select>
              </FormControl>
            </>
          )}
        </Box>
        <FormControl>
          <FormLabel>Address</FormLabel>
          <Textarea minRows={2} value={o.client_address} onChange={(e) => setO({ ...o, client_address: e.target.value })} />
        </FormControl>
        <Typography level="title-md">Lines</Typography>
        {lines.map((l, i) => (
          <Box key={i} sx={{ display: "flex", gap: 0.5 }} data-testid="so-line">
            <Input size="sm" placeholder="Description" sx={{ flex: 1 }} value={l.description} onChange={(e) => setLine(i, { description: e.target.value })} slotProps={{ input: { "data-testid": "so-line-desc" } }} />
            {gstOn && <Input size="sm" placeholder="HSN/SAC" sx={{ width: 100 }} value={l.hsn_sac ?? ""} onChange={(e) => setLine(i, { hsn_sac: e.target.value })} />}
            <Input size="sm" type="number" sx={{ width: 80 }} value={l.qty} onChange={(e) => setLine(i, { qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "so-line-qty" } }} />
            <Input size="sm" type="number" sx={{ width: 110 }} value={l.price} onChange={(e) => setLine(i, { price: Number(e.target.value) })} slotProps={{ input: { "data-testid": "so-line-price" } }} />
            <Input size="sm" type="number" sx={{ width: 80 }} placeholder="Tax %" value={l.tax_rate} onChange={(e) => setLine(i, { tax_rate: Number(e.target.value) })} slotProps={{ input: { "data-testid": "so-line-tax" } }} />
            <Button size="sm" variant="plain" color="danger" aria-label="Remove line" onClick={() => setO({ ...o, lines: lines.filter((_, j) => j !== i) })}>
              <DeleteOutline />
            </Button>
          </Box>
        ))}
        <Box>
          <Button size="sm" variant="outlined" onClick={() => setO({ ...o, lines: [...lines, emptyLine()] })}>
            + Line
          </Button>
        </Box>
        <FormControl>
          <FormLabel>Notes</FormLabel>
          <Textarea minRows={2} value={o.notes} onChange={(e) => setO({ ...o, notes: e.target.value })} />
        </FormControl>
        <Button onClick={save} data-testid="so-save">
          Save order
        </Button>
      </ModalDialog>
    </Modal>
  );
}

function OrderDetail({ order: initial, settings, onClose, onChanged, onOpenInvoice }: { order: SalesOrder; settings: Settings; onClose: () => void; onChanged: () => void; onOpenInvoice: (id: string) => void }) {
  const [o, setO] = useState(initial);
  const [pack, setPack] = useState<null | { qty: Record<string, number>; carrier: string; tracking: string }>(null);
  const [bill, setBill] = useState<null | Record<string, number>>(null);
  const [slip, setSlip] = useState<Delivery | null>(null);
  const [info, setInfo] = useState("");
  const { err, run } = useErr();
  const m = (n: number) => money(n, o.currency);
  const reload = async () => {
    const x = await run(() => getOrder(o.id));
    if (x) setO(x);
    onChanged();
  };
  const prog = (ref: string) => o.progress.find((p) => p.ref === ref);
  const live = o.status === "confirmed";

  const doStatus = async (s: string) => {
    const x = await run(() => setOrderStatus(o.id, s));
    if (x) {
      setO(x);
      onChanged();
    }
  };

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(980px, 96vw)", maxHeight: "94vh", overflow: "auto" }} data-testid="so-detail">
        <ModalClose />
        <Box className="invc-noprint" sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", pr: 4 }}>
          <Typography level="title-lg">{o.number}</Typography>
          <StateChips o={o} />
          {o.estimate_number && <Typography level="body-xs">from estimate {o.estimate_number}</Typography>}
          <Box sx={{ flex: 1 }} />
          {o.status === "draft" && (
            <Button size="sm" color="success" onClick={() => doStatus("confirmed")} data-testid="so-confirm">
              Confirm order
            </Button>
          )}
          {live && (
            <>
              <Button size="sm" variant="outlined" onClick={() => setPack({ qty: Object.fromEntries(o.progress.map((p) => [p.ref, p.to_pack])), carrier: "", tracking: "" })} disabled={o.progress.every((p) => p.to_pack <= 0)} data-testid="so-deliver">
                Create delivery note
              </Button>
              <Button size="sm" variant="outlined" onClick={() => setBill(Object.fromEntries(o.progress.map((p) => [p.ref, p.billable])))} disabled={o.progress.every((p) => p.to_invoice <= 0)} data-testid="so-invoice">
                Create invoice…
              </Button>
            </>
          )}
          {o.status !== "cancelled" && o.deliveries.every((d) => d.status === "cancelled") && o.invoices.length === 0 && (
            <Button size="sm" variant="plain" color="danger" onClick={() => doStatus("cancelled")}>
              Cancel order
            </Button>
          )}
        </Box>
        <ErrorAlert err={err} />
        {info && (
          <Typography level="body-sm" color="success" data-testid="so-info">
            {info}
          </Typography>
        )}
        <Typography level="body-sm">
          {o.client_name} · {o.order_date}
          {o.expected_date ? ` · expected ${o.expected_date}` : ""}
        </Typography>
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="so-lines">
            <thead>
              <tr>
                <th>Item</th>
                <th style={rightCell}>Ordered</th>
                <th style={rightCell}>Packed</th>
                <th style={rightCell}>Delivered</th>
                <th style={rightCell}>Invoiced</th>
                <th style={rightCell}>Price</th>
              </tr>
            </thead>
            <tbody>
              {o.lines.map((l) => {
                const p = prog(l.ref ?? "");
                return (
                  <tr key={l.ref} data-testid="so-line-row">
                    <td>
                      {l.description}
                      {l.hsn_sac ? <Typography level="body-xs"> HSN/SAC {l.hsn_sac}</Typography> : null}
                    </td>
                    <td style={rightCell}>{l.qty}</td>
                    <td style={rightCell}>{p?.packed ?? 0}</td>
                    <td style={rightCell}>{p?.delivered ?? 0}</td>
                    <td style={rightCell}>{p?.invoiced ?? 0}</td>
                    <td style={rightCell}>{m(l.price)}</td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Sheet>
        <Typography level="body-sm" sx={{ textAlign: "right" }}>
          Order total <b data-testid="so-total">{m(o.total)}</b> (tax {m(o.tax_total)})
        </Typography>

        {o.deliveries.length > 0 && (
          <>
            <Typography level="title-sm">Delivery notes</Typography>
            <Table size="sm" data-testid="so-deliveries">
              <tbody>
                {o.deliveries.map((d) => (
                  <tr key={d.id}>
                    <td>{d.number}</td>
                    <td>{d.lines.map((l) => `${l.qty} × ${l.description}`).join(", ")}</td>
                    <td>{d.status}</td>
                    <td style={rightCell}>
                      <Button size="sm" variant="plain" onClick={() => setSlip(d)} data-testid="so-slip">
                        Packing slip
                      </Button>
                      {d.status === "draft" && (
                        <>
                          <Button
                            size="sm"
                            variant="plain"
                            color="success"
                            data-testid="so-dispatch"
                            onClick={async () => {
                              const r = await run(() => dispatchDelivery(d.id));
                              if (r) {
                                setInfo(r.stock_warnings.length ? `Dispatched. Stock warnings: ${r.stock_warnings.join("; ")}` : `${d.number} dispatched and stock updated.`);
                                reload();
                              }
                            }}
                          >
                            Dispatch
                          </Button>
                          <Button
                            size="sm"
                            variant="plain"
                            color="danger"
                            onClick={async () => {
                              if (await run(() => cancelDelivery(d.id))) reload();
                            }}
                          >
                            Cancel
                          </Button>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </>
        )}
        {o.invoices.length > 0 && (
          <>
            <Typography level="title-sm">Invoices</Typography>
            <Table size="sm" data-testid="so-invoices">
              <tbody>
                {o.invoices.map((i) => (
                  <tr key={i.id}>
                    <td>
                      <Button size="sm" variant="plain" onClick={() => onOpenInvoice(i.id)}>
                        {i.number}
                      </Button>
                    </td>
                    <td>{i.status}</td>
                    <td style={rightCell}>{m(i.total)}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </>
        )}

        {pack && (
          <Modal open onClose={() => setPack(null)}>
            <ModalDialog sx={{ width: 520 }} data-testid="so-pack-dialog">
              <ModalClose />
              <Typography level="title-lg">Delivery note</Typography>
              <ErrorAlert err={err} />
              {o.lines.map((l) => (
                <Box key={l.ref} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                  <Typography level="body-sm" sx={{ flex: 1 }}>
                    {l.description} (left to pack {prog(l.ref ?? "")?.to_pack ?? 0})
                  </Typography>
                  <Input
                    size="sm"
                    type="number"
                    sx={{ width: 90 }}
                    value={pack.qty[l.ref ?? ""] ?? 0}
                    slotProps={{ input: { "data-testid": `so-pack-${l.ref}`, min: 0, max: prog(l.ref ?? "")?.to_pack ?? 0 } }}
                    onChange={(e) => setPack({ ...pack, qty: { ...pack.qty, [l.ref ?? ""]: Number(e.target.value) } })}
                  />
                </Box>
              ))}
              <Input placeholder="Carrier" value={pack.carrier} onChange={(e) => setPack({ ...pack, carrier: e.target.value })} />
              <Input placeholder="Tracking number" value={pack.tracking} onChange={(e) => setPack({ ...pack, tracking: e.target.value })} />
              <Button
                data-testid="so-pack-save"
                onClick={async () => {
                  const d = await run(() => createDelivery(o.id, { lines: Object.entries(pack.qty).map(([ref, qty]) => ({ ref, qty })), carrier: pack.carrier, tracking: pack.tracking }));
                  if (d) {
                    setPack(null);
                    setInfo(`Delivery note ${d.number} created. Dispatch it when the goods leave.`);
                    reload();
                  }
                }}
              >
                Create
              </Button>
            </ModalDialog>
          </Modal>
        )}
        {bill && (
          <Modal open onClose={() => setBill(null)}>
            <ModalDialog sx={{ width: 560 }} data-testid="so-bill-dialog">
              <ModalClose />
              <Typography level="title-lg">Invoice from {o.number}</Typography>
              <ErrorAlert err={err} />
              <Typography level="body-xs">Quantities start at what was delivered and not yet invoiced. You can bill up to what was ordered.</Typography>
              {o.lines.map((l) => (
                <Box key={l.ref} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                  <Typography level="body-sm" sx={{ flex: 1 }}>
                    {l.description} (left to invoice {prog(l.ref ?? "")?.to_invoice ?? 0})
                  </Typography>
                  <Input size="sm" type="number" sx={{ width: 90 }} value={bill[l.ref ?? ""] ?? 0} slotProps={{ input: { "data-testid": `so-bill-${l.ref}` } }} onChange={(e) => setBill({ ...bill, [l.ref ?? ""]: Number(e.target.value) })} />
                </Box>
              ))}
              <Box sx={{ display: "flex", gap: 1 }}>
                <Button
                  data-testid="so-bill-save"
                  onClick={async () => {
                    const d = await run(() => invoiceOrder(o.id, { lines: Object.entries(bill).map(([ref, qty]) => ({ ref, qty })) }));
                    if (d) {
                      setBill(null);
                      onChanged();
                      onOpenInvoice(d.id);
                    }
                  }}
                >
                  Create invoice
                </Button>
                <Button
                  variant="outlined"
                  onClick={async () => {
                    const d = await run(() => invoiceOrder(o.id, { mode: "remaining" }));
                    if (d) {
                      setBill(null);
                      onChanged();
                      onOpenInvoice(d.id);
                    }
                  }}
                >
                  Invoice everything left
                </Button>
              </Box>
            </ModalDialog>
          </Modal>
        )}
        {slip && <PackingSlip delivery={slip} settings={settings} onClose={() => setSlip(null)} />}
      </ModalDialog>
    </Modal>
  );
}

/** Printable packing slip (browser print / Save as PDF). No prices: it travels with the goods. */
export function PackingSlip({ delivery: d, settings, onClose }: { delivery: Delivery; settings: Settings; onClose: () => void }) {
  const biz: Business = settings;
  const cell = { padding: "6px 8px", borderBottom: "1px solid #e5e5e5", fontSize: 13 } as const;
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(860px, 96vw)", maxHeight: "94vh", overflow: "auto", bgcolor: "#f3f4f6" }} data-testid="slip-dialog">
        <ModalClose />
        <style>{`@media print { body * { visibility: hidden !important; } .invc-sheet, .invc-sheet * { visibility: visible !important; } .invc-sheet { position: absolute !important; inset: 0 auto auto 0 !important; width: 100% !important; box-shadow: none !important; border: 0 !important; } .invc-noprint { display: none !important; } @page { margin: 12mm; } }`}</style>
        <Box className="invc-noprint">
          <Button size="sm" onClick={() => window.print()} data-testid="slip-print">
            Print / save as PDF
          </Button>
        </Box>
        <Box className="invc-sheet" data-testid="slip-sheet" sx={{ bgcolor: "#fff", color: "#111", p: 4, border: "1px solid #ddd", borderRadius: "8px" }}>
          <Box sx={{ display: "flex", justifyContent: "space-between", mb: 3 }}>
            <Box>
              <Typography level="title-lg" sx={{ color: "#111" }}>
                {biz.business_name}
              </Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#444" }}>
                {biz.address}
              </Typography>
            </Box>
            <Box sx={{ textAlign: "right" }}>
              <Typography level="h2" sx={{ color: settings.sales?.pdf_accent || ACCENT }}>
                PACKING SLIP
              </Typography>
              <Typography level="body-sm" sx={{ color: "#444" }}>
                # {d.number} · Order {d.order_number}
              </Typography>
              <Typography level="body-sm" sx={{ color: "#444" }}>
                {d.delivery_date ? `Dispatched ${d.delivery_date}` : "Not yet dispatched"}
              </Typography>
            </Box>
          </Box>
          <Typography level="body-xs" sx={{ color: "#777", textTransform: "uppercase" }}>
            Ship to
          </Typography>
          <Typography level="title-md" sx={{ color: "#111" }}>
            {d.client_name}
          </Typography>
          <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#444", mb: 2 }}>
            {d.ship_to || d.client_address}
          </Typography>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ background: "#f5f5f5" }}>
                <th style={{ ...cell, textAlign: "left" }}>Item</th>
                <th style={{ ...cell, textAlign: "left" }}>HSN/SAC</th>
                <th style={{ ...cell, textAlign: "right" }}>Quantity</th>
              </tr>
            </thead>
            <tbody>
              {d.lines.map((l, i) => (
                <tr key={i}>
                  <td style={cell}>{l.description}</td>
                  <td style={cell}>{l.hsn_sac}</td>
                  <td style={{ ...cell, textAlign: "right" }}>
                    {l.qty} {l.unit}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {(d.carrier || d.tracking) && (
            <Typography level="body-sm" sx={{ mt: 2, color: "#333" }}>
              Carrier: {d.carrier || "-"} · Tracking: {d.tracking || "-"}
            </Typography>
          )}
          {d.notes && (
            <Typography level="body-sm" sx={{ mt: 1, color: "#333", whiteSpace: "pre-line" }}>
              {d.notes}
            </Typography>
          )}
          <Box sx={{ display: "flex", justifyContent: "space-between", mt: 6, color: "#555", fontSize: 13 }}>
            <span>Packed by: ______________</span>
            <span>Received by: ______________ Date: ________</span>
          </Box>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

export function DeliveriesView({ settings, version }: { settings: Settings; version: number }) {
  const [rows, setRows] = useState<Delivery[] | null>(null);
  const [slip, setSlip] = useState<Delivery | null>(null);
  useEffect(() => {
    listDeliveries().then(setRows).catch(() => setRows([]));
  }, [version]);
  return rows && rows.length === 0 ? (
    <Empty>No delivery notes yet. Create one from a confirmed sales order.</Empty>
  ) : (
    <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
      <Table hoverRow data-testid="dn-table">
        <thead>
          <tr>
            <th>Delivery note</th>
            <th>Order</th>
            <th>Customer</th>
            <th>Status</th>
            <th>Date</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {(rows ?? []).map((d) => (
            <tr key={d.id}>
              <td>{d.number}</td>
              <td>{d.order_number}</td>
              <td>{d.client_name}</td>
              <td>{d.status}</td>
              <td>{d.delivery_date || "-"}</td>
              <td style={rightCell}>
                <Button size="sm" variant="plain" onClick={() => setSlip(d)}>
                  Packing slip
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
      {slip && <PackingSlip delivery={slip} settings={settings} onClose={() => setSlip(null)} />}
    </Sheet>
  );
}
