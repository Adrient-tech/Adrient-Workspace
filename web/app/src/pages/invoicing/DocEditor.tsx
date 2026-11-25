import { useEffect, useMemo, useState } from "react";
import {
  Autocomplete,
  Box,
  Button,
  Checkbox,
  DialogActions,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Textarea,
  Typography,
} from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import { createContact, createDocument, listContacts, listPriceLists, listProducts, resolvePricing, updateDocument } from "./api";
import type { PriceList } from "./api";
import type { ContactLite, ProductLite } from "./api";
import { GST_STATES, computeTotals, emptyLine, isIntraState, money, splitSchedule } from "./logic";
import type { DocInput, InvDocument, Item, Settings, TaxRate } from "./types";
import { ErrorAlert, useErr } from "./ui";

export function DocEditor({
  initial,
  editing,
  settings,
  items,
  taxes,
  onClose,
  onSaved,
}: {
  initial: DocInput;
  editing: InvDocument | null;
  settings: Settings;
  items: Item[];
  taxes: TaxRate[];
  onClose: () => void;
  onSaved: (d: InvDocument) => void;
}) {
  const [doc, setDoc] = useState<DocInput>(initial);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [products, setProducts] = useState<ProductLite[]>([]);
  const [busy, setBusy] = useState(false);
  const [newContact, setNewContact] = useState<{ name: string; email: string } | null>(null);
  const [instalments, setInstalments] = useState(2);
  const [priceLists, setPriceLists] = useState<PriceList[]>([]);
  const [pricingNote, setPricingNote] = useState("");
  const { err, run, setErr } = useErr();

  useEffect(() => {
    listContacts().then(setContacts);
    listPriceLists().then(setPriceLists).catch(() => undefined);
  }, []);
  const set = (p: Partial<DocInput>) => setDoc((d) => ({ ...d, ...p }));
  const totals = useMemo(() => computeTotals(doc.lines, doc.discount_kind, doc.discount_value), [doc.lines, doc.discount_kind, doc.discount_value]);
  const setLine = (i: number, p: Partial<DocInput["lines"][number]>) =>
    set({ lines: doc.lines.map((l, j) => (j === i ? { ...l, ...p } : l)) });
  const isEstimate = doc.kind === "estimate";
  const m = (n: number) => money(n, doc.currency);
  const contact = contacts.find((c) => c.id === doc.contact_id) ?? null;

  const pickContact = (c: ContactLite | null) => {
    if (!c) return set({ contact_id: "" });
    set({ contact_id: c.id, client_name: c.company || c.display_name, client_email: c.emails?.[0] ?? doc.client_email });
  };

  const addProduct = (p: ProductLite | null) => {
    if (!p) return;
    set({ lines: [...doc.lines.filter((l) => l.description || l.price), { ...emptyLine(), description: p.name, price: p.price, unit: p.unit ?? "", product_id: p.id }] });
  };
  const addItem = (it: Item | null) => {
    if (!it) return;
    set({ lines: [...doc.lines.filter((l) => l.description || l.price), { ...emptyLine(), description: it.name + (it.description ? ` - ${it.description}` : ""), price: it.price, unit: it.unit, tax_rate: it.tax_rate }] });
  };

  const gstOn = !!settings.sales?.state_code;
  const supplier = settings.sales?.state_code ?? "";
  const intra = isIntraState(supplier, doc.place_of_supply ?? "");

  const applyPricing = async () => {
    const r = await run(() => resolvePricing({ contact_id: doc.contact_id, price_list_id: doc.price_list_id, date: doc.issue_date, lines: doc.lines }));
    if (!r) return;
    set({ lines: r.lines, discount_kind: r.discount_value > 0 ? r.discount_kind : doc.discount_kind, discount_value: r.discount_value > 0 ? r.discount_value : doc.discount_value });
    const changed = r.sources.filter((x) => x !== "base").length + r.applied.length;
    setPricingNote(changed ? `Applied: ${r.sources.filter((x) => x !== "base").length} price(s) from lists, ${r.applied.map((a) => a.rule).join(", ") || "no discount rules"}.` : "Nothing to apply: no matching price list rows or discount rules.");
  };

  const save = async () => {
    setBusy(true);
    const out = await run(async () => {
      const body = { ...doc, lines: doc.lines.filter((l) => l.description.trim() || l.price) };
      return editing ? updateDocument(editing.id, body) : createDocument(body);
    });
    setBusy(false);
    if (out) onSaved(out);
  };

  const quickCreate = async () => {
    if (!newContact?.name.trim()) return setErr("Enter a name for the new contact.");
    const c = await run(() => createContact({ display_name: newContact.name.trim(), emails: newContact.email.trim() ? [newContact.email.trim()] : [] }));
    if (c) {
      setContacts((l) => [c, ...l]);
      set({ contact_id: c.id, client_name: c.display_name, client_email: newContact.email.trim() });
      setNewContact(null);
    }
  };

  const scheduleSum = doc.schedule.reduce((s, x) => s + (x.amount || 0), 0);

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(960px, 96vw)", maxHeight: "94vh", overflow: "auto" }} data-testid="invc-editor">
        <ModalClose />
        <Typography level="h4">
          {editing ? `Edit ${editing.number}` : isEstimate ? "New estimate" : "New invoice"}
        </Typography>
        <ErrorAlert err={err} />

        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
          <Box>
            <FormControl>
              <FormLabel>Client (from Contacts)</FormLabel>
              <Autocomplete
                placeholder="Search contacts"
                options={contacts}
                value={contact}
                getOptionLabel={(c) => c.company || c.display_name}
                onChange={(_, v) => pickContact(v)}
                slotProps={{ input: { "data-testid": "invc-client-pick" } }}
              />
            </FormControl>
            <Button size="sm" variant="plain" sx={{ mt: 0.5 }} onClick={() => setNewContact(newContact ? null : { name: doc.client_name, email: doc.client_email })} data-testid="invc-new-contact">
              + New contact
            </Button>
            {newContact && (
              <Box sx={{ display: "flex", gap: 1, mb: 1, flexWrap: "wrap" }}>
                <Input placeholder="Name" value={newContact.name} onChange={(e) => setNewContact({ ...newContact, name: e.target.value })} slotProps={{ input: { "data-testid": "invc-nc-name" } }} />
                <Input placeholder="Email" value={newContact.email} onChange={(e) => setNewContact({ ...newContact, email: e.target.value })} />
                <Button size="sm" onClick={quickCreate} data-testid="invc-nc-save">
                  Create
                </Button>
              </Box>
            )}
            <FormControl sx={{ mt: 1 }}>
              <FormLabel>Client name</FormLabel>
              <Input value={doc.client_name} onChange={(e) => set({ client_name: e.target.value })} slotProps={{ input: { "data-testid": "invc-client-name" } }} />
            </FormControl>
            <FormControl sx={{ mt: 1 }}>
              <FormLabel>Client email</FormLabel>
              <Input value={doc.client_email} onChange={(e) => set({ client_email: e.target.value })} />
            </FormControl>
            <FormControl sx={{ mt: 1 }}>
              <FormLabel>Client address</FormLabel>
              <Textarea minRows={2} value={doc.client_address} onChange={(e) => set({ client_address: e.target.value })} />
            </FormControl>
            {gstOn && (
              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1, mt: 1 }}>
                <FormControl>
                  <FormLabel>Client GSTIN</FormLabel>
                  <Input value={doc.client_gstin ?? ""} onChange={(e) => set({ client_gstin: e.target.value.toUpperCase() })} slotProps={{ input: { "data-testid": "invc-client-gstin", maxLength: 15 } }} />
                </FormControl>
                <FormControl>
                  <FormLabel>Place of supply</FormLabel>
                  <Select
                    value={doc.place_of_supply || ""}
                    onChange={(_, v) => set({ place_of_supply: (v as string) || "" })}
                    data-testid="invc-pos"
                  >
                    <Option value="">Same as my state</Option>
                    {GST_STATES.map(([c, n]) => (
                      <Option key={c} value={c}>
                        {c} {n}
                      </Option>
                    ))}
                  </Select>
                </FormControl>
                <Typography level="body-xs" sx={{ gridColumn: "1 / -1" }} data-testid="invc-gst-mode">
                  {intra ? "Within the state: CGST + SGST" : "Between states: IGST"}
                </Typography>
              </Box>
            )}
          </Box>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1, alignContent: "start" }}>
            <FormControl>
              <FormLabel>Currency</FormLabel>
              <Input value={doc.currency} onChange={(e) => set({ currency: e.target.value.toUpperCase().slice(0, 3) })} slotProps={{ input: { "data-testid": "invc-currency" } }} />
            </FormControl>
            {editing && (
              <FormControl>
                <FormLabel>Number</FormLabel>
                <Input value={doc.number ?? ""} onChange={(e) => set({ number: e.target.value })} />
              </FormControl>
            )}
            <FormControl>
              <FormLabel>Issue date</FormLabel>
              <Input type="date" value={doc.issue_date} onChange={(e) => set({ issue_date: e.target.value })} />
            </FormControl>
            <FormControl>
              <FormLabel>{isEstimate ? "Valid until" : "Due date"}</FormLabel>
              {isEstimate ? (
                <Input type="date" value={doc.valid_until ?? ""} onChange={(e) => set({ valid_until: e.target.value })} slotProps={{ input: { "data-testid": "invc-valid-until" } }} />
              ) : (
                <Input type="date" value={doc.due_date} onChange={(e) => set({ due_date: e.target.value })} />
              )}
            </FormControl>
            <FormControl>
              <FormLabel>Discount</FormLabel>
              <Input
                type="number"
                value={doc.discount_value}
                onChange={(e) => set({ discount_value: Number(e.target.value) })}
                endDecorator={
                  <Select variant="plain" size="sm" value={doc.discount_kind} onChange={(_, v) => v && set({ discount_kind: v as "percent" | "amount" })}>
                    <Option value="percent">%</Option>
                    <Option value="amount">{doc.currency}</Option>
                  </Select>
                }
              />
            </FormControl>
          </Box>
        </Box>

        <Typography level="title-md" sx={{ mt: 2 }}>
          Items
        </Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1 }}>
          <Autocomplete
            sx={{ minWidth: 240 }}
            placeholder="Add from Inventory"
            options={products}
            getOptionLabel={(p) => `${p.name}${p.sku ? ` (${p.sku})` : ""}`}
            filterOptions={(o) => o}
            onInputChange={(_, v) => listProducts(v).then(setProducts)}
            onOpen={() => listProducts("").then(setProducts)}
            onChange={(_, v) => {
              addProduct(v);
            }}
            value={null}
            slotProps={{ input: { "data-testid": "invc-add-product" } }}
          />
          <Autocomplete
            sx={{ minWidth: 240 }}
            placeholder="Add from price list"
            options={items}
            getOptionLabel={(i) => i.name}
            onChange={(_, v) => addItem(v)}
            value={null}
            slotProps={{ input: { "data-testid": "invc-add-pricelist" } }}
          />
          <Checkbox
            size="sm"
            label="Prices include tax"
            checked={doc.lines.length > 0 && doc.lines.every((l) => !!l.tax_inclusive)}
            onChange={(e) => set({ lines: doc.lines.map((l) => ({ ...l, tax_inclusive: e.target.checked })) })}
            slotProps={{ input: { "data-testid": "invc-tax-inclusive" } }}
          />
          <Button variant="outlined" size="sm" onClick={() => set({ lines: [...doc.lines, emptyLine()] })} data-testid="invc-add-line">
            + Free-text line
          </Button>
          {priceLists.length > 0 && (
            <Select size="sm" placeholder="Price list" value={doc.price_list_id || null} onChange={(_, v) => set({ price_list_id: (v as string) || "" })} sx={{ minWidth: 150 }}>
              <Option value="">Automatic</Option>
              {priceLists.filter((p) => p.active).map((p) => (
                <Option key={p.id} value={p.id}>
                  {p.name}
                </Option>
              ))}
            </Select>
          )}
          <Button variant="outlined" size="sm" onClick={applyPricing} data-testid="invc-apply-pricing">
            Apply prices and discounts
          </Button>
        </Box>
        {pricingNote && (
          <Typography level="body-xs" sx={{ mb: 1 }} data-testid="invc-pricing-note">
            {pricingNote}
          </Typography>
        )}
        <Box sx={{ overflowX: "auto" }}>
          <Box component="table" sx={{ width: "100%", borderCollapse: "collapse", "& td": { p: 0.25, verticalAlign: "top" } }}>
            <thead>
              <tr>
                {["Description", ...(gstOn ? ["HSN/SAC"] : []), "Qty", "Unit", "Price", "Disc %", "Tax %", "Amount", ""].map((h) => (
                  <Box component="th" key={h} sx={{ textAlign: "left", fontSize: 12, fontWeight: 600, p: 0.5 }}>
                    {h}
                  </Box>
                ))}
              </tr>
            </thead>
            <tbody>
              {doc.lines.map((l, i) => (
                <tr key={i} data-testid="invc-line">
                  <td style={{ minWidth: 220 }}>
                    <Input size="sm" value={l.description} onChange={(e) => setLine(i, { description: e.target.value })} slotProps={{ input: { "data-testid": "invc-line-desc" } }} />
                  </td>
                  {gstOn && (
                    <td style={{ width: 100 }}>
                      <Input size="sm" value={l.hsn_sac ?? ""} onChange={(e) => setLine(i, { hsn_sac: e.target.value })} slotProps={{ input: { "data-testid": "invc-line-hsn", maxLength: 8 } }} />
                    </td>
                  )}
                  <td style={{ width: 80 }}>
                    <Input size="sm" type="number" value={l.qty} onChange={(e) => setLine(i, { qty: Number(e.target.value) })} slotProps={{ input: { "data-testid": "invc-line-qty" } }} />
                  </td>
                  <td style={{ width: 90 }}>
                    <Select size="sm" value={l.unit || "-"} onChange={(_, v) => setLine(i, { unit: v === "-" ? "" : (v as string) })}>
                      <Option value="-">-</Option>
                      {[...new Set([...settings.units, ...(l.unit ? [l.unit] : [])])].map((u) => (
                        <Option key={u} value={u}>
                          {u}
                        </Option>
                      ))}
                    </Select>
                  </td>
                  <td style={{ width: 110 }}>
                    <Input size="sm" type="number" value={l.price} onChange={(e) => setLine(i, { price: Number(e.target.value) })} slotProps={{ input: { "data-testid": "invc-line-price" } }} />
                  </td>
                  <td style={{ width: 80 }}>
                    <Input size="sm" type="number" value={l.discount_pct} onChange={(e) => setLine(i, { discount_pct: Number(e.target.value) })} />
                  </td>
                  <td style={{ width: 150 }}>
                    <Input
                      size="sm"
                      type="number"
                      value={l.tax_rate}
                      onChange={(e) => setLine(i, { tax_rate: Number(e.target.value), tax_name: "" })}
                      slotProps={{ input: { "data-testid": "invc-line-tax" } }}
                      endDecorator={
                        taxes.length > 0 && (
                          <Select
                            variant="plain"
                            size="sm"
                            placeholder="POS"
                            value={null}
                            onChange={(_, v) => {
                              const t = taxes.find((x) => x.id === v);
                              if (t) setLine(i, { tax_rate: t.rate, tax_name: t.name });
                            }}
                          >
                            {taxes.map((t) => (
                              <Option key={t.id} value={t.id}>
                                {t.name} {t.rate}%
                              </Option>
                            ))}
                          </Select>
                        )
                      }
                    />
                  </td>
                  <td style={{ width: 110, textAlign: "right", paddingTop: 8 }}>{m(l.qty * l.price * (1 - (l.discount_pct || 0) / 100))}</td>
                  <td style={{ width: 40 }}>
                    <IconButton size="sm" aria-label="Remove line" onClick={() => set({ lines: doc.lines.filter((_, j) => j !== i) })}>
                      <DeleteOutline />
                    </IconButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </Box>
        </Box>
        <Box sx={{ ml: "auto", width: 260, mt: 1 }} data-testid="invc-totals">
          <Box sx={{ display: "flex", justifyContent: "space-between" }}>
            <span>Subtotal</span>
            <span>{m(totals.subtotal)}</span>
          </Box>
          {totals.discount_total > 0 && (
            <Box sx={{ display: "flex", justifyContent: "space-between" }}>
              <span>Discount</span>
              <span>- {m(totals.discount_total)}</span>
            </Box>
          )}
          <Box sx={{ display: "flex", justifyContent: "space-between" }}>
            <span>Tax</span>
            <span>{m(totals.tax_total)}</span>
          </Box>
          <Box sx={{ display: "flex", justifyContent: "space-between", fontWeight: 700 }}>
            <span>Total</span>
            <span data-testid="invc-total">{m(totals.total)}</span>
          </Box>
        </Box>

        {!isEstimate && (
          <>
            <Typography level="title-md" sx={{ mt: 2 }}>
              Payment schedule
            </Typography>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1 }}>
              <Input size="sm" type="number" sx={{ width: 80 }} value={instalments} onChange={(e) => setInstalments(Math.max(1, Number(e.target.value)))} />
              <Button size="sm" variant="outlined" onClick={() => set({ schedule: splitSchedule(totals.total, instalments, doc.due_date || doc.issue_date) })}>
                Split into instalments
              </Button>
              {doc.schedule.length > 0 && (
                <Typography level="body-sm" color={Math.abs(scheduleSum - totals.total) < 0.01 ? "success" : "warning"}>
                  Scheduled {m(scheduleSum)} of {m(totals.total)}
                </Typography>
              )}
            </Box>
            {doc.schedule.map((s, i) => (
              <Box key={i} sx={{ display: "flex", gap: 1, mb: 0.5 }}>
                <Input size="sm" value={s.label} onChange={(e) => set({ schedule: doc.schedule.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)) })} sx={{ flex: 1 }} />
                <Input size="sm" type="date" value={s.due_date} onChange={(e) => set({ schedule: doc.schedule.map((x, j) => (j === i ? { ...x, due_date: e.target.value } : x)) })} />
                <Input size="sm" type="number" sx={{ width: 120 }} value={s.amount} onChange={(e) => set({ schedule: doc.schedule.map((x, j) => (j === i ? { ...x, amount: Number(e.target.value) } : x)) })} />
                <IconButton size="sm" aria-label="Remove instalment" onClick={() => set({ schedule: doc.schedule.filter((_, j) => j !== i) })}>
                  <DeleteOutline />
                </IconButton>
              </Box>
            ))}
            {doc.schedule.length === 0 && <Typography level="body-xs">Optional. Instalments are shown on the document; payments are still recorded against the invoice.</Typography>}
          </>
        )}

        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 1.5, mt: 2 }}>
          <FormControl>
            <FormLabel>Notes</FormLabel>
            <Textarea minRows={2} value={doc.notes} onChange={(e) => set({ notes: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Terms</FormLabel>
            <Textarea minRows={2} value={doc.terms} onChange={(e) => set({ terms: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Tax note (e.g. GSTIN, reverse charge)</FormLabel>
            <Textarea minRows={2} value={doc.tax_note} onChange={(e) => set({ tax_note: e.target.value })} />
          </FormControl>
          <Box>
            <FormLabel>Attachments (links, e.g. a Drive share link)</FormLabel>
            {doc.attachments.map((a, i) => (
              <Box key={i} sx={{ display: "flex", gap: 0.5, mb: 0.5 }}>
                <Input size="sm" placeholder="Name" value={a.name} onChange={(e) => set({ attachments: doc.attachments.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })} />
                <Input size="sm" placeholder="https://..." value={a.url} sx={{ flex: 1 }} onChange={(e) => set({ attachments: doc.attachments.map((x, j) => (j === i ? { ...x, url: e.target.value } : x)) })} />
                <IconButton size="sm" aria-label="Remove attachment" onClick={() => set({ attachments: doc.attachments.filter((_, j) => j !== i) })}>
                  <DeleteOutline />
                </IconButton>
              </Box>
            ))}
            <Button size="sm" variant="plain" onClick={() => set({ attachments: [...doc.attachments, { name: "", url: "" }] })}>
              + Add link
            </Button>
          </Box>
        </Box>

        <DialogActions>
          <Button onClick={save} loading={busy} data-testid="invc-save" sx={{ bgcolor: "#C2410C", "&:hover": { bgcolor: "#9A3412" } }}>
            {editing ? "Save changes" : "Save as draft"}
          </Button>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}
