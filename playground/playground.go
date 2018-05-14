package main

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

var availableOperators []string

func main() {
	// formula := "A1 + A2 + \"hello A1\""
	// formula := "A2*A1*A2/A1*A1 + \"hello 😭 A1\""
	// formula := "A$2*$A$1*$A2/A1*A1 + \"hello 😭 A1\""
	debug := false

	availableOperators = []string{"^", "*", "/", "+", "-", ">", "<", ">=", "<=", "==", "<>", "!="}

	if !debug {
		test("((A1 + A) - (1))", false)
		test("((A1 + A10) - (1))", true)
		test("A10 + 0.2", true)
		test("A10 + A0.2", false)
		test("0.1 + 0.2 * 0.3 / 0.1", true)
		test("0.1 + 0.2 * 0.3 / 0.1A", false)
		test("A1 * A20 + 0.2 - \"abc\"", true)
		test("A1 * A + 0.2 - \"abc\"", false)
		test("SUM(A1:10, 10)", false)
		test("A1 ^^ 10", false)
		test("A1 ^ 10", true)
		test("SUM(A1 ^ 10, 1, 1.05)", true)
		test("SUM(A1 ^ 10, 1, A1.05)", false)
		test("A.01", false)
		test("A10+0.01", true)
		test("A10+A", false)
		test("", true)
	} else {
		// test("SUM(A1:10, 10)", false)

		// test("((A1 + A10) - (1))", true)

		// test("SUM(A1:A10, 10)", true)
	}

	// test("A10 + A0.2", false)

	// test("A10 + 0.2", true)

}

func test(formula string, expected bool) {
	result := isValidFormula(formula)
	if result != expected {
		fmt.Println("[Test failed] Expected: " + strconv.FormatBool(expected) + ", got: " + strconv.FormatBool(result) + " formula: " + formula)
	} else {
		fmt.Println("[Test succeeded] formula: " + formula)
	}
}

func isValidFormula(formula string) bool {
	currentOperator := ""
	operatorsFound := []string{}

	parenDepth := 0
	quoteDepth := 0
	previousChar := ""

	// inFunction := false
	inReference := false
	referenceFoundRange := false
	validReference := false
	inDecimal := false
	inNumber := false
	validDecimal := false
	inOperator := false
	inFunction := false

	var buffer bytes.Buffer

	// skipCharacters := 0

	for _, r := range formula {

		// fmt.Println("char index: " + strconv.Itoa(k))

		c := string(r)

		// check for quotes
		if c == "\"" && quoteDepth == 0 && previousChar != "\\" {

			quoteDepth++

		} else if c == "\"" && quoteDepth == 1 && previousChar != "\\" {

			quoteDepth--

		}

		if quoteDepth == 0 {

			if c == "(" {

				parenDepth++

			} else if c == ")" {

				parenDepth--

				if parenDepth < 0 {
					return false
				}

			}

			/* reference checking */
			if !inReference && unicode.IsLetter(r) {
				inReference = true
			}

			if inReference && (r == ':' && []rune(previousChar)[0] != ':') {
				inReference = false
				validReference = false
				referenceFoundRange = true
			}

			if inReference && validReference && unicode.IsLetter(r) {
				return false
			}

			/* function checking */
			if inReference && !validReference && r == '(' {
				inFunction = true
				inReference = false
			}

			if inFunction && r == ')' {
				inFunction = false
			}

			if referenceFoundRange && !inReference && unicode.IsLetter(r) {
				inReference = true
			}

			if referenceFoundRange && !inReference && unicode.IsDigit(r) {
				return false
			}

			if inReference && unicode.IsDigit(r) {
				validReference = true
			}

			if inReference && referenceFoundRange && !(unicode.IsDigit(r) || unicode.IsLetter(r)) {

				if !validReference {
					return false
				}

				inReference = false
				validReference = false
				referenceFoundRange = false
			}

			if inReference && !referenceFoundRange && !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == ':') {

				if !validReference {
					return false
				}

				inReference = false
				validReference = false
			}

			/* number checking */
			if !inReference && !inDecimal && unicode.IsDigit(r) {
				inNumber = true
			}

			/* decimal checking */
			if !inReference && inNumber && r == '.' && unicode.IsDigit([]rune(previousChar)[0]) {
				inDecimal = true
			} else if inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && !validDecimal {
				return false
			} else if inDecimal && unicode.IsDigit(r) {
				validDecimal = true
			} else if inDecimal && unicode.IsLetter(r) {
				return false
			} else if !inDecimal && r == '.' {
				return false
			}

			if !(unicode.IsLetter(r) || unicode.IsDigit(r)) && r != '.' {
				inNumber = false
				inDecimal = false
			}

			/* operator checking */
			if !inReference && !inFunction && !inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && r != ' ' && r != '(' && r != ')' && r != ',' {
				// if not in reference and operator is not space
				currentOperator += c
				inOperator = true
			}

			if inOperator && (unicode.IsDigit(r) || unicode.IsLetter(r) || r == ' ') {
				inOperator = false
				operatorsFound = append(operatorsFound, currentOperator)
				currentOperator = ""
			}

		}

		buffer.WriteString(c)

		previousChar = c

	}

	for _, operator := range operatorsFound {
		if !contains(availableOperators, operator) {
			return false
		}
	}

	if parenDepth != 0 {
		return false
	}
	if quoteDepth != 0 {
		return false
	}

	if inReference && !validReference {
		return false
	}

	return true
}

// func isValidFormula(formula string) bool {

// 	currentOperator := ""
// 	operatorsFound := []string{}

// 	parenDepth := 0
// 	quoteDepth := 0
// 	previousChar := ""

// 	// inFunction := false
// 	inReference := false
// 	validReference := false
// 	inDecimal := false
// 	inNumber := false
// 	validDecimal := false
// 	inOperator := false
// 	inFunction := false

// 	var buffer bytes.Buffer

// 	// skipCharacters := 0

// 	for _, r := range formula {

// 		c := string(r)

// 		// check for quotes
// 		if c == "\"" && quoteDepth == 0 && previousChar != "\\" {

// 			quoteDepth++

// 		} else if c == "\"" && quoteDepth == 1 && previousChar != "\\" {

// 			quoteDepth--

// 		}

// 		if quoteDepth == 0 {

// 			if c == "(" {

// 				parenDepth++

// 			} else if c == ")" {

// 				parenDepth--

// 				if parenDepth < 0 {
// 					return false
// 				}

// 			}

// 			/* reference checking */
// 			if !inReference && unicode.IsLetter(r) {
// 				inReference = true
// 			}

// 			if inReference && (unicode.IsDigit(r) || r == ':' && []rune(previousChar)[0] != ':') {
// 				inReference = true
// 				validReference = false
// 			}

// 			if inReference && validReference && unicode.IsLetter(r) {
// 				return false
// 			}

// 			/* function checking */
// 			if inReference && !validReference && r == '(' {
// 				inFunction = true
// 				inReference = false
// 			}

// 			if inFunction && r == ')' {
// 				inFunction = false
// 			}

// 			if inFunction && r == ',' {
// 				continue
// 			}

// 			if inReference && !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == ':') {

// 				if !validReference {
// 					return false
// 				}

// 				inReference = false
// 				validReference = false
// 			}

// 			/* number checking */
// 			if !inReference && !inDecimal && unicode.IsDigit(r) {
// 				inNumber = true
// 			}

// 			/* decimal checking */
// 			if !inReference && inNumber && r == '.' && unicode.IsDigit([]rune(previousChar)[0]) {
// 				inDecimal = true
// 			} else if inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && !validDecimal {
// 				return false
// 			} else if inDecimal && unicode.IsDigit(r) {
// 				validDecimal = true
// 			} else if inDecimal && unicode.IsLetter(r) {
// 				return false
// 			} else if !inDecimal && r == '.' {
// 				return false
// 			}

// 			if !(unicode.IsLetter(r) || unicode.IsDigit(r)) && r != '.' {
// 				inNumber = false
// 				inDecimal = false
// 			}

// 			/* operator checking */
// 			if !inReference && !inFunction && !inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && r != ' ' && r != '(' && r != ')' {
// 				// if not in reference and operator is not space
// 				currentOperator += c
// 				inOperator = true
// 			}

// 			if inOperator && (unicode.IsDigit(r) || unicode.IsLetter(r) || r == ' ') {
// 				inOperator = false
// 				operatorsFound = append(operatorsFound, currentOperator)
// 				currentOperator = ""
// 			}

// 		}

// 		buffer.WriteString(c)

// 		previousChar = c

// 	}

// 	for _, operator := range operatorsFound {
// 		if !contains(availableOperators, operator) {
// 			return false
// 		}
// 	}

// 	if parenDepth != 0 {
// 		return false
// 	}
// 	if quoteDepth != 0 {
// 		return false
// 	}

// 	return true
// }

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
