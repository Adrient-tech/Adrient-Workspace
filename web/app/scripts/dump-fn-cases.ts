// Writes the sample cases for the browser test: npx vite-node scripts/dump-fn-cases.ts out.json
import { writeFileSync } from "node:fs";
import { sampleCases, FIXTURE_CELLS, FORMULA_COL } from "../src/pages/sheets/functionSamples";
writeFileSync(process.argv[2], JSON.stringify({ cases: sampleCases(), fixture: FIXTURE_CELLS, formulaCol: FORMULA_COL }));
