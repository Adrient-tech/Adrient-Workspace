import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { DataTable, rangeLabel, rowWindow, ROW_H, type Column, type TableQuery } from "./DataTable";

interface Row {
  id: string;
  name: string;
}
const cols: Column<Row>[] = [{ key: "name", header: "Name", sortable: true, render: (r) => r.name }];
const rows = (n: number, from = 0): Row[] => Array.from({ length: n }, (_, i) => ({ id: `r${from + i}`, name: `Row ${from + i}` }));

type Loader = (q: TableQuery) => Promise<{ items: Row[]; next_cursor?: string; total_estimate?: number; total_capped?: boolean }>;
function setup(load: Loader, url = "/") {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <DataTable columns={cols} rowKey={(r) => r.id} load={load} defaultSort="name" defaultOrder="asc" />
    </MemoryRouter>,
  );
}

describe("rangeLabel", () => {
  it("formats exact, capped and unknown totals", () => {
    expect(rangeLabel(1, 50, 12340)).toBe("1–50 of 12,340");
    expect(rangeLabel(1, 50, 10000, true)).toBe("1–50 of 10,000+");
    expect(rangeLabel(1, 50)).toBe("1–50");
    expect(rangeLabel(0, 0, 0)).toBe("0 results");
  });
});

describe("rowWindow", () => {
  it("windows a long list", () => {
    const w = rowWindow(ROW_H * 500, 560, 10000);
    expect(w.end - w.start).toBeLessThan(30);
  });
});

describe("DataTable", () => {
  it("shows skeleton rows, then data and the range", async () => {
    setup(async () => ({ items: rows(3), total_estimate: 3 }));
    expect(screen.getAllByTestId("table-skeleton").length).toBeGreaterThan(0);
    expect(await screen.findByText("Row 0")).toBeInTheDocument();
    expect(screen.getByTestId("table-range")).toHaveTextContent("1–3 of 3");
  });

  it("restores sort, size and page from the URL and asks the server for them", async () => {
    const load = vi.fn<Loader>(async () => ({ items: rows(2), next_cursor: "n", total_estimate: 500 }));
    setup(load, "/?sort=name&order=desc&size=100&page=3&cur=abc&q=ana");
    await screen.findByText("Row 0");
    expect(load.mock.calls[0][0]).toMatchObject({ limit: 100, sort: "name", order: "desc", cursor: "abc", page: 3, q: "ana" });
    expect(screen.getByTestId("table-range")).toHaveTextContent("201–202 of 500");
  });

  it("pages with the cursor and sorts on header click", async () => {
    const load = vi.fn<Loader>(async (q) =>
      q.cursor ? { items: rows(2, 25) } : { items: rows(25), next_cursor: "c2", total_estimate: 27 },
    );
    setup(load);
    await screen.findByText("Row 0");
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(await screen.findByText("Row 25")).toBeInTheDocument();
    expect(load.mock.calls.at(-1)![0].cursor).toBe("c2");
    fireEvent.click(screen.getByRole("button", { name: "Name" }));
    await waitFor(() => expect(load.mock.calls.at(-1)![0].order).toBe("desc"));
  });

  it("shows the empty state", async () => {
    setup(async () => ({ items: [] }));
    expect(await screen.findByTestId("table-empty")).toBeInTheDocument();
  });

  it("shows an error with retry", async () => {
    const load = vi.fn<Loader>().mockRejectedValueOnce(new Error("HTTP 500")).mockResolvedValue({ items: rows(1) });
    setup(load);
    expect(await screen.findByRole("alert")).toHaveTextContent("HTTP 500");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Row 0")).toBeInTheDocument();
  });
});
