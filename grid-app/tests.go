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
	// debug = true

	testCount = 0
	testFailCount = 0

	if !debug {
		testBool("((A1 + A10) - (1))", true)
		testBool("A10 + 0.2", true)
		testBool("A10 + A0.2", false)
		testBool("0.1 + 0.2 * 0.3 / 0.1", true)
		testBool("0.1 + 0.2 * 0.3 / 0.1A", false)
		testBool("A1 * A20 + 0.2 - \"abc\"", true)
		testBool("A1 * A + 0.2 - \"abc\"", false)
		testBool("SUM(A1:10, 10)", false)
		testBool("A1 ^^ 10", false)
		testBool("A1 ^ 10", true)
		testBool("SUM(A1 ^ 10, 1, 1.05))", false)
		testBool("SUM(A1 ^ 10, 1, 1.05)", true)
		testBool("SUM(A1 ^ 10, 1, A1.05)", false)
		testBool("A.01", false)
		testBool("A10+0.01", true)
		testBool("A10+A", false)
		testBool("$A$10+$A1+A$2", true)

		// dollar fixing references
		testBool("$$A1", false)
		testBool("$A$1", true)
		testBool("A$1", true)
		testBool("$A1", true)
		testBool("A1", true)

		testBool("A$$1", false)
		testBool("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)
		testBool("0!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", false)
		testBool("10+-10/10--10", true)
		testBool("", true)
		testBool("10+-10/10---10", false)
		testBool("A10+(-10)", true)
		testBool("A10+(--10)", false)
		testBool("A1*-5", true)
		testBool("*5", false)
		testBool("$B$1+CEIL(RAND()*1000)", true)

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
		// testBool("$B$1+CEIL(RAND()*1000)", true)

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

func testBool(formula string, expected bool) {
	testCount++
	result := isValidFormula(formula)
	if result != expected {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " failed] Expected: " + strconv.FormatBool(expected) + ", got: " + strconv.FormatBool(result) + " formula: " + formula)
		testFailCount++
	} else {
		fmt.Println("[Test #" + strconv.Itoa(testCount) + " succeeded] formula: " + formula)
	}
}
