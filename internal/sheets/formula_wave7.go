package sheets

// Wave 7 additions, from an audit against the Excel and Google Sheets function lists.
//
// (Google Sheets' operator functions ADD, MINUS, DIVIDE, EQ ... are left out on purpose:
// Excel has none of them, and the parity fixtures expect #NAME? for DIVIDE.)
//
//	TO_DATE TO_PERCENT TO_DOLLARS        value converters (Google Sheets); formatting is
//	TO_PURE_NUMBER TO_TEXT               a display matter, so these return the value
//	TRIMRANGE(range, [rows], [cols])     drop blank edge rows / columns (Excel 365)
//	PHONETIC(ref)                        the text of the cells (no furigana is stored)
func init() {
	toNumber := func(c *callCtx) value {
		if c.nargs() != 1 {
			return errNA
		}
		v := c.scalar(0)
		if v.isErr() {
			return v
		}
		n, ok := v.toNum()
		if !ok {
			return errValue
		}
		return numVal(n)
	}
	registerFunc("TO_DATE", toNumber)
	registerFunc("TO_PERCENT", toNumber)
	registerFunc("TO_DOLLARS", toNumber)
	registerFunc("TO_PURE_NUMBER", toNumber)
	registerFunc("TO_TEXT", func(c *callCtx) value {
		if c.nargs() != 1 {
			return errNA
		}
		v := c.scalar(0)
		if v.isErr() {
			return v
		}
		return strVal(v.toStr())
	})
	registerFunc("PHONETIC", func(c *callCtx) value {
		if c.nargs() != 1 {
			return errNA
		}
		out := ""
		for _, v := range flattenArgs(c.args[:1]) {
			if v.isErr() {
				return v
			}
			if !v.blank {
				out += v.toStr()
			}
		}
		return strVal(out)
	})
	registerFunc("TRIMRANGE", func(c *callCtx) value {
		if c.nargs() < 1 || c.nargs() > 3 {
			return errNA
		}
		rv, ok := c.rangeArg(0)
		if !ok {
			return errValue
		}
		// 0 none, 1 leading, 2 trailing, 3 both (the default for both axes).
		rowsMode, ok1 := c.numOr(1, 3)
		colsMode, ok2 := c.numOr(2, 3)
		if !ok1 || !ok2 || rowsMode < 0 || rowsMode > 3 || colsMode < 0 || colsMode > 3 {
			return errValue
		}
		empty := func(v value) bool { return v.blank || (v.kind == kindStr && v.str == "" && !v.isErr()) }
		rowEmpty := func(r int) bool {
			for _, v := range rv.cells[r] {
				if !empty(v) {
					return false
				}
			}
			return true
		}
		colEmpty := func(cc int) bool {
			for r := 0; r < rv.rows; r++ {
				if !empty(rv.cells[r][cc]) {
					return false
				}
			}
			return true
		}
		r0, r1, c0, c1 := 0, rv.rows-1, 0, rv.cols-1
		if int(rowsMode)&1 != 0 {
			for r0 <= r1 && rowEmpty(r0) {
				r0++
			}
		}
		if int(rowsMode)&2 != 0 {
			for r1 >= r0 && rowEmpty(r1) {
				r1--
			}
		}
		if int(colsMode)&1 != 0 {
			for c0 <= c1 && colEmpty(c0) {
				c0++
			}
		}
		if int(colsMode)&2 != 0 {
			for c1 >= c0 && colEmpty(c1) {
				c1--
			}
		}
		if r0 > r1 || c0 > c1 {
			return value{kind: kindErr, str: "#CALC!"}
		}
		out := make([][]value, 0, r1-r0+1)
		for r := r0; r <= r1; r++ {
			out = append(out, append([]value(nil), rv.cells[r][c0:c1+1]...))
		}
		return arrayValue(out)
	})
}
