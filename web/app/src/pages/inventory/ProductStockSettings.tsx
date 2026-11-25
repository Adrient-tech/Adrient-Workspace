import { useEffect, useState } from "react";
import { Box, Button, FormControl, FormLabel, Input, Option, Select, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { Loc, ProductSettings } from "./stockApi";
import { ErrorBar, useAction } from "./stockUi";

/** Shelf/bin code of a product at a location, edited in place. */
export function ShelfCell({ productId, locationId, value, onSaved }: { productId: string; locationId: string; value: string; onSaved: () => void }) {
  const [v, setV] = useState(value);
  const [err, setErr] = useState("");
  useEffect(() => setV(value), [value]);
  const save = async () => {
    if (v.trim() === value) return;
    try {
      setErr("");
      await api.setShelf(productId, locationId, v.trim());
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  return (
    <Box>
      <Input
        size="sm"
        value={v}
        placeholder="Shelf code"
        onChange={(e) => setV(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); (e.target as HTMLInputElement).blur(); } }}
        slotProps={{ input: { "aria-label": "Shelf code", "data-testid": `shelf-${locationId}` } }}
        sx={{ minWidth: 110 }}
      />
      {err && <Typography level="body-xs" color="danger">{err}</Typography>}
    </Box>
  );
}

/** Valuation method, lot/serial tracking and shelf life, plus shelf codes at other locations. */
export function StockSettings({ productId, onChanged }: { productId: string; onChanged: () => void }) {
  const [s, setS] = useState<ProductSettings | null>(null);
  const [locs, setLocs] = useState<Loc[]>([]);
  const [shelfLoc, setShelfLoc] = useState("");
  const [shelfCode, setShelfCode] = useState("");
  const [saved, setSaved] = useState("");
  const act = useAction();
  useEffect(() => {
    api.productSettings(productId).then(setS).catch(() => setS(null));
    api.locations().then(setLocs).catch(() => setLocs([]));
  }, [productId]);
  if (!s) return null;
  const set = (p: Partial<ProductSettings>) => { setSaved(""); setS({ ...s, ...p }); };
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }} data-testid="stock-settings">
      <Typography level="title-md">Stock control</Typography>
      <ErrorBar error={act.error} />
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(170px, 1fr))", gap: 1, alignItems: "end" }}>
        <FormControl>
          <FormLabel>Costing method</FormLabel>
          <Select value={s.valuation_method} onChange={(_, v) => set({ valuation_method: (v ?? "avg") as "avg" | "fifo" })} data-testid="ss-method">
            <Option value="avg">Weighted average</Option>
            <Option value="fifo">FIFO (oldest cost first)</Option>
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Tracking</FormLabel>
          <Select value={s.tracking} onChange={(_, v) => set({ tracking: (v ?? "none") as ProductSettings["tracking"] })} data-testid="ss-tracking">
            <Option value="none">None</Option>
            <Option value="lot">Lots / batches with expiry</Option>
            <Option value="serial">Serial numbers</Option>
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Shelf life (days)</FormLabel>
          <Input type="number" value={s.shelf_life_days} onChange={(e) => set({ shelf_life_days: Number(e.target.value) || 0 })} slotProps={{ input: { min: 0 } }} />
        </FormControl>
        <Button
          variant="outlined"
          loading={act.busy}
          onClick={async () => {
            const r = await act.run(() => api.saveProductSettings(productId, { valuation_method: s.valuation_method, tracking: s.tracking, shelf_life_days: s.shelf_life_days }));
            if (r) { setS(r); setSaved("Saved"); onChanged(); }
          }}
          data-testid="ss-save"
        >
          Save stock control
        </Button>
      </Box>
      {saved && <Typography level="body-sm" color="success">{saved}</Typography>}
      <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap" }}>
        <FormControl sx={{ minWidth: 200 }}>
          <FormLabel>Shelf or bin at another location</FormLabel>
          <Select value={shelfLoc} onChange={(_, v) => setShelfLoc(v ?? "")} placeholder="Choose location" data-testid="ss-shelf-loc">
            {locs.map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Code</FormLabel>
          <Input value={shelfCode} onChange={(e) => setShelfCode(e.target.value)} placeholder="A-03-2" data-testid="ss-shelf-code" />
        </FormControl>
        <Button
          variant="outlined"
          disabled={!shelfLoc}
          onClick={async () => { if (await act.run(() => api.setShelf(productId, shelfLoc, shelfCode.trim()))) { setShelfCode(""); setSaved("Shelf saved"); onChanged(); } }}
          data-testid="ss-shelf-save"
        >
          Set shelf
        </Button>
      </Box>
    </Box>
  );
}
