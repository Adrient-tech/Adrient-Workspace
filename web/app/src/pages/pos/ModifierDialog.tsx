import { useState } from "react";
import { Box, Button, Checkbox, Modal, ModalClose, ModalDialog, Radio, RadioGroup, Typography } from "@mui/joy";
import type { ModGroup, ModOption } from "./extApi";
import { money, round2 } from "./logic";

export interface ModChoice {
  group: string;
  options: ModOption[];
}

/** Final line name and price after options: "Latte (Large, Oat milk)" at base + sum of option prices. */
export function applyModifiers(baseName: string, basePrice: number, choices: ModChoice[]): { name: string; price: number; note: string } {
  const picked = choices.flatMap((c) => c.options);
  const price = round2(basePrice + picked.reduce((s, o) => s + o.price, 0));
  const name = picked.length ? `${baseName} (${picked.map((o) => o.name).join(", ")})` : baseName;
  return { name, price, note: "" };
}

/** Required groups with nothing picked, so the dialog can block "Add". */
export function missingRequired(groups: ModGroup[], picks: Record<string, string[]>): string[] {
  return groups.filter((g) => g.required && !(picks[g.id]?.length > 0)).map((g) => g.name);
}

export function ModifierDialog({
  productName,
  basePrice,
  groups,
  currency,
  onClose,
  onAdd,
}: {
  productName: string;
  basePrice: number;
  groups: ModGroup[];
  currency: string;
  onClose: () => void;
  onAdd: (name: string, price: number) => void;
}) {
  const [picks, setPicks] = useState<Record<string, string[]>>({});
  const m = (n: number) => money(n, currency);
  const choices: ModChoice[] = groups.map((g) => ({ group: g.name, options: g.options.filter((o) => (picks[g.id] ?? []).includes(o.id)) }));
  const res = applyModifiers(productName, basePrice, choices);
  const missing = missingRequired(groups, picks);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }} data-testid="pos-mod-dialog">
        <ModalClose />
        <Typography level="title-lg">{productName}</Typography>
        {groups.map((g) => (
          <Box key={g.id} data-testid="pos-mod-group">
            <Typography level="title-sm">
              {g.name}
              {g.required ? " (required)" : ""}
              {g.multi ? " · pick any" : ""}
            </Typography>
            {g.multi ? (
              g.options.map((o) => (
                <Checkbox
                  key={o.id}
                  sx={{ display: "flex", my: 0.5 }}
                  label={`${o.name}${o.price ? `  ${o.price > 0 ? "+" : ""}${m(o.price)}` : ""}`}
                  checked={(picks[g.id] ?? []).includes(o.id)}
                  onChange={(e) => setPicks({ ...picks, [g.id]: e.target.checked ? [...(picks[g.id] ?? []), o.id] : (picks[g.id] ?? []).filter((x) => x !== o.id) })}
                  slotProps={{ input: { "data-testid": "pos-mod-opt" } }}
                />
              ))
            ) : (
              <RadioGroup value={(picks[g.id] ?? [])[0] ?? ""} onChange={(e) => setPicks({ ...picks, [g.id]: [e.target.value] })}>
                {g.options.map((o) => (
                  <Radio key={o.id} value={o.id} sx={{ my: 0.5 }} label={`${o.name}${o.price ? `  ${o.price > 0 ? "+" : ""}${m(o.price)}` : ""}`} slotProps={{ input: { "data-testid": "pos-mod-opt" } }} />
                ))}
              </RadioGroup>
            )}
          </Box>
        ))}
        {missing.length > 0 && <Typography level="body-xs" color="warning">Choose: {missing.join(", ")}</Typography>}
        <Button disabled={missing.length > 0} onClick={() => onAdd(res.name, res.price)} data-testid="pos-mod-add">
          Add · {m(res.price)}
        </Button>
      </ModalDialog>
    </Modal>
  );
}
