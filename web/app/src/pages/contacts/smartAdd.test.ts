import { describe, expect, it } from "vitest";
import { nameFromEmail, parseSmart } from "./smartAdd";

describe("smart add", () => {
  it("reads a recipient list, including quoted commas", () => {
    const r = parseSmart('Jane Doe <jane@acme.com>, bob@y.org; "Lee, Ann" <ann@z.io>');
    expect(r.map((c) => c.emails[0])).toEqual(["jane@acme.com", "bob@y.org", "ann@z.io"]);
    expect(r[0].first_name).toBe("Jane");
    expect(r[0].last_name).toBe("Doe");
    expect(r[1].display_name).toBe("Bob");
    expect(r[2].display_name).toBe("Lee, Ann");
  });

  it("reads rows of name, email, phone, company", () => {
    const r = parseSmart("Ravi Kumar, ravi@x.in, +91 98765 43210, Acme\nMeena Rao, meena@x.in");
    expect(r).toHaveLength(2);
    expect(r[0]).toMatchObject({ display_name: "Ravi Kumar", emails: ["ravi@x.in"], phones: ["+91 98765 43210"], company: "Acme" });
    expect(r[1].phones).toEqual([]);
  });

  it("reads an e-mail signature block", () => {
    const r = parseSmart("Best regards,\n\nPriya Nair\nSenior Manager, Globex Corp\nTel: +91 22 5550 1234\npriya.nair@globex.com\nwww.globex.com");
    // "Best regards," alone is its own block and yields nothing useful.
    const p = r.find((c) => c.emails.includes("priya.nair@globex.com"))!;
    expect(p.display_name).toBe("Priya Nair");
    expect(p.job_title).toBe("Senior Manager");
    expect(p.company).toBe("Globex Corp");
    expect(p.phones).toEqual(["+91 22 5550 1234"]);
    expect(p.notes).toContain("www.globex.com");
  });

  it("splits people on blank lines and ignores junk", () => {
    expect(parseSmart("")).toEqual([]);
    expect(parseSmart("   \n\n  ")).toEqual([]);
    const r = parseSmart("Al Smith\nal@a.com\n\nBo Jones\nbo@b.com");
    expect(r.map((c) => c.display_name)).toEqual(["Al Smith", "Bo Jones"]);
  });

  it("derives a name from an address", () => {
    expect(nameFromEmail("jane.doe42@x.com")).toBe("Jane Doe");
  });
});
