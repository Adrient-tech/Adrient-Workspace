import { useEffect, useState } from "react";
import { Box, Button, Checkbox, IconButton, Input, Option, Select, Typography } from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import { Card, Field, Row, useErr, ErrorAlert } from "../hr/ui";
import { money } from "../ops/dates";
import { fs } from "./api";
import type { Customer, Discount, Item, Property, Service, Totals } from "./api";
import { blankItem, computeTotals } from "./logic";

/** Pick a customer from Contacts, or add a new one (it is created in Contacts). */
export function CustomerPicker({ value, name, onPick }: { value: string; name: string; onPick: (c: Customer) => void }) {
  const { err, run } = useErr();
  const [q, setQ] = useState("");
  const [found, setFound] = useState<Customer[]>([]);
  const [adding, setAdding] = useState(false);
  const [nc, setNc] = useState({ name: "", email: "", phone: "" });
  useEffect(() => {
    if (q.trim().length < 1) { setFound([]); return; }
    const h = window.setTimeout(() => void run(async () => setFound((await fs.customers(q)).slice(0, 6))), 200);
    return () => window.clearTimeout(h);
  }, [q, run]);
  return (
    <Box>
      <ErrorAlert err={err} />
      {value && <Typography level="body-sm" sx={{ mb: 0.5 }}>Customer: <b>{name}</b> (from Contacts)</Typography>}
      <Input placeholder="Search customers in Contacts" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search customers" } }} />
      <Row sx={{ mt: 0.5 }}>
        {found.map((c) => <Button key={c.id} size="sm" variant="soft" onClick={() => { onPick(c); setQ(""); setFound([]); }}>{c.name}{c.company ? ` · ${c.company}` : ""}</Button>)}
        <Button size="sm" variant="plain" onClick={() => setAdding(!adding)}>{adding ? "Cancel" : "New customer"}</Button>
      </Row>
      {adding && (
        <Row sx={{ mt: 0.5, alignItems: "flex-end" }}>
          <Field label="Name"><Input value={nc.name} onChange={(e) => setNc({ ...nc, name: e.target.value })} slotProps={{ input: { "aria-label": "New customer name" } }} /></Field>
          <Field label="Email"><Input value={nc.email} onChange={(e) => setNc({ ...nc, email: e.target.value })} /></Field>
          <Field label="Phone"><Input value={nc.phone} onChange={(e) => setNc({ ...nc, phone: e.target.value })} /></Field>
          <Button size="sm" disabled={!nc.name.trim()} onClick={() => void run(async () => { const c = await fs.createCustomer(nc); onPick(c); setAdding(false); setNc({ name: "", email: "", phone: "" }); })}>Add to Contacts</Button>
        </Row>
      )}
    </Box>
  );
}

/** Property select for the chosen customer, with a quick add. */
export function PropertyPicker({ contact, value, onPick }: { contact: string; value: string; onPick: (id: string) => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Property[]>([]);
  const [addr, setAddr] = useState("");
  useEffect(() => { if (contact) void run(async () => setList(await fs.properties(contact))); else setList([]); }, [contact, run]);
  if (!contact) return null;
  return (
    <Box>
      <ErrorAlert err={err} />
      <Field label="Property">
        <Select value={value} onChange={(_, v) => onPick(v ?? "")} slotProps={{ button: { "aria-label": "Property" } }} placeholder="No property">
          <Option value="">No property</Option>
          {list.map((p) => <Option key={p.id} value={p.id}>{p.address || p.name}</Option>)}
        </Select>
      </Field>
      <Row sx={{ mt: 0.5 }}>
        <Input size="sm" placeholder="Add an address" value={addr} onChange={(e) => setAddr(e.target.value)} slotProps={{ input: { "aria-label": "New property address" } }} />
        <Button size="sm" variant="outlined" disabled={!addr.trim()} onClick={() => void run(async () => { const p = await fs.saveProperty("", { contact_id: contact, name: addr.split(",")[0], address: addr }); setList([...list, p]); onPick(p.id); setAddr(""); })}>Add property</Button>
      </Row>
    </Box>
  );
}

/** Lines of a quote or job with catalogue shortcuts, optional add-ons and live totals. */
export function ItemsEditor({ items, onChange, discount, onDiscount, taxRate, onTax, currency, services }: {
  items: Item[]; onChange: (i: Item[]) => void; discount: Discount | null; onDiscount: (d: Discount | null) => void;
  taxRate: number; onTax: (n: number) => void; currency: string; services: Service[];
}) {
  const t: Totals = computeTotals(items, discount, taxRate);
  const set = (i: number, patch: Partial<Item>) => onChange(items.map((x, j) => (j === i ? { ...x, ...patch } : x)));
  return (
    <Card title="Lines">
      {items.map((it, i) => (
        <Row key={i} sx={{ alignItems: "flex-end", mb: 0.5 }}>
          <Field label={i === 0 ? "Item" : ""} sx={{ minWidth: 180 }}><Input size="sm" value={it.name} onChange={(e) => set(i, { name: e.target.value })} slotProps={{ input: { "aria-label": `Line ${i + 1} name` } }} /></Field>
          <Field label={i === 0 ? "Qty" : ""} sx={{ maxWidth: 80, minWidth: 70 }}><Input size="sm" type="number" value={it.qty} onChange={(e) => set(i, { qty: Number(e.target.value) })} slotProps={{ input: { "aria-label": `Line ${i + 1} quantity`, min: 0 } }} /></Field>
          <Field label={i === 0 ? "Price" : ""} sx={{ maxWidth: 110, minWidth: 90 }}><Input size="sm" type="number" value={it.unit_price} onChange={(e) => set(i, { unit_price: Number(e.target.value) })} slotProps={{ input: { "aria-label": `Line ${i + 1} price`, min: 0 } }} /></Field>
          <Field label={i === 0 ? "Cost" : ""} sx={{ maxWidth: 100, minWidth: 80 }}><Input size="sm" type="number" value={it.unit_cost} onChange={(e) => set(i, { unit_cost: Number(e.target.value) })} slotProps={{ input: { "aria-label": `Line ${i + 1} cost`, min: 0 } }} /></Field>
          <Checkbox size="sm" label="Tax" checked={it.taxable} onChange={(e) => set(i, { taxable: e.target.checked })} />
          <Checkbox size="sm" label="Optional" checked={!!it.optional} onChange={(e) => set(i, { optional: e.target.checked, selected: e.target.checked ? true : false })} />
          <IconButton size="sm" variant="plain" aria-label={`Remove line ${i + 1}`} onClick={() => onChange(items.filter((_, j) => j !== i))}><DeleteOutline /></IconButton>
        </Row>
      ))}
      <Row sx={{ mt: 0.5, alignItems: "center" }}>
        <Button size="sm" variant="outlined" onClick={() => onChange([...items, blankItem()])}>Add line</Button>
        {services.length > 0 && (
          <Select size="sm" placeholder="Add from catalogue" value={null} onChange={(_, v) => { const s = services.find((x) => x.id === v); if (s) onChange([...items, { name: s.name, description: s.description, qty: 1, unit_price: s.unit_price, unit_cost: s.unit_cost, taxable: s.taxable }]); }} slotProps={{ button: { "aria-label": "Add from catalogue" } }} sx={{ minWidth: 190 }}>
            {services.map((s) => <Option key={s.id} value={s.id}>{s.name} · {money(s.unit_price, currency)}</Option>)}
          </Select>
        )}
      </Row>
      <Row sx={{ mt: 1.5, alignItems: "flex-end" }}>
        <Field label="Discount" sx={{ maxWidth: 110 }}><Input size="sm" type="number" value={discount?.value ?? 0} onChange={(e) => onDiscount(Number(e.target.value) > 0 ? { kind: discount?.kind ?? "pct", value: Number(e.target.value) } : null)} slotProps={{ input: { "aria-label": "Discount", min: 0 } }} /></Field>
        <Select size="sm" value={discount?.kind ?? "pct"} onChange={(_, v) => discount && onDiscount({ ...discount, kind: (v ?? "pct") as "pct" | "amount" })} slotProps={{ button: { "aria-label": "Discount type" } }}><Option value="pct">%</Option><Option value="amount">{currency}</Option></Select>
        <Field label="Tax %" sx={{ maxWidth: 90 }}><Input size="sm" type="number" value={taxRate} onChange={(e) => onTax(Number(e.target.value))} slotProps={{ input: { "aria-label": "Tax percent", min: 0 } }} /></Field>
      </Row>
      <Typography level="body-sm" sx={{ mt: 1 }} data-testid="fs-totals">
        Subtotal {money(t.subtotal, currency)} · Discount {money(t.discount, currency)} · Tax {money(t.tax, currency)} · <b>Total {money(t.total, currency)}</b> · Est. profit {money(t.profit, currency)}
      </Typography>
    </Card>
  );
}
