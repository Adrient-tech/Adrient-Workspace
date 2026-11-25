/**
 * Normalises a date from a CSV export to YYYY-MM-DD, or "" when it is empty,
 * invalid or ambiguous. 05/06/1990 could be 5 June or 6 May, so it is not guessed.
 */
export function normDate(raw: string): string {
  const s = raw.trim();
  if (!s || /^0+[/.-]0+/.test(s)) return "";
  let y = 0, m = 0, d = 0;
  let r = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(s);
  if (r) {
    y = +r[1]; m = +r[2]; d = +r[3];
  } else if ((r = /^(\d{1,2})[-/.](\d{1,2})[-/.](\d{4})$/.exec(s))) {
    const a = +r[1], b = +r[2];
    y = +r[3];
    if (a > 12 && b <= 12) { d = a; m = b; }
    else if (b > 12 && a <= 12) { m = a; d = b; }
    else if (a === b) { m = a; d = b; }
    else return "";
  } else return "";
  const dt = new Date(Date.UTC(y, m - 1, d));
  if (y < 1900 || y > 2100 || dt.getUTCFullYear() !== y || dt.getUTCMonth() !== m - 1 || dt.getUTCDate() !== d) return "";
  return `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
}

