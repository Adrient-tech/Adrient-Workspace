import { describe, it, expect } from "vitest";
import { buildOrgTree } from "./index";
import type { Person } from "../../api/people";

const mk = (id: string, manager = ""): Person => ({
  id, name: id, email: id + "@x", job_title: "", phone: "", location: "", employee_id: "",
  start_date: "", department_id: "", department_name: "", manager_id: manager, manager_name: "",
});

describe("buildOrgTree", () => {
  it("nests reports under managers", () => {
    const t = buildOrgTree([mk("ceo"), mk("vp", "ceo"), mk("dev", "vp"), mk("dev2", "vp")]);
    expect(t).toHaveLength(1);
    expect(t[0].reports[0].reports.map((n) => n.person.id)).toEqual(["dev", "dev2"]);
  });
  it("treats unknown managers as roots and survives loops", () => {
    const t = buildOrgTree([mk("a", "ghost"), mk("b", "c"), mk("c", "b")]);
    const ids = new Set<string>();
    const walk = (ns: typeof t) => ns.forEach((n) => { ids.add(n.person.id); walk(n.reports); });
    walk(t);
    expect([...ids].sort()).toEqual(["a", "b", "c"]);
  });
});
