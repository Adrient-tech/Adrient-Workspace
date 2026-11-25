import { describe, expect, it } from "vitest";
import {
  appForFile, appForPath, appLabel, inAppHref, isExternalMeeting, openDriveFile, openLink, withReturn,
} from "./openWith";

const O = "http://workspace.localtest.me:8080";

describe("openLink", () => {
  it("returns null for empty and unsupported", () => {
    expect(openLink("  ", O)).toBeNull();
    expect(openLink("javascript:alert(1)", O)).toBeNull();
    expect(openLink("not a url", O)).toBeNull();
    expect(openLink("//evil.com/x", O)).toBeNull();
  });
  it("opens meeting codes in Meet", () => {
    expect(openLink("abc-defg-hij", O)).toMatchObject({ href: "/meet/abc-defg-hij", app: "meet", external: false });
  });
  it("same-origin links are in-app", () => {
    const t = openLink(`${O}/sheets/42?x=1#c`, O)!;
    expect(t).toMatchObject({ href: "/sheets/42?x=1#c", app: "sheets", external: false, embeddable: true });
    expect(t.label).toBe("Open in Sheets");
  });
  it("aliases count as same workspace", () => {
    expect(openLink("https://ws.adrient.com/docs/9", O, ["ws.adrient.com"])?.href).toBe("/docs/9");
  });
  it("drive share, docs share, invoice, tickets, calendar and chat links", () => {
    expect(openLink(`${O}/docs/share/abc`, O)?.app).toBe("docs");
    expect(openLink(`${O}/invoice/tok`, O)).toMatchObject({ app: "invoicing", external: false });
    expect(openLink(`${O}/tickets/submit/tok`, O)?.app).toBe("tickets");
    expect(openLink(`${O}/calendar?event=7`, O)?.app).toBe("calendar");
    expect(openLink(`${O}/chat/room/5?m=9`, O)?.app).toBe("chat");
    expect(openLink(`${O}/drive/file/xyz`, O)?.app).toBe("drive");
    expect(openLink(`${O}/video/watch/t`, O)?.app).toBe("video");
  });
  it("foreign links are external", () => {
    expect(openLink("https://example.com/a", O)).toMatchObject({ external: true, app: "other", embeddable: false });
  });
  it("external meeting links stay external with a video meeting label", () => {
    for (const u of [
      "https://meet.google.com/abc-defg-hij",
      "https://us02web.zoom.us/j/123",
      "https://teams.microsoft.com/l/meetup-join/x",
    ]) {
      expect(openLink(u, O)).toMatchObject({ external: true, label: "Join video meeting" });
    }
    expect(isExternalMeeting(new URL("https://notzoom.us/x"))).toBe(false);
  });
  it("mailto opens compose", () => {
    expect(openLink("mailto:a@b.com", O)?.href).toBe("/mail?compose=1&to=a%40b.com");
  });
});

describe("inAppHref", () => {
  it("returns path for workspace links only", () => {
    expect(inAppHref(`${O}/docs/1`, O)).toBe("/docs/1");
    expect(inAppHref("https://example.com", O)).toBeNull();
    expect(inAppHref("https://meet.google.com/a", O)).toBeNull();
  });
});

describe("appForPath / appLabel", () => {
  it("maps paths", () => {
    expect(appForPath("/sheets/1")).toBe("sheets");
    expect(appForPath("/org-chart")).toBe("contacts");
    expect(appForPath("/")).toBe("other");
    expect(appForPath("/zzz")).toBe("other");
    expect(appLabel("other")).toBe("Workspace");
    expect(appLabel("pdf")).toBe("PDF");
  });
});

describe("appForFile / openDriveFile", () => {
  it("by mime then extension", () => {
    expect(appForFile({ name: "a.bin", mime_type: "application/pdf" })).toBe("pdf");
    expect(appForFile({ name: "a.xlsx" })).toBe("sheets");
    expect(appForFile({ name: "a.docx" })).toBe("docs");
    expect(appForFile({ name: "a.pptx" })).toBe("slides");
    expect(appForFile({ name: "a.zip" })).toBe("drive");
  });
  it("editor, 3d and viewer routes", () => {
    expect(openDriveFile({ name: "a.xlsx", drive_id: "i1" }).href).toBe("/sheets/i1");
    expect(openDriveFile({ name: "a.glb", drive_id: "i2" }).href).toBe("/3d?file=i2");
    const img = openDriveFile({ name: "a.png", drive_id: "i3" });
    expect(img.href).toBe("/drive/file/i3");
    expect(img.label).toBe("Open image");
  });
});

describe("withReturn", () => {
  it("appends return and keeps hash", () => {
    expect(withReturn("/docs/1", "/chat")).toBe("/docs/1?return=%2Fchat");
    expect(withReturn("/docs/1?a=1#h", "/chat/x")).toBe("/docs/1?a=1&return=%2Fchat%2Fx#h");
  });
  it("rejects open redirects", () => {
    expect(withReturn("/docs/1", "//evil.com")).toBe("/docs/1");
    expect(withReturn("https://x.com", "/chat")).toBe("https://x.com");
  });
});
