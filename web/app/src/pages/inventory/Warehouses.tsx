import { useState } from "react";
import {
  Box,
  Button,
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
  Tab,
  TabList,
  Tabs,
  Typography,
} from "@mui/joy";
import { api } from "./stockApi";
import type { Avail, Loc } from "./stockApi";
import { qtyText } from "./logic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, dateTime, right, useAction, useLoad } from "./stockUi";

const KIND_LABEL = { warehouse: "Warehouse", zone: "Zone", shelf: "Shelf", bin: "Bin" } as const;

export function WarehousesView({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [tab, setTab] = useState<string>("availability");
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2, flexWrap: "wrap" }}>
          <Tab value="availability" data-testid="wh-tab-availability">Availability</Tab>
          <Tab value="locations" data-testid="wh-tab-locations">Locations</Tab>
          <Tab value="reservations" data-testid="wh-tab-reservations">Reservations</Tab>
        </TabList>
      </Tabs>
      {tab === "availability" && <Availability version={version} />}
      {tab === "locations" && <LocationTree version={version} onChanged={onChanged} />}
      {tab === "reservations" && <Reservations version={version} />}
    </Box>
  );
}

// ------------------------------------------------------------ availability

function Availability({ version }: { version: number }) {
  const locs = useLoad(() => api.locations(), [version]);
  const [loc, setLoc] = useState("");
  const [q, setQ] = useState("");
  const rows = useLoad(() => api.availability(q, loc), [q, loc, version]);
  const [reserve, setReserve] = useState<Avail | null>(null);
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
            <Input size="sm" placeholder="Search name or SKU" value={q} onChange={(e) => setQ(e.target.value)} data-testid="avail-search" />
            <Select size="sm" value={loc} onChange={(_, v) => setLoc(v ?? "")} placeholder="All locations" sx={{ minWidth: 200 }} data-testid="avail-location">
              <Option value="">All locations</Option>
              {(locs.data ?? []).map((l) => (
                <Option key={l.id} value={l.id}>
                  {"\u00a0\u00a0".repeat(l.depth)}{l.name}
                </Option>
              ))}
            </Select>
          </Box>
        }
      >
        Stock position
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        <b>On hand</b> is what is on the shelf. <b>Reserved</b> is held for orders. <b>Available</b> is on hand minus reserved. <b>Incoming</b> is
        on open purchase orders and transfers in transit. A kit&apos;s on hand is how many complete kits its parts allow.
      </Typography>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No products.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="avail-table">
            <thead>
              <tr>
                <th>Product</th>
                <th style={right}>On hand</th>
                <th style={right}>Reserved</th>
                <th style={right}>Available</th>
                <th style={right}>Incoming</th>
                <th style={right}>Forecast</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.data.map((a) => (
                <tr key={a.product_id}>
                  <td>
                    {a.name} {a.sku && <Typography level="body-xs" component="span">{a.sku}</Typography>}{" "}
                    {a.is_kit && <Chip size="sm" variant="soft">Kit</Chip>}
                    {a.tracking !== "none" && <Chip size="sm" variant="soft" color="primary">{a.tracking === "lot" ? "Lots" : "Serials"}</Chip>}
                  </td>
                  <td style={right}>{qtyText(a.on_hand)}</td>
                  <td style={right}>{a.reserved > 0 ? <Chip size="sm" color="warning" variant="soft">{qtyText(a.reserved)}</Chip> : "0"}</td>
                  <td style={{ ...right, fontWeight: 600, color: a.available <= 0 ? "var(--joy-palette-danger-600)" : undefined }}>{qtyText(a.available)}</td>
                  <td style={right}>{qtyText(a.incoming + a.in_transit)}</td>
                  <td style={right}>{qtyText(a.forecast)}</td>
                  <td style={right}>
                    <Button size="sm" variant="plain" onClick={() => setReserve(a)} data-testid={`reserve-${a.sku || a.product_id}`}>
                      Reserve
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {reserve && <ReserveDialog item={reserve} locations={locs.data ?? []} onClose={() => setReserve(null)} onDone={() => { setReserve(null); void rows.reload(); }} />}
    </Box>
  );
}

function ReserveDialog({ item, locations, onClose, onDone }: { item: Avail; locations: Loc[]; onClose: () => void; onDone: () => void }) {
  const [qty, setQty] = useState("1");
  const [ref, setRef] = useState("");
  const [loc, setLoc] = useState("");
  const [back, setBack] = useState(false);
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Reserve {item.name}</Typography>
        <Typography level="body-sm">Available now: {qtyText(item.available)} {item.unit}</Typography>
        <ErrorBar error={act.error} />
        <FormControl>
          <FormLabel>Order reference</FormLabel>
          <Input value={ref} onChange={(e) => setRef(e.target.value)} placeholder="SO-1042" autoFocus data-testid="reserve-ref" />
        </FormControl>
        <FormControl>
          <FormLabel>Quantity</FormLabel>
          <Input type="number" value={qty} onChange={(e) => setQty(e.target.value)} slotProps={{ input: { min: 0, step: "any" } }} data-testid="reserve-qty" />
        </FormControl>
        <FormControl>
          <FormLabel>Location (optional)</FormLabel>
          <Select value={loc} onChange={(_, v) => setLoc(v ?? "")}>
            <Option value="">Any</Option>
            {locations.map((l) => (
              <Option key={l.id} value={l.id}>{l.path}</Option>
            ))}
          </Select>
        </FormControl>
        <FormControl orientation="horizontal" sx={{ gap: 1 }}>
          <Switch checked={back} onChange={(e) => setBack(e.target.checked)} />
          <FormLabel>Allow more than available (back order)</FormLabel>
        </FormControl>
        <Button
          loading={act.busy}
          onClick={async () => {
            const ok = await act.run(() => api.reserve({ product_id: item.product_id, qty: Number(qty), ref, location_id: loc || undefined, backorder: back }));
            if (ok) onDone();
          }}
          data-testid="reserve-save"
        >
          Reserve
        </Button>
      </ModalDialog>
    </Modal>
  );
}

// -------------------------------------------------------------- locations

function LocationTree({ version, onChanged }: { version: number; onChanged: () => void }) {
  const locs = useLoad(() => api.locations(), [version]);
  const [edit, setEdit] = useState<Partial<Loc> | null>(null);
  const act = useAction();
  const reload = () => { onChanged(); void locs.reload(); };
  return (
    <Box>
      <Title right={<Button size="sm" onClick={() => setEdit({ kind: "warehouse", active: true })} data-testid="loc-add-warehouse">Add warehouse</Button>}>
        Warehouses and locations
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Build the tree your goods sit in: warehouse, then zone, shelf and bin. Stock can be held at any level. Shelf codes for a product at a location are edited in
        the product&apos;s stock table.
      </Typography>
      <ErrorBar error={locs.error || act.error} />
      <Loading show={locs.loading && !locs.data} />
      {locs.data && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="loc-table">
            <thead>
              <tr>
                <th>Location</th>
                <th>Type</th>
                <th>Code</th>
                <th style={right}>Units here</th>
                <th style={right}>Incl. inside</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {locs.data.map((l) => (
                <tr key={l.id} style={{ opacity: l.active ? 1 : 0.55 }}>
                  <td style={{ paddingLeft: 8 + l.depth * 22 }}>
                    {l.name} {l.is_default && <Chip size="sm" variant="soft">Default</Chip>}
                  </td>
                  <td>{KIND_LABEL[l.kind]}</td>
                  <td>{l.code}</td>
                  <td style={right}>{qtyText(l.units)}</td>
                  <td style={right}>{qtyText(l.subtree_units)}</td>
                  <td style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                    {l.kind !== "bin" && (
                      <Button
                        size="sm"
                        variant="plain"
                        onClick={() => setEdit({ kind: l.kind === "warehouse" ? "zone" : l.kind === "zone" ? "shelf" : "bin", parent_id: l.id, active: true })}
                        data-testid={`loc-add-child-${l.name}`}
                      >
                        Add inside
                      </Button>
                    )}
                    <Button size="sm" variant="plain" onClick={() => setEdit(l)} data-testid={`loc-edit-${l.name}`}>Edit</Button>
                    <IconButton
                      size="sm"
                      variant="plain"
                      color="danger"
                      aria-label={`Delete ${l.name}`}
                      onClick={async () => {
                        if (!window.confirm(`Delete ${l.path}?`)) return;
                        if (await act.run(() => api.deleteLocation(l.id))) reload();
                      }}
                    >
                      ✕
                    </IconButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && <LocationDialog value={edit} all={locs.data ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); }} />}
    </Box>
  );
}

function LocationDialog({ value, all, onClose, onSaved }: { value: Partial<Loc>; all: Loc[]; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState<Partial<Loc>>(value);
  const act = useAction();
  const set = (p: Partial<Loc>) => setV((x) => ({ ...x, ...p }));
  // A node cannot move below itself.
  const blocked = new Set<string>();
  if (v.id) {
    const walk = (id: string) => { blocked.add(id); all.filter((l) => l.parent_id === id).forEach((l) => walk(l.id)); };
    walk(v.id);
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">{v.id ? "Edit location" : "New location"}</Typography>
        <ErrorBar error={act.error} />
        <FormControl>
          <FormLabel>Name</FormLabel>
          <Input value={v.name ?? ""} onChange={(e) => set({ name: e.target.value })} autoFocus data-testid="loc-name" />
        </FormControl>
        <FormControl>
          <FormLabel>Type</FormLabel>
          <Select value={v.kind ?? "warehouse"} onChange={(_, k) => set({ kind: (k ?? "warehouse") as Loc["kind"], parent_id: k === "warehouse" ? "" : v.parent_id })} data-testid="loc-kind">
            {(Object.keys(KIND_LABEL) as (keyof typeof KIND_LABEL)[]).map((k) => (
              <Option key={k} value={k}>{KIND_LABEL[k]}</Option>
            ))}
          </Select>
        </FormControl>
        {v.kind !== "warehouse" && (
          <FormControl>
            <FormLabel>Inside</FormLabel>
            <Select value={v.parent_id ?? ""} onChange={(_, p) => set({ parent_id: p ?? "" })} data-testid="loc-parent">
              {all.filter((l) => !blocked.has(l.id) && l.kind !== "bin").map((l) => (
                <Option key={l.id} value={l.id}>{l.path}</Option>
              ))}
            </Select>
          </FormControl>
        )}
        <FormControl>
          <FormLabel>Code</FormLabel>
          <Input value={v.code ?? ""} onChange={(e) => set({ code: e.target.value })} placeholder="A-01-03" data-testid="loc-code" />
        </FormControl>
        {v.kind === "warehouse" && (
          <FormControl>
            <FormLabel>Address</FormLabel>
            <Input value={v.address ?? ""} onChange={(e) => set({ address: e.target.value })} />
          </FormControl>
        )}
        <FormControl orientation="horizontal" sx={{ gap: 1 }}>
          <Switch checked={!!v.is_default} onChange={(e) => set({ is_default: e.target.checked })} />
          <FormLabel>Default location for receipts and sales</FormLabel>
        </FormControl>
        {v.id && (
          <FormControl orientation="horizontal" sx={{ gap: 1 }}>
            <Switch checked={v.active !== false} onChange={(e) => set({ active: e.target.checked })} />
            <FormLabel>Active</FormLabel>
          </FormControl>
        )}
        <Button
          loading={act.busy}
          onClick={async () => {
            if (await act.run(() => api.saveLocation(v.id ?? "", v))) onSaved();
          }}
          data-testid="loc-save"
        >
          Save
        </Button>
      </ModalDialog>
    </Modal>
  );
}

// ------------------------------------------------------------ reservations

function Reservations({ version }: { version: number }) {
  const [state, setState] = useState("active");
  const rows = useLoad(() => api.reservations(state), [state, version]);
  const act = useAction();
  return (
    <Box>
      <Title
        right={
          <Select size="sm" value={state} onChange={(_, v) => setState(v ?? "active")} sx={{ minWidth: 140 }}>
            <Option value="active">Active</Option>
            <Option value="fulfilled">Fulfilled</Option>
            <Option value="released">Released</Option>
          </Select>
        }
      >
        Reservations
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Sales orders hold stock here. A sale that carries the same reference uses up its reservation; releasing frees the stock again.
        Others cannot remove or transfer reserved units.
      </Typography>
      <ErrorBar error={rows.error || act.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No {state} reservations.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="res-table">
            <thead>
              <tr>
                <th>Reference</th>
                <th>Product</th>
                <th style={right}>Qty</th>
                <th>Created</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.data.map((r) => (
                <tr key={r.id}>
                  <td>{r.ref}</td>
                  <td>{r.product_name}</td>
                  <td style={right}>{qtyText(r.qty)}</td>
                  <td>{dateTime(r.created_at)}</td>
                  <td style={right}>
                    {r.state === "active" && (
                      <Button
                        size="sm"
                        variant="plain"
                        color="danger"
                        onClick={async () => {
                          if (await act.run(() => api.release(r.ref))) void rows.reload();
                        }}
                      >
                        Release
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

export { ProductPicker };
