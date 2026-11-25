import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AssistantButton, ConnectionChip, EditorHeader, ShareButton } from "./EditorHeader";

afterEach(cleanup);

describe("EditorHeader", () => {
  it("renders back, title, menu and right-hand controls once each", () => {
    const onBack = vi.fn();
    const onTitle = vi.fn();
    render(
      <EditorHeader backLabel="Back to Sheets" onBack={onBack} icon={<i />} title="Budget" titleLabel="Spreadsheet title" onTitle={onTitle} onTitleCommit={() => {}} menu={<nav>menus</nav>}>
        <AssistantButton open={false} onClick={() => {}} />
        <ShareButton onClick={() => {}} />
      </EditorHeader>,
    );
    fireEvent.click(screen.getByLabelText("Back to Sheets"));
    expect(onBack).toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Spreadsheet title"), { target: { value: "B2" } });
    expect(onTitle).toHaveBeenCalledWith("B2");
    expect(screen.getByText("menus")).toBeTruthy();
    // text button + icon button (one is hidden per breakpoint) share one label and one toggle state
    expect(screen.getAllByRole("button", { name: "AI Assistant" }).length).toBe(2);
  });
  it("assistant buttons report the open state", () => {
    render(<AssistantButton open onClick={() => {}} />);
    screen.getAllByRole("button", { name: "AI Assistant" }).forEach((b) => expect(b.getAttribute("aria-pressed")).toBe("true"));
  });
  it("shows the connection chip only when not live", () => {
    const { rerender } = render(<ConnectionChip status="live" />);
    expect(screen.queryByTestId("conn-chip")).toBeNull();
    rerender(<ConnectionChip status="offline" />);
    expect(screen.getByTestId("conn-chip").textContent).toBe("Offline");
    rerender(<ConnectionChip status="connecting" />);
    expect(screen.getByTestId("conn-chip").textContent).toContain("Connecting");
  });
});
