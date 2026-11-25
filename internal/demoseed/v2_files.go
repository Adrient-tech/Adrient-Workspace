package demoseed

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/docs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/slides"
)

// DocTitles are the demo Docs. Their bodies are typed in by scripts/seed-docs.mjs
// (Docs keeps live-edit state that only the editor can write).
var DocTitles = []string{"Meeting notes: Quarterly business review", "Project brief: Warehouse automation", "Policy: Supplier onboarding", "Proposal: Coromandel Logistics partnership"}

func (s *seeder) runV2() {
	acct := accounting.NewRepository(s.pool)
	s.v2Users()
	s.v2HR(acct)
	s.v2Contacts()
	s.v2Sales(acct)
	s.v2Mail()
	s.v2Chat()
	s.v2Drive()
	s.v2Calendar()
	s.v2Projects()
	s.v2Tickets()
	s.v2Forms()
	s.v2Keep()
	s.v2Editors()
}

func (s *seeder) v2Editors() {
	dr := docs.NewRepository(s.pool)
	for _, t := range DocTitles {
		if _, err := dr.Create(s.ctx, s.org, s.admin, t); err != nil {
			s.warn("docs v2", err)
			continue
		}
		s.count("docs", 1)
	}
	sr := sheets.NewRepository(s.pool)
	for _, b := range demoSheets() {
		sh, err := sr.Create(s.ctx, s.org, s.admin, b.title)
		if err != nil {
			s.warn("sheets v2", err)
			continue
		}
		data, _ := json.Marshal(b.workbook())
		if err := sr.Save(s.ctx, s.org, sh.ID, string(data)); err != nil {
			s.warn("sheets save v2", err)
		}
		s.count("sheets", 1)
	}
	slr := slides.NewRepository(s.pool)
	for _, d := range demoDecks() {
		dk, err := slr.Create(s.ctx, s.org, s.admin, d.title)
		if err != nil {
			s.warn("slides v2", err)
			continue
		}
		data, _ := json.Marshal(map[string]any{"slides": d.slides})
		if err := slr.Save(s.ctx, s.org, dk.ID, string(data)); err != nil {
			s.warn("slides save v2", err)
		}
		s.count("slides", 1)
	}
}

// ---- Sheets -------------------------------------------------------------

type cell struct {
	r, c   int
	v      any
	f      string
	bold   bool
	bg, fc string
	fmt    string
}

type sheetBook struct {
	title  string
	cells  []cell
	cols   map[int]int
	extra  map[string]any // adrientCharts, adrientCF
	second *sheetTab
	freeze bool
}

type sheetTab struct {
	name  string
	cells []cell
	extra map[string]any
}

func colL(c int) string { return string(rune('A' + c)) }

func mkCells(cs []cell) []map[string]any {
	var out []map[string]any
	for _, c := range cs {
		v := map[string]any{"v": c.v, "m": fmt.Sprint(c.v)}
		if f, ok := c.v.(float64); ok {
			v["m"] = trimF(f)
		}
		if c.f != "" {
			v["f"] = c.f
		}
		if c.bold {
			v["bl"] = 1
		}
		if c.bg != "" {
			v["bg"] = c.bg
		}
		if c.fc != "" {
			v["fc"] = c.fc
		}
		if _, ok := c.v.(string); !ok {
			ct := map[string]any{"t": "n", "fa": "General"}
			if c.fmt != "" {
				ct["fa"] = c.fmt
			}
			v["ct"] = ct
		}
		out = append(out, map[string]any{"r": c.r, "c": c.c, "v": v})
	}
	return out
}

func trimF(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprint(int64(f))
	}
	return fmt.Sprintf("%.2f", f)
}

func (b sheetBook) workbook() []map[string]any {
	cfg := map[string]any{}
	if len(b.cols) > 0 {
		cl := map[string]int{}
		for k, w := range b.cols {
			cl[fmt.Sprint(k)] = w
		}
		cfg["columnlen"] = cl
	}
	s1 := map[string]any{"name": "Sheet1", "id": "sheet_1", "status": 1, "order": 0, "celldata": mkCells(b.cells), "config": cfg}
	if b.freeze {
		s1["frozen"] = map[string]any{"type": "row"}
	}
	for k, v := range b.extra {
		s1[k] = v
	}
	out := []map[string]any{s1}
	if b.second != nil {
		s2 := map[string]any{"name": b.second.name, "id": "sheet_2", "status": 0, "order": 1, "celldata": mkCells(b.second.cells)}
		for k, v := range b.second.extra {
			s2[k] = v
		}
		out = append(out, s2)
	}
	return out
}

func header(r int, names ...string) []cell {
	var out []cell
	for c, n := range names {
		out = append(out, cell{r: r, c: c, v: n, bold: true, bg: "#126C5A", fc: "#ffffff"})
	}
	return out
}

func chartCfg(id, typ, title string, r0, r1, c0, c1 int, anchorR, anchorC int) map[string]any {
	return map[string]any{"id": id, "type": typ, "title": title, "range": map[string]int{"r0": r0, "r1": r1, "c0": c0, "c1": c1},
		"headerRow": true, "labelCol": true, "sheetId": "sheet_1", "legend": "bottom",
		"anchor": map[string]any{"r": anchorR, "c": anchorC, "dx": 0, "dy": 0, "w": 520, "h": 300}}
}

func cfRule(id string, typ string, c1, r1, c2, r2 int, extra map[string]any) map[string]any {
	m := map[string]any{"id": id, "type": typ, "priority": 1, "ranges": []map[string]int{{"c1": c1, "r1": r1, "c2": c2, "r2": r2}}}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func demoSheets() []sheetBook {
	var books []sheetBook

	// 1. Budget FY with monthly totals.
	{
		months := []string{"Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec", "Jan", "Feb", "Mar"}
		cats := []struct {
			n string
			b float64
		}{{"Salaries", 2400000}, {"Rent and utilities", 320000}, {"Marketing", 260000}, {"Logistics", 410000}, {"Software", 150000}, {"Travel", 120000}, {"Professional fees", 90000}, {"Maintenance", 110000}}
		cells := header(0, append(append([]string{"Category"}, months...), "FY total", "Budget", "Variance")...)
		var tot [12]float64
		for i, c := range cats {
			r := i + 1
			cells = append(cells, cell{r: r, c: 0, v: c.n, bold: true})
			sum := 0.0
			for m := 0; m < 12; m++ {
				v := float64(int(c.b/12*(0.9+0.2*float64((i*5+m*3)%7)/6)/100) * 100)
				sum += v
				tot[m] += v
				cells = append(cells, cell{r: r, c: m + 1, v: v, fmt: "#,##0"})
			}
			cells = append(cells, cell{r: r, c: 13, v: sum, f: fmt.Sprintf("=SUM(B%d:M%d)", r+1, r+1), bold: true, fmt: "#,##0"},
				cell{r: r, c: 14, v: c.b, fmt: "#,##0"},
				cell{r: r, c: 15, v: c.b - sum, f: fmt.Sprintf("=O%d-N%d", r+1, r+1), fmt: "#,##0"})
		}
		tr := len(cats) + 1
		cells = append(cells, cell{r: tr, c: 0, v: "Monthly total", bold: true, bg: "#d7efe8"})
		grand, gb := 0.0, 0.0
		for m := 0; m < 12; m++ {
			grand += tot[m]
			cells = append(cells, cell{r: tr, c: m + 1, v: tot[m], f: fmt.Sprintf("=SUM(%s2:%s%d)", colL(m+1), colL(m+1), tr), bold: true, bg: "#d7efe8", fmt: "#,##0"})
		}
		for _, c := range cats {
			gb += c.b
		}
		cells = append(cells, cell{r: tr, c: 13, v: grand, f: fmt.Sprintf("=SUM(N2:N%d)", tr), bold: true, bg: "#d7efe8", fmt: "#,##0"},
			cell{r: tr, c: 14, v: gb, f: fmt.Sprintf("=SUM(O2:O%d)", tr), bold: true, bg: "#d7efe8", fmt: "#,##0"},
			cell{r: tr, c: 15, v: gb - grand, f: fmt.Sprintf("=O%d-N%d", tr+1, tr+1), bold: true, bg: "#d7efe8", fmt: "#,##0"})
		books = append(books, sheetBook{title: "Budget FY 2026-27", cells: cells, freeze: true, cols: map[int]int{0: 150},
			extra: map[string]any{
				"adrientCharts": []map[string]any{},
				"adrientCF": []map[string]any{cfRule("cf1", "cellIs", 15, 1, 15, len(cats), map[string]any{"operator": "lessThan", "formula1": "0", "style": map[string]any{"fill": "#fecaca", "color": "#991b1b"}}),
					cfRule("cf2", "dataBar", 13, 1, 13, len(cats), map[string]any{"bar": map[string]any{"min": map[string]any{"type": "autoMin"}, "max": map[string]any{"type": "autoMax"}, "color": "#38a89d", "negativeColor": "#e5484d", "axisPosition": "automatic", "axisColor": "#000000", "direction": "context", "showValue": true, "gradient": true, "minLength": 0, "maxLength": 100}}),
				},
			}})

		b := &books[len(books)-1]
		// A small summary block (category, FY total) feeds the chart with proper labels.
		b.cells = append(b.cells, cell{r: 0, c: 17, v: "Category", bold: true, bg: "#126C5A", fc: "#ffffff"}, cell{r: 0, c: 18, v: "FY total", bold: true, bg: "#126C5A", fc: "#ffffff"})
		for i, c := range cats {
			sum := 0.0
			for _, cc := range b.cells {
				if cc.r == i+1 && cc.c == 13 {
					sum = cc.v.(float64)
				}
			}
			b.cells = append(b.cells, cell{r: i + 1, c: 17, v: c.n, f: fmt.Sprintf("=A%d", i+2)}, cell{r: i + 1, c: 18, v: sum, f: fmt.Sprintf("=N%d", i+2), fmt: "#,##0"})
		}
		b.extra["adrientCharts"] = []map[string]any{chartCfg("c1", "bar", "Spend by category (FY)", 0, len(cats), 17, 18, 12, 0)}
		b.cols[17] = 150
	}

	// 2. Sales by region with pivot data source.
	{
		regions := []string{"West", "North", "South", "East", "Central"}
		products := []string{"Apparel", "Kitchen", "Electricals", "Stationery"}
		cells := header(0, "Region", "Q1", "Q2", "Q3", "Q4", "FY total", "Share")
		var rows [][]float64
		gt := 0.0
		for i, rg := range regions {
			var q [4]float64
			s := 0.0
			for k := range q {
				q[k] = float64(180000 + ((i*7+k*5)%9)*45000)
				s += q[k]
			}
			gt += s
			rows = append(rows, []float64{q[0], q[1], q[2], q[3], s})
			_ = rg
		}
		for i, rg := range regions {
			r := i + 1
			cells = append(cells, cell{r: r, c: 0, v: rg, bold: true})
			for k := 0; k < 4; k++ {
				cells = append(cells, cell{r: r, c: k + 1, v: rows[i][k], fmt: "#,##0"})
			}
			cells = append(cells, cell{r: r, c: 5, v: rows[i][4], f: fmt.Sprintf("=SUM(B%d:E%d)", r+1, r+1), bold: true, fmt: "#,##0"},
				cell{r: r, c: 6, v: rows[i][4] / gt, f: fmt.Sprintf("=F%d/F$7", r+1), fmt: "0.0%"})
		}
		cells = append(cells, cell{r: 6, c: 0, v: "Total", bold: true, bg: "#d7efe8"})
		for k := 0; k < 5; k++ {
			s := 0.0
			for i := range regions {
				s += rows[i][k]
			}
			cells = append(cells, cell{r: 6, c: k + 1, v: s, f: fmt.Sprintf("=SUM(%s2:%s6)", colL(k+1), colL(k+1)), bold: true, bg: "#d7efe8", fmt: "#,##0"})
		}
		cells = append(cells, cell{r: 6, c: 6, v: 1.0, f: "=SUM(G2:G6)", bold: true, bg: "#d7efe8", fmt: "0.0%"})
		// Detail table for the pivot.
		var det []cell
		det = append(det, header(0, "Region", "Category", "Quarter", "Sales")...)
		r := 1
		for i, rg := range regions {
			for pi, p := range products {
				for q := 0; q < 4; q++ {
					det = append(det, cell{r: r, c: 0, v: rg}, cell{r: r, c: 1, v: p}, cell{r: r, c: 2, v: fmt.Sprintf("Q%d", q+1)},
						cell{r: r, c: 3, v: float64(30000 + ((i*3+pi*5+q*7)%11)*9000), fmt: "#,##0"})
					r++
				}
			}
		}
		piv := map[string]any{"id": "p1", "title": "Sales by region and category", "range": map[string]int{"r0": 0, "r1": r - 1, "c0": 0, "c1": 3},
			"sourceSheetId": "sheet_2", "rows": []int{0}, "cols": []int{1}, "values": []map[string]any{{"field": 3, "agg": "sum", "numFmt": "#,##0"}},
			"anchor": map[string]any{"sheetId": "sheet_1", "r": 10, "c": 0}, "grandTotalRow": true, "grandTotalCol": true}
		books = append(books, sheetBook{title: "Sales by region", cells: cells, freeze: true, cols: map[int]int{0: 110},
			second: &sheetTab{name: "Sales detail", cells: det},
			extra: map[string]any{
				"adrientCharts": []map[string]any{chartCfg("c1", "column", "Quarterly sales by region", 0, 5, 0, 4, 1, 8)},
				"adrientPivots": []map[string]any{piv},
				"adrientCF":     []map[string]any{cfRule("cf1", "colorScale", 5, 1, 5, 5, map[string]any{"cfvos": []map[string]any{{"type": "min"}, {"type": "max"}}, "colors": []string{"#ffffff", "#38a89d"}})},
			}})
	}

	// 3. Inventory count.
	{
		items := []struct {
			sku, name string
			sys, cnt  float64
			cost      float64
		}{{"DEMO-0001", "Cotton Shirt Classic", 121, 120, 520}, {"DEMO-0002", "Steel Bottle Classic", 88, 88, 118}, {"DEMO-0003", "LED Bulb 9W Classic", 309, 310, 62}, {"DEMO-0004", "Notebook A5 Classic", 540, 536, 28}, {"DEMO-0005", "Ball Pen Box Classic", 210, 210, 74}, {"DEMO-0006", "Copier Paper Ream", 96, 90, 245}, {"DEMO-0007", "Packing Tape Classic", 44, 44, 38}, {"DEMO-0008", "Bubble Wrap Roll", 31, 28, 190}, {"DEMO-0009", "Safety Gloves", 150, 152, 55}, {"DEMO-0010", "Water Filter Cartridge", 62, 62, 310}, {"DEMO-0011", "Basmati Rice 5kg", 400, 396, 410}, {"DEMO-0012", "Tea Leaves 1kg", 180, 181, 260}}
		cells := header(0, "SKU", "Item", "System qty", "Counted", "Variance", "Unit cost", "Value of variance", "Status")
		for i, it := range items {
			r := i + 1
			v := it.cnt - it.sys
			st := "OK"
			if v != 0 {
				st = "Recount"
			}
			cells = append(cells, cell{r: r, c: 0, v: it.sku}, cell{r: r, c: 1, v: it.name, bold: true}, cell{r: r, c: 2, v: it.sys}, cell{r: r, c: 3, v: it.cnt},
				cell{r: r, c: 4, v: v, f: fmt.Sprintf("=D%d-C%d", r+1, r+1)}, cell{r: r, c: 5, v: it.cost, fmt: "#,##0"},
				cell{r: r, c: 6, v: v * it.cost, f: fmt.Sprintf("=E%d*F%d", r+1, r+1), fmt: "#,##0"},
				cell{r: r, c: 7, v: st, f: fmt.Sprintf(`=IF(E%d=0,"OK","Recount")`, r+1)})
		}
		n := len(items)
		sv, sc := 0.0, 0.0
		for _, it := range items {
			sv += (it.cnt - it.sys) * it.cost
			sc += it.cnt
		}
		cells = append(cells, cell{r: n + 1, c: 1, v: "Totals", bold: true, bg: "#d7efe8"}, cell{r: n + 1, c: 3, v: sc, f: fmt.Sprintf("=SUM(D2:D%d)", n+1), bold: true, bg: "#d7efe8"},
			cell{r: n + 1, c: 6, v: sv, f: fmt.Sprintf("=SUM(G2:G%d)", n+1), bold: true, bg: "#d7efe8", fmt: "#,##0"})
		books = append(books, sheetBook{title: "Inventory count: Pune warehouse", cells: cells, freeze: true, cols: map[int]int{1: 190, 6: 120},
			extra: map[string]any{
				"adrientCF": []map[string]any{cfRule("cf1", "cellIs", 4, 1, 4, n, map[string]any{"operator": "lessThan", "formula1": "0", "style": map[string]any{"fill": "#fecaca", "color": "#991b1b"}}),
					cfRule("cf2", "containsText", 7, 1, 7, n, map[string]any{"text": "Recount", "style": map[string]any{"fill": "#fde68a", "color": "#92400e", "bold": true}})},
			}})
	}

	// 4. Project tracker.
	{
		tasks := []struct {
			t, owner, st string
			pct          float64
			start, due   string
		}{
			{"Map bin locations", "Arjun Patil", "Done", 1, "2026-10-12", "2026-10-30"}, {"Label all bins", "Arjun Patil", "Done", 1, "2026-10-20", "2026-11-06"},
			{"Pilot scanning at Pune store", "Faisal Khan", "In progress", 0.6, "2026-11-03", "2026-11-28"}, {"Reorder points for top 150 SKUs", "Karan Malhotra", "In progress", 0.45, "2026-11-05", "2026-12-05"},
			{"Scanner app integration", "Imran Sheikh", "In progress", 0.7, "2026-11-01", "2026-11-26"}, {"Train shift A and B", "Arjun Patil", "Not started", 0, "2026-12-01", "2026-12-12"},
			{"Cut-over weekend", "Asha Menon", "Not started", 0, "2026-12-13", "2026-12-14"}, {"Go-live review", "Asha Menon", "Not started", 0, "2026-12-18", "2026-12-18"},
			{"Vendor contract: scanners", "Karan Malhotra", "At risk", 0.3, "2026-11-02", "2026-11-20"}, {"Budget approval", "Priya Nair", "Done", 1, "2026-10-05", "2026-10-15"},
		}
		cells := header(0, "Task", "Owner", "Status", "% complete", "Start", "Due", "Days left")
		for i, t := range tasks {
			r := i + 1
			cells = append(cells, cell{r: r, c: 0, v: t.t, bold: true}, cell{r: r, c: 1, v: t.owner}, cell{r: r, c: 2, v: t.st}, cell{r: r, c: 3, v: t.pct, fmt: "0%"}, cell{r: r, c: 4, v: t.start}, cell{r: r, c: 5, v: t.due})
			cells = append(cells, cell{r: r, c: 6, v: "", f: fmt.Sprintf("=DATEVALUE(F%d)-TODAY()", r+1)})
		}
		n := len(tasks)
		books = append(books, sheetBook{title: "Project tracker: Warehouse automation", cells: cells, freeze: true, cols: map[int]int{0: 230, 1: 130, 2: 100, 4: 100, 5: 100},
			extra: map[string]any{
				"adrientCharts": []map[string]any{chartCfg("c1", "column", "Completion by task", 0, n, 3, 3, 1, 8)},
				"adrientCF": []map[string]any{
					cfRule("cf1", "containsText", 2, 1, 2, n, map[string]any{"text": "Done", "style": map[string]any{"fill": "#bbf7d0", "color": "#166534"}}),
					cfRule("cf2", "containsText", 2, 1, 2, n, map[string]any{"text": "At risk", "style": map[string]any{"fill": "#fecaca", "color": "#991b1b", "bold": true}}),
					cfRule("cf3", "containsText", 2, 1, 2, n, map[string]any{"text": "In progress", "style": map[string]any{"fill": "#fef08a", "color": "#854d0e"}}),
					cfRule("cf4", "dataBar", 3, 1, 3, n, map[string]any{"bar": map[string]any{"min": map[string]any{"type": "num", "value": "0"}, "max": map[string]any{"type": "num", "value": "1"}, "color": "#38a89d", "negativeColor": "#e5484d", "axisPosition": "automatic", "axisColor": "#000000", "direction": "context", "showValue": true, "gradient": true, "minLength": 0, "maxLength": 100}}),
				}}})
	}
	return books
}

// ---- Slides -------------------------------------------------------------

const (
	gGreen = "#126C5A"
	gInk   = "#00261F"
	gMint  = "#d7efe8"
)

var idSeq int

func nid() string { idSeq++; return fmt.Sprintf("d%04d", idSeq) }

func stext(x, y, w, h float64, text string, size float64, bold bool, color, align string) map[string]any {
	return map[string]any{"id": nid(), "type": "text", "x": x, "y": y, "w": w, "h": h, "fontSize": size, "bold": bold, "color": color, "align": align, "valign": "top", "fontFamily": "Arial", "text": text}
}

func srect(x, y, w, h float64, fill string) map[string]any {
	return map[string]any{"id": nid(), "type": "rect", "x": x, "y": y, "w": w, "h": h, "fill": fill, "stroke": "none", "strokeWidth": 0}
}

func sslide(bg, notes string, els ...map[string]any) map[string]any {
	return map[string]any{"id": nid(), "background": bg, "notes": notes, "elements": els}
}

func scover(title, sub, foot, notes string) map[string]any {
	return sslide(gGreen, notes, srect(70, 196, 90, 6, gMint), stext(70, 214, 820, 100, title, 52, true, "#ffffff", "left"), stext(70, 330, 820, 50, sub, 24, false, gMint, "left"), stext(70, 470, 820, 30, foot, 14, false, gMint, "left"))
}

func sbullets(title string, pts []string, notes string) map[string]any {
	var b []string
	for _, p := range pts {
		b = append(b, "• "+p)
	}
	return sslide("#ffffff", notes, srect(0, 0, 960, 8, gGreen), stext(70, 50, 820, 70, title, 34, true, gGreen, "left"), stext(70, 150, 820, 330, strings.Join(b, "\n"), 22, false, "#2f3b38", "left"))
}

func sstats(title string, items [][2]string, notes string) map[string]any {
	els := []map[string]any{srect(0, 0, 960, 8, gGreen), stext(70, 50, 820, 70, title, 34, true, gGreen, "left")}
	w, gap := 260.0, 30.0
	x0 := (960 - (float64(len(items))*w + float64(len(items)-1)*gap)) / 2
	for i, it := range items {
		x := x0 + float64(i)*(w+gap)
		els = append(els, srect(x, 170, w, 220, gMint), stext(x, 210, w, 80, it[0], 48, true, gGreen, "center"), stext(x, 310, w, 60, it[1], 20, false, gInk, "center"))
	}
	return sslide("#ffffff", notes, els...)
}

func stable(title string, rows [][]string, notes string) map[string]any {
	cells := make([][]string, len(rows))
	copy(cells, rows)
	tbl := map[string]any{"rows": len(rows), "cols": len(rows[0]), "cells": cells}
	return sslide("#ffffff", notes, srect(0, 0, 960, 8, gGreen), stext(70, 50, 820, 70, title, 34, true, gGreen, "left"),
		map[string]any{"id": nid(), "type": "table", "x": 70, "y": 150, "w": 820, "h": float64(len(rows)) * 48, "table": tbl, "fill": "none", "stroke": "none", "strokeWidth": 0, "fontSize": 18, "fontFamily": "Arial", "color": gInk})
}

func schart(title, typ string, data [][]string, tx, notes string) map[string]any {
	return sslide("#ffffff", notes, srect(0, 0, 960, 8, gGreen), stext(70, 50, 820, 70, title, 34, true, gGreen, "left"),
		map[string]any{"id": nid(), "type": "chart", "x": 70, "y": 140, "w": 540, "h": 340, "chart": map[string]any{"type": typ, "title": "", "data": data, "legend": "bottom", "dataLabels": typ == "pie"}},
		srect(640, 160, 250, 300, gMint), stext(656, 176, 220, 270, tx, 18, false, gInk, "left"))
}

func sclosing(title, sub string) map[string]any {
	return sslide(gInk, "Close with the single action you want from the audience.", stext(70, 200, 820, 90, title, 48, true, "#ffffff", "center"), stext(70, 310, 820, 50, sub, 22, false, gMint, "center"))
}

func sroadmap(title string, cols [][2]string, notes string) map[string]any {
	els := []map[string]any{srect(0, 0, 960, 8, gGreen), stext(70, 50, 820, 70, title, 34, true, gGreen, "left"), srect(70, 190, 820, 4, gGreen)}
	w := 820.0 / float64(len(cols))
	for i, c := range cols {
		x := 70 + float64(i)*w
		els = append(els, map[string]any{"id": nid(), "type": "ellipse", "x": x + 8, "y": 178, "w": 28, "h": 28, "fill": gGreen, "stroke": "#ffffff", "strokeWidth": 3},
			stext(x, 130, w-10, 36, c[0], 22, true, gGreen, "left"), srect(x, 230, w-16, 230, gMint), stext(x+14, 244, w-44, 210, c[1], 17, false, gInk, "left"))
	}
	return sslide("#ffffff", notes, els...)
}

type deckDef struct {
	title  string
	slides []map[string]any
}

func demoDecks() []deckDef {
	return []deckDef{
		{"Adrient Trading: investor pitch", []map[string]any{
			scover("Adrient Trading", "Modern wholesale for small-town India", "Seed round, December 2026", "Open with the problem in one sentence."),
			sbullets("The problem", []string{"Small retailers reorder by phone and WhatsApp, and lose 8 percent of sales to stock-outs", "Distributors juggle Excel, Tally and notebooks", "No single view of stock, orders and money"}, "Use the Surat retailer story."),
			sbullets("Our solution", []string{"One workspace for orders, stock, invoicing and accounts", "Barcode receiving and automatic reorder points", "GST-ready invoices and bank reconciliation out of the box"}, "Show the product screens."),
			sstats("Traction", [][2]string{{"₹4.8 Cr", "Annual run rate"}, {"212", "Active retailers"}, {"38%", "Month-on-month order growth"}}, "Explain what drives each number."),
			schart("Revenue growth", "column", [][]string{{"", "Revenue (₹ lakh)"}, {"Q1", "62"}, {"Q2", "88"}, {"Q3", "131"}, {"Q4 (plan)", "184"}}, "Revenue has doubled in three quarters.\n\nGross margin holds at 31 percent.\n\nNo single customer is above 6 percent.", "Walk through the quarters."),
			stable("Business model", [][]string{{"Plan", "Price", "Who it is for"}, {"Retailer", "₹999 / month", "Single-store shops"}, {"Distributor", "₹2,999 / month", "Growing wholesale teams"}, {"Enterprise", "Custom", "Multi-warehouse groups"}}, "Explain the land and expand motion."),
			sbullets("The ask", []string{"Raising ₹6 Cr seed", "60% product and engineering, 30% sales, 10% operations", "18-month milestones: 1,000 retailers and ₹20 Cr run rate"}, "State the number clearly."),
			sclosing("Thank you", "founders@adrient.example"),
		}},
		{"Q3 results: quarterly business review", []map[string]any{
			scover("Q3 business review", "Results, learnings and Q4 priorities", "Operations and Finance, November 2026", "Share the headline number first."),
			sstats("Quarter at a glance", [][2]string{{"₹1.86 Cr", "Revenue"}, {"+12%", "Growth vs target"}, {"31.2%", "Gross margin"}}, "Headline numbers."),
			schart("Revenue by region", "column", [][]string{{"", "Q2", "Q3"}, {"West", "42", "51"}, {"North", "28", "33"}, {"South", "25", "29"}, {"East", "19", "22"}, {"Central", "14", "17"}}, "West and North drove 70 percent of the growth.\n\nCentral is small but growing fastest, +21 percent.", "Compare Q2 and Q3."),
			stable("Targets vs actuals", [][]string{{"Metric", "Target", "Actual", "Status"}, {"Revenue (₹ lakh)", "166", "186", "Ahead"}, {"Gross margin", "30%", "31.2%", "Ahead"}, {"New customers", "40", "37", "At risk"}, {"Receivables over 60 days", "< 3", "3", "On track"}}, "Highlight the one at risk."),
			sbullets("Wins", []string{"Closed Sahyadri Hardware, ₹4.2 lakh annual supply", "Pune store ready for the 1 December opening", "Barcode pilot cut receiving time by 40 percent"}, "Name owners."),
			sbullets("Challenges", []string{"Kaveri Textiles delivery slipped by six days", "Two renewals moved into October", "Customer escalation in Surat, resolved with a credit note"}, "What we are doing about each."),
			sbullets("Q4 priorities", []string{"Open the Pune store and hit ₹12 lakh in the first month", "Roll barcode scanning out to both warehouses", "Move new suppliers to 30-day terms"}, "One owner for each."),
			sclosing("Questions?", "Discussion"),
		}},
		{"Product roadmap 2027", []map[string]any{
			scover("Product roadmap 2027", "What we ship, and why", "Product and Engineering", "Frame the year in three themes."),
			sbullets("Themes", []string{"Faster daily work: barcode, mobile and offline", "Money in one place: GST, banking and payroll", "Open and connected: imports, APIs and integrations"}, "Each theme maps to a customer pain."),
			sroadmap("Roadmap at a glance", [][2]string{{"Q1", "Barcode receiving\nMobile order entry\nGST summary v2"}, {"Q2", "Offline POS\nBank feeds\nRole-based dashboards"}, {"Q3", "Vendor portal\nPayroll v2\nAPI webhooks"}, {"Q4", "Multi-warehouse\nForecasting\nMarketplace sync"}}, "Four quarters, three items each."),
			sstats("What success looks like", [][2]string{{"-40%", "Receiving time"}, {"99.9%", "Uptime"}, {"4.6/5", "Customer rating"}}, "Targets for year end."),
			stable("Investment by theme", [][]string{{"Theme", "Engineers", "Budget (₹ lakh)", "Owner"}, {"Faster daily work", "4", "96", "Imran Sheikh"}, {"Money in one place", "3", "72", "Priya Nair"}, {"Open and connected", "2", "48", "Faisal Khan"}}, "Staffing plan."),
			sbullets("Risks", []string{"Hardware supply for scanners", "Hiring two senior engineers by March", "Regulatory change in e-invoicing"}, "Mitigations are in the project plan."),
			sbullets("How we will measure", []string{"Monthly active retailers", "Order-to-dispatch time", "Support tickets per 100 orders"}, "Dashboard link in the appendix."),
			sclosing("Let's build it", "Feedback welcome until 15 December"),
		}},
	}
}
