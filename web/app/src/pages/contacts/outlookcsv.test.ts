import { describe, expect, it } from "vitest";
import { normDate } from "./dates";
import { isOutlookHeader, parseContactsCSV } from "./outlookcsv";
import { parseVCards } from "./vcard";

describe("dates", () => {
  it("accepts ISO and unambiguous dates only", () => {
    expect(normDate("1990-05-17")).toBe("1990-05-17");
    expect(normDate("17/05/1990")).toBe("1990-05-17");
    expect(normDate("05/17/1990")).toBe("1990-05-17");
    expect(normDate("05/06/1990")).toBe(""); // ambiguous, not guessed
    expect(normDate("06/06/1990")).toBe("1990-06-06");
    expect(normDate("0/0/00")).toBe("");
    expect(normDate("1990-02-31")).toBe("");
    expect(normDate("")).toBe("");
  });
});

describe("outlook csv", () => {
  const csv =
    "First Name,Last Name,E-mail Address,Mobile Phone,Home Phone,Company,Job Title,Birthday,Notes,Web Page,Categories\n" +
    'Asha,Menon,asha@x.in,98765 43210,,Acme,Lead,1991-03-04,"likes, tea",acme.in,Friends;Work\n' +
    ",,,,,,,,,,\n" +
    "Raj,,raj@y.in,,0222 555 1234,,,05/06/1990,,,\n";
  it("detects the format and maps columns", () => {
    expect(isOutlookHeader(csv.split("\n")[0].split(","))).toBe(true);
    const { contacts, source } = parseContactsCSV(csv);
    expect(source).toBe("outlook");
    expect(contacts).toHaveLength(2);
    expect(contacts[0]).toMatchObject({ display_name: "Asha Menon", emails: ["asha@x.in"], phones: ["98765 43210"], company: "Acme", job_title: "Lead", notes: "likes, tea", labels: ["Friends", "Work"] });
    expect(contacts[0].extras).toMatchObject({ birthday: "1991-03-04", website: "acme.in" });
    expect(contacts[1].extras).toBeUndefined(); // ambiguous birthday dropped
    expect(contacts[1].phones).toEqual(["0222 555 1234"]);
  });
  it("still reads Google exports", () => {
    const g = "Name,Given Name,Family Name,E-mail 1 - Value,Birthday\nA B,A,B,a@b.co,1980-01-02\n";
    const { contacts, source } = parseContactsCSV(g);
    expect(source).toBe("google");
    expect(contacts[0].extras?.birthday).toBe("1980-01-02");
  });
});

describe("vcard extras", () => {
  it("reads BDAY, URL and ADR", () => {
    const v = "BEGIN:VCARD\nVERSION:3.0\nFN:Zed\nBDAY:19900517\nURL:https://z.io\nADR;TYPE=home:;;1 Main St;Pune;;411001;India\nEND:VCARD\n";
    const [c] = parseVCards(v);
    expect(c.extras).toEqual({ birthday: "1990-05-17", website: "https://z.io", address: "1 Main St, Pune, 411001, India" });
  });
});
