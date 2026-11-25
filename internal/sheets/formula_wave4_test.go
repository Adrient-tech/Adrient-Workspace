package sheets

import "testing"

func TestWave4PercentOf(t *testing.T) {
	cells := []FsCellData{cell(0, 0, 1), cell(1, 0, 3), cell(2, 0, 6)}
	mustNum(t, eval(t, `PERCENTOF(A1:A2,A1:A3)`, cells...), 0.4)
	mustErr(t, eval(t, `PERCENTOF(A1,0)`, cells...), "#DIV/0!")
}

func TestWave4IsDate(t *testing.T) {
	mustBool(t, eval(t, `ISDATE("2026-03-15")`), true)
	mustBool(t, eval(t, `ISDATE("hello")`), false)
	mustBool(t, eval(t, `ISDATE(45000)`), true)
	mustBool(t, eval(t, `ISDATE(0)`), false)
}

func TestWave4EpochToDate(t *testing.T) {
	mustNum(t, eval(t, `EPOCHTODATE(0)`), 25569)
	mustNum(t, eval(t, `EPOCHTODATE(86400000,2)`), 25570)
	mustErr(t, eval(t, `EPOCHTODATE(1,9)`), "#NUM!")
}

func TestWave4ArrayConstrain(t *testing.T) {
	cells := []FsCellData{cell(0, 0, 1), cell(0, 1, 2), cell(1, 0, 3), cell(1, 1, 4)}
	wantGrid(t, eval(t, `ARRAY_CONSTRAIN(A1:B2,1,2)`, cells...), [][]interface{}{{1.0, 2.0}})
	wantGrid(t, eval(t, `ARRAY_CONSTRAIN(A1:B2,5,1)`, cells...), [][]interface{}{{1.0}, {3.0}})
}
