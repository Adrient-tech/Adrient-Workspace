package sheets

import "testing"

func TestWave7Converters(t *testing.T) {
	mustNum(t, eval(t, `TO_PURE_NUMBER("12.5")`), 12.5)
	mustNum(t, eval(t, `TO_PERCENT(0.25)`), 0.25)
	mustNum(t, eval(t, `TO_DOLLARS(7)`), 7)
	mustStr(t, eval(t, `TO_TEXT(42)`), "42")
	mustErr(t, eval(t, `TO_PURE_NUMBER("abc")`), "#VALUE!")
	mustStr(t, eval(t, `PHONETIC("ab")`), "ab")
}

func TestWave7ZTest(t *testing.T) {
	cells := []FsCellData{cell(0, 0, 3), cell(1, 0, 6), cell(2, 0, 7), cell(3, 0, 8), cell(4, 0, 6), cell(5, 0, 5), cell(6, 0, 4), cell(7, 0, 2), cell(8, 0, 1), cell(9, 0, 9)}
	a, b := eval(t, `ZTEST(A1:A10,4)`, cells...), eval(t, `Z.TEST(A1:A10,4)`, cells...)
	if a.num != b.num || a.isErr() {
		t.Fatalf("ZTEST %v != Z.TEST %v", a, b)
	}
}

func TestWave7TrimRange(t *testing.T) {
	// B2:C3 holds data inside the A1:D4 range.
	cells := []FsCellData{cell(1, 1, 1), cell(1, 2, 2), cell(2, 1, 3), cell(2, 2, 4)}
	wantGrid(t, eval(t, `TRIMRANGE(A1:D4)`, cells...), [][]interface{}{{1.0, 2.0}, {3.0, 4.0}})
	wantGrid(t, eval(t, `TRIMRANGE(A1:D4,1,0)`, cells...), [][]interface{}{{0.0, 1.0, 2.0, 0.0}, {0.0, 3.0, 4.0, 0.0}, {0.0, 0.0, 0.0, 0.0}})
}
