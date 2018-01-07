package main

const CellTypeFormula int8 = 0
const CellTypeInteger int8 = 1
const CellTypeFloat int8 = 2

type cell struct {
	CellType    int8
	DataFloat   float64
	DataInteger int32
	DataString  string
}
