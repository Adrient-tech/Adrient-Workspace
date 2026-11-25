import { useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { LossReason } from "./stockApi";
import { money, qtyText } from "./logic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, right, useAction, useLoad } from "./stockUi";

export const LOSS_LABEL: Record<LossReason, string> = { damaged: "Damaged", expired: "Expired", lost: "Lost or stolen", internal: "Used in-house" };

export function WriteOffDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const locs = useLoad(() => api.locations(), []);
  const [product, setProduct] = useState("");
  const [loc, setLoc] = useState("");
  const [qty, setQty] = useState("1");
  const [reason, setReason] = useState<LossReason>("damaged");
  const [note, setNote] = useState("");
  const act = useAction();
  const locId = loc || locs.data?.find((l) => l.is_default)?.id || locs.data?.[0]?.id || "";
  const save = async () => {
    const r = await act.run(() => api.writeOff({ product_id: product, location_id: locId, qty: Number(qty), loss_reason: reason, note }));
    if (r !== undefined) {
      setProduct("");
      setQty("1");
      setNote("");
      onDone();
      onClose();
    }
  };
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "95vw" }} data-testid="writeoff-dialog">
        <ModalClose />
        <Typography level="title-lg">Write off stock</Typography>
        <Typography level="body-sm">Takes the stock out for a stated reason. The cost is booked as an inventory adjustment and counted in the shrinkage report.</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>Product</FormLabel><ProductPicker value={product} onChange={(id) => setProduct(id)} testId="writeoff-product" /></FormControl>
        <FormControl>
          <FormLabel>Location</FormLabel>
          <Select size="sm" value={locId} onChange={(_, v) => setLoc(v ?? "")}>
            {(locs.data ?? []).map((l) => <Option key={l.id} value={l.id}>{l.name}</Option>)}
          </Select>
        </FormControl>
        <Box sx={{ display: "flex", gap: 1 }}>
          <FormControl sx={{ flex: 1 }}><FormLabel>Quantity</FormLabel><Input size="sm" type="number" value={qty} onChange={(e) => setQty(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": "writeoff-qty" } }} /></FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Reason</FormLabel>
            <Select size="sm" value={reason} onChange={(_, v) => setReason((v as LossReason) ?? "damaged")} slotProps={{ button: { "data-testid": "writeoff-reason" } as never }}>
              {(Object.keys(LOSS_LABEL) as LossReason[]).map((k) => <Option key={k} value={k}>{LOSS_LABEL[k]}</Option>)}
            </Select>
          </FormControl>
        </Box>
        <FormControl><FormLabel>Note</FormLabel><Input size="sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder="What happened" /></FormControl>
        <Button onClick={save} loading={act.busy} disabled={!product || !(Number(qty) > 0)} data-testid="writeoff-save">Write off</Button>
      </ModalDialog>
    </Modal>
  );
}

export function Shrinkage({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [days, setDays] = useState("30");
  const [dlg, setDlg] = useState(false);
  const rep = useLoad(() => api.shrinkage(Number(days)), [days, version]);
  const d = rep.data;
  const rows = d?.rows ?? [];
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1 }}>
            <Select size="sm" value={days} onChange={(_, v) => setDays(v ?? "30")} sx={{ minWidth: 130 }}>
              <Option value="7">Last 7 days</Option>
              <Option value="30">Last 30 days</Option>
              <Option value="90">Last 90 days</Option>
              <Option value="365">Last year</Option>
            </Select>
            <Button size="sm" onClick={() => setDlg(true)} data-testid="writeoff-new">Write off stock</Button>
          </Box>
        }
      >
        Shrinkage
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>What was thrown away: stock written off as damaged, expired or lost, valued at the cost it was carried at. In-house use is shown apart because it is not a loss.</Typography>
      <ErrorBar error={rep.error} />
      <Loading show={rep.loading && !d} />
      {d && (
        <>
          {d.unspecified_removals > 0 && (
            <Alert color="neutral" variant="soft" sx={{ mb: 1.5 }} data-testid="shrink-unspecified">
              {d.unspecified_removals} removal{d.unspecified_removals === 1 ? "" : "s"} in this period have no reason, so they are not counted here.
            </Alert>
          )}
          {rows.length === 0 ? (
            <Empty>No write-offs in this period.</Empty>
          ) : (
            <>
              <Alert color="warning" variant="soft" sx={{ mb: 1.5 }} data-testid="shrink-total">
                {qtyText(d.total_units)} units worth {money(d.total_value)} lost{d.internal_use_units > 0 ? `, plus ${qtyText(d.internal_use_units)} used in-house` : ""}.
              </Alert>
              <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
                <Table size="sm" data-testid="shrink-table">
                  <thead><tr><th>Product</th><th>Reasons</th><th style={right}>Units</th><th style={right}>Value</th></tr></thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.product_id}>
                        <td>{r.name}</td>
                        <td>{Object.entries(r.reasons).map(([k, v]) => <Chip key={k} size="sm" variant="soft" sx={{ mr: 0.5 }}>{LOSS_LABEL[k as LossReason] ?? k} {qtyText(v)}</Chip>)}</td>
                        <td style={right}>{qtyText(r.units)}</td>
                        <td style={right}>{money(r.value)}</td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </Sheet>
            </>
          )}
        </>
      )}
      <WriteOffDialog open={dlg} onClose={() => setDlg(false)} onDone={() => { onChanged(); void rep.reload(); }} />
    </Box>
  );
}

export function Margins({ version }: { version: number }) {
  const rows = useLoad(() => api.margins(), [version]);
  const below = (rows.data ?? []).filter((r) => r.below_cost).length;
  return (
    <Box>
      <Title>Margins</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>Selling price against cost for every product that has both, thinnest first. Margin is the share of the price you keep.</Typography>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No product has both a price and a cost yet.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <>
          {below > 0 && <Alert color="danger" variant="soft" sx={{ mb: 1.5 }} data-testid="margin-below">{below} product{below === 1 ? " is" : "s are"} priced below cost.</Alert>}
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
            <Table size="sm" data-testid="margin-table">
              <thead><tr><th>Product</th><th style={right}>Price</th><th style={right}>Cost</th><th style={right}>Margin</th><th style={right}>%</th></tr></thead>
              <tbody>
                {rows.data.map((r) => (
                  <tr key={r.product_id}>
                    <td>{r.name}</td>
                    <td style={right}>{money(r.price)}</td>
                    <td style={right}>{money(r.cost)}</td>
                    <td style={{ ...right, color: r.below_cost ? "var(--joy-palette-danger-600)" : undefined }}>{money(r.margin)}</td>
                    <td style={right}>{r.percent}%</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
        </>
      )}
    </Box>
  );
}
