import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  Chip,
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
  Sheet,
  Textarea,
  Typography,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import RemoveIcon from "@mui/icons-material/Remove";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import { listProducts, photoURL, searchContacts } from "../inventory/api";
import type { ContactLite } from "../inventory/api";
import type { InvProduct } from "../inventory/types";
import { completeSale, deleteHeldSale, getBalance, getSale, holdSale, listSales, openShift } from "./api";
import { getExt, getExtCaps, getFavourites, getLoyalty, getModifiers } from "./extApi";
import type { ExtSettings, Favourite, Loyalty, ModGroup } from "./extApi";
import { EmailReceiptDialog, GiftCardLookup, OfflineBar, PinDialog, useOutbox } from "./PosExtDialogs";
import { ModifierDialog } from "./ModifierDialog";
import { cacheGet, cachePut, completeOrQueue, offlineSale } from "./saleSync";
import { idleDisplay, publishDisplay } from "./Display";
import { connectPrinter, kickDrawer, printerConnected, receiptBytes, sendToPrinter, usbSupport } from "./hardware";
import {
  addToCart,
  cartFromSale,
  methodLabel,
  money,
  pointsFor,
  priceCart,
  qtyText,
  quickTender,
  receiptText,
  round2,
  summarizeTenders,
  toPaymentInputs,
} from "./logic";
import type { Tender } from "./logic";
import type {
  CartLine,
  DiscountKind,
  PayMethod,
  PosBalance,
  PosDiscountPreset,
  PosRegister,
  PosSale,
  PosSettings,
  PosTaxRate,
} from "./types";
import { ACCENT, Empty, ErrorAlert, Receipt, emailReceiptHref, rightCell, useErr } from "./ui";

interface Props {
  registers: PosRegister[];
  taxes: PosTaxRate[];
  presets: PosDiscountPreset[];
  settings: PosSettings;
  registerId: string;
  setRegisterId: (id: string) => void;
  onChanged: () => void;
}

export function RegisterView({ registers, taxes, presets, settings, registerId, setRegisterId, onChanged }: Props) {
  const reg = registers.find((r) => r.id === registerId);
  const defaultTax = taxes.find((t) => t.is_default)?.id ?? "";
  const m = (n: number) => money(n, settings.currency);

  const [products, setProducts] = useState<InvProduct[]>([]);
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const [cart, setCart] = useState<CartLine[]>([]);
  const [orderKind, setOrderKind] = useState<DiscountKind>("");
  const [orderValue, setOrderValue] = useState(0);
  const [customer, setCustomer] = useState<{ id: string; name: string } | null>(null);
  const [balance, setBalance] = useState<PosBalance | null>(null);
  const [heldId, setHeldId] = useState("");
  const [note, setNote] = useState("");
  const [editKey, setEditKey] = useState("");
  const [dlg, setDlg] = useState<"" | "custom" | "discount" | "customer" | "held" | "pay" | "shift">("");
  const [receipt, setReceipt] = useState<PosSale | null>(null);
  const [heldCount, setHeldCount] = useState(0);
  const [mods, setMods] = useState<{ groups: ModGroup[]; products: Record<string, string[]> }>(() => cacheGet("mods") ?? { groups: [], products: {} });
  const [pickMods, setPickMods] = useState<InvProduct | null>(null);
  const [favs, setFavs] = useState<Favourite[]>(() => cacheGet("favs") ?? []);
  const [ext, setExt] = useState<ExtSettings | null>(() => cacheGet("ext"));
  const [serverEmail, setServerEmail] = useState(false);
  const [pinAsk, setPinAsk] = useState<null | { tenders: Tender[] }>(null);
  const [emailOpen, setEmailOpen] = useState(false);
  const [offlineReceipt, setOfflineReceipt] = useState(false);
  const [hwMsg, setHwMsg] = useState("");
  const [loyalty, setLoyalty] = useState<Loyalty | null>(null);
  const [autoDrawer, setAutoDrawer] = useState(() => localStorage.getItem("adrient-pos-auto-drawer") === "1");
  const { err, setErr, run } = useErr();
  const searchRef = useRef<HTMLInputElement>(null);

  const loadProducts = useCallback(async () => {
    let list: InvProduct[];
    try {
      list = await listProducts({ flat: true });
      cachePut("products", list);
    } catch {
      list = cacheGet<InvProduct[]>("products") ?? []; // offline: sell from the last catalogue this device saw
    }
    // A family row with variants is not sold itself; its variants are.
    setProducts(list.filter((p) => !p.archived && !(p.variants && p.variants.length > 0)));
  }, []);
  useEffect(() => {
    loadProducts();
  }, [loadProducts]);

  const ob = useOutbox(() => {
    loadProducts();
    onChanged();
  });

  useEffect(() => {
    getModifiers().then((m) => { setMods(m); cachePut("mods", m); }).catch(() => undefined);
    getFavourites(registerId).then((f) => { setFavs(f); cachePut("favs", f); }).catch(() => undefined);
    getExt().then((e) => { setExt(e); cachePut("ext", e); }).catch(() => undefined);
    getExtCaps().then((c) => setServerEmail(c.email)).catch(() => undefined);
  }, [registerId]);

  const loadHeld = useCallback(async () => {
    if (!registerId) return setHeldCount(0);
    const l = await listSales({ status: "held", register_id: registerId }).catch(() => []);
    setHeldCount(l.length);
  }, [registerId]);
  useEffect(() => {
    loadHeld();
  }, [loadHeld]);

  useEffect(() => {
    if (!customer) {
      setLoyalty(null);
      return setBalance(null);
    }
    getBalance(customer.id).then(setBalance).catch(() => setBalance(null));
    getLoyalty(customer.id).then(setLoyalty).catch(() => setLoyalty(null));
  }, [customer]);

  const categories = useMemo(() => [...new Set(products.map((p) => p.category).filter(Boolean))].sort(), [products]);
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    const favIds = new Set(favs.map((f) => f.product_id));
    return products.filter(
      (p) =>
        (category === "\u2605" ? favIds.has(p.id) : !category || p.category === category) &&
        (!q || p.name.toLowerCase().includes(q) || p.sku.toLowerCase().includes(q) || p.barcode === query.trim()),
    );
  }, [products, query, category, favs]);
  const favOrder = useMemo(() => new Map(favs.map((f, i) => [f.product_id, i])), [favs]);
  const shownSorted = useMemo(() => (category === "\u2605" ? [...shown].sort((a, b) => (favOrder.get(a.id) ?? 0) - (favOrder.get(b.id) ?? 0)) : shown), [shown, category, favOrder]);

  const priced = useMemo(() => priceCart(cart, taxes, orderKind, orderValue), [cart, taxes, orderKind, orderValue]);

  useEffect(() => {
    if (!cart.length) {
      if (!receipt) publishDisplay(idleDisplay(settings.store_name, settings.currency));
      return;
    }
    publishDisplay({
      kind: "cart",
      store: settings.store_name,
      currency: settings.currency,
      lines: cart.map((l, i) => ({ name: l.name, qty: l.qty, total: priced.lines[i].total })),
      subtotal: priced.subtotal,
      discount: priced.discount,
      tax: priced.tax,
      total: priced.total,
      change: 0,
      at: Date.now(),
    });
  }, [cart, priced, settings.store_name, settings.currency, receipt]);

  const addProduct = (p: InvProduct) => {
    if ((mods.products[p.id] ?? []).length > 0) {
      setPickMods(p);
      return;
    }
    addPlain(p);
  };
  const addPlain = (p: InvProduct, name?: string, price?: number) =>
    setCart((c) =>
      addToCart(c, {
        product_id: p.id,
        name: name ?? (p.variant_label ? `${p.name} (${p.variant_label})` : p.name),
        sku: p.sku,
        unit_price: price ?? p.price,
        tax_rate_id: defaultTax,
      }),
    );

  const onSearchKey = (e: React.KeyboardEvent) => {
    if (e.key !== "Enter") return;
    const q = query.trim();
    if (!q) return;
    // A scanner types the code and presses Enter: exact barcode / SKU rings it up.
    const hit = products.find((p) => p.barcode === q || p.sku.toLowerCase() === q.toLowerCase());
    if (hit) {
      addProduct(hit);
      setQuery("");
    } else if (shown.length === 1) {
      addProduct(shown[0]);
      setQuery("");
    }
  };

  const reset = () => {
    setCart([]);
    setOrderKind("");
    setOrderValue(0);
    setCustomer(null);
    setHeldId("");
    setNote("");
    setErr("");
  };

  const saleInput = (payments?: Tender[]) => ({
    id: heldId || undefined,
    register_id: registerId,
    customer_id: customer?.id,
    customer_name: customer?.name,
    lines: cart.map((l) => ({
      product_id: l.product_id || undefined,
      name: l.name,
      sku: l.sku,
      qty: l.qty,
      unit_price: l.unit_price,
      discount_kind: l.discount_kind,
      discount_value: l.discount_value,
      tax_rate_id: l.tax_rate_id || undefined,
      note: l.note,
    })),
    discount_kind: orderKind,
    discount_value: orderValue,
    note,
    payments: payments ? toPaymentInputs(payments) : undefined,
  });

  const hold = async () => {
    if (!cart.length) return;
    const label = window.prompt("Name for this held sale (optional)", customer?.name ?? "") ?? null;
    if (label === null) return;
    const s = await run(() => holdSale({ ...saleInput(), hold_label: label }));
    if (s) {
      reset();
      loadHeld();
    }
  };

  const resume = (s: PosSale) => {
    setCart(cartFromSale(s));
    setOrderKind(s.discount_kind);
    setOrderValue(s.discount_value);
    setCustomer(s.customer_id ? { id: s.customer_id, name: s.customer_name } : null);
    setHeldId(s.id);
    setNote(s.note);
    setDlg("");
  };

  /** Discount share of the cart, in percent of the undiscounted value. */
  const discountPct = (() => {
    const gross = cart.reduce((a, l) => a + l.qty * l.unit_price, 0);
    return gross > 0 ? (priced.discount / gross) * 100 : 0;
  })();

  const finishSale = (s: PosSale, offline: boolean) => {
    setDlg("");
    setPinAsk(null);
    setOfflineReceipt(offline);
    setReceipt(s);
    publishDisplay({ kind: "thanks", store: settings.store_name, currency: settings.currency, lines: [], subtotal: 0, discount: 0, tax: 0, total: s.total, change: s.change_due, at: Date.now() });
    if (autoDrawer && printerConnected() && s.payments.some((p) => p.method === "cash")) kickDrawer().catch(() => undefined);
    reset();
    loadProducts();
    loadHeld();
    onChanged();
  };

  /** Completes the sale: held sales go the classic way; new sales are idempotent and fall back to the offline outbox. */
  const pay = async (tenders: Tender[], pin?: string): Promise<boolean> => {
    if (heldId) {
      const s = await run(() => completeSale(saleInput(tenders)));
      if (!s) return false;
      finishSale(s, false);
      return true;
    }
    if (ext && ext.discount_pin_over_pct > 0 && discountPct > ext.discount_pin_over_pct + 1e-9 && !pin) {
      setPinAsk({ tenders });
      return false;
    }
    const input = saleInput(tenders);
    const summary = { total: priced.total, lines: cart.length, customer: customer?.name ?? "", register: reg?.name ?? "" };
    const r = await completeOrQueue(ob, input as never, summary, pin);
    if (r.kind === "sale") {
      finishSale(r.sale, false);
      return true;
    }
    if (r.kind === "queued") {
      finishSale(offlineSale(r.entry, cart, priced, tenders, customer?.name ?? "", reg?.name ?? ""), true);
      return true;
    }
    setErr(r.message);
    return false;
  };

  const editing = cart.find((l) => l.key === editKey);
  const noShift = !!reg && !reg.open_shift_id;

  if (!registers.length) {
    return <Empty>Add a register in Settings to start selling.</Empty>;
  }

  return (
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1fr) 400px" }, gap: 2, alignItems: "start" }}>
      {/* Catalog */}
      <Box sx={{ minWidth: 0 }}>
        <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
          <Select
            size="sm"
            value={registerId}
            onChange={(_, v) => v && setRegisterId(v)}
            sx={{ minWidth: 150 }}
            slotProps={{ button: { "data-testid": "pos-register-select" } }}
          >
            {registers.map((r) => (
              <Option key={r.id} value={r.id}>
                {r.name}
              </Option>
            ))}
          </Select>
          {noShift ? (
            <Button size="sm" color="warning" variant="soft" onClick={() => setDlg("shift")} data-testid="pos-open-shift">
              Open shift to sell
            </Button>
          ) : (
            <Chip color="success" variant="soft" data-testid="pos-shift-open">
              Shift open
            </Chip>
          )}
          <Input
            size="sm"
            placeholder="Search or scan a barcode, press Enter"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onSearchKey}
            slotProps={{ input: { ref: searchRef, "data-testid": "pos-search" } }}
            sx={{ flex: 1, minWidth: 200 }}
          />
          <Button size="sm" variant="outlined" onClick={() => setDlg("custom")} data-testid="pos-custom-item">
            Custom item
          </Button>
          <Button size="sm" variant="plain" onClick={() => window.open("/pos/display", "adrient-pos-display", "popup,width=900,height=600")} data-testid="pos-open-display">
            Customer display
          </Button>
        </Box>
        <OfflineBar ob={ob} currency={settings.currency} />
        {(categories.length > 0 || favs.length > 0) && (
          <Box sx={{ display: "flex", gap: 0.75, mb: 1.5, flexWrap: "wrap" }}>
            <Chip variant={category === "" ? "solid" : "outlined"} onClick={() => setCategory("")} sx={category === "" ? { bgcolor: ACCENT, color: "#fff" } : {}}>
              All
            </Chip>
            {favs.length > 0 && (
              <Chip variant={category === "\u2605" ? "solid" : "outlined"} onClick={() => setCategory("\u2605")} sx={category === "\u2605" ? { bgcolor: ACCENT, color: "#fff" } : {}} data-testid="pos-fav-chip">
                {"\u2605"} Favourites
              </Chip>
            )}
            {categories.map((c) => (
              <Chip
                key={c}
                variant={category === c ? "solid" : "outlined"}
                onClick={() => setCategory(c)}
                sx={category === c ? { bgcolor: ACCENT, color: "#fff" } : {}}
              >
                {c}
              </Chip>
            ))}
          </Box>
        )}
        {shownSorted.length === 0 ? (
          <Empty>{products.length ? "No product matches." : "No products yet. Add products in Inventory, or ring up a custom item."}</Empty>
        ) : (
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 1 }}>
            {shownSorted.map((p) => (
              <Sheet
                key={p.id}
                variant="outlined"
                role="button"
                tabIndex={0}
                aria-label={`Add ${p.name}`}
                data-testid="pos-product"
                onClick={() => addProduct(p)}
                onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), addProduct(p))}
                sx={{
                  p: 1,
                  borderRadius: "md",
                  cursor: "pointer",
                  userSelect: "none",
                  minHeight: 96,
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "space-between",
                  "&:hover, &:focus-visible": { borderColor: ACCENT, boxShadow: "sm" },
                }}
              >
                {p.photo_file_id && (
                  <Box component="img" src={photoURL(p.photo_file_id)} alt="" sx={{ width: "100%", height: 64, objectFit: "cover", borderRadius: "sm", mb: 0.5 }} />
                )}
                <Typography level="title-sm" sx={{ lineHeight: 1.2 }}>
                  {p.variant_label ? `${p.name} (${p.variant_label})` : p.name}
                </Typography>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mt: 0.5 }}>
                  <Typography level="title-md" sx={{ color: ACCENT }}>
                    {m(p.price)}
                  </Typography>
                  <Typography level="body-xs" color={p.total_stock <= 0 ? "danger" : "neutral"}>
                    {p.total_stock <= 0 ? "Out" : qtyText(p.total_stock)}
                  </Typography>
                </Box>
              </Sheet>
            ))}
          </Box>
        )}
      </Box>

      {/* Cart */}
      <Sheet variant="outlined" sx={{ borderRadius: "md", p: 1.5, position: { lg: "sticky" }, top: { lg: 72 } }} data-testid="pos-cart">
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
          <Typography level="title-md">{heldId ? "Held sale" : "Current sale"}</Typography>
          <Box sx={{ display: "flex", gap: 0.5 }}>
            <Button size="sm" variant="plain" onClick={() => setDlg("held")} data-testid="pos-held-btn">
              Held{heldCount ? ` (${heldCount})` : ""}
            </Button>
            <Button size="sm" variant="plain" color="danger" disabled={!cart.length} onClick={reset} data-testid="pos-clear">
              Clear
            </Button>
          </Box>
        </Box>
        <Button size="sm" variant="soft" fullWidth onClick={() => setDlg("customer")} sx={{ mb: 1, justifyContent: "flex-start" }} data-testid="pos-customer-btn">
          {customer ? `Customer: ${customer.name}` : "Add customer (optional)"}
        </Button>
        {cart.length === 0 ? (
          <Typography level="body-sm" sx={{ py: 3, textAlign: "center", color: "text.tertiary" }}>
            Tap a product to add it.
          </Typography>
        ) : (
          <Box sx={{ maxHeight: { lg: "42vh" }, overflow: "auto", mb: 1 }}>
            {cart.map((l, i) => (
              <Box key={l.key} data-testid="pos-line" sx={{ display: "flex", gap: 0.5, alignItems: "center", py: 0.75, borderBottom: "1px solid", borderColor: "divider" }}>
                <Box sx={{ flex: 1, minWidth: 0, cursor: "pointer" }} onClick={() => setEditKey(l.key)}>
                  <Typography level="title-sm" noWrap>
                    {l.name}
                  </Typography>
                  <Typography level="body-xs">
                    {m(l.unit_price)}
                    {priced.lines[i].discount > 0 && ` · -${m(priced.lines[i].discount)}`}
                    {l.note && ` · ${l.note}`}
                  </Typography>
                </Box>
                <IconButton size="sm" variant="plain" aria-label={`Decrease ${l.name}`} onClick={() => setCart(cart.map((x) => (x.key === l.key ? { ...x, qty: Math.max(0, round2(x.qty - 1)) } : x)).filter((x) => x.qty > 0))}>
                  <RemoveIcon fontSize="small" />
                </IconButton>
                <Typography level="body-sm" sx={{ minWidth: 24, textAlign: "center" }} data-testid="pos-line-qty">
                  {qtyText(l.qty)}
                </Typography>
                <IconButton size="sm" variant="plain" aria-label={`Increase ${l.name}`} onClick={() => setCart(cart.map((x) => (x.key === l.key ? { ...x, qty: round2(x.qty + 1) } : x)))}>
                  <AddIcon fontSize="small" />
                </IconButton>
                <Typography level="title-sm" sx={{ minWidth: 64, textAlign: "right" }}>
                  {m(priced.lines[i].total)}
                </Typography>
              </Box>
            ))}
          </Box>
        )}
        <Box sx={{ display: "flex", justifyContent: "space-between", fontSize: "sm" }}>
          <span>Subtotal</span>
          <span data-testid="pos-subtotal">{m(priced.subtotal)}</span>
        </Box>
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: "sm" }}>
          <Button size="sm" variant="plain" onClick={() => setDlg("discount")} sx={{ px: 0 }} data-testid="pos-discount-btn">
            {orderKind ? "Order discount" : "Add discount"}
          </Button>
          <span>{priced.discount > 0 ? `-${m(priced.discount)}` : ""}</span>
        </Box>
        {priced.tax > 0 && (
          <Box sx={{ display: "flex", justifyContent: "space-between", fontSize: "sm" }}>
            <span>Tax</span>
            <span data-testid="pos-tax">{m(priced.tax)}</span>
          </Box>
        )}
        <Box sx={{ display: "flex", justifyContent: "space-between", mt: 0.5 }}>
          <Typography level="title-lg">Total</Typography>
          <Typography level="title-lg" data-testid="pos-total">
            {m(priced.total)}
          </Typography>
        </Box>
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
          <Button variant="outlined" disabled={!cart.length || !registerId} onClick={hold} data-testid="pos-hold">
            Hold
          </Button>
          <Button
            fullWidth
            size="lg"
            disabled={!cart.length || !registerId}
            onClick={() => (noShift ? setDlg("shift") : setDlg("pay"))}
            sx={{ bgcolor: ACCENT, "&:hover": { bgcolor: "#6A33A0" } }}
            data-testid="pos-charge"
          >
            Charge {m(priced.total)}
          </Button>
        </Box>
      </Sheet>

      {editing && (
        <LineDialog
          line={editing}
          taxes={taxes}
          currency={settings.currency}
          onClose={() => setEditKey("")}
          onSave={(l) => {
            setCart(cart.map((x) => (x.key === l.key ? l : x)));
            setEditKey("");
          }}
          onRemove={() => {
            setCart(cart.filter((x) => x.key !== editing.key));
            setEditKey("");
          }}
        />
      )}
      {dlg === "custom" && (
        <CustomItemDialog
          taxes={taxes}
          defaultTax={defaultTax}
          onClose={() => setDlg("")}
          onAdd={(name, price, taxId) => {
            setCart((c) => [...c, { key: `c${Date.now().toString(36)}`, product_id: "", name, sku: "", qty: 1, unit_price: price, discount_kind: "", discount_value: 0, tax_rate_id: taxId, note: "" }]);
            setDlg("");
          }}
        />
      )}
      {dlg === "discount" && (
        <DiscountDialog
          presets={presets}
          kind={orderKind}
          value={orderValue}
          onClose={() => setDlg("")}
          onApply={(k, v) => {
            setOrderKind(k);
            setOrderValue(v);
            setDlg("");
          }}
        />
      )}
      {dlg === "customer" && (
        <CustomerDialog
          current={customer}
          balance={balance}
          settings={settings}
          onClose={() => setDlg("")}
          onPick={(c) => {
            setCustomer(c);
            setDlg("");
          }}
        />
      )}
      {dlg === "held" && (
        <HeldDialog
          registerId={registerId}
          currency={settings.currency}
          onClose={() => {
            setDlg("");
            loadHeld();
          }}
          onResume={resume}
        />
      )}
      {dlg === "shift" && (
        <OpenShiftDialog
          register={reg}
          onClose={() => setDlg("")}
          onOpened={() => {
            setDlg("");
            onChanged();
          }}
        />
      )}
      {dlg === "pay" && (
        <PayDialog
          total={priced.total}
          settings={settings}
          balance={balance}
          hasCustomer={!!customer}
          note={note}
          setNote={setNote}
          err={err}
          onClose={() => setDlg("")}
          onPay={(tenders) => pay(tenders)}
        />
      )}
      {pickMods && (
        <ModifierDialog
          productName={pickMods.variant_label ? `${pickMods.name} (${pickMods.variant_label})` : pickMods.name}
          basePrice={pickMods.price}
          groups={mods.groups.filter((g) => (mods.products[pickMods.id] ?? []).includes(g.id))}
          currency={settings.currency}
          onClose={() => setPickMods(null)}
          onAdd={(name, price) => {
            addPlain(pickMods, name, price);
            setPickMods(null);
          }}
        />
      )}
      {pinAsk && (
        <PinDialog
          title={`Approve ${discountPct.toFixed(1)}% discount`}
          scope="discount"
          onClose={() => setPinAsk(null)}
          onApproved={async (pin) => {
            const t = pinAsk.tenders;
            setPinAsk(null);
            await pay(t, pin);
          }}
        />
      )}
      {emailOpen && receipt && (
        <EmailReceiptDialog saleId={receipt.id} defaultTo="" serverEmail={serverEmail && !offlineReceipt} fallbackHref={emailReceiptHref(receipt, settings)} onClose={() => setEmailOpen(false)} />
      )}
      {receipt && (
        <Modal open onClose={() => setReceipt(null)}>
          <ModalDialog sx={{ width: 420, maxWidth: "96vw", maxHeight: "94vh", overflow: "auto" }} data-testid="pos-receipt-dialog">
            <ModalClose />
            <Typography level="title-lg">Sale complete</Typography>
            {receipt.change_due > 0 && (
              <Typography level="h3" color="success" data-testid="pos-change">
                Change {m(receipt.change_due)}
              </Typography>
            )}
            {offlineReceipt && (
              <Typography level="body-sm" color="warning" data-testid="pos-offline-receipt">
                Saved on this device. It syncs automatically and gets its receipt number then.
              </Typography>
            )}
            {loyalty && loyalty.tier && receipt.customer_name && (
              <Typography level="body-xs" data-testid="pos-tier-note">
                {receipt.customer_name} is {loyalty.tier}
                {loyalty.stamp_goal > 0 ? ` · ${loyalty.stamps}/${loyalty.stamp_goal} stamps` : ""}
              </Typography>
            )}
            <Receipt sale={receipt} settings={settings} />
            {hwMsg && (
              <Typography level="body-xs" color="warning" data-testid="pos-hw-msg">
                {hwMsg}
              </Typography>
            )}
            <DialogActions sx={{ flexWrap: "wrap" }}>
              <Button variant="outlined" onClick={() => window.print()} data-testid="pos-print">
                Print
              </Button>
              <Button variant="outlined" onClick={() => setEmailOpen(true)} disabled={offlineReceipt} data-testid="pos-email">
                Email
              </Button>
              <Button
                variant="outlined"
                data-testid="pos-hw"
                onClick={async () => {
                  setHwMsg("");
                  const u = usbSupport();
                  if (!u.webusb) return setHwMsg(u.reason);
                  try {
                    if (!printerConnected()) await connectPrinter();
                    await sendToPrinter(receiptBytes(receiptText(receipt, settings), { kickDrawer: autoDrawer && receipt.payments.some((p) => p.method === "cash") }));
                  } catch (e) {
                    setHwMsg((e as Error).message);
                  }
                }}
              >
                USB printer
              </Button>
              <Button
                variant="outlined"
                data-testid="pos-drawer"
                onClick={async () => {
                  setHwMsg("");
                  const u = usbSupport();
                  if (!u.webusb) return setHwMsg(`${u.reason} If your printer has an "open drawer on print" setting, printing a receipt will also open the drawer.`);
                  try {
                    if (!printerConnected()) await connectPrinter();
                    await kickDrawer();
                  } catch (e) {
                    setHwMsg((e as Error).message);
                  }
                }}
              >
                Open drawer
              </Button>
              <Button
                variant="plain"
                onClick={() => {
                  const next = !autoDrawer;
                  setAutoDrawer(next);
                  localStorage.setItem("adrient-pos-auto-drawer", next ? "1" : "0");
                }}
              >
                Auto-open drawer on cash: {autoDrawer ? "on" : "off"}
              </Button>
              <Button onClick={() => setReceipt(null)} data-testid="pos-new-sale">
                New sale
              </Button>
            </DialogActions>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---------------------------------------------------------------- dialogs

function Num({ value, onChange, ...rest }: { value: number; onChange: (n: number) => void; min?: number; step?: number; testid?: string }) {
  const [text, setText] = useState(String(value));
  useEffect(() => {
    if (Number(text) !== value) setText(String(value));
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Input
      type="number"
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        const n = parseFloat(e.target.value);
        onChange(Number.isFinite(n) ? n : 0);
      }}
      slotProps={{ input: { min: rest.min ?? 0, step: rest.step ?? "any", "data-testid": rest.testid } }}
    />
  );
}

function LineDialog({
  line,
  taxes,
  currency,
  onClose,
  onSave,
  onRemove,
}: {
  line: CartLine;
  taxes: PosTaxRate[];
  currency: string;
  onClose: () => void;
  onSave: (l: CartLine) => void;
  onRemove: () => void;
}) {
  const [l, setL] = useState(line);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">{l.name}</Typography>
        <Box sx={{ display: "flex", gap: 1 }}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Quantity</FormLabel>
            <Num value={l.qty} onChange={(n) => setL({ ...l, qty: n })} step={1} testid="pos-line-qty-input" />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Unit price {currency && `(${currency})`}</FormLabel>
            <Num value={l.unit_price} onChange={(n) => setL({ ...l, unit_price: n })} testid="pos-line-price-input" />
          </FormControl>
        </Box>
        <Box sx={{ display: "flex", gap: 1 }}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Discount</FormLabel>
            <Select value={l.discount_kind || "none"} onChange={(_, v) => setL({ ...l, discount_kind: v === "none" ? "" : (v as DiscountKind), discount_value: v === "none" ? 0 : l.discount_value })}>
              <Option value="none">None</Option>
              <Option value="percent">Percent</Option>
              <Option value="amount">Amount</Option>
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Value</FormLabel>
            <Num value={l.discount_value} onChange={(n) => setL({ ...l, discount_value: n })} testid="pos-line-discount-input" />
          </FormControl>
        </Box>
        <FormControl>
          <FormLabel>Tax</FormLabel>
          <Select value={l.tax_rate_id || "none"} onChange={(_, v) => setL({ ...l, tax_rate_id: v === "none" ? "" : (v as string) })}>
            <Option value="none">No tax</Option>
            {taxes.map((t) => (
              <Option key={t.id} value={t.id}>
                {t.name} ({t.rate}%{t.inclusive ? ", incl." : ""})
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Note</FormLabel>
          <Input value={l.note} onChange={(e) => setL({ ...l, note: e.target.value })} />
        </FormControl>
        <DialogActions sx={{ justifyContent: "space-between" }}>
          <Button variant="outlined" color="danger" startDecorator={<DeleteOutline />} onClick={onRemove}>
            Remove
          </Button>
          <Button disabled={l.qty <= 0} onClick={() => onSave(l)} data-testid="pos-line-save">
            Done
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function CustomItemDialog({
  taxes,
  defaultTax,
  onClose,
  onAdd,
}: {
  taxes: PosTaxRate[];
  defaultTax: string;
  onClose: () => void;
  onAdd: (name: string, price: number, taxId: string) => void;
}) {
  const [name, setName] = useState("");
  const [price, setPrice] = useState(0);
  const [tax, setTax] = useState(defaultTax);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Custom item</Typography>
        <FormControl required>
          <FormLabel>Name</FormLabel>
          <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "pos-custom-name" } }} />
        </FormControl>
        <FormControl required>
          <FormLabel>Price</FormLabel>
          <Num value={price} onChange={setPrice} testid="pos-custom-price" />
        </FormControl>
        <FormControl>
          <FormLabel>Tax</FormLabel>
          <Select value={tax || "none"} onChange={(_, v) => setTax(v === "none" ? "" : (v as string))}>
            <Option value="none">No tax</Option>
            {taxes.map((t) => (
              <Option key={t.id} value={t.id}>
                {t.name} ({t.rate}%)
              </Option>
            ))}
          </Select>
        </FormControl>
        <DialogActions>
          <Button disabled={!name.trim() || price < 0} onClick={() => onAdd(name.trim(), round2(price), tax)} data-testid="pos-custom-add">
            Add to sale
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function DiscountDialog({
  presets,
  kind,
  value,
  onClose,
  onApply,
}: {
  presets: PosDiscountPreset[];
  kind: DiscountKind;
  value: number;
  onClose: () => void;
  onApply: (k: DiscountKind, v: number) => void;
}) {
  const [k, setK] = useState<DiscountKind>(kind || "percent");
  const [v, setV] = useState(value);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Order discount</Typography>
        {presets.length > 0 && (
          <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
            {presets.map((p) => (
              <Chip key={p.id} variant="outlined" onClick={() => onApply(p.kind, p.value)} data-testid="pos-preset">
                {p.name} ({p.kind === "percent" ? `${p.value}%` : p.value})
              </Chip>
            ))}
          </Box>
        )}
        <Box sx={{ display: "flex", gap: 1 }}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Type</FormLabel>
            <Select value={k || "percent"} onChange={(_, x) => setK(x as DiscountKind)}>
              <Option value="percent">Percent</Option>
              <Option value="amount">Amount</Option>
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Value</FormLabel>
            <Num value={v} onChange={setV} testid="pos-discount-value" />
          </FormControl>
        </Box>
        <DialogActions sx={{ justifyContent: "space-between" }}>
          <Button variant="outlined" onClick={() => onApply("", 0)}>
            Remove
          </Button>
          <Button disabled={v <= 0} onClick={() => onApply(k || "percent", v)} data-testid="pos-discount-apply">
            Apply
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function CustomerDialog({
  current,
  balance,
  settings,
  onClose,
  onPick,
}: {
  current: { id: string; name: string } | null;
  balance: PosBalance | null;
  settings: PosSettings;
  onClose: () => void;
  onPick: (c: { id: string; name: string } | null) => void;
}) {
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [q, setQ] = useState("");
  useEffect(() => {
    searchContacts().then(setContacts).catch(() => setContacts([]));
  }, []);
  const label = (c: ContactLite) => c.display_name || c.company || c.emails?.[0] || "Unnamed";
  const list = contacts.filter((c) => !q || label(c).toLowerCase().includes(q.toLowerCase())).slice(0, 50);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">Customer</Typography>
        {current && balance && (
          <Typography level="body-sm">
            {current.name}: {balance.loyalty_points} points, store credit {money(balance.store_credit, settings.currency)}
          </Typography>
        )}
        <Input placeholder="Search contacts" value={q} onChange={(e) => setQ(e.target.value)} />
        <Box sx={{ maxHeight: 280, overflow: "auto" }}>
          {list.length === 0 && <Typography level="body-sm">No contacts found. Add customers in Contacts.</Typography>}
          {list.map((c) => (
            <Box
              key={c.id}
              role="button"
              tabIndex={0}
              data-testid="pos-contact"
              onClick={() => onPick({ id: c.id, name: label(c) })}
              onKeyDown={(e) => e.key === "Enter" && onPick({ id: c.id, name: label(c) })}
              sx={{ py: 0.75, px: 1, borderRadius: "sm", cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}
            >
              {label(c)}
              <Typography level="body-xs">{c.emails?.[0] ?? c.phones?.[0] ?? ""}</Typography>
            </Box>
          ))}
        </Box>
        {current && (
          <Button variant="outlined" color="neutral" onClick={() => onPick(null)}>
            Remove customer
          </Button>
        )}
      </ModalDialog>
    </Modal>
  );
}

function HeldDialog({
  registerId,
  currency,
  onClose,
  onResume,
}: {
  registerId: string;
  currency: string;
  onClose: () => void;
  onResume: (s: PosSale) => void;
}) {
  const [list, setList] = useState<PosSale[] | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => {
    listSales({ status: "held", register_id: registerId }).then(setList).catch(() => setList([]));
  }, [registerId]);
  useEffect(load, [load]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">Held sales</Typography>
        <ErrorAlert err={err} />
        {list && list.length === 0 && <Typography level="body-sm">Nothing on hold.</Typography>}
        {list?.map((s) => (
          <Sheet key={s.id} variant="outlined" sx={{ p: 1, borderRadius: "sm", display: "flex", alignItems: "center", gap: 1 }} data-testid="pos-held-row">
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm">{s.hold_label || s.customer_name || "Held sale"}</Typography>
              <Typography level="body-xs">
                {new Date(s.created_at).toLocaleString()} · {money(s.total, currency)}
              </Typography>
            </Box>
            <Button size="sm" onClick={async () => { const full = await run(() => getSale(s.id)); if (full) onResume(full); }} data-testid="pos-resume">
              Resume
            </Button>
            <IconButton
              size="sm"
              color="danger"
              variant="plain"
              aria-label="Discard held sale"
              onClick={async () => {
                if (!window.confirm("Discard this held sale?")) return;
                if ((await run(() => deleteHeldSale(s.id))) !== undefined) load();
              }}
            >
              <DeleteOutline fontSize="small" />
            </IconButton>
          </Sheet>
        ))}
      </ModalDialog>
    </Modal>
  );
}

function OpenShiftDialog({ register, onClose, onOpened }: { register?: PosRegister; onClose: () => void; onOpened: () => void }) {
  const [cash, setCash] = useState(0);
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 380, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Open shift on {register?.name}</Typography>
        <Typography level="body-sm">Count the cash in the drawer and enter it as the opening float.</Typography>
        <FormControl>
          <FormLabel>Opening cash</FormLabel>
          <Num value={cash} onChange={setCash} testid="pos-opening-cash" />
        </FormControl>
        <ErrorAlert err={err} />
        <DialogActions>
          <Button
            onClick={async () => {
              if (register && (await run(() => openShift(register.id, round2(cash))))) onOpened();
            }}
            data-testid="pos-open-shift-confirm"
          >
            Open shift
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function PayDialog({
  total,
  settings,
  balance,
  hasCustomer,
  note,
  setNote,
  err,
  onClose,
  onPay,
}: {
  total: number;
  settings: PosSettings;
  balance: PosBalance | null;
  hasCustomer: boolean;
  note: string;
  setNote: (s: string) => void;
  err: string;
  onClose: () => void;
  onPay: (t: Tender[]) => Promise<boolean>;
}) {
  const m = (n: number) => money(n, settings.currency);
  const [tenders, setTenders] = useState<Tender[]>([]);
  const [method, setMethod] = useState<PayMethod>("cash");
  const [giftMode, setGiftMode] = useState(false);
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState(false);
  const sum = summarizeTenders(total, tenders);
  const credit = hasCustomer ? balance?.store_credit ?? 0 : 0;
  const pointsValue = hasCustomer ? round2((balance?.loyalty_points ?? 0) * settings.loyalty_point_value) : 0;
  const methods: PayMethod[] = ["cash", "card", "other", ...(credit > 0 ? (["store_credit"] as PayMethod[]) : []), ...(pointsValue > 0 ? (["loyalty"] as PayMethod[]) : [])];
  const cap = method === "store_credit" ? credit : method === "loyalty" ? pointsValue : Infinity;
  const entered = parseFloat(amount);
  const add = (a: number) => {
    const v = round2(Math.min(a, method === "cash" ? a : Math.min(cap, sum.remaining)));
    if (v <= 0) return;
    setTenders([...tenders, { method, amount: v }]);
    setAmount("");
  };
  const exact = round2(Math.min(sum.remaining, cap));
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "96vw", maxHeight: "94vh", overflow: "auto" }} data-testid="pos-pay-dialog">
        <ModalClose />
        <Typography level="title-lg">Payment</Typography>
        <Box sx={{ display: "flex", justifyContent: "space-between" }}>
          <Typography level="body-md">Total due</Typography>
          <Typography level="h3" data-testid="pos-pay-total">
            {m(total)}
          </Typography>
        </Box>
        {tenders.map((t, i) => (
          <Box key={i} sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }} data-testid="pos-tender">
            <span>
              {t.ref?.startsWith("gift:") ? `Gift card ${t.ref.slice(5)}` : methodLabel(t.method)}
              {t.method === "loyalty" && ` (${pointsFor(t.amount, settings.loyalty_point_value)} pts)`}
            </span>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              {m(t.amount)}
              <IconButton size="sm" variant="plain" aria-label="Remove payment" onClick={() => setTenders(tenders.filter((_, j) => j !== i))}>
                <DeleteOutline fontSize="small" />
              </IconButton>
            </Box>
          </Box>
        ))}
        <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
          {methods.map((x) => (
            <Chip key={x} variant={method === x && !giftMode ? "solid" : "outlined"} onClick={() => { setGiftMode(false); setMethod(x); }} sx={method === x && !giftMode ? { bgcolor: ACCENT, color: "#fff" } : {}} data-testid={`pos-method-${x}`}>
              {methodLabel(x)}
            </Chip>
          ))}
          <Chip variant={giftMode ? "solid" : "outlined"} onClick={() => setGiftMode(true)} sx={giftMode ? { bgcolor: ACCENT, color: "#fff" } : {}} data-testid="pos-method-gift">
            Gift card
          </Chip>
        </Box>
        {giftMode && sum.remaining > 0 && (
          <GiftCardLookup
            currency={settings.currency}
            remaining={sum.remaining}
            onUse={(code, amt) => {
              setTenders([...tenders, { method: "other", amount: round2(amt), ref: `gift:${code}` }]);
              setGiftMode(false);
              setMethod("cash");
            }}
          />
        )}
        {sum.remaining > 0 && !giftMode && (
          <>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input
                type="number"
                placeholder={method === "cash" ? "Cash received" : "Amount"}
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && Number.isFinite(entered) && add(entered)}
                sx={{ flex: 1 }}
                slotProps={{ input: { min: 0, step: "any", "data-testid": "pos-pay-amount" } }}
              />
              <Button variant="outlined" disabled={!Number.isFinite(entered) || entered <= 0} onClick={() => add(entered)} data-testid="pos-pay-add">
                Add
              </Button>
            </Box>
            <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
              {(method === "cash" ? quickTender(sum.remaining) : [exact]).filter((n) => n > 0).map((n) => (
                <Button key={n} size="sm" variant="soft" onClick={() => add(n)} data-testid="pos-quick">
                  {method === "cash" && n === round2(sum.remaining) ? "Exact " : ""}
                  {m(n)}
                </Button>
              ))}
            </Box>
          </>
        )}
        <Box sx={{ display: "flex", justifyContent: "space-between" }}>
          <span>Remaining</span>
          <strong data-testid="pos-remaining">{m(sum.remaining)}</strong>
        </Box>
        {sum.change > 0 && (
          <Box sx={{ display: "flex", justifyContent: "space-between", color: "success.plainColor" }}>
            <span>Change</span>
            <strong data-testid="pos-pay-change">{m(sum.change)}</strong>
          </Box>
        )}
        {hasCustomer && settings.loyalty_earn_per_unit > 0 && (
          <Typography level="body-xs">Earns {Math.floor(total * settings.loyalty_earn_per_unit)} loyalty points.</Typography>
        )}
        <FormControl>
          <FormLabel>Sale note</FormLabel>
          <Textarea minRows={1} value={note} onChange={(e) => setNote(e.target.value)} />
        </FormControl>
        <ErrorAlert err={err} />
        <DialogActions>
          <Button
            fullWidth
            size="lg"
            loading={busy}
            disabled={!sum.ok}
            data-testid="pos-complete"
            sx={{ bgcolor: ACCENT, "&:hover": { bgcolor: "#6A33A0" } }}
            onClick={async () => {
              setBusy(true);
              const ok = await onPay(tenders);
              if (!ok) setBusy(false);
            }}
          >
            Complete sale
          </Button>
        </DialogActions>
        <Typography level="body-xs" sx={rightCell}>
          Card and other payments are recorded manually.
        </Typography>
      </ModalDialog>
    </Modal>
  );
}
