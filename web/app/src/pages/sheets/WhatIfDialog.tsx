/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet API is loosely typed. */
import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  FormControl,
  FormHelperText,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Stack,
  Tab,
  TabList,
  TabPanel,
  Table,
  Tabs,
  Typography,
} from "@mui/joy";
import DeleteIcon from "@mui/icons-material/Delete";
import {
  dataTable2Grid,
  dataTableGrid,
  isCellRef,
  parseAssignments,
  parseCellList,
  parseValueList,
  type DataTableResult,
  type Scenario,
  type ScenarioResult,
  type SolverConstraint,
  type SolverRequestUI,
  type SolverResult,
} from "./whatIf";

export type WhatIfTab = "datatable" | "scenarios" | "solver";

export interface WhatIfDialogProps {
  open: boolean;
  tab: WhatIfTab;
  onClose: () => void;
  /** Scenarios saved with the workbook. */
  scenarios: Scenario[];
  onScenariosChange: (s: Scenario[]) => void;
  runDataTable: (req: Record<string, unknown>) => Promise<DataTableResult>;
  runScenarios: (req: Record<string, unknown>) => Promise<ScenarioResult>;
  runSolver: (req: SolverRequestUI) => Promise<SolverResult>;
  /** Writes a block of values at the active cell (one undo step). */
  insertBlock: (rows: (string | number | boolean | null)[][]) => void;
  /** Writes cell → value (Solver "Keep solution", scenario "Show"). */
  setValues: (values: Record<string, number>) => void;
}

const fmt = (v: unknown) => (typeof v === "number" ? Number(v.toPrecision(10)).toLocaleString(undefined, { maximumFractionDigits: 6 }) : String(v ?? ""));

export function WhatIfDialog(p: WhatIfDialogProps) {
  const [tab, setTab] = useState<WhatIfTab>(p.tab);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (p.open) {
      setTab(p.tab);
      setError("");
    }
  }, [p.open, p.tab]);
  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message || "Failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={p.open} onClose={p.onClose}>
      <ModalDialog sx={{ width: 640, maxWidth: "96vw", maxHeight: "90vh", overflow: "auto" }} aria-labelledby="whatif-title">
        <ModalClose />
        <Typography id="whatif-title" level="title-lg">
          What-if analysis
        </Typography>
        <Tabs value={tab} onChange={(_, v) => setTab(v as WhatIfTab)}>
          <TabList>
            <Tab value="datatable">Data table</Tab>
            <Tab value="scenarios">Scenario manager</Tab>
            <Tab value="solver">Solver-lite</Tab>
          </TabList>
          <TabPanel value="datatable">
            <DataTablePanel {...p} busy={busy} guard={guard} />
          </TabPanel>
          <TabPanel value="scenarios">
            <ScenarioPanel {...p} busy={busy} guard={guard} />
          </TabPanel>
          <TabPanel value="solver">
            <SolverPanel {...p} busy={busy} guard={guard} />
          </TabPanel>
        </Tabs>
        {error && <Alert color="danger">{error}</Alert>}
      </ModalDialog>
    </Modal>
  );
}

type PanelProps = WhatIfDialogProps & { busy: boolean; guard: (fn: () => Promise<void>) => Promise<void> };

function ResultTable({ rows }: { rows: (string | number | boolean | null)[][] }) {
  return (
    <Table size="sm" borderAxis="both" sx={{ mt: 1, "& td, & th": { textAlign: "right" } }} data-testid="whatif-result">
      <tbody>
        {rows.map((r, i) => (
          <tr key={i}>
            {r.map((v, j) => (i === 0 || j === 0 ? <th key={j}>{fmt(v)}</th> : <td key={j}>{fmt(v)}</td>))}
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function DataTablePanel({ runDataTable, insertBlock, busy, guard }: PanelProps) {
  const [mode, setMode] = useState<"one" | "two">("one");
  const [input, setInput] = useState("");
  const [values, setValues] = useState("");
  const [formulas, setFormulas] = useState("");
  const [colInput, setColInput] = useState("");
  const [colValues, setColValues] = useState("");
  const [grid, setGrid] = useState<(string | number | boolean | null)[][] | null>(null);
  const vals = parseValueList(values);
  const cvals = parseValueList(colValues);
  const fs = parseCellList(formulas);
  const valid = isCellRef(input) && !!vals && !!fs && (mode === "one" || (isCellRef(colInput) && !!cvals));
  const run = () =>
    guard(async () => {
      if (!valid) return;
      if (mode === "one") {
        const res = await runDataTable({ inputCell: input.trim(), values: vals, formulas: fs });
        setGrid(dataTableGrid(input.trim().toUpperCase(), fs!, vals!, res));
      } else {
        const res = await runDataTable({ rowInput: input.trim(), colInput: colInput.trim(), rowValues: vals, colValues: cvals, formulas: fs!.slice(0, 1) });
        setGrid(dataTable2Grid(fs![0], vals!, cvals!, res));
      }
    });
  return (
    <Stack spacing={1.25}>
      <Select size="sm" value={mode} onChange={(_, v) => v && setMode(v)} aria-label="Data table type">
        <Option value="one">One variable</Option>
        <Option value="two">Two variables</Option>
      </Select>
      <Stack direction="row" spacing={1}>
        <FormControl sx={{ flex: 1 }}>
          <FormLabel>{mode === "one" ? "Input cell" : "Row input cell"}</FormLabel>
          <Input size="sm" value={input} onChange={(e) => setInput(e.target.value)} placeholder="B1" />
        </FormControl>
        <FormControl sx={{ flex: 2 }}>
          <FormLabel>Values</FormLabel>
          <Input size="sm" value={values} onChange={(e) => setValues(e.target.value)} placeholder="0.05, 0.06, 0.07 or 0:0.01:0.1" />
        </FormControl>
      </Stack>
      {mode === "two" && (
        <Stack direction="row" spacing={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Column input cell</FormLabel>
            <Input size="sm" value={colInput} onChange={(e) => setColInput(e.target.value)} placeholder="B2" />
          </FormControl>
          <FormControl sx={{ flex: 2 }}>
            <FormLabel>Column values</FormLabel>
            <Input size="sm" value={colValues} onChange={(e) => setColValues(e.target.value)} placeholder="12, 24, 36" />
          </FormControl>
        </Stack>
      )}
      <FormControl>
        <FormLabel>{mode === "one" ? "Result cells (formulas)" : "Result cell (formula)"}</FormLabel>
        <Input size="sm" value={formulas} onChange={(e) => setFormulas(e.target.value)} placeholder="C1, D1" />
        <FormHelperText>Every value is tried on a copy of the workbook; the sheet is only changed when you insert the table.</FormHelperText>
      </FormControl>
      <Stack direction="row" spacing={1}>
        <Button size="sm" disabled={!valid || busy} loading={busy} onClick={run}>
          Calculate
        </Button>
        {grid && (
          <Button size="sm" variant="outlined" onClick={() => insertBlock(grid)}>
            Insert at selected cell
          </Button>
        )}
      </Stack>
      {grid && <ResultTable rows={grid} />}
    </Stack>
  );
}

function ScenarioPanel({ scenarios, onScenariosChange, runScenarios, setValues, insertBlock, busy, guard }: PanelProps) {
  const [name, setName] = useState("");
  const [assign, setAssign] = useState("");
  const [results, setResults] = useState("");
  const [summary, setSummary] = useState<ScenarioResult | null>(null);
  const parsed = parseAssignments(assign);
  const resCells = parseCellList(results);
  const summaryGrid = summary && resCells ? [["Scenario", ...resCells], ...summary.rows.map((r) => [r.name, ...r.results])] : null;
  return (
    <Stack spacing={1.25}>
      {scenarios.length === 0 ? (
        <Typography level="body-sm">No scenarios yet. Add one below; scenarios are saved with the workbook.</Typography>
      ) : (
        <Table size="sm" data-testid="scenario-list">
          <tbody>
            {scenarios.map((s, i) => (
              <tr key={i}>
                <td style={{ width: "30%" }}>{s.name}</td>
                <td>
                  {Object.entries(s.values)
                    .map(([k, v]) => `${k}=${v}`)
                    .join(", ")}
                </td>
                <td style={{ width: 120, textAlign: "right" }}>
                  <Button size="sm" variant="plain" onClick={() => setValues(s.values)}>
                    Show
                  </Button>
                  <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete scenario ${s.name}`} onClick={() => onScenariosChange(scenarios.filter((_, j) => j !== i))}>
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      <Stack direction="row" spacing={1} alignItems="flex-end">
        <FormControl sx={{ flex: 1 }}>
          <FormLabel>Scenario name</FormLabel>
          <Input size="sm" value={name} onChange={(e) => setName(e.target.value)} placeholder="Best case" />
        </FormControl>
        <FormControl sx={{ flex: 2 }}>
          <FormLabel>Changing cells</FormLabel>
          <Input size="sm" value={assign} onChange={(e) => setAssign(e.target.value)} placeholder="B2=1500, B3=400" />
        </FormControl>
        <Button
          size="sm"
          variant="outlined"
          disabled={!name.trim() || !parsed}
          onClick={() => {
            onScenariosChange([...scenarios.filter((s) => s.name !== name.trim()), { name: name.trim(), values: parsed! }]);
            setName("");
            setAssign("");
          }}
        >
          Add
        </Button>
      </Stack>
      <FormControl>
        <FormLabel>Result cells for the summary</FormLabel>
        <Input size="sm" value={results} onChange={(e) => setResults(e.target.value)} placeholder="E6" />
      </FormControl>
      <Stack direction="row" spacing={1}>
        <Button
          size="sm"
          disabled={!resCells || !scenarios.length || busy}
          loading={busy}
          onClick={() => guard(async () => setSummary(await runScenarios({ scenarios, results: resCells })))}
        >
          Summary
        </Button>
        {summaryGrid && (
          <Button size="sm" variant="outlined" onClick={() => insertBlock(summaryGrid)}>
            Insert summary at selected cell
          </Button>
        )}
      </Stack>
      {summaryGrid && <ResultTable rows={summaryGrid} />}
    </Stack>
  );
}

function SolverPanel({ runSolver, setValues, busy, guard }: PanelProps) {
  const [objective, setObjective] = useState("");
  const [goal, setGoal] = useState<"max" | "min" | "value">("max");
  const [target, setTarget] = useState("");
  const [vars, setVars] = useState("");
  const [lower, setLower] = useState("");
  const [cons, setCons] = useState<SolverConstraint[]>([]);
  const [cCell, setCCell] = useState("");
  const [cOp, setCOp] = useState<SolverConstraint["op"]>("<=");
  const [cVal, setCVal] = useState("");
  const [res, setRes] = useState<SolverResult | null>(null);
  const varList = parseCellList(vars);
  const lo = lower.trim() === "" ? undefined : Number(lower);
  const valid = isCellRef(objective) && !!varList && varList.length <= 16 && (goal !== "value" || Number.isFinite(Number(target))) && (lo === undefined || Number.isFinite(lo));
  return (
    <Stack spacing={1.25}>
      <Alert size="sm" color="neutral" variant="soft">
        Solver-lite: one objective, up to 16 changing cells, constraints of the form cell ≤ / ≥ / = number. It runs a local search (penalty Nelder-Mead) from the current values, so it can stop at a local optimum and has no integer constraints.
      </Alert>
      <Stack direction="row" spacing={1}>
        <FormControl sx={{ flex: 1 }}>
          <FormLabel>Objective cell</FormLabel>
          <Input size="sm" value={objective} onChange={(e) => setObjective(e.target.value)} placeholder="E6" />
        </FormControl>
        <FormControl sx={{ flex: 1 }}>
          <FormLabel>To</FormLabel>
          <Select size="sm" value={goal} onChange={(_, v) => v && setGoal(v)} aria-label="Goal">
            <Option value="max">Max</Option>
            <Option value="min">Min</Option>
            <Option value="value">Value of</Option>
          </Select>
        </FormControl>
        {goal === "value" && (
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Value</FormLabel>
            <Input size="sm" value={target} onChange={(e) => setTarget(e.target.value)} />
          </FormControl>
        )}
      </Stack>
      <Stack direction="row" spacing={1}>
        <FormControl sx={{ flex: 2 }}>
          <FormLabel>By changing cells</FormLabel>
          <Input size="sm" value={vars} onChange={(e) => setVars(e.target.value)} placeholder="B2:B4" />
        </FormControl>
        <FormControl sx={{ flex: 1 }}>
          <FormLabel>Lower bound (all)</FormLabel>
          <Input size="sm" value={lower} onChange={(e) => setLower(e.target.value)} placeholder="none" />
        </FormControl>
      </Stack>
      <FormLabel>Constraints</FormLabel>
      {cons.map((c, i) => (
        <Stack key={i} direction="row" spacing={1} alignItems="center">
          <Typography level="body-sm" sx={{ flex: 1 }}>
            {c.cell} {c.op} {c.value}
          </Typography>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Remove constraint ${i + 1}`} onClick={() => setCons(cons.filter((_, j) => j !== i))}>
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Stack>
      ))}
      <Stack direction="row" spacing={1}>
        <Input size="sm" value={cCell} onChange={(e) => setCCell(e.target.value)} placeholder="Cell" sx={{ flex: 1 }} slotProps={{ input: { "aria-label": "Constraint cell" } }} />
        <Select size="sm" value={cOp} onChange={(_, v) => v && setCOp(v)} aria-label="Constraint operator">
          <Option value="<=">≤</Option>
          <Option value=">=">≥</Option>
          <Option value="=">=</Option>
        </Select>
        <Input size="sm" value={cVal} onChange={(e) => setCVal(e.target.value)} placeholder="Value" sx={{ flex: 1 }} slotProps={{ input: { "aria-label": "Constraint value" } }} />
        <Button
          size="sm"
          variant="outlined"
          disabled={!isCellRef(cCell) || cVal.trim() === "" || !Number.isFinite(Number(cVal))}
          onClick={() => {
            setCons([...cons, { cell: cCell.trim().toUpperCase(), op: cOp, value: Number(cVal) }]);
            setCCell("");
            setCVal("");
          }}
        >
          Add
        </Button>
      </Stack>
      <Stack direction="row" spacing={1}>
        <Button
          size="sm"
          disabled={!valid || busy}
          loading={busy}
          onClick={() =>
            guard(async () =>
              setRes(
                await runSolver({
                  objective: objective.trim(),
                  goal,
                  target: goal === "value" ? Number(target) : undefined,
                  variables: varList!.map((cell) => ({ cell, min: lo })),
                  constraints: cons,
                }),
              ),
            )
          }
        >
          Solve
        </Button>
        {res && (
          <Button size="sm" variant="outlined" onClick={() => setValues(res.values)}>
            Keep solution
          </Button>
        )}
      </Stack>
      {res && (
        <Box data-testid="solver-result">
          <Alert size="sm" color={res.feasible ? "success" : "warning"}>
            {res.feasible ? "Solution found; all constraints hold." : `No feasible point found. Broken: ${(res.violations ?? []).join(", ")}`} Objective: {fmt(res.objective)} ({res.evaluations} evaluations)
          </Alert>
          <ResultTable rows={[["Cell", "Value"], ...Object.entries(res.values)]} />
        </Box>
      )}
    </Stack>
  );
}
