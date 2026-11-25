import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AppSplash, appForPath } from "./AppSplash";

describe("AppSplash", () => {
  it("resolves apps by first path segment", () => {
    expect(appForPath("/drive/x/y")?.id).toBe("drive");
    expect(appForPath("/")).toBeUndefined();
    expect(appForPath("/nope")).toBeUndefined();
  });

  it("shows the app name for an app route", () => {
    render(
      <MemoryRouter initialEntries={["/sheets"]}>
        <AppSplash />
      </MemoryRouter>,
    );
    expect(screen.getByTestId("app-splash")).toBeTruthy();
    expect(screen.getByText(/Adrient Sheets/)).toBeTruthy();
  });

  it("falls back to the Adrient mark on the dashboard", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSplash />
      </MemoryRouter>,
    );
    expect(screen.getByText("Adrient Workspace")).toBeTruthy();
    expect(screen.getByAltText("Adrient")).toBeTruthy();
  });
});
