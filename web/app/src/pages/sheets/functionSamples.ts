// Sample formulas for every function in functionCatalog.ts, used by the
// browser test (scripts/sheet-fn-probe.cjs) and its vitest guard. The sample
// workbook is FIXTURE_CELLS: small ranges that suit most signatures.
import { functionCatalog, type FunctionInfo } from "./functionCatalog";

/** Cells of the sample workbook, [row, col, value] (zero based). */
export const FIXTURE_CELLS: [number, number, string | number | boolean][] = (() => {
  const out: [number, number, string | number | boolean][] = [];
  const A = [10, 20, 30, 40, 50];
  const B = [1, 2, 3, 4, 5];
  const C = ["apple", "pear", "kiwi", "plum", "fig"];
  const D = [45000, 45031, 45061, 45092, 45122];
  const E = [0.1, 0.2, 0.3, 0.4, 0.5];
  const F = [true, false, true, false, true];
  const G = [2, 4, 5, 4, 5];
  const cols = [A, B, C, D, E, F, G];
  cols.forEach((col, c) => col.forEach((v, r) => out.push([r, c, v])));
  // Database table J1:L6 (header row + 5 records) and criteria N1:N2.
  const head = ["Name", "Qty", "Price"];
  head.forEach((h, c) => out.push([0, 9 + c, h]));
  const rows: [string, number, number][] = [["apple", 10, 1.5], ["pear", 20, 2], ["kiwi", 30, 2.5], ["plum", 40, 3], ["fig", 50, 3.5]];
  rows.forEach((row, i) => row.forEach((v, c) => out.push([1 + i, 9 + c, v])));
  out.push([0, 13, "Qty"], [1, 13, ">15"], [3, 13, "Name"], [4, 13, "kiwi"]);
  // Pivot-ish text matrix, a roman numeral and a complex number.
  out.push([7, 0, "MCMXC"], [7, 1, "3+4i"], [7, 2, "hello world"], [7, 3, "a,b,c"], [7, 4, "x1y22z333"], [9, 0, "=SUM(A1:A5)"]);
  return out;
})();

/** First column of the sample formulas (column P, zero based 15). */
export const FORMULA_COL = 15;

const SETTLE = "DATE(2024,1,15)";
const MATURE = "DATE(2029,1,15)";

// Full-formula overrides: functions whose arguments depend on each other.
const OVERRIDE: Record<string, string> = {
  // Bonds with odd periods: issue < settlement < first coupon < maturity.
  ODDFPRICE: `ODDFPRICE(DATE(2024,2,15),DATE(2029,1,15),DATE(2024,1,1),DATE(2024,7,15),0.05,0.06,100,2)`,
  ODDFYIELD: `ODDFYIELD(DATE(2024,2,15),DATE(2029,1,15),DATE(2024,1,1),DATE(2024,7,15),0.05,98,100,2)`,
  ODDLPRICE: `ODDLPRICE(DATE(2028,12,1),DATE(2029,1,15),DATE(2028,7,15),0.05,0.06,100,2)`,
  ODDLYIELD: `ODDLYIELD(DATE(2028,12,1),DATE(2029,1,15),DATE(2028,7,15),0.05,99.5,100,2)`,
  ACCRINT: `ACCRINT(DATE(2023,1,15),DATE(2023,7,15),DATE(2024,1,15),0.05,1000,2)`,
  YEARFRAC: `YEARFRAC(DATE(2024,1,1),DATE(2024,7,1))`,
  // Functions that need data shaped as a range or a callable.
  IRR: `IRR({-100,30,40,50,60})`,
  MIRR: `MIRR({-100,30,40,50,60},0.1,0.12)`,
  XIRR: `XIRR({-100,30,40,50,60},{45000,45200,45400,45600,45800})`,
  XNPV: `XNPV(0.1,{-100,30,40,50,60},{45000,45200,45400,45600,45800})`,
  RATE: `RATE(10,-100,800)`,
  CHOOSE: `CHOOSE(2,"a","b","c")`,
  CHOOSECOLS: `CHOOSECOLS(A1:C3,1,3)`,
  CHOOSEROWS: `CHOOSEROWS(A1:C3,1,3)`,
  IFS: `IFS(A1>5,"big",TRUE,"small")`,
  SWITCH: `SWITCH(B1,1,"one",2,"two","other")`,
  XOR: `XOR(TRUE,FALSE)`,
  LAMBDA: `LAMBDA(x,x+1)(5)`,
  LET: `LET(x,2,y,3,x*y)`,
  MAP: `MAP(A1:A3,LAMBDA(v,v*2))`,
  REDUCE: `REDUCE(0,A1:A3,LAMBDA(acc,v,acc+v))`,
  SCAN: `SCAN(0,A1:A3,LAMBDA(acc,v,acc+v))`,
  BYROW: `BYROW(A1:B3,LAMBDA(r,SUM(r)))`,
  BYCOL: `BYCOL(A1:B3,LAMBDA(c,SUM(c)))`,
  MAKEARRAY: `MAKEARRAY(2,2,LAMBDA(r,c,r*c))`,
  ISOMITTED: `LAMBDA(x,ISOMITTED(x))()`,
  FILTER: `FILTER(A1:A5,B1:B5>2)`,
  SORTBY: `SORTBY(C1:C5,A1:A5,-1)`,
  GROUPBY: `GROUPBY(C1:C5,A1:A5,LAMBDA(v,SUM(v)))`,
  PIVOTBY: `PIVOTBY(C1:C5,F1:F5,A1:A5,LAMBDA(v,SUM(v)))`,
  "BETA.DIST": `BETA.DIST(0.5,2,3,TRUE)`,
  BETADIST: `BETADIST(0.5,2,3)`,
  CUMIPMT: `CUMIPMT(0.09/12,360,125000,1,5,0)`,
  CUMPRINC: `CUMPRINC(0.09/12,360,125000,1,5,0)`,
  DGET: `DGET(J1:L6,"Qty",N4:N5)`,
  "EXPON.DIST": `EXPON.DIST(1,2,TRUE)`,
  EXPONDIST: `EXPONDIST(1,2,TRUE)`,
  EXPAND: `EXPAND(A1:A2,4)`,
  FISHER: `FISHER(0.5)`,
  PERCENTILE: `PERCENTILE(A1:A5,0.5)`,
  "PERCENTILE.EXC": `PERCENTILE.EXC(A1:A5,0.5)`,
  "PERCENTILE.INC": `PERCENTILE.INC(A1:A5,0.5)`,
  PERCENTRANK: `PERCENTRANK(A1:A5,30)`,
  "PERCENTRANK.EXC": `PERCENTRANK.EXC(A1:A5,30)`,
  "PERCENTRANK.INC": `PERCENTRANK.INC(A1:A5,30)`,
  PROB: `PROB(B1:B5,{0.1;0.2;0.3;0.2;0.2},2,4)`,
  PDURATION: `PDURATION(0.05,1000,2000)`,
  RANK: `RANK(30,A1:A5)`,
  "RANK.AVG": `RANK.AVG(30,A1:A5)`,
  "RANK.EQ": `RANK.EQ(30,A1:A5)`,
  TBILLEQ: `TBILLEQ(DATE(2024,1,15),DATE(2024,6,15),0.05)`,
  TBILLPRICE: `TBILLPRICE(DATE(2024,1,15),DATE(2024,6,15),0.05)`,
  TBILLYIELD: `TBILLYIELD(DATE(2024,1,15),DATE(2024,6,15),98)`,
  "T.TEST": `T.TEST(A1:A5,G1:G5,2,2)`,
  TTEST: `TTEST(A1:A5,G1:G5,2,2)`,
  MODE: `MODE(G1:G5)`,
  "MODE.SNGL": `MODE.SNGL(G1:G5)`,
  "MODE.MULT": `MODE.MULT(G1:G5)`,
  FORMULATEXT: `FORMULATEXT(A10)`,
  PERCENTOF: `PERCENTOF(A1:A2,A1:A5)`,
  TEXTSPLIT: `TEXTSPLIT(D8,",")`,
  TEXTBEFORE: `TEXTBEFORE(D8,",")`,
  TEXTAFTER: `TEXTAFTER(D8,",")`,
  REGEXEXTRACT: `REGEXEXTRACT(E8,"[0-9]+")`,
  REGEXREPLACE: `REGEXREPLACE(E8,"[0-9]+","#")`,
  REGEXTEST: `REGEXTEST(E8,"[0-9]+")`,
  QUERY: `QUERY(J1:L6,"select A, C where B > 15")`,
  INDIRECT: `INDIRECT("A1")`,
  ADDRESS: `ADDRESS(2,3)`,
  OFFSET: `OFFSET(A1,1,1)`,
  HYPERLINK: `HYPERLINK("https://example.com","site")`,
  SPARKLINE: `SPARKLINE(A1:A5)`,
  GETPIVOTDATA: `GETPIVOTDATA("Qty",J1:L6)`,
  CELL: `CELL("address",A1)`,
  INFO: `INFO("recalc")`,
  "ERROR.TYPE": `ERROR.TYPE(NA())`,
  CONVERT: `CONVERT(1,"m","ft")`,
  GCD: `GCD(12,18)`,
  LCM: `LCM(4,6)`,
  MULTINOMIAL: `MULTINOMIAL(2,3,4)`,
  SERIESSUM: `SERIESSUM(2,0,1,{1,2,3})`,
  ROMAN: `ROMAN(1994)`,
  ARABIC: `ARABIC(A8)`,
  BASE: `BASE(255,16)`,
  DECIMAL: `DECIMAL("FF",16)`,
  AGGREGATE: `AGGREGATE(9,6,A1:A5)`,
  SUBTOTAL: `SUBTOTAL(9,A1:A5)`,
  MINVERSE: `MINVERSE({2,0;0,2})`,
  MDETERM: `MDETERM({2,0;0,2})`,
  MMULT: `MMULT({1,2;3,4},{1,0;0,1})`,
  MUNIT: `MUNIT(2)`,
  SUMX2MY2: `SUMX2MY2(A1:A5,B1:B5)`,
  SUMX2PY2: `SUMX2PY2(A1:A5,B1:B5)`,
  SUMXMY2: `SUMXMY2(A1:A5,B1:B5)`,
  FREQUENCY: `FREQUENCY(A1:A5,{20,40})`,
  RANDARRAY: `RANDARRAY(2,2)`,
  SEQUENCE: `SEQUENCE(3)`,
  TREND: `TREND(A1:A5,B1:B5,6)`,
  FORECAST: `FORECAST(6,A1:A5,B1:B5)`,
  LINEST: `LINEST(A1:A5,B1:B5)`,
  LOGEST: `LOGEST(A1:A5,B1:B5)`,
  GROWTH: `GROWTH(A1:A5,B1:B5,6)`,
  IMPOWER: `IMPOWER("3+4i",2)`,
  IMROOT: `IMROOT("3+4i",2)`,
  COMPLEX: `COMPLEX(3,4)`,
  DELTA: `DELTA(5,5)`,
  GESTEP: `GESTEP(5,4)`,
  BIN2DEC: `BIN2DEC("1010")`,
  BIN2HEX: `BIN2HEX("1010")`,
  BIN2OCT: `BIN2OCT("1010")`,
  DEC2BIN: `DEC2BIN(10)`,
  DEC2HEX: `DEC2HEX(255)`,
  DEC2OCT: `DEC2OCT(8)`,
  HEX2BIN: `HEX2BIN("F")`,
  HEX2DEC: `HEX2DEC("FF")`,
  HEX2OCT: `HEX2OCT("FF")`,
  OCT2BIN: `OCT2BIN("7")`,
  OCT2DEC: `OCT2DEC("17")`,
  OCT2HEX: `OCT2HEX("17")`,
  BITAND: `BITAND(12,10)`,
  BITOR: `BITOR(12,10)`,
  BITXOR: `BITXOR(12,10)`,
  BITLSHIFT: `BITLSHIFT(4,2)`,
  BITRSHIFT: `BITRSHIFT(16,2)`,
  ERF: `ERF(1)`,
  "ERF.PRECISE": `ERF.PRECISE(1)`,
  ERFC: `ERFC(1)`,
  "ERFC.PRECISE": `ERFC.PRECISE(1)`,
  TEXTJOIN: `TEXTJOIN(",",TRUE,C1:C3)`,
  CONCAT: `CONCAT(C1:C3)`,
  CONCATENATE: `CONCATENATE(C1,"-",C2)`,
  DATEVALUE: `DATEVALUE("2024-01-15")`,
  TIMEVALUE: `TIMEVALUE("10:30")`,
  DATEDIF: `DATEDIF(DATE(2020,1,1),DATE(2024,1,1),"Y")`,
  NUMBERVALUE: `NUMBERVALUE("1.234,5",",",".")`,
  VALUE: `VALUE("12.5")`,
  TEXT: `TEXT(1234.5,"#,##0.00")`,
  DOLLAR: `DOLLAR(1234.5)`,
  FIXED: `FIXED(1234.567,2)`,
  CHAR: `CHAR(65)`,
  CODE: `CODE("A")`,
  UNICHAR: `UNICHAR(9731)`,
  UNICODE: `UNICODE("A")`,
  T: `T(C1)`,
  N: `N(A1)`,
  NA: `NA()`,
  ISFORMULA: `ISFORMULA(A1)`,
  ISREF: `ISREF(A1)`,
  TYPE: `TYPE(A1)`,
  SHEET: `SHEET()`,
  SHEETS: `SHEETS()`,
  ROW: `ROW(A3)`,
  COLUMN: `COLUMN(C1)`,
  ROWS: `ROWS(A1:A5)`,
  COLUMNS: `COLUMNS(A1:C1)`,
  AREAS: `AREAS(A1:B2)`,
  TRANSPOSE: `TRANSPOSE(A1:B3)`,
  MATCH: `MATCH(30,A1:A5,0)`,
  XMATCH: `XMATCH(30,A1:A5)`,
  XLOOKUP: `XLOOKUP("pear",C1:C5,A1:A5)`,
  VLOOKUP: `VLOOKUP("pear",C1:G5,3,FALSE)`,
  HLOOKUP: `HLOOKUP(10,A1:E5,2,FALSE)`,
  LOOKUP: `LOOKUP(30,A1:A5,B1:B5)`,
  INDEX: `INDEX(A1:C5,2,3)`,
  GETPIVOT: `GETPIVOTDATA("Qty",J1:L6)`,
  STOCKHISTORY: `STOCKHISTORY("X",DATE(2024,1,1))`,
};

// Sample per argument name. Numbered names reuse the name without digits.
const BY_NAME: Record<string, string> = {
  number: "2", rate: "0.05", par: "1000", frequency: "2", basis: "0", issue: "DATE(2023,1,15)",
  settlement: SETTLE, maturity: MATURE, first_interest: "DATE(2023,7,15)", first_coupon: "DATE(2024,7,15)", last_interest: "DATE(2023,7,15)",
  calc_method: "TRUE", row_num: "2", column_num: "2", abs_num: "1", a: "TRUE", sheet_text: '"Sheet1"', function_num: "9", options: "6",
  ref: "A1:A5", cost: "1000", date_purchased: "DATE(2023,1,15)", first_period: "DATE(2023,12,31)", salvage: "100", period: "1", logical: "TRUE",
  text: '"Hello World"', reference: "A1:B3", input_range: "A1:A5", num_rows: "1", num_cols: "1", array_formula: "A1:A5", array: "A1:A5", format: '"0.00"',
  x_num: "1", y_num: "1", value: "A1:A5", range: "B1:B5", criteria: '">2"', average_range: "A1:A5", criteria_range: "B1:B5", radix: "16", min_length: "4",
  x: "2", n: "3", alpha: "0.05", beta: "2", cumulative: "TRUE", A: "A1:A5", B: "B1:B5", probability: "0.5", places: "4", number_s: "3", trials: "10",
  probability_s: "0.5", shift_amount: "2", lambda: "LAMBDA(x,x)", significance: "1", mode: "0", info_type: '"recalc"', actual_range: "A1:A5", expected_range: "B1:B5",
  index_num: "2", col_num: "2", number_chosen: "2", real_num: "3", i_num: "4", suffix: '"i"', standard_dev: "1", size: "3", from_unit: '"m"', to_unit: '"ft"',
  nper: "10", pv: "-1000", start_period: "1", end_period: "5", type: "0", year: "2024", month: "6", day: "15", start_date: "DATE(2024,1,1)",
  end_date: "DATE(2024,6,30)", unit: "1", date_text: '"2024-01-15"', database: "J1:L6", field: '"Qty"', serial_number: "45000", method: "0", life: "5",
  factor: "2", angle: "1", pr: "98", redemption: "100", decimals: "2", fractional_dollar: "1.02", fraction: "16", decimal_dollar: "1.125", rows: "2",
  columns: "2", coupon: "0.05", yld: "0.06", months: "2", nominal_rate: "0.05", npery: "4", timestamp: "1700000000", lower_limit: "0", upper_limit: "1",
  error_val: "NA()", pad_with: "0", include: "B1:B5>2", if_empty: '"none"', find_text: '"o"', within_text: '"Hello World"', start_num: "1", y: "2",
  no_commas: "FALSE", known_ys: "A1:A5", known_xs: "B1:B5", data_array: "A1:A5", bins_array: "{20,40}", pmt: "-100", principal: "1000", schedule: "{0.05,0.06}",
  z: "1", step: "1", data_field: '"Qty"', pivot_table: "J1:L6", item: '"apple"', row_fields: '"Name"', values: "A1:A5", function: "SUM", field_headers: "1",
  total_depth: "1", sort_order: "1", filter_array: "B1:B5>2", new_xs: "B1:B3", const: "TRUE", lookup_value: "30", table_array: "A1:C5", row_index_num: "2",
  range_lookup: "FALSE", link_location: '"https://example.com"', friendly_name: '"link"', sample_s: "2", number_sample: "5", population_s: "4",
  number_pop: "10", logical_test: "TRUE", value_if_true: '"yes"', value_if_false: '"no"', value_if_error: '"err"', value_if_na: '"na"', inumber: '"3+4i"',
  area_num: "1", ref_text: '"A1"', investment: "1000", per: "1", fv: "0", guess: "0.1", lower_value: "1", upper_value: "3", lower_inclusive: "TRUE",
  upper_inclusive: "TRUE", date: "45000", delimiter: '","', parameter: "1", calculation: '"x+1"', k: "2", num_chars: "2", num_bytes: "2", name: '"x"',
  name_value: "5", stats: "FALSE", base: "10", mean: "10", lookup_vector: "A1:A5", result_vector: "B1:B5", cols: "1", lookup_array: "A1:A5", match_type: "0",
  max_range: "A1:A5", min_range: "A1:A5", finance_rate: "0.1", reinvest_rate: "0.12", divisor: "3", multiple: "5", dimension: "2", number_f: "3",
  holidays: "D1:D2", weekend: "1", effect_rate: "0.05", decimal_separator: '"."', group_separator: '","', height: "2", width: "2", subset: "1", all: "TRUE",
  col_fields: '"Name"', row_total_depth: "1", row_sort_order: "1", col_total_depth: "1", col_sort_order: "1", power: "2", discount: "0.05", x_range: "A1:A5",
  prob_range: "E1:E5", quart: "1", data: "A1:A5", query: '"select *"', headers: "1", numerator: "7", denominator: "2", min: "1", max: "10", whole_number: "TRUE",
  bottom: "1", top: "10", order: "1", initial_value: "0", regular_expression: '"o"', replacement: '"0"', pattern: '"o"', case_sensitivity: "0", old_text: '"World"',
  new_text: '"There"', number_times: "2", form: "0", num_digits: "2", start: "1", m: "1", coefficients: "{1,2,3}", sort_index: "1", by_col: "FALSE", by_array: "A1:A5",
  display_ties_mode: "0", sort_column: "1", is_ascending: "TRUE", split_by_each: "FALSE", remove_empty_text: "TRUE", instance_num: "1", sum_range: "A1:A5",
  array_x: "A1:A5", array_y: "B1:B5", expression: "1", result: "1", default_or_value: "0", tails: "2", format_text: '"0.00"', match_mode: "0", match_end: "0",
  if_not_found: '"none"', ignore_empty: "TRUE", col_delimiter: '","', row_delimiter: '";"', hour: "10", minute: "30", second: "15", time_text: '"10:30"',
  ignore: "0", scan_by_column: "FALSE", percent: "0.5", trim_rows: "0", trim_cols: "0", exactly_once: "FALSE", no_switch: "FALSE", col_index_num: "2",
  return_type: "1", days: "10", vector: "A1:A5", wrap_count: "2", dates: "D1:D5", return_array: "A1:A5", search_mode: "1", sigma: "1", deg_freedom: "5",
  mean2: "0",
};

// Names whose domain is narrower than the default sample.
const NARROW: Record<string, string> = {
  ACOS: "0.5", ASIN: "0.5", ATANH: "0.5", ACOSH: "2", ACOTH: "2", FACT: "5", FACTDOUBLE: "5", LN: "2", LOG: "100", LOG10: "100", SQRT: "16", SQRTPI: "2",
  EVEN: "3", ODD: "3", COMBIN: "5", COMBINA: "5", PERMUT: "5", PERMUTATIONA: "5", GAMMALN: "3", "GAMMALN.PRECISE": "3", GAMMA: "3", NORMSDIST: "1", "NORM.S.DIST": "1",
};

function sampleFor(f: FunctionInfo, a: FunctionInfo["args"][number], idx: number): string {
  const base = a.name.replace(/\d+$/, "");
  const numbered = /\d+$/.test(a.name);
  if (base === "number" && NARROW[f.name] && idx === 0) return NARROW[f.name];
  if ((base === "number" || base === "value" || base === "ref" || base === "array") && numbered) {
    return /^[12]$/.test(a.name.slice(base.length)) ? (a.name.endsWith("1") ? "A1:A5" : "B1:B5") : "A1:A5";
  }
  if (base === "number" && /^(SUM|AVERAGE|COUNT|MAX|MIN|PRODUCT|STDEV|VAR|MEDIAN|MODE|LARGE|SMALL)/.test(f.name)) return "A1:A5";
  return BY_NAME[a.name] ?? BY_NAME[base] ?? "1";
}

/** The bare formula (no "=") that exercises f. */
export function sampleFormula(f: FunctionInfo): string {
  if (OVERRIDE[f.name]) return OVERRIDE[f.name];
  const args = f.args.filter((a) => !a.optional).map((a) => ({ a, i: 0 }));
  const parts = args.map(({ a }, i) => sampleFor(f, a, i));
  return `${f.name}(${parts.join(",")})`;
}

export interface SampleCase {
  name: string;
  category: string;
  formula: string;
}

export function sampleCases(): SampleCase[] {
  return functionCatalog().map((f) => ({ name: f.name, category: f.category, formula: sampleFormula(f) }));
}
