import { useEffect, useState } from "react";
import { Autocomplete, Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import {
  createSubscription,
  deleteDiscountRule,
  deletePriceList,
  getAging,
  getStatement,
  listContacts,
  listDiscountRules,
  listPlans,
  listPriceLists,
  listProducts,
  listSubscriptions,
  portalLink,
  runSubscriptions,
  saveDiscountRule,
  savePlan,
  savePriceList,
  subscriptionAction,
} from "./api";
import type { AgingRow, ContactLite, DiscountRule, Plan, PriceList, ProductLite, Statement, Subscription } from "./api";
import { ACCENT, money, todayISO } from "./logic";
import type { Item, Settings } from "./types";
import { Empty, ErrorAlert, rightCell, useErr } from "./ui";

const btn = { bgcolor: ACCENT, "&:hover": { bgcolor: "#9A3412" } };

// ---- Pricing: price lists and discount rules ----

export function PricingView({ items }: { items: Item[] }) {
  const [lists, setLists] = useState<PriceList[]>([]);
  const [rules, setRules] = useState<DiscountRule[]>([]);
  const [products, setProducts] = useState<ProductLite[]>([]);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [editList, setEditList] = useState<PriceList | null>(null);
  const [editRule, setEditRule] = useState<DiscountRule | null>(null);
  const { err, run } = useErr();
  const load = () => {
    listPriceLists().then(setLists).catch(() => setLists([]));
    listDiscountRules().then(setRules).catch(() => setRules([]));
  };
  useEffect(() => {
    load();
    listProducts("").then(setProducts);
    listContacts().then(setContacts);
  }, []);
  const productName = (id: string) => products.find((p) => p.id === id)?.name ?? items.find((i) => i.id === id)?.name ?? id.slice(0, 8);
  const contactName = (id: string) => contacts.find((c) => c.id === id)?.company || contacts.find((c) => c.id === id)?.display_name || "";
  const options = [...products.map((p) => ({ id: p.id, name: p.name })), ...items.map((i) => ({ id: i.id, name: `${i.name} (price list item)` }))];

  return (
    <Box data-testid="pricing-view">
      <ErrorAlert err={err} />
      <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
        <Typography level="title-md" sx={{ flex: 1 }}>
          Price lists
        </Typography>
        <Button size="sm" sx={btn} data-testid="pl-new" onClick={() => setEditList({ id: "", name: "", currency: "INR", is_default: false, active: true, customers: [], rows: [] })}>
          New price list
        </Button>
      </Box>
      <Typography level="body-xs" sx={{ mb: 1 }}>
        Price for a customer is the first match of: a price set for that customer, a list assigned to them (or picked on the document), the default list, then the item's own price. Rows with a minimum quantity are volume tiers.
      </Typography>
      {lists.length === 0 ? (
        <Empty>No price lists yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 3 }}>
          <Table hoverRow data-testid="pl-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Rows</th>
                <th>Customers</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {lists.map((l) => (
                <tr key={l.id} style={{ cursor: "pointer" }} onClick={() => setEditList(l)} data-testid="pl-row">
                  <td>
                    {l.name} {l.is_default && <Chip size="sm" color="primary">default</Chip>}
                  </td>
                  <td>{l.rows.length}</td>
                  <td>{l.customers.map(contactName).filter(Boolean).join(", ") || "-"}</td>
                  <td>{l.active ? "Active" : "Off"}</td>
                  <td style={rightCell} onClick={(e) => e.stopPropagation()}>
                    <Button
                      size="sm"
                      variant="plain"
                      color="danger"
                      onClick={async () => {
                        if (window.confirm(`Delete price list ${l.name}?`)) {
                          await run(() => deletePriceList(l.id));
                          load();
                        }
                      }}
                    >
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}

      <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
        <Typography level="title-md" sx={{ flex: 1 }}>
          Discount rules
        </Typography>
        <Button size="sm" sx={btn} data-testid="dr-new" onClick={() => setEditRule({ id: "", name: "", scope: "line", kind: "percent", value: 10, min_qty: 0, min_amount: 0, product_id: "", contact_id: "", valid_from: "", valid_to: "", priority: 0, active: true })}>
          New rule
        </Button>
      </Box>
      <Typography level="body-xs" sx={{ mb: 1 }}>
        On a document, "Apply prices and discounts" applies the best matching rule per line and for the whole order. Rules never stack, and a bigger discount you typed is kept.
      </Typography>
      {rules.length === 0 ? (
        <Empty>No discount rules yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="dr-table">
            <thead>
              <tr>
                <th>Rule</th>
                <th>Applies to</th>
                <th>Discount</th>
                <th>When</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rules.map((r) => (
                <tr key={r.id} style={{ cursor: "pointer" }} onClick={() => setEditRule(r)}>
                  <td>{r.name}</td>
                  <td>{r.scope === "line" ? (r.product_id ? productName(r.product_id) : "Every line") : "Whole order"}</td>
                  <td>{r.kind === "percent" ? `${r.value}%` : money(r.value)}</td>
                  <td>
                    {[r.min_qty > 0 && `qty ≥ ${r.min_qty}`, r.min_amount > 0 && `amount ≥ ${r.min_amount}`, r.contact_id && contactName(r.contact_id), r.valid_from && `from ${r.valid_from}`, r.valid_to && `to ${r.valid_to}`].filter(Boolean).join(" · ") || "Always"}
                  </td>
                  <td>{r.active ? "Active" : "Off"}</td>
                  <td style={rightCell} onClick={(e) => e.stopPropagation()}>
                    <Button
                      size="sm"
                      variant="plain"
                      color="danger"
                      onClick={async () => {
                        await run(() => deleteDiscountRule(r.id));
                        load();
                      }}
                    >
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}

      {editList && (
        <Modal open onClose={() => setEditList(null)}>
          <ModalDialog sx={{ width: "min(820px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="pl-editor">
            <ModalClose />
            <Typography level="h4">{editList.id ? "Edit price list" : "New price list"}</Typography>
            <ErrorAlert err={err} />
            <Box sx={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 1 }}>
              <FormControl>
                <FormLabel>Name</FormLabel>
                <Input value={editList.name} onChange={(e) => setEditList({ ...editList, name: e.target.value })} slotProps={{ input: { "data-testid": "pl-name" } }} />
              </FormControl>
              <FormControl>
                <FormLabel>Currency</FormLabel>
                <Input value={editList.currency} onChange={(e) => setEditList({ ...editList, currency: e.target.value.toUpperCase().slice(0, 3) })} />
              </FormControl>
            </Box>
            <Box sx={{ display: "flex", gap: 3 }}>
              <Checkbox label="Default list (used for everyone without a closer match)" checked={editList.is_default} onChange={(e) => setEditList({ ...editList, is_default: e.target.checked })} />
              <Checkbox label="Active" checked={editList.active} onChange={(e) => setEditList({ ...editList, active: e.target.checked })} />
            </Box>
            <FormControl>
              <FormLabel>Customers who get this list</FormLabel>
              <Autocomplete
                multiple
                options={contacts}
                getOptionLabel={(c) => c.company || c.display_name}
                value={contacts.filter((c) => editList.customers.includes(c.id))}
                onChange={(_, v) => setEditList({ ...editList, customers: v.map((c) => c.id) })}
              />
            </FormControl>
            <Typography level="title-sm">Prices</Typography>
            {editList.rows.map((r, i) => (
              <Box key={i} sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }} data-testid="pl-rowedit">
                <Autocomplete
                  size="sm"
                  sx={{ flex: 2, minWidth: 200 }}
                  placeholder="Product"
                  options={options}
                  getOptionLabel={(o) => o.name}
                  value={options.find((o) => o.id === r.product_id) ?? null}
                  onChange={(_, v) => setEditList({ ...editList, rows: editList.rows.map((x, j) => (j === i ? { ...x, product_id: v?.id ?? "" } : x)) })}
                  slotProps={{ input: { "data-testid": "pl-row-product" } }}
                />
                <Input size="sm" type="number" sx={{ width: 90 }} placeholder="Min qty" value={r.min_qty} onChange={(e) => setEditList({ ...editList, rows: editList.rows.map((x, j) => (j === i ? { ...x, min_qty: Number(e.target.value) } : x)) })} slotProps={{ input: { "data-testid": "pl-row-minqty" } }} />
                <Input size="sm" type="number" sx={{ width: 110 }} placeholder="Price" value={r.price} onChange={(e) => setEditList({ ...editList, rows: editList.rows.map((x, j) => (j === i ? { ...x, price: Number(e.target.value) } : x)) })} slotProps={{ input: { "data-testid": "pl-row-price" } }} />
                <Select
                  size="sm"
                  sx={{ width: 170 }}
                  value={r.contact_id || ""}
                  onChange={(_, v) => setEditList({ ...editList, rows: editList.rows.map((x, j) => (j === i ? { ...x, contact_id: (v as string) || "" } : x)) })}
                >
                  <Option value="">Everyone on the list</Option>
                  {contacts.map((c) => (
                    <Option key={c.id} value={c.id}>
                      Only {c.company || c.display_name}
                    </Option>
                  ))}
                </Select>
                <Button size="sm" variant="plain" color="danger" onClick={() => setEditList({ ...editList, rows: editList.rows.filter((_, j) => j !== i) })}>
                  Remove
                </Button>
              </Box>
            ))}
            <Box>
              <Button size="sm" variant="outlined" onClick={() => setEditList({ ...editList, rows: [...editList.rows, { product_id: "", contact_id: "", min_qty: 1, price: 0 }] })} data-testid="pl-add-row">
                + Price
              </Button>
            </Box>
            <Button
              data-testid="pl-save"
              onClick={async () => {
                const out = await run(() => savePriceList(editList.id, { ...editList, rows: editList.rows.filter((r) => r.product_id) }));
                if (out) {
                  setEditList(null);
                  load();
                }
              }}
            >
              Save price list
            </Button>
          </ModalDialog>
        </Modal>
      )}

      {editRule && (
        <Modal open onClose={() => setEditRule(null)}>
          <ModalDialog sx={{ width: "min(620px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="dr-editor">
            <ModalClose />
            <Typography level="h4">{editRule.id ? "Edit discount rule" : "New discount rule"}</Typography>
            <ErrorAlert err={err} />
            <FormControl>
              <FormLabel>Name</FormLabel>
              <Input value={editRule.name} onChange={(e) => setEditRule({ ...editRule, name: e.target.value })} slotProps={{ input: { "data-testid": "dr-name" } }} />
            </FormControl>
            <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1 }}>
              <FormControl>
                <FormLabel>Applies to</FormLabel>
                <Select value={editRule.scope} onChange={(_, v) => setEditRule({ ...editRule, scope: v as "line" | "order", kind: v === "line" ? "percent" : editRule.kind })}>
                  <Option value="line">Each line</Option>
                  <Option value="order">Whole order</Option>
                </Select>
              </FormControl>
              <FormControl>
                <FormLabel>Type</FormLabel>
                <Select value={editRule.kind} onChange={(_, v) => setEditRule({ ...editRule, kind: v as "percent" | "amount" })} disabled={editRule.scope === "line"}>
                  <Option value="percent">Percent</Option>
                  <Option value="amount">Amount</Option>
                </Select>
              </FormControl>
              <FormControl>
                <FormLabel>Value</FormLabel>
                <Input type="number" value={editRule.value} onChange={(e) => setEditRule({ ...editRule, value: Number(e.target.value) })} slotProps={{ input: { "data-testid": "dr-value" } }} />
              </FormControl>
              {editRule.scope === "line" ? (
                <FormControl>
                  <FormLabel>Minimum quantity</FormLabel>
                  <Input type="number" value={editRule.min_qty} onChange={(e) => setEditRule({ ...editRule, min_qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "dr-minqty" } }} />
                </FormControl>
              ) : (
                <FormControl>
                  <FormLabel>Minimum order amount</FormLabel>
                  <Input type="number" value={editRule.min_amount} onChange={(e) => setEditRule({ ...editRule, min_amount: Number(e.target.value) })} />
                </FormControl>
              )}
              <FormControl>
                <FormLabel>Valid from</FormLabel>
                <Input type="date" value={editRule.valid_from} onChange={(e) => setEditRule({ ...editRule, valid_from: e.target.value })} />
              </FormControl>
              <FormControl>
                <FormLabel>Valid to</FormLabel>
                <Input type="date" value={editRule.valid_to} onChange={(e) => setEditRule({ ...editRule, valid_to: e.target.value })} />
              </FormControl>
            </Box>
            {editRule.scope === "line" && (
              <FormControl>
                <FormLabel>Only this product (empty = every line)</FormLabel>
                <Autocomplete
                  options={options}
                  getOptionLabel={(o) => o.name}
                  value={options.find((o) => o.id === editRule.product_id) ?? null}
                  onChange={(_, v) => setEditRule({ ...editRule, product_id: v?.id ?? "" })}
                />
              </FormControl>
            )}
            <FormControl>
              <FormLabel>Only this customer (empty = everyone)</FormLabel>
              <Autocomplete
                options={contacts}
                getOptionLabel={(c) => c.company || c.display_name}
                value={contacts.find((c) => c.id === editRule.contact_id) ?? null}
                onChange={(_, v) => setEditRule({ ...editRule, contact_id: v?.id ?? "" })}
              />
            </FormControl>
            <Box sx={{ display: "flex", gap: 2, alignItems: "end" }}>
              <FormControl sx={{ width: 120 }}>
                <FormLabel>Priority</FormLabel>
                <Input type="number" value={editRule.priority} onChange={(e) => setEditRule({ ...editRule, priority: Number(e.target.value) })} />
              </FormControl>
              <Checkbox label="Active" checked={editRule.active} onChange={(e) => setEditRule({ ...editRule, active: e.target.checked })} />
            </Box>
            <Button
              data-testid="dr-save"
              onClick={async () => {
                const out = await run(() => saveDiscountRule(editRule.id, editRule));
                if (out) {
                  setEditRule(null);
                  load();
                }
              }}
            >
              Save rule
            </Button>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---- Customers: aging and statements ----

export function AgingView({ settings, version }: { settings: Settings; version: number }) {
  const [rows, setRows] = useState<AgingRow[] | null>(null);
  const [st, setSt] = useState<Statement | null>(null);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [cur, setCur] = useState("");
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const m = (n: number) => money(n, settings.default_currency);
  useEffect(() => {
    getAging().then(setRows).catch(() => setRows([]));
  }, [version]);
  const open = async (key: string, f = from, t = to) => {
    const s = await run(() => getStatement(key, f, t));
    if (s) {
      setSt(s);
      setCur(key);
    }
  };
  const total = (k: keyof AgingRow) => (rows ?? []).reduce((s, r) => s + (r[k] as number), 0);
  return (
    <Box data-testid="aging-view">
      <ErrorAlert err={err} />
      {note && (
        <Typography level="body-sm" color="success" data-testid="aging-note">
          {note}
        </Typography>
      )}
      {rows && rows.length === 0 ? (
        <Empty>Nothing owed. Open balances by customer appear here once invoices are sent.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="aging-table">
            <thead>
              <tr>
                <th>Customer</th>
                <th style={rightCell}>Not due</th>
                <th style={rightCell}>1-30</th>
                <th style={rightCell}>31-60</th>
                <th style={rightCell}>61-90</th>
                <th style={rightCell}>90+</th>
                <th style={rightCell}>Credit</th>
                <th style={rightCell}>Owed</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(rows ?? []).map((r) => (
                <tr key={r.customer_key} data-testid="aging-row">
                  <td>{r.customer}</td>
                  <td style={rightCell}>{m(r.current)}</td>
                  <td style={rightCell}>{m(r.d1_30)}</td>
                  <td style={rightCell}>{m(r.d31_60)}</td>
                  <td style={rightCell}>{m(r.d61_90)}</td>
                  <td style={{ ...rightCell, color: r.d90_plus > 0 ? "#B91C1C" : undefined }}>{m(r.d90_plus)}</td>
                  <td style={rightCell}>{r.unapplied_credit ? `-${m(r.unapplied_credit)}` : ""}</td>
                  <td style={{ ...rightCell, fontWeight: 700 }}>{m(r.total)}</td>
                  <td style={rightCell}>
                    <Button size="sm" variant="plain" onClick={() => open(r.customer_key)} data-testid="aging-statement">
                      Statement
                    </Button>
                    <Button
                      size="sm"
                      variant="plain"
                      onClick={async () => {
                        const p = await run(() => portalLink(r.customer_key));
                        if (p) {
                          const url = `${window.location.origin}${p.path}`;
                          await navigator.clipboard?.writeText(url).catch(() => undefined);
                          setNote(`Portal link for ${r.customer} copied: ${url}`);
                        }
                      }}
                    >
                      Portal link
                    </Button>
                  </td>
                </tr>
              ))}
              <tr style={{ fontWeight: 700 }}>
                <td>Total</td>
                {(["current", "d1_30", "d31_60", "d61_90", "d90_plus"] as const).map((k) => (
                  <td key={k} style={rightCell}>
                    {m(total(k))}
                  </td>
                ))}
                <td style={rightCell}>{total("unapplied_credit") ? `-${m(total("unapplied_credit"))}` : ""}</td>
                <td style={rightCell}>{m(total("total"))}</td>
                <td />
              </tr>
            </tbody>
          </Table>
        </Sheet>
      )}
      {st && (
        <Modal open onClose={() => setSt(null)}>
          <ModalDialog sx={{ width: "min(860px, 96vw)", maxHeight: "94vh", overflow: "auto", bgcolor: "#f3f4f6" }} data-testid="statement-dialog">
            <ModalClose />
            <style>{`@media print { body * { visibility: hidden !important; } .invc-sheet, .invc-sheet * { visibility: visible !important; } .invc-sheet { position: absolute !important; inset: 0 auto auto 0 !important; width: 100% !important; box-shadow: none !important; border: 0 !important; } .invc-noprint { display: none !important; } @page { margin: 12mm; } }`}</style>
            <Box className="invc-noprint" sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap" }}>
              <FormControl size="sm">
                <FormLabel>From</FormLabel>
                <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
              </FormControl>
              <FormControl size="sm">
                <FormLabel>To</FormLabel>
                <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
              </FormControl>
              <Button size="sm" variant="outlined" onClick={() => open(cur, from, to)}>
                Update
              </Button>
              <Button size="sm" onClick={() => window.print()} data-testid="statement-print">
                Print / save as PDF
              </Button>
            </Box>
            <Box className="invc-sheet" data-testid="statement-sheet" sx={{ bgcolor: "#fff", color: "#111", p: 4, border: "1px solid #ddd", borderRadius: "8px" }}>
              <Typography level="title-lg" sx={{ color: "#111" }}>
                {settings.business_name}
              </Typography>
              <Typography level="h3" sx={{ color: settings.sales?.pdf_accent || ACCENT, mt: 1 }}>
                STATEMENT OF ACCOUNT
              </Typography>
              <Typography level="body-md" sx={{ color: "#111" }}>
                {st.customer}
              </Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#444" }}>
                {st.address}
              </Typography>
              <Typography level="body-sm" sx={{ color: "#444", mb: 2 }}>
                {st.from || "Start"} to {st.to || todayISO()} · Currency {st.currency}
              </Typography>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                <thead>
                  <tr style={{ background: "#f5f5f5" }}>
                    {["Date", "Document", "Details", "Debit", "Credit", "Balance"].map((h, i) => (
                      <th key={h} style={{ padding: "6px 8px", textAlign: i > 2 ? "right" : "left" }}>
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {st.opening !== 0 && (
                    <tr>
                      <td colSpan={5} style={{ padding: "6px 8px" }}>
                        Opening balance
                      </td>
                      <td style={{ padding: "6px 8px", textAlign: "right" }}>{money(st.opening, st.currency)}</td>
                    </tr>
                  )}
                  {st.lines.map((l, i) => (
                    <tr key={i} data-testid="statement-line">
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee" }}>{l.date}</td>
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee" }}>{l.number}</td>
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee" }}>{l.description}</td>
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee", textAlign: "right" }}>{l.debit ? money(l.debit, st.currency) : ""}</td>
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee", textAlign: "right" }}>{l.credit ? money(l.credit, st.currency) : ""}</td>
                      <td style={{ padding: "6px 8px", borderBottom: "1px solid #eee", textAlign: "right" }}>{money(l.balance, st.currency)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <Box sx={{ textAlign: "right", mt: 2, color: "#111" }}>
                <Typography level="title-md" sx={{ color: "#111" }} data-testid="statement-closing">
                  Balance due {money(st.closing, st.currency)}
                </Typography>
                {st.overdue > 0 && (
                  <Typography level="body-sm" sx={{ color: "#B91C1C" }}>
                    Of which overdue {money(st.overdue, st.currency)}
                  </Typography>
                )}
              </Box>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---- Subscriptions ----

export function SubscriptionsView({ settings, version, onChanged }: { settings: Settings; version: number; onChanged: () => void }) {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [subs, setSubs] = useState<Subscription[]>([]);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [sub, setSub] = useState<Partial<Subscription> | null>(null);
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const load = () => {
    listPlans().then(setPlans).catch(() => setPlans([]));
    listSubscriptions().then(setSubs).catch(() => setSubs([]));
  };
  useEffect(() => {
    load();
    listContacts().then(setContacts);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version]);
  const chip = (s: Subscription["status"]) => (
    <Chip size="sm" variant="soft" color={s === "active" ? "success" : s === "past_due" ? "warning" : s === "paused" ? "danger" : "neutral"}>
      {s.replace("_", " ")}
    </Chip>
  );
  return (
    <Box data-testid="subs-view">
      <ErrorAlert err={err} />
      {note && (
        <Typography level="body-sm" color="success" data-testid="subs-note" sx={{ mb: 1 }}>
          {note}
        </Typography>
      )}
      <Box sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center", flexWrap: "wrap" }}>
        <Typography level="title-md" sx={{ flex: 1 }}>
          Subscriptions
        </Typography>
        <Button
          size="sm"
          variant="outlined"
          data-testid="subs-run"
          onClick={async () => {
            const r = await run(() => runSubscriptions());
            if (r) {
              setNote(`Renewals run: ${r.invoiced.length} invoice(s) created, ${r.past_due} marked past due, ${r.paused} paused, ${r.resumed} back to active.`);
              load();
              onChanged();
            }
          }}
        >
          Run renewals now
        </Button>
        <Button size="sm" sx={btn} disabled={plans.filter((p) => p.active).length === 0} onClick={() => setSub({ quantity: 1, start_date: todayISO(), dunning_days: 7, client_name: "", client_email: "" })} data-testid="subs-new">
          New subscription
        </Button>
      </Box>
      {subs.length === 0 ? (
        <Empty>No subscriptions yet. Create a plan below, then subscribe a customer. Renewals invoice automatically every 15 minutes.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 3 }}>
          <Table data-testid="subs-table">
            <thead>
              <tr>
                <th>Customer</th>
                <th>Plan</th>
                <th>Status</th>
                <th>Next renewal</th>
                <th style={rightCell}>Renewals</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {subs.map((s) => (
                <tr key={s.id} data-testid="subs-row">
                  <td>{s.client_name}</td>
                  <td>
                    {s.plan_name} · {money(s.price * s.quantity, settings.default_currency)} / {s.interval}
                  </td>
                  <td>
                    {chip(s.status)} {s.cancel_at_period_end && <Chip size="sm">ends after this period</Chip>}
                  </td>
                  <td>{s.status === "cancelled" ? "-" : s.next_renewal}</td>
                  <td style={rightCell}>{s.renewals}</td>
                  <td style={rightCell}>
                    {s.status !== "cancelled" && (
                      <>
                        {s.status === "paused" ? (
                          <Button size="sm" variant="plain" onClick={async () => { await run(() => subscriptionAction(s.id, "resume")); load(); }}>
                            Resume
                          </Button>
                        ) : (
                          <Button size="sm" variant="plain" onClick={async () => { await run(() => subscriptionAction(s.id, "pause")); load(); }}>
                            Pause
                          </Button>
                        )}
                        <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Cancel ${s.client_name}'s subscription now?`)) { await run(() => subscriptionAction(s.id, "cancel")); load(); } }} data-testid="subs-cancel">
                          Cancel
                        </Button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Box sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center" }}>
        <Typography level="title-md" sx={{ flex: 1 }}>
          Plans
        </Typography>
        <Button size="sm" variant="outlined" onClick={() => setPlan({ id: "", name: "", description: "", interval: "monthly", price: 0, tax_rate: 0, hsn_sac: "", currency: settings.default_currency, active: true })} data-testid="plan-new">
          New plan
        </Button>
      </Box>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table hoverRow data-testid="plans-table">
          <tbody>
            {plans.map((p) => (
              <tr key={p.id} style={{ cursor: "pointer" }} onClick={() => setPlan(p)}>
                <td>{p.name}</td>
                <td>
                  {money(p.price, p.currency)} / {p.interval}
                </td>
                <td>{p.tax_rate ? `${p.tax_rate}% tax` : "no tax"}</td>
                <td>{p.active ? "Active" : "Off"}</td>
              </tr>
            ))}
            {plans.length === 0 && (
              <tr>
                <td>No plans yet.</td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>

      {plan && (
        <Modal open onClose={() => setPlan(null)}>
          <ModalDialog sx={{ width: 460 }} data-testid="plan-editor">
            <ModalClose />
            <Typography level="title-lg">{plan.id ? "Edit plan" : "New plan"}</Typography>
            <ErrorAlert err={err} />
            <Input placeholder="Name" value={plan.name} onChange={(e) => setPlan({ ...plan, name: e.target.value })} slotProps={{ input: { "data-testid": "plan-name" } }} />
            <Input placeholder="Description" value={plan.description} onChange={(e) => setPlan({ ...plan, description: e.target.value })} />
            <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
              <Select value={plan.interval} onChange={(_, v) => setPlan({ ...plan, interval: v as Plan["interval"] })}>
                {["weekly", "monthly", "quarterly", "yearly"].map((x) => (
                  <Option key={x} value={x}>
                    {x}
                  </Option>
                ))}
              </Select>
              <Input type="number" placeholder="Price" value={plan.price} onChange={(e) => setPlan({ ...plan, price: Number(e.target.value) })} slotProps={{ input: { "data-testid": "plan-price" } }} />
              <Input type="number" placeholder="Tax %" value={plan.tax_rate} onChange={(e) => setPlan({ ...plan, tax_rate: Number(e.target.value) })} />
              <Input placeholder="HSN/SAC" value={plan.hsn_sac} onChange={(e) => setPlan({ ...plan, hsn_sac: e.target.value })} />
            </Box>
            <Checkbox label="Active (open for new subscriptions)" checked={plan.active} onChange={(e) => setPlan({ ...plan, active: e.target.checked })} />
            <Button
              data-testid="plan-save"
              onClick={async () => {
                if (await run(() => savePlan(plan.id, plan))) {
                  setPlan(null);
                  load();
                }
              }}
            >
              Save plan
            </Button>
          </ModalDialog>
        </Modal>
      )}
      {sub && (
        <Modal open onClose={() => setSub(null)}>
          <ModalDialog sx={{ width: 460 }} data-testid="sub-editor">
            <ModalClose />
            <Typography level="title-lg">New subscription</Typography>
            <ErrorAlert err={err} />
            <Select placeholder="Plan" value={sub.plan_id ?? null} onChange={(_, v) => setSub({ ...sub, plan_id: v as string })} data-testid="sub-plan">
              {plans
                .filter((p) => p.active)
                .map((p) => (
                  <Option key={p.id} value={p.id}>
                    {p.name} · {money(p.price, p.currency)} / {p.interval}
                  </Option>
                ))}
            </Select>
            <Select
              placeholder="Customer (from Contacts)"
              value={sub.contact_id ?? null}
              onChange={(_, v) => {
                const c = contacts.find((x) => x.id === v);
                if (c) setSub({ ...sub, contact_id: c.id, client_name: c.company || c.display_name, client_email: c.emails?.[0] ?? "" });
              }}
            >
              {contacts.map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.company || c.display_name}
                </Option>
              ))}
            </Select>
            <Input placeholder="Customer name" value={sub.client_name ?? ""} onChange={(e) => setSub({ ...sub, client_name: e.target.value })} slotProps={{ input: { "data-testid": "sub-client" } }} />
            <Input placeholder="Email" value={sub.client_email ?? ""} onChange={(e) => setSub({ ...sub, client_email: e.target.value })} />
            <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1 }}>
              <FormControl>
                <FormLabel>Start</FormLabel>
                <Input type="date" value={sub.start_date ?? ""} onChange={(e) => setSub({ ...sub, start_date: e.target.value })} slotProps={{ input: { "data-testid": "sub-start" } }} />
              </FormControl>
              <FormControl>
                <FormLabel>Quantity</FormLabel>
                <Input type="number" value={sub.quantity ?? 1} onChange={(e) => setSub({ ...sub, quantity: Number(e.target.value) })} />
              </FormControl>
              <FormControl>
                <FormLabel>Pause after (days overdue)</FormLabel>
                <Input type="number" value={sub.dunning_days ?? 7} onChange={(e) => setSub({ ...sub, dunning_days: Number(e.target.value) })} />
              </FormControl>
            </Box>
            <Button
              data-testid="sub-save"
              onClick={async () => {
                if (await run(() => createSubscription(sub))) {
                  setSub(null);
                  load();
                }
              }}
            >
              Subscribe
            </Button>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}
