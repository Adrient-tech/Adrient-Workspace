import { useMemo, useState } from "react";
import { Alert, Box, Button, Checkbox, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Typography } from "@mui/joy";
import { api } from "./stockApi";
import { LABEL_PRESETS, labelBarcodeSvg, labelSlots, sheetCount } from "./stockLogic";
import { ProductPicker, Title, useLoad } from "./stockUi";
import type { InvProduct } from "./types";
import { money } from "./logic";

interface Item {
  title: string;
  sub: string;
  code: string;
  copies: number;
}

/** Label sheets for products, shelf/bin locations and free text. Prints from the browser. */
export function LabelsView() {
  const locs = useLoad(() => api.locations(), []);
  const [presetId, setPresetId] = useState(LABEL_PRESETS[0].id);
  const [items, setItems] = useState<Item[]>([]);
  const [start, setStart] = useState("1");
  const [showPrice, setShowPrice] = useState(true);
  const [custom, setCustom] = useState("");
  const preset = LABEL_PRESETS.find((p) => p.id === presetId) ?? LABEL_PRESETS[0];
  const expanded = useMemo(() => items.flatMap((it) => Array.from({ length: Math.max(0, it.copies) }, () => it)), [items]);
  const startAt = Math.max(0, (Number(start) || 1) - 1);
  const slots = useMemo(() => labelSlots(preset, expanded.length, startAt), [preset, expanded.length, startAt]);
  const pages = sheetCount(preset, expanded.length, startAt);
  const addProduct = (_id: string, p?: InvProduct) => {
    if (!p) return;
    const code = p.barcode || p.sku;
    if (!code) { window.alert("This product has no barcode or SKU to print. Add one in the product first."); return; }
    setItems((l) => [...l, { title: p.name + (p.variant_label ? ` / ${p.variant_label}` : ""), sub: showPrice && p.price > 0 ? money(p.price) : p.sku, code, copies: 1 }]);
  };
  const set = (i: number, p: Partial<Item>) => setItems((l) => l.map((x, j) => (j === i ? { ...x, ...p } : x)));
  return (
    <Box>
      <style>{`
        @media print {
          body * { visibility: hidden !important; }
          .lbl-print, .lbl-print * { visibility: visible !important; }
          .lbl-print { position: absolute; left: 0; top: 0; }
          .lbl-page { break-after: page; box-shadow: none !important; margin: 0 !important; }
          @page { size: ${preset.pageW}mm ${preset.pageH}mm; margin: 0; }
        }
      `}</style>
      <Title right={<Button size="sm" disabled={expanded.length === 0} onClick={() => window.print()} data-testid="lbl-print">Print {pages > 0 ? `${pages} sheet${pages === 1 ? "" : "s"}` : ""}</Button>}>Barcode labels</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Pick a label size, add products, shelf or bin locations, or your own text, then print. When printing, set margins to none and scale to 100%. Barcodes are EAN-13 where the code is one, otherwise Code 128.
      </Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "320px 1fr" }, gap: 2 }}>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25 }} data-testid="lbl-controls">
          <FormControl><FormLabel>Label size</FormLabel>
            <Select value={presetId} onChange={(_, v) => setPresetId(v ?? LABEL_PRESETS[0].id)} data-testid="lbl-preset">
              {LABEL_PRESETS.map((p) => <Option key={p.id} value={p.id}>{p.name}</Option>)}
            </Select>
          </FormControl>
          {preset.cols * preset.rows > 1 && (
            <FormControl><FormLabel>Start at label no. (to reuse a part-used sheet)</FormLabel><Input type="number" value={start} onChange={(e) => setStart(e.target.value)} slotProps={{ input: { min: 1, max: preset.cols * preset.rows, "data-testid": "lbl-start" } }} /></FormControl>
          )}
          <Checkbox label="Show price" checked={showPrice} onChange={(e) => setShowPrice(e.target.checked)} />
          <FormControl><FormLabel>Add a product</FormLabel><ProductPicker value="" onChange={addProduct} testId="lbl-product" /></FormControl>
          <FormControl><FormLabel>Add a shelf or bin</FormLabel>
            <Select value="" placeholder="Choose location" onChange={(_, id) => { const l = (locs.data ?? []).find((x) => x.id === id); if (l) setItems((it) => [...it, { title: l.path, sub: l.kind, code: l.code || l.name, copies: 1 }]); }} data-testid="lbl-location">
              {(locs.data ?? []).map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
            </Select>
          </FormControl>
          <FormControl><FormLabel>Add your own text or code</FormLabel>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="LOT-2026-A" data-testid="lbl-custom" />
              <Button variant="outlined" disabled={!custom.trim()} onClick={() => { setItems((it) => [...it, { title: custom.trim(), sub: "", code: custom.trim(), copies: 1 }]); setCustom(""); }} data-testid="lbl-custom-add">Add</Button>
            </Box>
          </FormControl>
          {items.map((it, i) => (
            <Sheet key={i} variant="soft" sx={{ p: 1, borderRadius: "md", display: "flex", gap: 1, alignItems: "center" }}>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography level="title-sm" noWrap>{it.title}</Typography>
                <Typography level="body-xs" noWrap>{it.code}</Typography>
              </Box>
              <Input size="sm" type="number" sx={{ width: 72 }} value={it.copies} onChange={(e) => set(i, { copies: Math.max(0, Math.min(500, Number(e.target.value) || 0)) })} slotProps={{ input: { min: 0, max: 500, "aria-label": `Copies of ${it.title}`, "data-testid": `lbl-copies-${i}` } }} />
              <IconButton size="sm" variant="plain" aria-label="Remove" onClick={() => setItems((l) => l.filter((_, j) => j !== i))}>✕</IconButton>
            </Sheet>
          ))}
          {expanded.length > 0 && <Alert variant="soft" color="neutral">{expanded.length} label{expanded.length === 1 ? "" : "s"} on {pages} sheet{pages === 1 ? "" : "s"}.</Alert>}
        </Box>
        <Box sx={{ overflow: "auto", bgcolor: "background.level1", p: 1.5, borderRadius: "md", minHeight: 240 }} data-testid="lbl-preview">
          <Box className="lbl-print">
            {Array.from({ length: pages }, (_, pg) => (
              <Box
                key={pg}
                className="lbl-page"
                sx={{ position: "relative", width: `${preset.pageW}mm`, height: `${preset.pageH}mm`, bgcolor: "#fff", color: "#000", mb: 2, boxShadow: "0 1px 6px rgba(0,0,0,.25)", overflow: "hidden", flexShrink: 0 }}
                data-testid={`lbl-page-${pg}`}
              >
                {slots.map((s, i) => s.page !== pg ? null : (
                  <Box
                    key={i}
                    sx={{ position: "absolute", left: `${s.x}mm`, top: `${s.y}mm`, width: `${preset.labelW}mm`, height: `${preset.labelH}mm`, p: "1mm", boxSizing: "border-box", display: "flex", flexDirection: "column", overflow: "hidden", lineHeight: 1.1 }}
                  >
                    <Box sx={{ fontSize: preset.labelH < 25 ? "6.5pt" : "8.5pt", fontWeight: 700, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{expanded[i].title}</Box>
                    {expanded[i].sub && <Box sx={{ fontSize: preset.labelH < 25 ? "6pt" : "8pt" }}>{expanded[i].sub}</Box>}
                    <Box sx={{ flex: 1, minHeight: 0 }} dangerouslySetInnerHTML={{ __html: labelBarcodeSvg(expanded[i].code, 40) }} />
                  </Box>
                ))}
              </Box>
            ))}
            {pages === 0 && <Typography level="body-sm" sx={{ p: 2 }}>Add something to see the sheet.</Typography>}
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
