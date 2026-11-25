import { useEffect, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
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
  adjustStock,
  archiveProduct,
  createProduct,
  deleteProduct,
  getProduct,
  listHistory,
  photoURL,
  setProductSuppliers,
  transferStock,
  updateProduct,
  uploadPhoto,
} from "./api";
import { barcodeSvg, generateBarcode, money, qtyText, stockStatus } from "./logic";
import { ScanDialog } from "./ScanDialog";
import { ShelfCell, StockSettings } from "./ProductStockSettings";
import type {
  InvHistoryEntry,
  InvLocation,
  InvProduct,
  InvProductSupplier,
  InvSupplier,
  ProductInput,
  StockReason,
} from "./types";

interface Props {
  open: boolean;
  /** Existing product to edit, or null to create. */
  product: InvProduct | null;
  /** When creating, make this a variant of the given family. */
  family?: InvProduct | null;
  locations: InvLocation[];
  suppliers: InvSupplier[];
  categories: string[];
  onClose: () => void;
  onChanged: () => void;
  /** Open another product (used for variants). */
  onOpenProduct: (p: InvProduct) => void;
  onAddVariant: (family: InvProduct) => void;
}

const blank: ProductInput = {
  name: "",
  sku: "",
  barcode: "",
  category: "",
  unit: "pcs",
  price: 0,
  cost: 0,
  min_stock: 0,
  reorder_qty: 0,
  description: "",
  photo_file_id: "",
  variant_label: "",
  opening_qty: 0,
};

function fromProduct(p: InvProduct): ProductInput {
  return {
    name: p.name,
    sku: p.sku,
    barcode: p.barcode,
    category: p.category,
    unit: p.unit,
    price: p.price,
    cost: p.cost,
    min_stock: p.min_stock,
    reorder_qty: p.reorder_qty,
    description: p.description,
    photo_file_id: p.photo_file_id,
    variant_label: p.variant_label,
    parent_id: p.parent_id,
  };
}

type Action = StockReason | "transfer";

const numVal = (s: string): number => (s === "" ? 0 : Number(s));

export function ProductDialog(props: Props) {
  const { open, product, family, locations, suppliers, categories, onClose, onChanged } = props;
  const [form, setForm] = useState<ProductInput>(blank);
  const [current, setCurrent] = useState<InvProduct | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [scan, setScan] = useState(false);
  const [history, setHistory] = useState<InvHistoryEntry[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);

  // Stock adjustment form.
  const [reason, setReason] = useState<Action>("receive");
  const [adjLoc, setAdjLoc] = useState("");
  const [adjQty, setAdjQty] = useState("");
  const [adjNote, setAdjNote] = useState("");
  const [toLoc, setToLoc] = useState("");

  // Supplier source form.
  const [supId, setSupId] = useState("");
  const [supSku, setSupSku] = useState("");
  const [supCost, setSupCost] = useState("");

  const isVariant = Boolean(current?.parent_id || family);
  const defLoc = locations.find((l) => l.is_default)?.id ?? locations[0]?.id ?? "";

  useEffect(() => {
    if (!open) return;
    setError("");
    setCurrent(product);
    if (product) setForm(fromProduct(product));
    else if (family)
      setForm({
        ...blank,
        name: family.name,
        category: family.category,
        unit: family.unit,
        price: family.price,
        cost: family.cost,
        parent_id: family.id,
      });
    else setForm(blank);
    setAdjLoc(defLoc);
    setAdjQty("");
    setAdjNote("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, product, family]);

  useEffect(() => {
    if (!open || !current) {
      setHistory([]);
      return;
    }
    listHistory({ product_id: current.id, limit: 10 })
      .then(setHistory)
      .catch(() => setHistory([]));
  }, [open, current]);

  const set = <K extends keyof ProductInput>(k: K, v: ProductInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  async function run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    setBusy(true);
    setError("");
    try {
      return await fn();
    } catch (e) {
      setError((e as Error).message);
      return undefined;
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (!form.name.trim()) return setError("Name is required.");
    const saved = await run(() =>
      current ? updateProduct(current.id, form) : createProduct({ ...form, opening_location_id: adjLoc }),
    );
    if (saved) {
      setCurrent(saved);
      onChanged();
    }
  }

  async function refresh(p: InvProduct) {
    setCurrent(p);
    onChanged();
  }

  async function doAdjust() {
    if (!current) return;
    const q = Number(adjQty);
    if (!adjQty || Number.isNaN(q)) return setError("Enter a quantity.");
    if (reason === "transfer") return;
    const p = await run(() =>
      adjustStock(current.id, {
        location_id: adjLoc,
        qty: q,
        absolute: reason === "count",
        reason,
        note: adjNote,
      }),
    );
    if (p) {
      setAdjQty("");
      setAdjNote("");
      refresh(p);
    }
  }

  async function doTransfer() {
    if (!current) return;
    const q = Number(adjQty);
    if (!q || q <= 0) return setError("Enter a quantity to move.");
    const p = await run(() => transferStock(current.id, adjLoc, toLoc, q, adjNote));
    if (p) {
      setAdjQty("");
      refresh(p);
    }
  }

  async function addSupplier() {
    if (!current || !supId) return;
    const existing: Partial<InvProductSupplier>[] = current.suppliers.filter((s) => s.supplier_id !== supId);
    const next = [
      ...existing,
      { supplier_id: supId, supplier_sku: supSku, cost: numVal(supCost), pack_size: 1, preferred: existing.length === 0 },
    ];
    const p = await run(() => setProductSuppliers(current.id, next));
    if (p) {
      setSupId("");
      setSupSku("");
      setSupCost("");
      refresh(p);
    }
  }

  async function dropSupplier(id: string) {
    if (!current) return;
    const next = current.suppliers.filter((s) => s.supplier_id !== id);
    if (next.length > 0 && !next.some((s) => s.preferred)) next[0] = { ...next[0], preferred: true };
    const p = await run(() => setProductSuppliers(current.id, next));
    if (p) refresh(p);
  }

  async function makePreferred(id: string) {
    if (!current) return;
    const p = await run(() =>
      setProductSuppliers(current.id, current.suppliers.map((s) => ({ ...s, preferred: s.supplier_id === id }))),
    );
    if (p) refresh(p);
  }

  async function photo(file: File | undefined) {
    if (!file) return;
    const id = await run(() => uploadPhoto(file));
    if (id) set("photo_file_id", id);
  }

  function printLabel() {
    const svg = barcodeSvg(form.barcode ?? "");
    if (!svg) return setError("Labels need a valid EAN-13 barcode. Use Generate for an internal one.");
    const w = window.open("", "_blank", "width=420,height=320");
    if (!w) return;
    w.document.write(
      `<html><body style="font-family:sans-serif;text-align:center"><h3>${escapeHtml(form.name)}</h3>${svg}<p>${escapeHtml(form.sku ?? "")}</p><script>window.print()</script></body></html>`,
    );
    w.document.close();
  }

  const svg = barcodeSvg(form.barcode ?? "", 50);
  const needsTransferTarget = reason === "transfer";

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog
        sx={{ width: 760, maxWidth: "96vw", maxHeight: "92vh", overflowY: "auto", overflowX: "hidden", "& > *": { minWidth: 0, flexShrink: 0 } }}
        aria-labelledby="inv-product-title"
      >
        <ModalClose />
        <Typography id="inv-product-title" level="title-lg">
          {current ? (current.parent_id ? "Edit variant" : "Edit product") : family ? `New variant of ${family.name}` : "New product"}
        </Typography>
        {error && (
          <Alert color="danger" size="sm">
            {error}
          </Alert>
        )}

        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)" }, gap: 1.5 }}>
          <FormControl required sx={{ gridColumn: "1 / -1" }}>
            <FormLabel>Name</FormLabel>
            <Input value={form.name} onChange={(e) => set("name", e.target.value)} slotProps={{ input: { "data-testid": "inv-name" } }} />
          </FormControl>
          {isVariant && (
            <FormControl sx={{ gridColumn: "1 / -1" }}>
              <FormLabel>Variant (e.g. Red / L)</FormLabel>
              <Input value={form.variant_label ?? ""} onChange={(e) => set("variant_label", e.target.value)} />
            </FormControl>
          )}
          <FormControl>
            <FormLabel>SKU</FormLabel>
            <Input value={form.sku ?? ""} onChange={(e) => set("sku", e.target.value)} slotProps={{ input: { "data-testid": "inv-sku" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Barcode</FormLabel>
            <Box sx={{ display: "flex", gap: 0.5 }}>
              <Input sx={{ flex: 1 }} value={form.barcode ?? ""} onChange={(e) => set("barcode", e.target.value)} />
              <Button size="sm" variant="outlined" onClick={() => set("barcode", generateBarcode(Date.now() % 1e9))}>
                Generate
              </Button>
              <Button size="sm" variant="outlined" onClick={() => setScan(true)}>
                Scan
              </Button>
            </Box>
          </FormControl>
          {svg && (
            <Box sx={{ gridColumn: "1 / -1", display: "flex", alignItems: "center", gap: 2 }}>
              <Box
                sx={{ "& svg": { height: 60, width: "auto" } }}
                dangerouslySetInnerHTML={{ __html: svg }}
                aria-label="Barcode preview"
              />
              <Button size="sm" variant="plain" onClick={printLabel}>
                Print label
              </Button>
            </Box>
          )}
          <FormControl>
            <FormLabel>Category</FormLabel>
            <Input
              value={form.category ?? ""}
              onChange={(e) => set("category", e.target.value)}
              slotProps={{ input: { list: "inv-cats" } }}
            />
            <datalist id="inv-cats">
              {categories.map((c) => (
                <option key={c} value={c} />
              ))}
            </datalist>
          </FormControl>
          <FormControl>
            <FormLabel>Unit</FormLabel>
            <Input value={form.unit ?? ""} onChange={(e) => set("unit", e.target.value)} placeholder="pcs, kg, L" />
          </FormControl>
          <FormControl>
            <FormLabel>Selling price</FormLabel>
            <Input type="number" value={form.price ?? 0} onChange={(e) => set("price", numVal(e.target.value))} slotProps={{ input: { min: 0, step: "0.01" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Cost</FormLabel>
            <Input type="number" value={form.cost ?? 0} onChange={(e) => set("cost", numVal(e.target.value))} slotProps={{ input: { min: 0, step: "0.01" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Low-stock level</FormLabel>
            <Input type="number" value={form.min_stock ?? 0} onChange={(e) => set("min_stock", numVal(e.target.value))} slotProps={{ input: { min: 0 } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Reorder quantity (0 = auto)</FormLabel>
            <Input type="number" value={form.reorder_qty ?? 0} onChange={(e) => set("reorder_qty", numVal(e.target.value))} slotProps={{ input: { min: 0 } }} />
          </FormControl>
          {!current && (
            <>
              <FormControl>
                <FormLabel>Opening stock</FormLabel>
                <Input type="number" value={form.opening_qty ?? 0} onChange={(e) => set("opening_qty", numVal(e.target.value))} slotProps={{ input: { min: 0 } }} />
              </FormControl>
              <FormControl>
                <FormLabel>Location</FormLabel>
                <Select value={adjLoc} onChange={(_, v) => setAdjLoc(v ?? "")}>
                  {locations.map((l) => (
                    <Option key={l.id} value={l.id}>
                      {l.name}
                    </Option>
                  ))}
                </Select>
              </FormControl>
            </>
          )}
          <FormControl sx={{ gridColumn: "1 / -1" }}>
            <FormLabel>Description</FormLabel>
            <Textarea minRows={2} value={form.description ?? ""} onChange={(e) => set("description", e.target.value)} />
          </FormControl>
          <Box sx={{ gridColumn: "1 / -1", display: "flex", alignItems: "center", gap: 1.5 }}>
            {form.photo_file_id ? (
              <Box component="img" src={photoURL(form.photo_file_id)} alt="Product" sx={{ width: 64, height: 64, objectFit: "cover", borderRadius: "sm" }} />
            ) : (
              <Sheet variant="soft" sx={{ width: 64, height: 64, borderRadius: "sm", display: "grid", placeItems: "center" }}>
                <Typography level="body-xs">No photo</Typography>
              </Sheet>
            )}
            <input ref={fileRef} type="file" accept="image/*" hidden onChange={(e) => photo(e.target.files?.[0])} />
            <Button size="sm" variant="outlined" onClick={() => fileRef.current?.click()}>
              {form.photo_file_id ? "Replace photo" : "Add photo"}
            </Button>
            {form.photo_file_id && (
              <Button size="sm" variant="plain" color="neutral" onClick={() => set("photo_file_id", "")}>
                Remove
              </Button>
            )}
          </Box>
        </Box>

        <Box sx={{ display: "flex", gap: 1, justifyContent: "space-between", flexWrap: "wrap" }}>
          <Box sx={{ display: "flex", gap: 1 }}>
            {current && (
              <>
                <Button
                  size="sm"
                  variant="outlined"
                  color="neutral"
                  onClick={async () => {
                    const p = await run(() => archiveProduct(current.id, !current.archived));
                    if (p) {
                      onChanged();
                      onClose();
                    }
                  }}
                >
                  {current.archived ? "Restore" : "Archive"}
                </Button>
                <Button
                  size="sm"
                  variant="outlined"
                  color="danger"
                  onClick={async () => {
                    if (!window.confirm(`Delete "${current.name}" and its history? Archive keeps the history.`)) return;
                    const ok = await run(async () => {
                      await deleteProduct(current.id);
                      return true;
                    });
                    if (ok) {
                      onChanged();
                      onClose();
                    }
                  }}
                >
                  Delete
                </Button>
              </>
            )}
          </Box>
          <Button loading={busy} onClick={save} data-testid="inv-save">
            {current ? "Save changes" : "Create"}
          </Button>
        </Box>

        {current && (
          <>
            <Divider />
            <Typography level="title-md">
              Stock{" "}
              <Chip size="sm" color={stockStatus(current.total_stock, current.min_stock) === "ok" ? "success" : stockStatus(current.total_stock, current.min_stock) === "low" ? "warning" : "danger"}>
                {qtyText(current.total_stock)} {current.unit}
              </Chip>
            </Typography>
            {current.stock.length === 0 && current.variants.length === 0 && (
              <Typography level="body-sm">No stock recorded yet.</Typography>
            )}
            {current.stock.length > 0 && (
              <Table size="sm">
                <thead>
                  <tr>
                    <th>Location</th>
                    <th>Shelf</th>
                    <th style={{ textAlign: "right" }}>Qty</th>
                  </tr>
                </thead>
                <tbody>
                  {current.stock.map((s) => (
                    <tr key={s.location_id}>
                      <td>{s.location_name}</td>
                      <td><ShelfCell productId={current.id} locationId={s.location_id} value={s.shelf_code} onSaved={async () => { try { refresh(await getProduct(current.id)); } catch { onChanged(); } }} /></td>
                      <td style={{ textAlign: "right" }}>{qtyText(s.qty)}</td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
            {(current.variants.length === 0 || current.parent_id) && <StockSettings productId={current.id} onChanged={onChanged} />}
            {!current.parent_id && current.variants.length > 0 ? (
              <Typography level="body-sm">Stock is held by the variants below.</Typography>
            ) : (
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(130px, 1fr))", gap: 1, alignItems: "end", minWidth: 0 }}>
                <FormControl>
                  <FormLabel>Action</FormLabel>
                  <Select value={reason} onChange={(_, v) => setReason((v ?? "receive") as Action)}>
                    <Option value="receive">Receive</Option>
                    <Option value="remove">Remove</Option>
                    <Option value="count">Count (set to)</Option>
                    <Option value="sale">Sale</Option>
                    <Option value="refund">Refund</Option>
                    <Option value="correction">Correction (±)</Option>
                    <Option value="transfer">Move to…</Option>
                  </Select>
                </FormControl>
                <FormControl>
                  <FormLabel>{needsTransferTarget ? "From" : "Location"}</FormLabel>
                  <Select value={adjLoc} onChange={(_, v) => setAdjLoc(v ?? "")}>
                    {locations.map((l) => (
                      <Option key={l.id} value={l.id}>
                        {l.name}
                      </Option>
                    ))}
                  </Select>
                </FormControl>
                {needsTransferTarget && (
                  <FormControl>
                    <FormLabel>To</FormLabel>
                    <Select value={toLoc} onChange={(_, v) => setToLoc(v ?? "")}>
                      {locations.filter((l) => l.id !== adjLoc).map((l) => (
                        <Option key={l.id} value={l.id}>
                          {l.name}
                        </Option>
                      ))}
                    </Select>
                  </FormControl>
                )}
                <FormControl>
                  <FormLabel>Quantity</FormLabel>
                  <Input type="number" value={adjQty} onChange={(e) => setAdjQty(e.target.value)} slotProps={{ input: { "data-testid": "inv-adj-qty" } }} />
                </FormControl>
                <FormControl>
                  <FormLabel>Note</FormLabel>
                  <Input value={adjNote} onChange={(e) => setAdjNote(e.target.value)} />
                </FormControl>
                <Button loading={busy} onClick={needsTransferTarget ? doTransfer : doAdjust} data-testid="inv-adj-go">
                  Apply
                </Button>
              </Box>
            )}

            {!current.parent_id && (
              <>
                <Divider />
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography level="title-md">Variants ({current.variants.length})</Typography>
                  <Button size="sm" variant="outlined" onClick={() => props.onAddVariant(current)}>
                    Add variant
                  </Button>
                </Box>
                {current.variants.length > 0 && (
                  <Table size="sm" hoverRow>
                    <thead>
                      <tr>
                        <th>Variant</th>
                        <th>SKU</th>
                        <th style={{ textAlign: "right" }}>Price</th>
                        <th style={{ textAlign: "right" }}>Stock</th>
                      </tr>
                    </thead>
                    <tbody>
                      {current.variants.map((v) => (
                        <tr key={v.id} onClick={() => props.onOpenProduct(v)} style={{ cursor: "pointer" }}>
                          <td>{v.variant_label || "-"}</td>
                          <td>{v.sku}</td>
                          <td style={{ textAlign: "right" }}>{money(v.price)}</td>
                          <td style={{ textAlign: "right" }}>{qtyText(v.total_stock)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                )}
              </>
            )}

            <Divider />
            <Typography level="title-md">Suppliers</Typography>
            {current.suppliers.map((s) => (
              <Box key={s.supplier_id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  {s.supplier_name}
                  {s.supplier_sku ? ` · ${s.supplier_sku}` : ""} · {money(s.cost)}
                </Typography>
                {s.preferred ? (
                  <Chip size="sm" color="success">
                    Preferred
                  </Chip>
                ) : (
                  <Button size="sm" variant="plain" onClick={() => makePreferred(s.supplier_id)}>
                    Make preferred
                  </Button>
                )}
                <Button size="sm" variant="plain" color="danger" onClick={() => dropSupplier(s.supplier_id)}>
                  Remove
                </Button>
              </Box>
            ))}
            {suppliers.length === 0 ? (
              <Typography level="body-sm">Add suppliers in the Suppliers section first.</Typography>
            ) : (
              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "2fr 1fr 1fr auto" }, gap: 1 }}>
                <Select placeholder="Supplier" value={supId || null} onChange={(_, v) => setSupId(v ?? "")}>
                  {suppliers.map((s) => (
                    <Option key={s.id} value={s.id}>
                      {s.name}
                    </Option>
                  ))}
                </Select>
                <Input placeholder="Their code" value={supSku} onChange={(e) => setSupSku(e.target.value)} />
                <Input type="number" placeholder="Price" value={supCost} onChange={(e) => setSupCost(e.target.value)} />
                <Button variant="outlined" disabled={!supId} onClick={addSupplier}>
                  Add
                </Button>
              </Box>
            )}

            <Divider />
            <Typography level="title-md">Recent history</Typography>
            {history.length === 0 ? (
              <Typography level="body-sm">No movements yet.</Typography>
            ) : (
              <Table size="sm">
                <tbody>
                  {history.map((h) => (
                    <tr key={h.id}>
                      <td>{new Date(h.created_at).toLocaleString()}</td>
                      <td>{h.reason}</td>
                      <td>{h.location_name}</td>
                      <td style={{ textAlign: "right" }}>
                        {h.delta > 0 ? "+" : ""}
                        {qtyText(h.delta)}
                      </td>
                      <td>{h.ref || h.note}</td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
          </>
        )}

        <ScanDialog
          open={scan}
          onClose={() => setScan(false)}
          onCode={(c) => {
            set("barcode", c);
            setScan(false);
          }}
        />
      </ModalDialog>
    </Modal>
  );
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);
}

