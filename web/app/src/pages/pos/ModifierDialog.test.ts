import { describe, expect, it } from "vitest";
import { applyModifiers, missingRequired } from "./ModifierDialog";
import { addToCart } from "./logic";
import type { ModGroup } from "./extApi";

const size: ModGroup = { id: "g1", name: "Size", required: true, multi: false, options: [{ id: "s", name: "Small", price: 0 }, { id: "l", name: "Large", price: 30 }] };
const extras: ModGroup = { id: "g2", name: "Extras", required: false, multi: true, options: [{ id: "a", name: "Oat milk", price: 20 }, { id: "b", name: "Extra shot", price: 25 }] };

describe("product options", () => {
  it("adds option prices to the base price and lists the choices in the name", () => {
    const r = applyModifiers("Latte", 120, [
      { group: "Size", options: [size.options[1]] },
      { group: "Extras", options: extras.options },
    ]);
    expect(r.name).toBe("Latte (Large, Oat milk, Extra shot)");
    expect(r.price).toBe(195);
  });
  it("keeps the plain name when nothing is chosen", () => {
    expect(applyModifiers("Latte", 120, [{ group: "Size", options: [] }])).toMatchObject({ name: "Latte", price: 120 });
  });
  it("blocks adding while a required group has no choice", () => {
    expect(missingRequired([size, extras], {})).toEqual(["Size"]);
    expect(missingRequired([size, extras], { g1: ["s"] })).toEqual([]);
  });
  it("does not merge a customised line into the plain one with the same price", () => {
    const plain = addToCart([], { product_id: "p", name: "Latte", sku: "", unit_price: 120, tax_rate_id: "" });
    const withOpt = addToCart(plain, { product_id: "p", name: "Latte (Small)", sku: "", unit_price: 120, tax_rate_id: "" });
    expect(withOpt).toHaveLength(2);
    expect(addToCart(withOpt, { product_id: "p", name: "Latte (Small)", sku: "", unit_price: 120, tax_rate_id: "" })[1].qty).toBe(2);
  });
});
