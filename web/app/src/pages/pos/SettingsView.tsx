import { useEffect, useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  DialogActions,
  FormControl,
  FormHelperText,
  FormLabel,
  IconButton,
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
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import { listLocations, searchContacts } from "../inventory/api";
import type { ContactLite } from "../inventory/api";
import type { InvLocation } from "../inventory/types";
import {
  adjustBalance,
  archiveRegister,
  createDiscountPreset,
  createRegister,
  createTaxRate,
  deleteDiscountPreset,
  deleteTaxRate,
  listBalances,
  saveSettings,
  updateDiscountPreset,
  updateRegister,
  updateTaxRate,
} from "./api";
import { money } from "./logic";
import type { PosBalance, PosDiscountPreset, PosRegister, PosSettings, PosTaxRate } from "./types";
import { ErrorAlert, rightCell, useErr } from "./ui";

interface Props {
  registers: PosRegister[];
  taxes: PosTaxRate[];
  presets: PosDiscountPreset[];
  settings: PosSettings;
  onChanged: () => void;
}

function Section({ title, hint, action, children }: { title: string; hint?: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
        <Box>
          <Typography level="title-md">{title}</Typography>
          {hint && <Typography level="body-xs">{hint}</Typography>}
        </Box>
        {action}
      </Box>
      {children}
    </Sheet>
  );
}

export function SettingsView({ registers, taxes, presets, settings, onChanged }: Props) {
  return (
    <Box>
      <StoreSection settings={settings} onChanged={onChanged} />
      <RegistersSection registers={registers} onChanged={onChanged} />
      <TaxesSection taxes={taxes} onChanged={onChanged} />
      <PresetsSection presets={presets} onChanged={onChanged} />
      <BalancesSection settings={settings} />
    </Box>
  );
}

function StoreSection({ settings, onChanged }: { settings: PosSettings; onChanged: () => void }) {
  const [s, setS] = useState(settings);
  const [saved, setSaved] = useState(false);
  const { err, run } = useErr();
  useEffect(() => setS(settings), [settings]);
  const num = (k: "loyalty_earn_per_unit" | "loyalty_point_value") => (
    <Input
      type="number"
      value={s[k]}
      onChange={(e) => setS({ ...s, [k]: parseFloat(e.target.value) || 0 })}
      slotProps={{ input: { min: 0, step: "any", "data-testid": `pos-set-${k}` } }}
    />
  );
  return (
    <Section title="Store and receipts" hint="Shown on receipts. Currency is a 3-letter code such as USD or INR.">
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 1.5 }}>
        <FormControl>
          <FormLabel>Store name</FormLabel>
          <Input value={s.store_name} onChange={(e) => setS({ ...s, store_name: e.target.value })} slotProps={{ input: { "data-testid": "pos-set-store" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Currency</FormLabel>
          <Input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value.toUpperCase().slice(0, 3) })} slotProps={{ input: { "data-testid": "pos-set-currency" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Receipt header</FormLabel>
          <Textarea minRows={2} value={s.receipt_header} onChange={(e) => setS({ ...s, receipt_header: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Receipt footer</FormLabel>
          <Textarea minRows={2} value={s.receipt_footer} onChange={(e) => setS({ ...s, receipt_footer: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Loyalty points per 1 spent</FormLabel>
          {num("loyalty_earn_per_unit")}
          <FormHelperText>0 turns loyalty off.</FormHelperText>
        </FormControl>
        <FormControl>
          <FormLabel>Value of one point</FormLabel>
          {num("loyalty_point_value")}
          <FormHelperText>Used when a customer pays with points.</FormHelperText>
        </FormControl>
      </Box>
      <ErrorAlert err={err} />
      <Box sx={{ mt: 1.5, display: "flex", gap: 1, alignItems: "center" }}>
        <Button
          data-testid="pos-set-save"
          onClick={async () => {
            if (await run(() => saveSettings(s))) {
              setSaved(true);
              onChanged();
              setTimeout(() => setSaved(false), 2000);
            }
          }}
        >
          Save
        </Button>
        {saved && <Typography level="body-sm">Saved.</Typography>}
      </Box>
    </Section>
  );
}

function RegistersSection({ registers, onChanged }: { registers: PosRegister[]; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<PosRegister> | null>(null);
  const [locs, setLocs] = useState<InvLocation[]>([]);
  const { err, setErr, run } = useErr();
  useEffect(() => {
    listLocations().then(setLocs).catch(() => setLocs([]));
  }, []);
  return (
    <Section
      title="Registers"
      hint="Each register has its own cash drawer and takes stock from one Inventory location."
      action={
        <Button size="sm" onClick={() => (setErr(""), setEdit({ name: "", location_id: "" }))} data-testid="pos-new-register">
          New register
        </Button>
      }
    >
      <Table aria-label="Registers">
        <thead>
          <tr>
            <th>Name</th>
            <th>Stock location</th>
            <th style={rightCell}></th>
          </tr>
        </thead>
        <tbody>
          {registers.map((r) => (
            <tr key={r.id} data-testid="pos-register-row">
              <td>{r.name}</td>
              <td>{locs.find((l) => l.id === r.location_id)?.name ?? "Default location"}</td>
              <td style={rightCell}>
                <Button size="sm" variant="plain" onClick={() => (setErr(""), setEdit(r))}>
                  Edit
                </Button>
                <IconButton
                  size="sm"
                  variant="plain"
                  color="danger"
                  aria-label={`Archive ${r.name}`}
                  onClick={async () => {
                    if (!window.confirm(`Archive register "${r.name}"? Its sales stay.`)) return;
                    if ((await run(() => archiveRegister(r.id))) !== undefined) onChanged();
                  }}
                >
                  <DeleteOutline fontSize="small" />
                </IconButton>
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!edit && <ErrorAlert err={err} />}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
            <ModalClose />
            <Typography level="title-lg">{edit.id ? "Edit register" : "New register"}</Typography>
            <FormControl required>
              <FormLabel>Name</FormLabel>
              <Input value={edit.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "pos-register-name" } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Stock location</FormLabel>
              <Select value={edit.location_id || "default"} onChange={(_, v) => setEdit({ ...edit, location_id: v === "default" ? "" : (v as string) })}>
                <Option value="default">Default location</Option>
                {locs.map((l) => (
                  <Option key={l.id} value={l.id}>
                    {l.name}
                  </Option>
                ))}
              </Select>
            </FormControl>
            <ErrorAlert err={err} />
            <DialogActions>
              <Button
                data-testid="pos-register-save"
                onClick={async () => {
                  const r = await run(() => (edit.id ? updateRegister(edit.id, edit) : createRegister(edit)));
                  if (r) {
                    setEdit(null);
                    onChanged();
                  }
                }}
              >
                Save
              </Button>
            </DialogActions>
          </ModalDialog>
        </Modal>
      )}
    </Section>
  );
}

function TaxesSection({ taxes, onChanged }: { taxes: PosTaxRate[]; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<PosTaxRate> | null>(null);
  const { err, setErr, run } = useErr();
  return (
    <Section
      title="Tax rates"
      hint="The default rate is applied to new lines. Inclusive means the shelf price already contains the tax."
      action={
        <Button size="sm" onClick={() => (setErr(""), setEdit({ name: "", rate: 0, inclusive: false, is_default: taxes.length === 0 }))} data-testid="pos-new-tax">
          New tax rate
        </Button>
      }
    >
      {taxes.length === 0 ? (
        <Typography level="body-sm">No tax rates. Sales are untaxed until you add one.</Typography>
      ) : (
        <Table aria-label="Tax rates">
          <thead>
            <tr>
              <th>Name</th>
              <th style={rightCell}>Rate</th>
              <th>Type</th>
              <th>Default</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {taxes.map((t) => (
              <tr key={t.id} data-testid="pos-tax-row">
                <td>{t.name}</td>
                <td style={rightCell}>{t.rate}%</td>
                <td>{t.inclusive ? "Inclusive" : "Added on top"}</td>
                <td>{t.is_default ? "Yes" : ""}</td>
                <td style={rightCell}>
                  <Button size="sm" variant="plain" onClick={() => (setErr(""), setEdit(t))}>
                    Edit
                  </Button>
                  <IconButton
                    size="sm"
                    variant="plain"
                    color="danger"
                    aria-label={`Delete ${t.name}`}
                    onClick={async () => {
                      if (!window.confirm(`Delete tax rate "${t.name}"? Past sales keep their tax.`)) return;
                      if ((await run(() => deleteTaxRate(t.id))) !== undefined) onChanged();
                    }}
                  >
                    <DeleteOutline fontSize="small" />
                  </IconButton>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      {!edit && <ErrorAlert err={err} />}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
            <ModalClose />
            <Typography level="title-lg">{edit.id ? "Edit tax rate" : "New tax rate"}</Typography>
            <FormControl required>
              <FormLabel>Name</FormLabel>
              <Input value={edit.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "pos-tax-name" } }} />
            </FormControl>
            <FormControl required>
              <FormLabel>Rate (%)</FormLabel>
              <Input type="number" value={edit.rate ?? 0} onChange={(e) => setEdit({ ...edit, rate: parseFloat(e.target.value) || 0 })} slotProps={{ input: { min: 0, max: 100, step: "any", "data-testid": "pos-tax-rate" } }} />
            </FormControl>
            <Checkbox label="Price includes this tax" checked={!!edit.inclusive} onChange={(e) => setEdit({ ...edit, inclusive: e.target.checked })} />
            <Checkbox label="Default for new lines" checked={!!edit.is_default} onChange={(e) => setEdit({ ...edit, is_default: e.target.checked })} />
            <ErrorAlert err={err} />
            <DialogActions>
              <Button
                data-testid="pos-tax-save"
                onClick={async () => {
                  const r = await run(() => (edit.id ? updateTaxRate(edit.id, edit) : createTaxRate(edit)));
                  if (r) {
                    setEdit(null);
                    onChanged();
                  }
                }}
              >
                Save
              </Button>
            </DialogActions>
          </ModalDialog>
        </Modal>
      )}
    </Section>
  );
}

function PresetsSection({ presets, onChanged }: { presets: PosDiscountPreset[]; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<PosDiscountPreset> | null>(null);
  const { err, setErr, run } = useErr();
  return (
    <Section
      title="Discount presets"
      hint="One-tap order discounts at the register."
      action={
        <Button size="sm" onClick={() => (setErr(""), setEdit({ name: "", kind: "percent", value: 10 }))} data-testid="pos-new-preset">
          New preset
        </Button>
      }
    >
      {presets.length === 0 ? (
        <Typography level="body-sm">No presets yet.</Typography>
      ) : (
        <Table aria-label="Discount presets">
          <tbody>
            {presets.map((p) => (
              <tr key={p.id} data-testid="pos-preset-row">
                <td>{p.name}</td>
                <td style={rightCell}>{p.kind === "percent" ? `${p.value}%` : p.value.toFixed(2)}</td>
                <td style={rightCell}>
                  <Button size="sm" variant="plain" onClick={() => (setErr(""), setEdit(p))}>
                    Edit
                  </Button>
                  <IconButton
                    size="sm"
                    variant="plain"
                    color="danger"
                    aria-label={`Delete ${p.name}`}
                    onClick={async () => {
                      if ((await run(() => deleteDiscountPreset(p.id))) !== undefined) onChanged();
                    }}
                  >
                    <DeleteOutline fontSize="small" />
                  </IconButton>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      {!edit && <ErrorAlert err={err} />}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
            <ModalClose />
            <Typography level="title-lg">{edit.id ? "Edit preset" : "New discount preset"}</Typography>
            <FormControl required>
              <FormLabel>Name</FormLabel>
              <Input value={edit.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} slotProps={{ input: { "data-testid": "pos-preset-name" } }} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Type</FormLabel>
                <Select value={edit.kind ?? "percent"} onChange={(_, v) => setEdit({ ...edit, kind: v as "percent" | "amount" })}>
                  <Option value="percent">Percent</Option>
                  <Option value="amount">Amount</Option>
                </Select>
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Value</FormLabel>
                <Input type="number" value={edit.value ?? 0} onChange={(e) => setEdit({ ...edit, value: parseFloat(e.target.value) || 0 })} slotProps={{ input: { min: 0, step: "any", "data-testid": "pos-preset-value" } }} />
              </FormControl>
            </Box>
            <ErrorAlert err={err} />
            <DialogActions>
              <Button
                data-testid="pos-preset-save"
                onClick={async () => {
                  const r = await run(() => (edit.id ? updateDiscountPreset(edit.id, edit) : createDiscountPreset(edit)));
                  if (r) {
                    setEdit(null);
                    onChanged();
                  }
                }}
              >
                Save
              </Button>
            </DialogActions>
          </ModalDialog>
        </Modal>
      )}
    </Section>
  );
}

function BalancesSection({ settings }: { settings: PosSettings }) {
  const [balances, setBalances] = useState<PosBalance[]>([]);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [adj, setAdj] = useState<{ contact: string; kind: "store_credit" | "loyalty"; amount: string; note: string } | null>(null);
  const { err, setErr, run } = useErr();
  const load = () => listBalances().then(setBalances).catch(() => setBalances([]));
  useEffect(() => {
    load();
    searchContacts().then(setContacts).catch(() => setContacts([]));
  }, []);
  const name = (id: string) => {
    const c = contacts.find((x) => x.id === id);
    return c ? c.display_name || c.company || c.emails?.[0] || id : id;
  };
  return (
    <Section
      title="Customer balances"
      hint="Store credit and loyalty points per contact. Credit comes from refunds or a manual top-up."
      action={
        <Button size="sm" variant="outlined" onClick={() => (setErr(""), setAdj({ contact: "", kind: "store_credit", amount: "", note: "" }))} data-testid="pos-adjust-balance">
          Adjust balance
        </Button>
      }
    >
      {balances.length === 0 ? (
        <Typography level="body-sm">No customer has credit or points yet.</Typography>
      ) : (
        <Table aria-label="Customer balances">
          <thead>
            <tr>
              <th>Customer</th>
              <th style={rightCell}>Store credit</th>
              <th style={rightCell}>Points</th>
            </tr>
          </thead>
          <tbody>
            {balances.map((b) => (
              <tr key={b.contact_id} data-testid="pos-balance-row">
                <td>{name(b.contact_id)}</td>
                <td style={rightCell}>{money(b.store_credit, settings.currency)}</td>
                <td style={rightCell}>{b.loyalty_points}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      {adj && (
        <Modal open onClose={() => setAdj(null)}>
          <ModalDialog sx={{ width: 420, maxWidth: "95vw" }}>
            <ModalClose />
            <Typography level="title-lg">Adjust balance</Typography>
            <FormControl required>
              <FormLabel>Customer</FormLabel>
              <Select placeholder="Choose a contact" value={adj.contact || null} onChange={(_, v) => setAdj({ ...adj, contact: (v as string) ?? "" })} slotProps={{ button: { "data-testid": "pos-adj-contact" } }}>
                {contacts.map((c) => (
                  <Option key={c.id} value={c.id}>
                    {c.display_name || c.company || c.emails?.[0]}
                  </Option>
                ))}
              </Select>
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Balance</FormLabel>
                <Select value={adj.kind} onChange={(_, v) => setAdj({ ...adj, kind: v as "store_credit" | "loyalty" })}>
                  <Option value="store_credit">Store credit</Option>
                  <Option value="loyalty">Loyalty points</Option>
                </Select>
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Change (+/-)</FormLabel>
                <Input type="number" value={adj.amount} onChange={(e) => setAdj({ ...adj, amount: e.target.value })} slotProps={{ input: { step: "any", "data-testid": "pos-adj-amount" } }} />
              </FormControl>
            </Box>
            <FormControl>
              <FormLabel>Note</FormLabel>
              <Input value={adj.note} onChange={(e) => setAdj({ ...adj, note: e.target.value })} />
            </FormControl>
            <ErrorAlert err={err} />
            <DialogActions>
              <Button
                disabled={!adj.contact || !parseFloat(adj.amount)}
                data-testid="pos-adj-save"
                onClick={async () => {
                  if (await run(() => adjustBalance(adj.contact, adj.kind, parseFloat(adj.amount), adj.note))) {
                    setAdj(null);
                    load();
                  }
                }}
              >
                Save
              </Button>
            </DialogActions>
          </ModalDialog>
        </Modal>
      )}
    </Section>
  );
}
