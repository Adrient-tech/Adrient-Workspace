import { describe, expect, it } from "vitest";
import { interceptHref } from "../lib/linkIntercept";

const O = "http://workspace.localtest.me:8080";
const ev = (o: object = {}) => ({ button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, defaultPrevented: false, ...o });
function a(href: string, attrs: Record<string, string> = {}, target = "") {
  const abs = new URL(href, O).toString();
  return {
    href: abs, target,
    hasAttribute: (n: string) => n in attrs,
    getAttribute: (n: string) => (n === "href" ? href : attrs[n] ?? null),
  };
}

describe("interceptHref", () => {
  it("intercepts same-origin workspace links", () => {
    expect(interceptHref(a("/docs/1"), ev(), O, "/")).toBe("/docs/1");
    expect(interceptHref(a(`${O}/sheets/2?x=1`), ev(), O, "/")).toBe("/sheets/2?x=1");
    expect(interceptHref(a("/docs/1", {}, "_blank"), ev(), O, "/")).toBe("/docs/1");
  });
  it("leaves modifier / middle clicks alone", () => {
    expect(interceptHref(a("/docs/1"), ev({ ctrlKey: true }), O, "/")).toBeNull();
    expect(interceptHref(a("/docs/1"), ev({ metaKey: true }), O, "/")).toBeNull();
    expect(interceptHref(a("/docs/1"), ev({ button: 1 }), O, "/")).toBeNull();
    expect(interceptHref(a("/docs/1"), ev({ defaultPrevented: true }), O, "/")).toBeNull();
  });
  it("leaves downloads, external, server and special links alone", () => {
    expect(interceptHref(a("/api/v1/files/1", { download: "" }), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("/docs/1", { download: "" }), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("https://example.com/x"), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("/api/v1/me"), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("/auth/login"), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("mailto:a@b.com"), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("#top"), ev(), O, "/docs/1")).toBeNull();
    expect(interceptHref(a("/docs/1#h"), ev(), O, "/docs/1")).toBeNull();
    expect(interceptHref(a("/docs/2", { "data-no-intercept": "" }), ev(), O, "/")).toBeNull();
    expect(interceptHref(a("/docs/2", {}, "myframe"), ev(), O, "/")).toBeNull();
  });
});
