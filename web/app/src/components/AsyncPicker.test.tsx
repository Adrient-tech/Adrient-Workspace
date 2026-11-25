import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AsyncPicker, ROW_HEIGHT, visibleRange, type PickerItem } from "./AsyncPicker";

const people = (n: number, from = 0): PickerItem[] =>
  Array.from({ length: n }, (_, i) => ({ id: `u${from + i}`, label: `Person ${from + i}`, secondary: `p${from + i}@x.test` }));

describe("visibleRange", () => {
  it("renders only a window of rows", () => {
    const { start, end } = visibleRange(0, ROW_HEIGHT * 6, 20000);
    expect(start).toBe(0);
    expect(end).toBeLessThan(15);
    const mid = visibleRange(ROW_HEIGHT * 10000, ROW_HEIGHT * 6, 20000);
    expect(mid.end - mid.start).toBeLessThan(15);
  });
});

describe("AsyncPicker", () => {
  it("debounces typing and asks the server for the query", async () => {
    const fetchPage = vi.fn(async (q: string) => ({ items: q ? people(2) : people(5) }));
    render(<AsyncPicker fetchPage={fetchPage} value={[]} onChange={() => {}} ariaLabel="People" debounceMs={20} />);
    const input = screen.getByRole("combobox", { name: "People" });
    fireEvent.focus(input);
    await waitFor(() => expect(fetchPage).toHaveBeenCalledWith("", undefined, expect.anything()));
    fireEvent.change(input, { target: { value: "ana" } });
    fireEvent.change(input, { target: { value: "anal" } });
    await waitFor(() => expect(fetchPage).toHaveBeenCalledWith("anal", undefined, expect.anything()));
    expect(fetchPage.mock.calls.some((c) => c[0] === "ana")).toBe(false);
  });

  it("shows the empty and the error state", async () => {
    const fetchPage = vi.fn().mockResolvedValueOnce({ items: [] }).mockRejectedValueOnce(new Error("boom"));
    render(<AsyncPicker fetchPage={fetchPage} value={[]} onChange={() => {}} ariaLabel="P" debounceMs={5} emptyText="Nobody" />);
    fireEvent.focus(screen.getByRole("combobox"));
    expect(await screen.findByTestId("picker-empty")).toHaveTextContent("Nobody");
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "zz" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  });

  it("selects with the keyboard and keeps chips with a +N overflow", async () => {
    const onChange = vi.fn();
    const fetchPage = async () => ({ items: people(5) });
    const selected = people(5, 100);
    render(<AsyncPicker multiple maxChips={2} fetchPage={fetchPage} value={selected} onChange={onChange} ariaLabel="People" />);
    expect(screen.getByText("+3")).toBeInTheDocument();
    const input = screen.getByRole("combobox");
    fireEvent.focus(input);
    await screen.findAllByTestId("picker-option");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith([...selected, expect.objectContaining({ id: "u1" })]);
  });

  it("loads the next page when the list scrolls near the end", async () => {
    const fetchPage = vi.fn(async (_q: string, cursor?: string) =>
      cursor ? { items: people(30, 30) } : { items: people(30), next_cursor: "c1" },
    );
    render(<AsyncPicker fetchPage={fetchPage} value={[]} onChange={() => {}} ariaLabel="People" />);
    fireEvent.focus(screen.getByRole("combobox"));
    const list = await screen.findByTestId("picker-list");
    await screen.findAllByTestId("picker-option");
    Object.defineProperty(list, "clientHeight", { value: ROW_HEIGHT * 6, configurable: true });
    Object.defineProperty(list, "scrollHeight", { value: ROW_HEIGHT * 30, configurable: true });
    list.scrollTop = ROW_HEIGHT * 24;
    fireEvent.scroll(list);
    await waitFor(() => expect(fetchPage).toHaveBeenCalledWith("", "c1", expect.anything()));
  });
});
