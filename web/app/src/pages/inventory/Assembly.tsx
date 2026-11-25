import { useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { Bom } from "./stockApi";
import { money, qtyText } from "./logic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, dateTime, right, useAction, useLoad } from "./stockUi";

export function AssemblyView({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [tab, setTab] = useState("boms");
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2 }}>
          <Tab value="boms" data-testid="asm-tab-boms">Kits and bills of material</Tab>
          <Tab value="orders" data-testid="asm-tab-orders">Assembly history</Tab>
        </TabList>
      </Tabs>
      {tab === "boms" ? <Boms version={version} onChanged={onChanged} /> : <Orders version={version} />}
    </Box>
  );
}

function Boms({ version, onChanged }: { version: number; onChanged: () => void }) {
  const list = useLoad(() => api.boms(), [version]);
  const [edit, setEdit] = useState<Bom | "new" | null>(null);
  const [build, setBuild] = useState<Bom | null>(null);
  const act = useAction();
  const reload = () => { onChanged(); void list.reload(); };
  return (
    <Box>
      <Title right={<Button size="sm" onClick={() => setEdit("new")} data-testid="bom-new">New kit or assembly</Button>}>Kits and assemblies</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        A <b>kit</b> is sold as one item; selling it takes its parts out of stock (no stock of its own). An <b>assembly</b> is built ahead: building it uses up the
        parts and adds the finished product to stock at the parts&apos; cost plus labour.
      </Typography>
      <ErrorBar error={list.error || act.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>Nothing defined yet.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="bom-table">
            <thead><tr><th>Product</th><th>Type</th><th>Parts</th><th style={right}>Part cost</th><th style={right}>Can make</th><th /></tr></thead>
            <tbody>
              {list.data.map((b) => (
                <tr key={b.id}>
                  <td>{b.product_name} <Typography level="body-xs" component="span">{b.sku}</Typography></td>
                  <td><Chip size="sm" variant="soft" color={b.kind === "kit" ? "primary" : "success"}>{b.kind === "kit" ? "Kit" : "Assembly"}</Chip></td>
                  <td>{b.lines.map((l) => `${qtyText(l.qty)} × ${l.name}`).join(", ")}</td>
                  <td style={right}>{money(b.unit_cost)}</td>
                  <td style={right}>{qtyText(b.buildable)}</td>
                  <td style={{ ...right, whiteSpace: "nowrap" }}>
                    {b.kind === "assembly" && <Button size="sm" variant="plain" onClick={() => setBuild(b)} data-testid={`bom-build-${b.sku || b.id}`}>Build</Button>}
                    <Button size="sm" variant="plain" onClick={() => setEdit(b)}>Edit</Button>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Remove the parts list of ${b.product_name}?`) && (await act.run(() => api.deleteBom(b.id)))) reload(); }}>Remove</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && <BomDialog value={edit === "new" ? null : edit} onClose={() => setEdit(null)} onDone={() => { setEdit(null); reload(); }} />}
      {build && <BuildDialog bom={build} onClose={() => setBuild(null)} onDone={() => { setBuild(null); reload(); }} />}
    </Box>
  );
}

function BomDialog({ value, onClose, onDone }: { value: Bom | null; onClose: () => void; onDone: () => void }) {
  const [product, setProduct] = useState(value?.product_id ?? "");
  const [kind, setKind] = useState(value?.kind ?? "kit");
  const [qty, setQty] = useState(String(value?.qty ?? 1));
  const [lines, setLines] = useState(value ? value.lines.map((l) => ({ component_id: l.component_id, qty: String(l.qty) })) : [{ component_id: "", qty: "1" }]);
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, sm: 560 }, maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-md">{value ? "Edit parts list" : "New kit or assembly"}</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>Finished product</FormLabel><ProductPicker value={product} testId="bom-product" onChange={(id) => setProduct(id)} /></FormControl>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl><FormLabel>Type</FormLabel>
            <Select value={kind} onChange={(_, v) => setKind((v ?? "kit") as "kit" | "assembly")} data-testid="bom-kind">
              <Option value="kit">Kit (sold as a bundle)</Option>
              <Option value="assembly">Assembly (built into stock)</Option>
            </Select>
          </FormControl>
          <FormControl><FormLabel>Parts below make</FormLabel><Input type="number" value={qty} onChange={(e) => setQty(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": "bom-qty" } }} /></FormControl>
        </Box>
        <Typography level="title-sm">Parts</Typography>
        {lines.map((l, i) => (
          <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Box sx={{ flex: 1 }}><ProductPicker value={l.component_id} testId={`bom-part-${i}`} onChange={(id) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, component_id: id } : x)))} /></Box>
            <Input size="sm" type="number" sx={{ width: 90 }} value={l.qty} onChange={(e) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, qty: e.target.value } : x)))} slotProps={{ input: { min: 0, step: "any", "data-testid": `bom-part-qty-${i}` } }} />
            <IconButton size="sm" variant="plain" aria-label="Remove part" disabled={lines.length === 1} onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}>✕</IconButton>
          </Box>
        ))}
        <Button size="sm" variant="soft" sx={{ alignSelf: "flex-start" }} onClick={() => setLines((ls) => [...ls, { component_id: "", qty: "1" }])}>Add part</Button>
        <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.saveBom({ product_id: product, kind, qty: Number(qty) || 1, lines: lines.filter((l) => l.component_id).map((l) => ({ component_id: l.component_id, qty: Number(l.qty) })) }))) onDone(); }} data-testid="bom-save">Save</Button>
      </ModalDialog>
    </Modal>
  );
}

function BuildDialog({ bom, onClose, onDone }: { bom: Bom; onClose: () => void; onDone: () => void }) {
  const locs = useLoad(() => api.locations(), []);
  const [qty, setQty] = useState("1");
  const [loc, setLoc] = useState("");
  const [labor, setLabor] = useState("0");
  const [lot, setLot] = useState("");
  const [expiry, setExpiry] = useState("");
  const act = useAction();
  const n = Number(qty) || 0;
  const partCost = (bom.unit_cost * n) / (bom.qty > 0 ? 1 : 1);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Build {bom.product_name}</Typography>
        <Typography level="body-sm">You can build up to {qtyText(bom.buildable)} with the parts in stock.</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>How many</FormLabel><Input type="number" value={qty} onChange={(e) => setQty(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": "build-qty" } }} /></FormControl>
        <FormControl><FormLabel>Where (parts leave and the product arrives here)</FormLabel>
          <Select value={loc} onChange={(_, v) => setLoc(v ?? "")} data-testid="build-location">
            <Option value="">Default location</Option>
            {(locs.data ?? []).map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
          </Select>
        </FormControl>
        <FormControl><FormLabel>Labour cost for this build</FormLabel><Input type="number" value={labor} onChange={(e) => setLabor(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": "build-labor" } }} /></FormControl>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl><FormLabel>Lot (if tracked)</FormLabel><Input value={lot} onChange={(e) => setLot(e.target.value)} /></FormControl>
          <FormControl><FormLabel>Expiry</FormLabel><Input type="date" value={expiry} onChange={(e) => setExpiry(e.target.value)} /></FormControl>
        </Box>
        <Alert variant="soft" color="neutral">Estimated part cost {money(partCost)}; the finished cost per unit is set from the parts actually used.</Alert>
        <Button loading={act.busy} onClick={async () => { if (await act.run(() => api.assemble({ bom_id: bom.id, qty: n, location_id: loc || undefined, labor_cost: Number(labor) || 0, lot_no: lot || undefined, expiry: expiry || undefined }))) onDone(); }} data-testid="build-save">Build</Button>
      </ModalDialog>
    </Modal>
  );
}

function Orders({ version }: { version: number }) {
  const list = useLoad(() => api.assemblies(), [version]);
  return (
    <Box>
      <Title>Assembly history</Title>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>Nothing built yet.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="asm-table">
            <thead><tr><th>Number</th><th>Product</th><th style={right}>Built</th><th style={right}>Labour</th><th style={right}>Cost each</th><th>When</th></tr></thead>
            <tbody>
              {list.data.map((a) => (
                <tr key={a.id}><td>{a.number}</td><td>{a.product_name}</td><td style={right}>{qtyText(a.qty)}</td><td style={right}>{money(a.labor_cost)}</td><td style={right}>{money(a.unit_cost)}</td><td>{dateTime(a.created_at)}</td></tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}
