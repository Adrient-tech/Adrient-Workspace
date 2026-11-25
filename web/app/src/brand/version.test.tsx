import { render, screen } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { AboutDialog } from "./AboutDialog";
import { ProductBadge } from "./ProductBadge";
import { APP_VERSION, SUPPORT_EMAIL } from "./version";

const fileVersion = readFileSync(resolve(__dirname, "../../../../VERSION"), "utf8").trim();

describe("product version", () => {
  it("is injected from the repo-root VERSION file", () => {
    expect(APP_VERSION).toBe(fileVersion);
  });

  it("the badge links to adrient.com in a new tab with rel=noopener", () => {
    render(<ProductBadge />);
    const a = screen.getByTestId("product-badge-link");
    expect(a).toHaveAttribute("href", "https://adrient.com");
    expect(a).toHaveAttribute("target", "_blank");
    expect(a.getAttribute("rel")).toContain("noopener");
    expect(a).toHaveTextContent(`Adrient Workspace`);
    expect(a).toHaveTextContent(`v${fileVersion}`);
  });

  it("the About dialog shows product, version, build, site, support and licence", () => {
    render(<AboutDialog open onClose={() => {}} />);
    const d = screen.getByTestId("about-dialog");
    expect(d).toHaveTextContent("Adrient Workspace");
    expect(screen.getByTestId("about-version")).toHaveTextContent(fileVersion);
    expect(screen.getByTestId("about-build").textContent).not.toBe("");
    expect(screen.getByRole("link", { name: "adrient.com" })).toHaveAttribute("href", "https://adrient.com");
    expect(screen.getByRole("link", { name: SUPPORT_EMAIL })).toHaveAttribute("href", `mailto:${SUPPORT_EMAIL}`);
    expect(d).toHaveTextContent("MIT");
  });
});
