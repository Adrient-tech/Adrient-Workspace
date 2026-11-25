import { describe, expect, it } from "vitest";
import { matchesCustom, showValue } from "./CustomFields";
import { telHref, whatsappHref } from "./Comms";
import { slaState, stageSlaBreach } from "./sla";
import { embedSnippet } from "./Capture";
import type { FieldDef, Settings } from "./types";

const def = (p: Partial<FieldDef>): FieldDef => ({ id: "1", entity: "deal", key: "k", label: "K", type: "text", options: [], required: false, position: 0, ...p });

describe("custom field helpers", () => {
  it("formats values by type", () => {
    expect(showValue(def({ type: "multiselect" }), ["a", "b"])).toBe("a, b");
    expect(showValue(def({ type: "user" }), "u1", [{ id: "u1", name: "Asha", email: "a@x" }])).toBe("Asha");
    expect(showValue(def({}), undefined)).toBe("");
  });
  it("filters by contains, case-insensitively", () => {
    const d = def({ key: "region" });
    expect(matchesCustom(d, { region: "North East" }, "east", [])).toBe(true);
    expect(matchesCustom(d, { region: "South" }, "east", [])).toBe(false);
    expect(matchesCustom(d, {}, "", [])).toBe(true);
  });
});

describe("comms links", () => {
  it("builds WhatsApp links with the default country code for 10-digit numbers", () => {
    expect(whatsappHref("098765 43210")).toBe("https://wa.me/919876543210");
    expect(whatsappHref("+44 20 7946 0958")).toBe("https://wa.me/442079460958");
    expect(whatsappHref("123")).toBe("");
    expect(telHref("+91 98765-43210")).toBe("tel:+919876543210");
  });
});

describe("SLA state", () => {
  const now = Date.parse("2026-01-10T12:00:00Z");
  it("is breached when unanswered past due, ok when answered in time", () => {
    expect(slaState("2026-01-10T10:00:00Z", null, now)).toBe("breached");
    expect(slaState("2026-01-10T10:00:00Z", "2026-01-10T09:00:00Z", now)).toBe("ok");
    expect(slaState("2026-01-10T10:00:00Z", "2026-01-10T11:00:00Z", now)).toBe("breached");
    expect(slaState("2026-01-10T13:00:00Z", null, now)).toBe("due");
    expect(slaState(null, null, now)).toBe("none");
  });
  it("flags deals over the stage limit", () => {
    const s = { sla_stage_days: { new: 3 } } as unknown as Settings;
    expect(stageSlaBreach({ stage: "new", stage_entered_at: "2026-01-01T00:00:00Z" }, s, now)).toBe(3);
    expect(stageSlaBreach({ stage: "new", stage_entered_at: "2026-01-09T00:00:00Z" }, s, now)).toBe(0);
    expect(stageSlaBreach({ stage: "other", stage_entered_at: "2020-01-01T00:00:00Z" }, s, now)).toBe(0);
  });
});

describe("embed snippet", () => {
  it("points an iframe at the public form", () => {
    expect(embedSnippet("https://crm.example.com", "abc")).toContain('src="https://crm.example.com/api/v1/public/crm/lead/abc"');
  });
});

import { dealsWithoutNextStep } from "./logic";
describe("deals without a next step", () => {
  it("lists open deals that have no open follow-up", () => {
    const deals = [{ id: "a", stage: "new" }, { id: "b", stage: "new" }, { id: "c", stage: "won" }, { id: "d", stage: "prop" }] as never[];
    const fus = [{ id: "1", deal_id: "a", done: false }, { id: "2", deal_id: "b", done: true }] as never[];
    expect(dealsWithoutNextStep(deals, fus).map((d: { id: string }) => d.id)).toEqual(["b", "d"]);
  });
});
