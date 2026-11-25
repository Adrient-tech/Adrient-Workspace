package sheets

import "time"

// Wave 4 additions: a few Excel 365 / Google Sheets functions the engine lacked.
//
//	PERCENTOF(subset, all)             sum(subset) / sum(all)
//	ISDATE(value)                      TRUE for a plausible date serial or date-like text
//	EPOCHTODATE(seconds[, unit])       Unix time to a date-time serial (unit 1=s, 2=ms, 3=us)
//	ARRAY_CONSTRAIN(range, rows, cols) the top-left rows x cols block
func init() {
	registerFunc("PERCENTOF", func(c *callCtx) value {
		if c.nargs() != 2 {
			return errNA
		}
		sum := func(i int) (float64, bool) {
			total := 0.0
			for _, v := range flattenArgs(c.args[i : i+1]) {
				if v.isErr() {
					return 0, false
				}
				if v.kind == kindNum {
					total += v.num
				}
			}
			return total, true
		}
		a, ok1 := sum(0)
		b, ok2 := sum(1)
		if !ok1 || !ok2 {
			return errValue
		}
		if b == 0 {
			return value{kind: kindErr, str: "#DIV/0!"}
		}
		return numVal(a / b)
	})
	registerFunc("ISDATE", func(c *callCtx) value {
		if c.nargs() != 1 {
			return errNA
		}
		v := c.scalar(0)
		switch v.kind {
		case kindNum:
			return boolVal(v.num >= 1 && v.num < 2958466)
		case kindStr:
			for _, layout := range []string{"2006-01-02", "2006/01/02", "1/2/2006", "2 Jan 2006", "Jan 2, 2006", "2006-01-02 15:04:05"} {
				if _, err := time.Parse(layout, v.str); err == nil {
					return boolVal(true)
				}
			}
		}
		return boolVal(false)
	})
	registerFunc("EPOCHTODATE", func(c *callCtx) value {
		if c.nargs() < 1 || c.nargs() > 2 {
			return errNA
		}
		n, ok := c.num(0)
		if !ok {
			return errValue
		}
		unit, ok := c.numOr(1, 1)
		if !ok {
			return errValue
		}
		var sec float64
		switch int(unit) {
		case 1:
			sec = n
		case 2:
			sec = n / 1e3
		case 3:
			sec = n / 1e6
		default:
			return value{kind: kindErr, str: "#NUM!"}
		}
		return numVal(timeToSerial(time.Unix(0, int64(sec*1e9)).UTC()))
	})
	registerFunc("ARRAY_CONSTRAIN", func(c *callCtx) value {
		if c.nargs() != 3 {
			return errNA
		}
		rv, ok := c.rangeArg(0)
		if !ok {
			return errValue
		}
		r, ok1 := c.num(1)
		cc, ok2 := c.num(2)
		if !ok1 || !ok2 || r < 1 || cc < 1 {
			return value{kind: kindErr, str: "#NUM!"}
		}
		nr, nc := min(int(r), rv.rows), min(int(cc), rv.cols)
		out := make([][]value, nr)
		for i := 0; i < nr; i++ {
			out[i] = append([]value(nil), rv.cells[i][:nc]...)
		}
		return arrayValue(out)
	})
}
