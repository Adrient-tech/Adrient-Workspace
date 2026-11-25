import { useEffect, useState } from "react";
import { Autocomplete, Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { listProducts } from "../inventory/api";
import type { InvProduct } from "../inventory/types";
import {
  assignModGroups,
  assignStore,
  deleteModGroup,
  getExt,
  getFavourites,
  getModifiers,
  getPin,
  getStores,
  issueGiftCard,
  listGiftCards,
  removePin,
  saveExt,
  saveFavourites,
  saveModGroup,
  saveStore,
  setGiftCardStatus,
  setPin,
  topUpGiftCard,
} from "./extApi";
import type { ExtSettings, Favourite, GiftCard, ModGroup, PinInfo, Store } from "./extApi";
import { money } from "./logic";
import type { PosRegister, PosSettings } from "./types";
import { ErrorAlert, useErr } from "./ui";

function Box2({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
      <Typography level="title-md">{title}</Typography>
      {hint && (
        <Typography level="body-xs" sx={{ mb: 1 }}>
          {hint}
        </Typography>
      )}
      {children}
    </Sheet>
  );
}

/** Wave-3 POS settings: manager PIN and approvals, loyalty tiers and stamps, options, favourites, gift cards, stores. */
export function AdvancedSettings({ registers, settings, onChanged }: { registers: PosRegister[]; settings: PosSettings; onChanged: () => void }) {
  const { err, run, setErr } = useErr();
  const [note, setNote] = useState("");
  const m = (n: number) => money(n, settings.currency);

  // ---- PIN and approvals ----
  const [pin, setPinState] = useState<PinInfo | null>(null);
  const [pinText, setPinText] = useState("");
  const [flags, setFlags] = useState({ can_void: true, can_refund: true, can_discount: true });
  const [ext, setExt] = useState<ExtSettings | null>(null);
  useEffect(() => {
    getPin().then((p) => {
      setPinState(p);
      if (p.set) setFlags({ can_void: p.can_void, can_refund: p.can_refund, can_discount: p.can_discount });
    }).catch(() => undefined);
    getExt().then(setExt).catch(() => undefined);
  }, []);

  // ---- products for modifiers and favourites ----
  const [products, setProducts] = useState<InvProduct[]>([]);
  useEffect(() => {
    listProducts({ flat: true }).then((l) => setProducts(l.filter((p) => !p.archived && !(p.variants && p.variants.length > 0)))).catch(() => undefined);
  }, []);
  const pname = (id: string) => products.find((p) => p.id === id)?.name ?? id.slice(0, 8);

  // ---- modifiers ----
  const [mods, setMods] = useState<{ groups: ModGroup[]; products: Record<string, string[]> }>({ groups: [], products: {} });
  const [grp, setGrp] = useState<ModGroup | null>(null);
  const loadMods = () => getModifiers().then(setMods).catch(() => undefined);
  useEffect(() => {
    loadMods();
  }, []);

  // ---- favourites ----
  const [favs, setFavs] = useState<Favourite[]>([]);
  useEffect(() => {
    getFavourites().then(setFavs).catch(() => undefined);
  }, []);
  const move = (i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= favs.length) return;
    const c = [...favs];
    [c[i], c[j]] = [c[j], c[i]];
    setFavs(c);
  };

  // ---- gift cards ----
  const [cards, setCards] = useState<GiftCard[]>([]);
  const [newCard, setNewCard] = useState({ amount: "", name: "", email: "" });
  const loadCards = () => listGiftCards().then(setCards).catch(() => undefined);
  useEffect(() => {
    loadCards();
  }, []);

  // ---- stores ----
  const [stores, setStores] = useState<{ stores: Store[]; registers: Record<string, string> }>({ stores: [], registers: {} });
  const [store, setStore] = useState<Store | null>(null);
  const loadStores = () => getStores().then(setStores).catch(() => undefined);
  useEffect(() => {
    loadStores();
  }, []);

  return (
    <Box data-testid="pos-advanced">
      <ErrorAlert err={err} />
      {note && (
        <Typography level="body-sm" color="success" data-testid="pos-adv-note" sx={{ mb: 1 }}>
          {note}
        </Typography>
      )}

      <Box2 title="Manager PIN and approvals" hint="A PIN lets a manager approve a void, a refund or a big discount at the till. Only a hash is stored. Five wrong tries lock the person typing for five minutes.">
        <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap" }}>
          <FormControl>
            <FormLabel>{pin?.set ? "Change my PIN" : "Set my PIN"} (4 to 8 digits)</FormLabel>
            <Input type="password" value={pinText} onChange={(e) => setPinText(e.target.value.replace(/\D/g, "").slice(0, 8))} slotProps={{ input: { inputMode: "numeric", autoComplete: "new-password", "data-testid": "pos-pin-set" } }} />
          </FormControl>
          <Checkbox label="void" checked={flags.can_void} onChange={(e) => setFlags({ ...flags, can_void: e.target.checked })} />
          <Checkbox label="refund" checked={flags.can_refund} onChange={(e) => setFlags({ ...flags, can_refund: e.target.checked })} />
          <Checkbox label="discount" checked={flags.can_discount} onChange={(e) => setFlags({ ...flags, can_discount: e.target.checked })} />
          <Button
            disabled={pinText.length < 4}
            data-testid="pos-pin-save"
            onClick={async () => {
              const p = await run(() => setPin(pinText, flags));
              if (p) {
                setPinState(p);
                setPinText("");
                setNote("PIN saved.");
              }
            }}
          >
            Save PIN
          </Button>
          {pin?.set && (
            <Button
              variant="plain"
              color="danger"
              onClick={async () => {
                await run(() => removePin());
                setPinState({ ...pin, set: false });
              }}
            >
              Remove PIN
            </Button>
          )}
        </Box>
        {ext && (
          <Box sx={{ mt: 1.5, display: "grid", gap: 1 }}>
            <Checkbox label="Voids need a manager PIN" checked={ext.void_needs_pin} onChange={(e) => setExt({ ...ext, void_needs_pin: e.target.checked })} />
            <Checkbox label="Refunds need a manager PIN" checked={ext.refund_needs_pin} onChange={(e) => setExt({ ...ext, refund_needs_pin: e.target.checked })} />
            <FormControl sx={{ maxWidth: 360 }}>
              <FormLabel>Discounts above this percent need a PIN (0 = never)</FormLabel>
              <Input type="number" value={ext.discount_pin_over_pct} onChange={(e) => setExt({ ...ext, discount_pin_over_pct: Number(e.target.value) })} slotProps={{ input: { "data-testid": "pos-disc-pin" } }} />
            </FormControl>
            <Typography level="body-xs">Approvals are enforced for sales, voids and refunds made through the register. Roles still control who can use the underlying API directly.</Typography>
          </Box>
        )}
      </Box2>

      {ext && (
        <Box2 title="Loyalty tiers and stamp card" hint="Tiers raise the points a customer earns once their lifetime spend reaches a level. A stamp card pays store credit after a number of qualifying sales.">
          {ext.tiers.map((t, i) => (
            <Box key={i} sx={{ display: "flex", gap: 1, mb: 0.75 }} data-testid="pos-tier-row">
              <Input size="sm" placeholder="Name" value={t.name} onChange={(e) => setExt({ ...ext, tiers: ext.tiers.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })} />
              <Input size="sm" type="number" placeholder="From spend" value={t.min_spend} onChange={(e) => setExt({ ...ext, tiers: ext.tiers.map((x, j) => (j === i ? { ...x, min_spend: Number(e.target.value) } : x)) })} />
              <Input size="sm" type="number" placeholder="Points x" value={t.multiplier} slotProps={{ input: { step: "0.25" } }} onChange={(e) => setExt({ ...ext, tiers: ext.tiers.map((x, j) => (j === i ? { ...x, multiplier: Number(e.target.value) } : x)) })} />
              <Button size="sm" variant="plain" color="danger" onClick={() => setExt({ ...ext, tiers: ext.tiers.filter((_, j) => j !== i) })}>
                Remove
              </Button>
            </Box>
          ))}
          <Button size="sm" variant="outlined" onClick={() => setExt({ ...ext, tiers: [...ext.tiers, { name: "", min_spend: 0, multiplier: 1 }] })} data-testid="pos-tier-add">
            Add tier
          </Button>
          <Checkbox sx={{ mt: 1.5, display: "flex" }} label="Stamp card" checked={ext.stamps.enabled} onChange={(e) => setExt({ ...ext, stamps: { ...ext.stamps, enabled: e.target.checked } })} slotProps={{ input: { "data-testid": "pos-stamps-on" } }} />
          {ext.stamps.enabled && (
            <Box sx={{ display: "flex", gap: 1, mt: 0.5, flexWrap: "wrap" }}>
              <FormControl>
                <FormLabel>Sale of at least</FormLabel>
                <Input type="number" value={ext.stamps.min_sale} onChange={(e) => setExt({ ...ext, stamps: { ...ext.stamps, min_sale: Number(e.target.value) } })} />
              </FormControl>
              <FormControl>
                <FormLabel>Stamps for a reward</FormLabel>
                <Input type="number" value={ext.stamps.goal} onChange={(e) => setExt({ ...ext, stamps: { ...ext.stamps, goal: Number(e.target.value) } })} slotProps={{ input: { "data-testid": "pos-stamps-goal" } }} />
              </FormControl>
              <FormControl>
                <FormLabel>Reward (store credit)</FormLabel>
                <Input type="number" value={ext.stamps.reward_credit} onChange={(e) => setExt({ ...ext, stamps: { ...ext.stamps, reward_credit: Number(e.target.value) } })} slotProps={{ input: { "data-testid": "pos-stamps-reward" } }} />
              </FormControl>
            </Box>
          )}
          <Box sx={{ mt: 1.5 }}>
            <Button
              data-testid="pos-ext-save"
              onClick={async () => {
                const e = await run(() => saveExt(ext));
                if (e) {
                  setExt(e);
                  setNote("Approval and loyalty settings saved.");
                }
              }}
            >
              Save approvals and loyalty
            </Button>
          </Box>
        </Box2>
      )}

      <Box2 title="Product options" hint="Groups of choices such as size or milk type. Assign a group to a product and the register asks before adding it.">
        <Table size="sm" data-testid="pos-mod-table">
          <tbody>
            {mods.groups.map((g) => (
              <tr key={g.id}>
                <td>
                  {g.name} {g.required && <Chip size="sm">required</Chip>} {g.multi && <Chip size="sm">pick any</Chip>}
                </td>
                <td>{g.options.map((o) => `${o.name}${o.price ? ` ${o.price > 0 ? "+" : ""}${m(o.price)}` : ""}`).join(", ")}</td>
                <td style={{ textAlign: "right" }}>
                  <Button size="sm" variant="plain" onClick={() => setGrp(g)}>
                    Edit
                  </Button>
                  <Button
                    size="sm"
                    variant="plain"
                    color="danger"
                    onClick={async () => {
                      await run(() => deleteModGroup(g.id));
                      loadMods();
                    }}
                  >
                    Delete
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Button size="sm" variant="outlined" sx={{ mt: 1 }} onClick={() => setGrp({ id: "", name: "", required: false, multi: false, options: [{ id: "", name: "", price: 0 }] })} data-testid="pos-mod-new">
          New option group
        </Button>
        {grp && (
          <Sheet variant="soft" sx={{ p: 1.5, mt: 1, borderRadius: "md" }} data-testid="pos-mod-editor">
            <Input placeholder="Group name, e.g. Size" value={grp.name} onChange={(e) => setGrp({ ...grp, name: e.target.value })} slotProps={{ input: { "data-testid": "pos-mod-name" } }} />
            <Box sx={{ display: "flex", gap: 2, my: 1 }}>
              <Checkbox label="Required" checked={grp.required} onChange={(e) => setGrp({ ...grp, required: e.target.checked })} />
              <Checkbox label="Pick several" checked={grp.multi} onChange={(e) => setGrp({ ...grp, multi: e.target.checked })} />
            </Box>
            {grp.options.map((o, i) => (
              <Box key={i} sx={{ display: "flex", gap: 0.5, mb: 0.5 }}>
                <Input size="sm" sx={{ flex: 1 }} placeholder="Option" value={o.name} onChange={(e) => setGrp({ ...grp, options: grp.options.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })} slotProps={{ input: { "data-testid": "pos-mod-opt-name" } }} />
                <Input size="sm" type="number" sx={{ width: 110 }} placeholder="Extra price" value={o.price} onChange={(e) => setGrp({ ...grp, options: grp.options.map((x, j) => (j === i ? { ...x, price: Number(e.target.value) } : x)) })} slotProps={{ input: { "data-testid": "pos-mod-opt-price" } }} />
                <Button size="sm" variant="plain" color="danger" onClick={() => setGrp({ ...grp, options: grp.options.filter((_, j) => j !== i) })}>
                  Remove
                </Button>
              </Box>
            ))}
            <Box sx={{ display: "flex", gap: 1 }}>
              <Button size="sm" variant="outlined" onClick={() => setGrp({ ...grp, options: [...grp.options, { id: "", name: "", price: 0 }] })}>
                + Option
              </Button>
              <Button
                size="sm"
                data-testid="pos-mod-save"
                onClick={async () => {
                  if (await run(() => saveModGroup(grp.id, grp))) {
                    setGrp(null);
                    loadMods();
                  }
                }}
              >
                Save group
              </Button>
              <Button size="sm" variant="plain" onClick={() => setGrp(null)}>
                Cancel
              </Button>
            </Box>
          </Sheet>
        )}
        {mods.groups.length > 0 && (
          <Box sx={{ mt: 1.5 }}>
            <FormLabel>Assign to a product</FormLabel>
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
              <Autocomplete
                size="sm"
                sx={{ minWidth: 240 }}
                placeholder="Product"
                options={products}
                getOptionLabel={(p) => p.name}
                value={null}
                onChange={async (_, p) => {
                  if (!p) return;
                  const cur = mods.products[p.id] ?? [];
                  const pick = mods.groups.filter((g) => !cur.includes(g.id))[0];
                  if (!pick) return setErr("This product already has every group.");
                  await run(() => assignModGroups(p.id, [...cur, pick.id]));
                  setNote(`${pick.name} added to ${p.name}.`);
                  loadMods();
                }}
                slotProps={{ input: { "data-testid": "pos-mod-assign" } }}
              />
              <Typography level="body-xs">Picking a product adds the next unassigned group; remove with the chips below.</Typography>
            </Box>
            <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", mt: 1 }}>
              {Object.entries(mods.products).flatMap(([pid, gids]) =>
                gids.map((gid) => (
                  <Chip
                    key={pid + gid}
                    size="sm"
                    variant="outlined"
                    onClick={async () => {
                      await run(() => assignModGroups(pid, gids.filter((x) => x !== gid)));
                      loadMods();
                    }}
                  >
                    {pname(pid)}: {mods.groups.find((g) => g.id === gid)?.name} ×
                  </Chip>
                )),
              )}
            </Box>
          </Box>
        )}
      </Box2>

      <Box2 title="Favourites grid" hint="The products on the register's Favourites tab, in this order.">
        {favs.map((f, i) => (
          <Box key={f.product_id} sx={{ display: "flex", gap: 0.5, alignItems: "center", py: 0.25 }} data-testid="pos-fav-row">
            <Typography level="body-sm" sx={{ flex: 1 }}>
              {i + 1}. {pname(f.product_id)}
            </Typography>
            <Button size="sm" variant="plain" disabled={i === 0} onClick={() => move(i, -1)} aria-label="Move up">
              ↑
            </Button>
            <Button size="sm" variant="plain" disabled={i === favs.length - 1} onClick={() => move(i, 1)} aria-label="Move down">
              ↓
            </Button>
            <Button size="sm" variant="plain" color="danger" onClick={() => setFavs(favs.filter((_, j) => j !== i))}>
              Remove
            </Button>
          </Box>
        ))}
        <Box sx={{ display: "flex", gap: 1, mt: 1, flexWrap: "wrap" }}>
          <Autocomplete
            size="sm"
            sx={{ minWidth: 240 }}
            placeholder="Add a product"
            options={products.filter((p) => !favs.some((f) => f.product_id === p.id))}
            getOptionLabel={(p) => p.name}
            value={null}
            onChange={(_, p) => p && setFavs([...favs, { product_id: p.id, label: "", color: "" }])}
            slotProps={{ input: { "data-testid": "pos-fav-add" } }}
          />
          <Button
            size="sm"
            data-testid="pos-fav-save"
            onClick={async () => {
              if (await run(() => saveFavourites(favs))) {
                setNote("Favourites saved.");
                onChanged();
              }
            }}
          >
            Save favourites
          </Button>
        </Box>
      </Box2>

      <Box2 title="Gift cards" hint="Sell a card with a value; customers pay with the code at the register (Payment > Gift card).">
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "end", mb: 1 }}>
          <FormControl>
            <FormLabel>Amount</FormLabel>
            <Input type="number" sx={{ width: 120 }} value={newCard.amount} onChange={(e) => setNewCard({ ...newCard, amount: e.target.value })} slotProps={{ input: { "data-testid": "pos-gc-amount" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Holder (optional)</FormLabel>
            <Input value={newCard.name} onChange={(e) => setNewCard({ ...newCard, name: e.target.value })} />
          </FormControl>
          <Button
            disabled={!(parseFloat(newCard.amount) > 0)}
            data-testid="pos-gc-issue"
            onClick={async () => {
              const c = await run(() => issueGiftCard({ amount: parseFloat(newCard.amount), holder_name: newCard.name, holder_email: newCard.email }));
              if (c) {
                setNote(`Gift card ${c.code} issued for ${m(c.initial)}. Hand over or send this code.`);
                setNewCard({ amount: "", name: "", email: "" });
                loadCards();
              }
            }}
          >
            Issue card
          </Button>
        </Box>
        <Table size="sm" data-testid="pos-gc-table">
          <tbody>
            {cards.slice(0, 20).map((c) => (
              <tr key={c.id}>
                <td style={{ fontFamily: "monospace" }}>{c.code}</td>
                <td>{c.holder_name}</td>
                <td>{m(c.balance)} of {m(c.initial)}</td>
                <td>{c.status}</td>
                <td style={{ textAlign: "right" }}>
                  <Button
                    size="sm"
                    variant="plain"
                    onClick={async () => {
                      const v = window.prompt("Add how much to this card?");
                      const a = parseFloat(v ?? "");
                      if (a > 0 && (await run(() => topUpGiftCard(c.code, a)))) loadCards();
                    }}
                  >
                    Top up
                  </Button>
                  <Button
                    size="sm"
                    variant="plain"
                    color={c.status === "active" ? "danger" : "success"}
                    onClick={async () => {
                      await run(() => setGiftCardStatus(c.code, c.status === "active" ? "disabled" : "active"));
                      loadCards();
                    }}
                  >
                    {c.status === "active" ? "Disable" : "Enable"}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box2>

      <Box2 title="Stores" hint="Group registers into shops. A store's name, address and receipt lines print on its receipts and Z reports.">
        <Table size="sm" data-testid="pos-store-table">
          <tbody>
            {stores.stores.map((s) => (
              <tr key={s.id}>
                <td>{s.name}</td>
                <td>{s.address}</td>
                <td style={{ textAlign: "right" }}>
                  <Button size="sm" variant="plain" onClick={() => setStore(s)}>
                    Edit
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Button size="sm" variant="outlined" sx={{ mt: 1 }} onClick={() => setStore({ id: "", name: "", address: "", phone: "", receipt_header: "", receipt_footer: "", active: true })} data-testid="pos-store-new">
          New store
        </Button>
        {store && (
          <Sheet variant="soft" sx={{ p: 1.5, mt: 1, borderRadius: "md", display: "grid", gap: 1 }}>
            <Input placeholder="Store name" value={store.name} onChange={(e) => setStore({ ...store, name: e.target.value })} slotProps={{ input: { "data-testid": "pos-store-name" } }} />
            <Input placeholder="Address" value={store.address} onChange={(e) => setStore({ ...store, address: e.target.value })} />
            <Input placeholder="Receipt footer" value={store.receipt_footer} onChange={(e) => setStore({ ...store, receipt_footer: e.target.value })} />
            <Box sx={{ display: "flex", gap: 1 }}>
              <Button
                size="sm"
                data-testid="pos-store-save"
                onClick={async () => {
                  if (await run(() => saveStore(store.id, store))) {
                    setStore(null);
                    loadStores();
                  }
                }}
              >
                Save store
              </Button>
              <Button size="sm" variant="plain" onClick={() => setStore(null)}>
                Cancel
              </Button>
            </Box>
          </Sheet>
        )}
        {stores.stores.length > 0 && (
          <Box sx={{ mt: 1.5 }}>
            <FormLabel>Which store each register belongs to</FormLabel>
            {registers.map((r) => (
              <Box key={r.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.25 }}>
                <Typography level="body-sm" sx={{ width: 160 }}>
                  {r.name}
                </Typography>
                <Select
                  size="sm"
                  sx={{ minWidth: 180 }}
                  value={stores.registers[r.id] ?? ""}
                  onChange={async (_, v) => {
                    await run(() => assignStore(r.id, (v as string) || ""));
                    loadStores();
                  }}
                >
                  <Option value="">No store</Option>
                  {stores.stores.map((s) => (
                    <Option key={s.id} value={s.id}>
                      {s.name}
                    </Option>
                  ))}
                </Select>
              </Box>
            ))}
          </Box>
        )}
      </Box2>
    </Box>
  );
}
