import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Checkbox,
  CircularProgress,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import { archiveProduct, importProducts, listProducts, listProductsPage, photoURL } from "./api";
import { LoadOlder } from "../../components/LoadOlder";
import { csvToImport, money, productsToCsv, qtyText, stockStatus } from "./logic";
import { ProductDialog } from "./ProductDialog";
import { ScanDialog } from "./ScanDialog";
import type { ImportResult, InvLocation, InvProduct, InvSupplier } from "./types";

interface Props {
  locations: InvLocation[];
  suppliers: InvSupplier[];
  /** Bumps when another view changed data. */
  version: number;
  onChanged: () => void;
  initialLowOnly?: boolean;
}

function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

export function ProductsView({ locations, suppliers, version, onChanged, initialLowOnly }: Props) {
  const [products, setProducts] = useState<InvProduct[] | null>(null);
  // Server-paged (50 families per page); "Load more" appends the next page.
  const [next, setNext] = useState("");
  const [error, setError] = useState("");
  const [q, setQ] = useState("");
  const [category, setCategory] = useState("");
  const [locationId, setLocationId] = useState("");
  const [lowOnly, setLowOnly] = useState(Boolean(initialLowOnly));
  const [archived, setArchived] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [dialog, setDialog] = useState<{ open: boolean; product: InvProduct | null; family: InvProduct | null }>({
    open: false,
    product: null,
    family: null,
  });
  const [scan, setScan] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [importMode, setImportMode] = useState<"update" | "add" | "skip">("update");
  const [importText, setImportText] = useState("");
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importBusy, setImportBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    try {
      const page = await listProductsPage({ q, category, location_id: locationId, low: lowOnly, archived });
      setProducts(page.items);
      setNext(page.next);
      setError("");
    } catch (e) {
      setError((e as Error).message);
    }
  }, [q, category, locationId, lowOnly, archived]);

  useEffect(() => {
    const t = setTimeout(load, q ? 200 : 0);
    return () => clearTimeout(t);
  }, [load, version, q]);

  const categories = useMemo(
    () => Array.from(new Set((products ?? []).map((p) => p.category).filter(Boolean))).sort(),
    [products],
  );

  function changed() {
    onChanged();
    load();
  }

  async function scanned(code: string) {
    setScan(false);
    const hits = await listProducts({ barcode: code, flat: true }).catch(() => []);
    if (hits.length > 0) {
      setDialog({ open: true, product: hits[0], family: null });
    } else {
      setQ(code);
    }
  }

  async function bulkArchive() {
    for (const id of selected) await archiveProduct(id, !archived).catch(() => undefined);
    setSelected(new Set());
    changed();
  }

  async function exportCsv() {
    const all = await listProducts({ archived_only: archived });
    download("inventory.csv", productsToCsv(all));
  }

  async function runImport() {
    const parsed = csvToImport(importText);
    if (parsed.rows.length === 0) {
      setImportResult({ created: 0, updated: 0, skipped: 0, errors: parsed.errors });
      return;
    }
    setImportBusy(true);
    try {
      const r = await importProducts(importMode, parsed.rows, parsed.locations);
      setImportResult({ ...r, errors: [...parsed.errors, ...(r.errors ?? [])] });
      changed();
    } catch (e) {
      setImportResult({ created: 0, updated: 0, skipped: 0, errors: [(e as Error).message] });
    } finally {
      setImportBusy(false);
    }
  }

  const toggle = (id: string) =>
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 2, alignItems: "center" }}>
        <Input
          placeholder="Search name, SKU, barcode, category"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          sx={{ flex: "1 1 240px", minWidth: 200 }}
          slotProps={{ input: { "aria-label": "Search products", "data-testid": "inv-search" } }}
        />
        <Select
          placeholder="Category"
          value={category || null}
          onChange={(_, v) => setCategory(v ?? "")}
          sx={{ minWidth: 140 }}
          slotProps={{ button: { "aria-label": "Category filter" } }}
        >
          <Option value="">All categories</Option>
          {categories.map((c) => (
            <Option key={c} value={c}>
              {c}
            </Option>
          ))}
        </Select>
        {locations.length > 1 && (
          <Select placeholder="Location" value={locationId || null} onChange={(_, v) => setLocationId(v ?? "")} sx={{ minWidth: 140 }}>
            <Option value="">All locations</Option>
            {locations.map((l) => (
              <Option key={l.id} value={l.id}>
                {l.name}
              </Option>
            ))}
          </Select>
        )}
        <Checkbox label="Low stock" checked={lowOnly} onChange={(e) => setLowOnly(e.target.checked)} />
        <Checkbox label="Archived" checked={archived} onChange={(e) => setArchived(e.target.checked)} />
      </Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 2 }}>
        <Button onClick={() => setDialog({ open: true, product: null, family: null })} data-testid="inv-new">
          New product
        </Button>
        <Button variant="outlined" onClick={() => setScan(true)}>
          Scan
        </Button>
        <Button variant="outlined" onClick={() => setImportOpen(true)}>
          Import CSV
        </Button>
        <Button variant="outlined" onClick={exportCsv}>
          Export CSV
        </Button>
        {selected.size > 0 && (
          <Button variant="soft" color="neutral" onClick={bulkArchive}>
            {archived ? "Restore" : "Archive"} {selected.size} selected
          </Button>
        )}
      </Box>

      {error && (
        <Alert color="danger" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      {products === null ? (
        <Box sx={{ display: "grid", placeItems: "center", py: 6 }}>
          <CircularProgress />
        </Box>
      ) : products.length === 0 ? (
        <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
          <Typography level="title-md">{q || lowOnly || archived ? "No products match." : "No products yet."}</Typography>
          {!q && !lowOnly && !archived && (
            <Typography level="body-sm">Create one, or import a CSV to get started.</Typography>
          )}
        </Sheet>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow stickyHeader aria-label="Products">
            <thead>
              <tr>
                <th style={{ width: 40 }} />
                <th style={{ width: 56 }} />
                <th>Name</th>
                <th>SKU</th>
                <th>Category</th>
                <th style={{ textAlign: "right" }}>Price</th>
                <th style={{ textAlign: "right" }}>Cost</th>
                <th style={{ textAlign: "right" }}>Stock</th>
              </tr>
            </thead>
            <tbody>
              {products.map((p) => {
                const st = stockStatus(p.total_stock, p.min_stock);
                return (
                  <tr key={p.id} style={{ cursor: "pointer" }} onClick={() => setDialog({ open: true, product: p, family: null })} data-testid="inv-row">
                    <td onClick={(e) => e.stopPropagation()}>
                      <Checkbox size="sm" checked={selected.has(p.id)} onChange={() => toggle(p.id)} slotProps={{ input: { "aria-label": `Select ${p.name}` } }} />
                    </td>
                    <td>
                      {p.photo_file_id ? (
                        <Box component="img" src={photoURL(p.photo_file_id)} alt="" sx={{ width: 36, height: 36, objectFit: "cover", borderRadius: "sm" }} />
                      ) : (
                        <Box sx={{ width: 36, height: 36, borderRadius: "sm", bgcolor: "background.level2" }} />
                      )}
                    </td>
                    <td>
                      {p.name}
                      {p.variants.length > 0 && (
                        <Chip size="sm" variant="soft" sx={{ ml: 1 }}>
                          {p.variants.length} variants
                        </Chip>
                      )}
                      {p.archived && (
                        <Chip size="sm" variant="outlined" sx={{ ml: 1 }}>
                          Archived
                        </Chip>
                      )}
                    </td>
                    <td>{p.sku}</td>
                    <td>{p.category}</td>
                    <td style={{ textAlign: "right" }}>{money(p.price)}</td>
                    <td style={{ textAlign: "right" }}>{money(p.cost)}</td>
                    <td style={{ textAlign: "right" }}>
                      <Chip size="sm" variant="soft" color={st === "ok" ? "success" : st === "low" ? "warning" : "danger"}>
                        {qtyText(p.total_stock)} {p.unit}
                      </Chip>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
          <LoadOlder
            next={next}
            shown={products.length}
            label="Load more products"
            testId="inv-load-more"
            loadMore={async (c) => {
              const page = await listProductsPage({ q, category, location_id: locationId, low: lowOnly, archived }, c);
              setProducts((ps) => [...(ps ?? []), ...page.items]);
              setNext(page.next);
            }}
          />
        </Sheet>
      )}

      <ProductDialog
        open={dialog.open}
        product={dialog.product}
        family={dialog.family}
        locations={locations}
        suppliers={suppliers}
        categories={categories}
        onClose={() => setDialog({ open: false, product: null, family: null })}
        onChanged={changed}
        onOpenProduct={(p) => setDialog({ open: true, product: p, family: null })}
        onAddVariant={(f) => setDialog({ open: true, product: null, family: f })}
      />
      <ScanDialog open={scan} onClose={() => setScan(false)} onCode={scanned} />

      <Modal open={importOpen} onClose={() => setImportOpen(false)}>
        <ModalDialog sx={{ width: 560, maxWidth: "95vw" }}>
          <ModalClose />
          <Typography level="title-lg">Import products from CSV</Typography>
          <Typography level="body-sm">
            Columns are detected by name: Name, SKU, Barcode, Category, Unit, Price, Cost, Min stock, Location, Quantity.
            Rows match existing products by SKU.
          </Typography>
          <Select value={importMode} onChange={(_, v) => setImportMode((v ?? "update") as typeof importMode)}>
            <Option value="update">Update matches (quantity sets stock)</Option>
            <Option value="add">Update matches (quantity adds to stock)</Option>
            <Option value="skip">Skip matches</Option>
          </Select>
          <input
            ref={fileRef}
            type="file"
            accept=".csv,text/csv"
            hidden
            data-testid="inv-import-file"
            onChange={async (e) => {
              const f = e.target.files?.[0];
              if (f) setImportText(await f.text());
              e.target.value = "";
            }}
          />
          <Button variant="outlined" onClick={() => fileRef.current?.click()}>
            Choose CSV file
          </Button>
          <Box
            component="textarea"
            value={importText}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setImportText(e.target.value)}
            placeholder="…or paste CSV here"
            aria-label="CSV text"
            sx={{ minHeight: 120, p: 1, fontFamily: "monospace", borderRadius: "sm", border: "1px solid", borderColor: "divider" }}
          />
          {importResult && (
            <Alert color={importResult.errors.length ? "warning" : "success"}>
              <Box>
                Created {importResult.created}, updated {importResult.updated}, skipped {importResult.skipped}.
                {importResult.errors.slice(0, 5).map((e, i) => (
                  <Box key={i}>{e}</Box>
                ))}
              </Box>
            </Alert>
          )}
          <Button loading={importBusy} disabled={!importText.trim()} onClick={runImport} data-testid="inv-import-go">
            Import
          </Button>
        </ModalDialog>
      </Modal>
    </Box>
  );
}
