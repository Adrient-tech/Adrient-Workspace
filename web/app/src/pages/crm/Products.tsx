import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Stack, Table, Typography } from "@mui/joy";
import * as api2 from "./api2";
import { money } from "./logic";
import type { DealItem } from "./types";
import { ConfirmButton, Dialog, Empty, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

interface PriceItem {
  id: string;
  inv_product_id: string;
  name: string;
  sku: string;
  price: number;
  currency: string;
  active: boolean;
}

/** Product lines on a deal; picks from the price book. The deal value becomes the line total. */
export function DealItemsEditor({ items, currency, onChange }: { items: DealItem[]; currency: string; onChange: (i: DealItem[]) => void }) {
  const [book, setBook] = useState<PriceItem[]>([]);
  useEffect(() => {
    void api2.list<PriceItem>("pricebook", "items").then((b) => setBook(b.filter((x) => x.active))).catch(() => undefined);
  }, []);
  const total = items.reduce((a, i) => a + Math.round(i.qty * i.unit_price * (1 - i.discount_pct / 100) * 100) / 100, 0);
  const upd = (i: number, p: Partial<DealItem>) => onChange(items.map((x, j) => (j === i ? { ...x, ...p } : x)));
  return (
    <FormControl data-testid="crm-deal-items-editor">
      <FormLabel>Products</FormLabel>
      <Stack gap={0.5}>
        {items.map((it, i) => (
          <Stack key={i} direction="row" gap={0.5} alignItems="center">
            <Input size="sm" sx={{ flex: 2 }} value={it.name} placeholder="Product" onChange={(e) => upd(i, { name: e.target.value })} slotProps={{ input: { "aria-label": "Product name" } }} />
            <Input size="sm" sx={{ width: 70 }} type="number" value={it.qty} onChange={(e) => upd(i, { qty: Number(e.target.value) })} slotProps={{ input: { min: 0, step: "any", "aria-label": "Quantity" } }} />
            <Input size="sm" sx={{ width: 100 }} type="number" value={it.unit_price} onChange={(e) => upd(i, { unit_price: Number(e.target.value) })} slotProps={{ input: { min: 0, "aria-label": "Unit price" } }} />
            <Input size="sm" sx={{ width: 70 }} type="number" value={it.discount_pct} onChange={(e) => upd(i, { discount_pct: Number(e.target.value) })} slotProps={{ input: { min: 0, max: 100, "aria-label": "Discount percent" } }} endDecorator="%" />
            <IconButton size="sm" aria-label="Remove product" onClick={() => onChange(items.filter((_, j) => j !== i))}>
              ×
            </IconButton>
          </Stack>
        ))}
        <Stack direction="row" gap={1} alignItems="center">
          <Select size="sm" placeholder="Add from price book" value={null} onChange={(_, id) => {
            const p = book.find((b) => b.id === id);
            if (p) onChange([...items, { product_id: p.id, name: p.name, qty: 1, unit_price: p.price, discount_pct: 0 }]);
          }} sx={{ width: 220 }} slotProps={{ button: { "data-testid": "crm-deal-add-product" } as never }}>
            {book.map((b) => (
              <Option key={b.id} value={b.id}>
                {b.name} · {money(b.price, b.currency)}
              </Option>
            ))}
          </Select>
          <Button size="sm" variant="plain" onClick={() => onChange([...items, { product_id: "", name: "", qty: 1, unit_price: 0, discount_pct: 0 }])}>
            Custom line
          </Button>
          {items.length > 0 && <Typography level="body-sm">Total {money(total, currency)} (sets the deal value)</Typography>}
        </Stack>
      </Stack>
    </FormControl>
  );
}

export function Products({ data }: { data: Data }) {
  const [items, setItems] = useState<PriceItem[]>([]);
  const [editing, setEditing] = useState<PriceItem | "new" | null>(null);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const admin = data.me.is_admin;
  const load = useCallback(() => {
    void run(() => api2.list<PriceItem>("pricebook", "items")).then((l) => l && setItems(l));
  }, [run]);
  useEffect(load, [load]);
  return (
    <Box>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>
          Products and price book
        </Typography>
        {admin && (
          <>
            <Button
              size="sm"
              variant="outlined"
              data-testid="crm-sync-inventory"
              onClick={async () => {
                const r = await run(() => api2.post<{ added: number; updated: number }>("pricebook/sync"));
                if (r) {
                  setMsg(`Inventory synced: ${r.added} added, ${r.updated} updated.`);
                  load();
                }
              }}
            >
              Sync from Inventory
            </Button>
            <Button size="sm" onClick={() => setEditing("new")} data-testid="crm-new-product">
              New product
            </Button>
          </>
        )}
      </Stack>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Products here can be added to deals. Items synced from Inventory keep their link, so prices refresh when you sync again.
      </Typography>
      {msg && <Typography level="body-sm" color="success" data-testid="crm-product-msg">{msg}</Typography>}
      <ErrorAlert err={err} />
      {items.length === 0 ? (
        <Empty>No products yet. {admin ? "Sync from Inventory or add one." : "An administrator can add products."}</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-price-book">
            <thead>
              <tr>
                <th>Name</th>
                <th>SKU</th>
                <th style={{ textAlign: "right" }}>Price</th>
                <th>Source</th>
                <th>Active</th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.id} onClick={() => admin && setEditing(p)} style={{ cursor: admin ? "pointer" : undefined }}>
                  <td>{p.name}</td>
                  <td>{p.sku}</td>
                  <td style={{ textAlign: "right" }}>{money(p.price, p.currency)}</td>
                  <td>{p.inv_product_id ? "Inventory" : "CRM"}</td>
                  <td>{p.active ? "Yes" : "No"}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {editing && <ProductDialog item={editing === "new" ? null : editing} currency={data.settings.currency} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </Box>
  );
}

function ProductDialog({ item, currency, onClose, onSaved }: { item: PriceItem | null; currency: string; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ name: item?.name ?? "", sku: item?.sku ?? "", price: item?.price ?? 0, active: item?.active ?? true });
  const { err, run } = useErr();
  return (
    <Dialog title={item ? "Edit product" : "New product"} onClose={onClose} width={440}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>Name</FormLabel>
          <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-product-name" } }} />
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>SKU</FormLabel>
            <Input value={f.sku} onChange={(e) => setF({ ...f, sku: e.target.value })} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Price ({item?.currency ?? currency})</FormLabel>
            <Input type="number" value={f.price} onChange={(e) => setF({ ...f, price: Number(e.target.value) })} slotProps={{ input: { min: 0, "data-testid": "crm-product-price" } }} />
          </FormControl>
        </Stack>
        <Checkbox label="Active (can be added to deals)" checked={f.active} onChange={(e) => setF({ ...f, active: e.target.checked })} />
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {item ? <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("pricebook", item.id))) onSaved(); }} /> : <span />}
          <Button
            disabled={!f.name.trim()}
            data-testid="crm-product-save"
            onClick={async () => {
              if (await run(() => api2.save("pricebook", item?.id ?? "", { ...f, currency: item?.currency ?? currency, inv_product_id: item?.inv_product_id ?? "" }))) onSaved();
            }}
          >
            Save
          </Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}
