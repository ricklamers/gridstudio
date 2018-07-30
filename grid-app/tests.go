package main

import (
	"fmt"
	"strconv"
)

var testCount int
var testFailCount int
var debug bool

func runTests() {

	debug = false
	debug = true

	testCount = 0
	testFailCount = 0

	// initialize the datastructure for the matrix
	columnCount := 10
	rowCount := 10

	sheetSizes := []SheetSize{SheetSize{RowCount: rowCount, ColumnCount: columnCount}, SheetSize{RowCount: rowCount, ColumnCount: columnCount}}

	// For now make this a two way mapping for ordered loops and O(1) access times -- aware of redundancy of state which could cause problems
	sheetNames := make(map[string]int8)
	sheetNames["Sheet1"] = 0
	sheetNames["Sheet2"] = 1

	sheetList := []string{"Sheet1", "Sheet2"}

	grid := Grid{Data: make(map[string]*DynamicValue), PerformanceCounting: make(map[string]int), DirtyCells: make(map[string]bool), ActiveSheet: 0, SheetNames: sheetNames, SheetList: sheetList, SheetSizes: sheetSizes}

	if !debug {

		testFormula("((A1 + A10) - (1))", true)
		testFormula("A10 + 0.2", true)
		testFormula("A10 + A0.2", false)
		testFormula("0.1 + 0.2 * 0.3 / 0.1", true)
		testFormula("0.1 + 0.2 * 0.3 / 0.1A", false)
		testFormula("A1 * A20 + 0.2 - \"abc\"", true)
		testFormula("A1 * A + 0.2 - \"abc\"", false)
		testFormula("SUM(A1:10, 10)", false)
		testFormula("A1 ^^ 10", false)
		testFormula("A1 ^ 10", true)
		testFormula("SUM(A1 ^ 10, 1, 1.05))", false)
		testFormula("SUM(A1 ^ 10, 1, 1.05)", true)
		testFormula("SUM(A1 ^ 10, 1, A1.05)", false)
		testFormula("A.01", false)
		testFormula("A10+0.01", true)
		testFormula("A10+A", false)
		testFormula("$A$10+$A1+A$2", true)

		// dollar fixing references
		testFormula("$$A1", false)

		testFormula("Sheet!$A$$1", false)
		testFormula("Sheet!$A$1", true)

		testFormula("Sheet1!$A$$1", false)
		testFormula("Sheet1!$A$1", true)
		testFormula("VLOOKUP(Sheet1!$A$1)", true)
		testFormula("VLOOKUP(A1,Sheet1!$A$1,1)", true)
		testFormula("VLOOKUP(A1,Sheet1!$A$1:$A$1,1)", true)
		testFormula("SUM(Sheet1!$A$1:$A$1)", true)
		testFormula("SUM($A$1:$A$1)", true)
		testFormula("SUM($A$$1:$A$1)", false)
		testFormula("SUM($$A$1:$A$1)", false)
		testFormula("SUM($A$$1:$A$$1)", false)
		testFormula("Sheet1!$A1", true)
		testFormula("Sheet1!A$1", true)
		testFormula("Sheet1!A1", true)
		testFormula("'Sheet 1'!$A$1", true)
		testFormula("'Sheet 1'!$A1", true)
		testFormula("'Sheet 1'!A$1", true)
		testFormula("'Sheet 1'!A1", true)
		testFormula("'Sheet1'!$A$1", true)
		testFormula("'Sheet1'!$A1", true)
		testFormula("'Sheet1'!A$1", true)
		testFormula("'Sheet1'!A1", true)
		testFormula("$A$1", true)
		testFormula("A$1", true)
		testFormula("$A1", true)
		testFormula("A1", true)

		testFormula("A$$1", false)
		testFormula("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)
		testFormula("0!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", false)
		testFormula("10+-10/10--10", true)
		testFormula("", true)
		testFormula("10+-10/10---10", false)
		testFormula("A10+(-10)", true)
		testFormula("A10+(--10)", false)
		testFormula("A1*-5", true)
		testFormula("*5", false)
		testFormula("$B$1+CEIL(RAND()*1000)", true)

		referenceCount := findReferenceStrings("\"abudh\\\"ijdso\"")
		testBool(len(referenceCount) == 0, true)

		referenceCount2 := findReferenceStrings("\"Then there's a pair of us -- don't tell!\"")
		testBool(len(referenceCount2) == 0, true)

		referenceMap := make(map[string]string)
		referenceMap["Sheet2!A1"] = "Sheet2!A2"
		testString(replaceReferenceStringInFormula("Sheet2!A1", referenceMap), "Sheet2!A2")

		referenceMap = make(map[string]string)
		referenceMap["'Sheet 2'!A1"] = "'Sheet 2'!A2"
		testString(replaceReferenceStringInFormula("'Sheet 2'!A1", referenceMap), "'Sheet 2'!A2")

		testString(findReferenceStrings("'Sheet  3'!A1")[0], "'Sheet  3'!A1")

		someReferences := findReferenceStrings("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100")
		testString(someReferences[0], "'0'!A5")
		testString(someReferences[1], "'Blad 2'!A10")
		testString(someReferences[2], "A10")
		testString(someReferences[3], "Blad15!$A$100")

		fmt.Println(strconv.Itoa(testCount-testFailCount) + "/" + strconv.Itoa(testCount) + " tests succeeded. Failed: " + strconv.Itoa(testFailCount))

	} else {
		// space to run single test cases
		// testFormula("$B$1+CEIL(RAND()*1000)", true)
		// testFormula("'Sheet1'!$A$1", true)
		testFormula("\"Then there's a pair of us -- don't tell!\"", true)

		sampleDv := makeDv("\"Then there's a pair of us -- don't tell!\"")

		resultDv := parse(sampleDv, &grid, Reference{String: "A1", SheetIndex: 0})

		fmt.Println(resultDv.DataString)

	}

}

func testString(result string, expected string) {
	testCount++
	if result != expected {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " failed] Expected: " + expected + ", got: " + result)
		testFailCount++
	} else {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " succeeded] Got: " + result)
	}
}

func testBool(result bool, expected bool) {
	testCount++
	if result != expected {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " failed] Expected: " + strconv.FormatBool(expected) + ", got: " + strconv.FormatBool(result))
		testFailCount++
	} else {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " succeeded] Expected: " + strconv.FormatBool(expected) + ", got: " + strconv.FormatBool(result))
	}
}

func testFormula(formula string, expected bool) {
	testCount++
	result := isValidFormula(formula)
	if result != expected {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " failed] Expected: " + strconv.FormatBool(expected) + ", got: " + strconv.FormatBool(result) + " formula: " + formula)
		testFailCount++
	} else {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " succeeded] formula: " + formula)
	}
}
