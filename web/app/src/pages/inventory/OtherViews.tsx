import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
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
  createLocation,
  createSupplier,
  deleteLocation,
  deleteSupplier,
  getReport,
  listHistory,
  reorderList,
  searchContacts,
  updateLocation,
  updateSupplier,
} from "./api";
import { money, orderSheets, qtyText } from "./logic";
import type {
  InvHistoryEntry,
  InvLocation,
  InvReport,
  InvReportRow,
  InvSupplier,
  ReorderLine,
} from "./types";
import type { ContactLite } from "./api";

const th = { textAlign: "right" } as const;

function useErr() {
  const [err, setErr] = useState("");
  return { err, setErr, run: async (fn: () => Promise<unknown>) => {
    try {
      setErr("");
      await fn();
      return true;
    } catch (e) {
      setErr((e as Error).message);
      return false;
    }
  } };
}

// ---------------------------------------------------------------- Reorder

export function ReorderView({ version, onPlan }: { version: number; onPlan?: () => void }) {
  const [lines, setLines] = useState<ReorderLine[] | null>(null);
  const [text, setText] = useState("");
  useEffect(() => {
    reorderList().then(setLines).catch(() => setLines([]));
  }, [version]);
  if (lines === null) return null;
  return (
    <Box>
      {onPlan && (
        <Alert color="primary" variant="soft" sx={{ mb: 2 }} endDecorator={<Button size="sm" onClick={onPlan} data-testid="reorder-to-po">Create draft purchase orders</Button>}>
          Turn this list into draft purchase orders, one per supplier, using your reorder rules and supplier price lists.
        </Alert>
      )}
      <Typography level="body-sm" sx={{ mb: 2 }}>
        Products at or below their low-stock level, with the preferred supplier. Suggested quantity is the reorder
        quantity, or twice the low-stock level minus stock, rounded up to the supplier pack size.
      </Typography>
      {lines.length === 0 ? (
        <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
          <Typography level="title-md">Nothing to reorder.</Typography>
        </Sheet>
      ) : (
        <>
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 2 }}>
            <Table aria-label="Reorder list">
              <thead>
                <tr>
                  <th>Product</th>
                  <th>Supplier</th>
                  <th style={th}>Stock</th>
                  <th style={th}>Low at</th>
                  <th style={th}>Order</th>
                  <th style={th}>Unit cost</th>
                </tr>
              </thead>
              <tbody>
                {lines.map((l) => (
                  <tr key={l.product.id} data-testid="inv-reorder-row">
                    <td>
                      {l.product.name}
                      {l.product.variant_label ? ` (${l.product.variant_label})` : ""}
                      <Typography level="body-xs">{l.product.sku}</Typography>
                    </td>
                    <td>{l.supplier_name || <Chip size="sm" color="warning">No supplier</Chip>}</td>
                    <td style={th}>{qtyText(l.stock)}</td>
                    <td style={th}>{qtyText(l.product.min_stock)}</td>
                    <td style={th}>{qtyText(l.suggested_qty)}</td>
                    <td style={th}>{money(l.unit_cost)}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
          <Button variant="outlined" onClick={() => setText(orderSheets(lines))}>
            Build order sheets
          </Button>
          {text && (
            <Box sx={{ mt: 2 }}>
              <Textarea minRows={8} value={text} readOnly />
              <Button sx={{ mt: 1 }} variant="plain" onClick={() => navigator.clipboard?.writeText(text)}>
                Copy
              </Button>
            </Box>
          )}
        </>
      )}
    </Box>
  );
}

// ---------------------------------------------------------------- History

export function HistoryView({ version }: { version: number }) {
  const [entries, setEntries] = useState<InvHistoryEntry[] | null>(null);
  const [reason, setReason] = useState("");
  useEffect(() => {
    listHistory({ reason, limit: 300 }).then(setEntries).catch(() => setEntries([]));
  }, [reason, version]);
  return (
    <Box>
      <Select
        placeholder="All reasons"
        value={reason || null}
        onChange={(_, v) => setReason(v ?? "")}
        sx={{ width: 200, mb: 2 }}
        slotProps={{ button: { "aria-label": "Reason filter" } }}
      >
        <Option value="">All reasons</Option>
        {["sale", "refund", "receive", "count", "remove", "correction", "transfer"].map((r) => (
          <Option key={r} value={r}>
            {r}
          </Option>
        ))}
      </Select>
      {entries && entries.length === 0 ? (
        <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
          <Typography level="title-md">No stock movements yet.</Typography>
        </Sheet>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table aria-label="Stock history">
            <thead>
              <tr>
                <th>When</th>
                <th>Product</th>
                <th>Reason</th>
                <th>Location</th>
                <th style={th}>Change</th>
                <th style={th}>After</th>
                <th>Reference</th>
              </tr>
            </thead>
            <tbody>
              {(entries ?? []).map((h) => (
                <tr key={h.id}>
                  <td>{new Date(h.created_at).toLocaleString()}</td>
                  <td>{h.product_name}</td>
                  <td>{h.reason}</td>
                  <td>{h.location_name}</td>
                  <td style={th}>
                    {h.delta > 0 ? "+" : ""}
                    {qtyText(h.delta)}
                  </td>
                  <td style={th}>{qtyText(h.qty_after)}</td>
                  <td>{h.ref || h.note}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

// ---------------------------------------------------------------- Reports

function RowsTable({ title, rows, unitsOnly }: { title: string; rows: InvReportRow[]; unitsOnly?: boolean }) {
  return (
    <Card variant="outlined" sx={{ flex: "1 1 320px" }}>
      <Typography level="title-md">{title}</Typography>
      {rows.length === 0 ? (
        <Typography level="body-sm">No data yet.</Typography>
      ) : (
        <Table size="sm">
          <tbody>
            {rows.map((r) => (
              <tr key={r.label}>
                <td>{r.label}</td>
                <td style={th}>{qtyText(r.units)}</td>
                {!unitsOnly && <td style={th}>{money(r.cost_value)}</td>}
                {!unitsOnly && <td style={th}>{money(r.retail_value)}</td>}
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </Card>
  );
}

export function ReportsView({ version }: { version: number }) {
  const [r, setR] = useState<InvReport | null>(null);
  useEffect(() => {
    getReport().then(setR).catch(() => setR(null));
  }, [version]);
  if (!r) return null;
  const stat = (label: string, value: string, id: string) => (
    <Card variant="soft" sx={{ flex: "1 1 160px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h3" data-testid={id}>
        {value}
      </Typography>
    </Card>
  );
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
        {stat("Products", String(r.product_count), "rep-products")}
        {stat("Units in stock", qtyText(r.units), "rep-units")}
        {stat("Stock value (cost)", money(r.cost_value), "rep-cost")}
        {stat("Stock value (retail)", money(r.retail_value), "rep-retail")}
        {stat("Low stock", String(r.low_stock_count), "rep-low")}
        {stat("Out of stock", String(r.out_of_stock_count), "rep-out")}
      </Box>
      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
        <RowsTable title="Value by category (units, cost, retail)" rows={r.by_category} />
        <RowsTable title="Value by location (units, cost, retail)" rows={r.by_location} />
      </Box>
      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
        <RowsTable title="Movement, last 30 days (net units)" rows={r.movement} unitsOnly />
        <RowsTable title="Top sellers, last 30 days (units)" rows={r.top_movers} unitsOnly />
      </Box>
    </Box>
  );
}

// -------------------------------------------------------------- Suppliers

export function SuppliersView({ suppliers, onChanged }: { suppliers: InvSupplier[]; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<InvSupplier> | null>(null);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const { err, setErr, run } = useErr();
  useEffect(() => {
    if (edit) searchContacts().then(setContacts).catch(() => setContacts([]));
  }, [edit === null]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    if (!edit?.name?.trim()) return setErr("Name is required.");
    const ok = await run(() => (edit.id ? updateSupplier(edit.id, edit) : createSupplier(edit)));
    if (ok) {
      setEdit(null);
      onChanged();
    }
  };
  const pick = (id: string | null) => {
    const c = contacts.find((x) => x.id === id);
    setEdit((e) => ({
      ...e,
      contact_id: id ?? "",
      ...(c ? { name: e?.name || c.display_name || c.company, email: e?.email || c.emails?.[0] || "", phone: e?.phone || c.phones?.[0] || "" } : {}),
    }));
  };
  return (
    <Box>
      <Button sx={{ mb: 2 }} onClick={() => setEdit({ name: "" })} data-testid="inv-new-supplier">
        New supplier
      </Button>
      {suppliers.length === 0 ? (
        <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
          <Typography level="title-md">No suppliers yet.</Typography>
        </Sheet>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow aria-label="Suppliers">
            <thead>
              <tr>
                <th>Name</th>
                <th>Email</th>
                <th>Phone</th>
                <th style={th}>Products</th>
                <th>Contact</th>
              </tr>
            </thead>
            <tbody>
              {suppliers.map((s) => (
                <tr key={s.id} style={{ cursor: "pointer" }} onClick={() => setEdit(s)} data-testid="inv-supplier-row">
                  <td>{s.name}</td>
                  <td>{s.email}</td>
                  <td>{s.phone}</td>
                  <td style={th}>{s.product_count}</td>
                  <td>{s.contact_id ? <Chip size="sm">Linked</Chip> : ""}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Modal open={edit !== null} onClose={() => setEdit(null)}>
        <ModalDialog sx={{ width: 480, maxWidth: "95vw" }}>
          <ModalClose />
          <Typography level="title-lg">{edit?.id ? "Edit supplier" : "New supplier"}</Typography>
          {err && <Alert color="danger">{err}</Alert>}
          <FormControl>
            <FormLabel>Link to a contact (optional)</FormLabel>
            <Select placeholder="No contact" value={edit?.contact_id || null} onChange={(_, v) => pick(v)}>
              <Option value="">No contact</Option>
              {contacts.map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.display_name || c.company}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl required>
            <FormLabel>Name</FormLabel>
            <Input value={edit?.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "inv-supplier-name" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Email</FormLabel>
            <Input value={edit?.email ?? ""} onChange={(e) => setEdit({ ...edit, email: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Phone</FormLabel>
            <Input value={edit?.phone ?? ""} onChange={(e) => setEdit({ ...edit, phone: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Order notes</FormLabel>
            <Textarea minRows={2} value={edit?.notes ?? ""} onChange={(e) => setEdit({ ...edit, notes: e.target.value })} />
          </FormControl>
          <Box sx={{ display: "flex", justifyContent: "space-between" }}>
            {edit?.id ? (
              <Button
                variant="outlined"
                color="danger"
                onClick={async () => {
                  if (!window.confirm(`Delete supplier "${edit.name}"?`)) return;
                  if (await run(() => deleteSupplier(edit.id as string))) {
                    setEdit(null);
                    onChanged();
                  }
                }}
              >
                Delete
              </Button>
            ) : (
              <span />
            )}
            <Button onClick={save} data-testid="inv-supplier-save">
              Save
            </Button>
          </Box>
        </ModalDialog>
      </Modal>
    </Box>
  );
}

// -------------------------------------------------------------- Locations

export function LocationsView({ locations, onChanged }: { locations: InvLocation[]; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<InvLocation> | null>(null);
  const { err, setErr, run } = useErr();
  const save = useCallback(async () => {
    if (!edit?.name?.trim()) return setErr("Name is required.");
    const ok = await run(() => (edit.id ? updateLocation(edit.id, edit) : createLocation(edit)));
    if (ok) {
      setEdit(null);
      onChanged();
    }
  }, [edit, run, setErr, onChanged]);
  return (
    <Box>
      <Button sx={{ mb: 2 }} onClick={() => setEdit({ name: "" })} data-testid="inv-new-location">
        New location
      </Button>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table hoverRow aria-label="Locations">
          <thead>
            <tr>
              <th>Name</th>
              <th>Code</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {locations.map((l) => (
              <tr key={l.id} style={{ cursor: "pointer" }} onClick={() => setEdit(l)}>
                <td>{l.name}</td>
                <td>{l.code}</td>
                <td>{l.is_default && <Chip size="sm">Default</Chip>}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
      <Modal open={edit !== null} onClose={() => setEdit(null)}>
        <ModalDialog sx={{ width: 420, maxWidth: "95vw" }}>
          <ModalClose />
          <Typography level="title-lg">{edit?.id ? "Edit location" : "New location"}</Typography>
          {err && <Alert color="danger">{err}</Alert>}
          <FormControl required>
            <FormLabel>Name</FormLabel>
            <Input value={edit?.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "inv-location-name" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Code</FormLabel>
            <Input value={edit?.code ?? ""} onChange={(e) => setEdit({ ...edit, code: e.target.value })} />
          </FormControl>
          <Select value={edit?.is_default ? "yes" : "no"} onChange={(_, v) => setEdit({ ...edit, is_default: v === "yes" })}>
            <Option value="no">Not the default</Option>
            <Option value="yes">Default location (used when none is chosen)</Option>
          </Select>
          <Box sx={{ display: "flex", justifyContent: "space-between" }}>
            {edit?.id ? (
              <Button
                variant="outlined"
                color="danger"
                onClick={async () => {
                  if (!window.confirm(`Delete location "${edit.name}"?`)) return;
                  if (await run(() => deleteLocation(edit.id as string))) {
                    setEdit(null);
                    onChanged();
                  }
                }}
              >
                Delete
              </Button>
            ) : (
              <span />
            )}
            <Button onClick={save} data-testid="inv-location-save">
              Save
            </Button>
          </Box>
        </ModalDialog>
      </Modal>
    </Box>
  );
}
