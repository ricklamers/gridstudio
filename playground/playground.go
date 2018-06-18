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
	debug := true

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
		test("$A$10+$A1+A$2", true)
		test("$$A1", false)
		test("A$$1", false)
		test("", true)
	} else {
		// test("SUM(A1:10, 10)", false)

		// test("((A1 + A10) - (1))", true)

		// test("SUM(A1:A10, 10)", true)
		// test("$A$10+$A1+A$2", true)

		referenceMap := make(map[string]string)

		referenceMap["A1:B10"] = "B1:C10"
		referenceMap["A1"] = "B1"

		replaceReferencesInFormula("SUM( A1:B10 ) + A1", referenceMap)

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
	dollarInColumn := false
	dollarInRow := false
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

			if !inReference && dollarInColumn && r == '$' {
				return false
			}

			if !inReference && r == '$' {
				dollarInColumn = true
			}

			if dollarInRow && inReference && r == '$' {
				return false
			}

			if inReference && !dollarInColumn && r == '$' {
				dollarInRow = true
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
				dollarInColumn = false
				dollarInRow = false
				validReference = false
				referenceFoundRange = false
			}

			if inReference && !referenceFoundRange && !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == ':' || r == '$') {

				if !validReference {
					return false
				}

				inReference = false
				validReference = false
				dollarInColumn = false
				dollarInRow = false
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
			if !inReference && !inFunction && !inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && r != ' ' && r != '(' && r != ')' && r != ',' && r != '$' {
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

func replaceReferencesInFormula(formula string, referenceMap map[string]string) string {

	// take into account replacements that elongate the string while in the loop
	// e.g. A9 => A10, after replacing the index should be incremented by one (use IsDigit from unicode package)

	// check for empty referenceMap inputs
	if len(referenceMap) == 0 {
		return formula
	}

	// loop through formula string and only replace references in the map that are
	index := 0

	referenceStartIndex := 0
	referenceEndIndex := 0
	haveValidReference := false
	inQuoteSection := false

	for {

		// set default characters
		character := ' '

		// get character
		if index < len(formula) {
			character = rune(formula[index])
		}

		if inQuoteSection {

			if character == '"' && formula[index-1] != '\\' {
				// exit quote section
				inQuoteSection = false
				referenceStartIndex = index + 1
				referenceEndIndex = index + 1
			}

		} else if character == '"' {
			inQuoteSection = true
		} else if haveValidReference {

			if character == ':' {

				referenceEndIndex = index
				haveValidReference = false

			} else if unicode.IsDigit(character) {
				// append digit to valid reference
				referenceEndIndex = index
			} else {
				// replace reference
				leftSubstring := formula[:referenceStartIndex]
				rightSubstring := formula[referenceEndIndex+1:]

				reference := formula[referenceStartIndex : referenceEndIndex+1]

				newReference := reference

				// if reference is not in referenceMap, use existing
				if _, ok := referenceMap[reference]; ok {
					//do something here
					newReference = referenceMap[reference]
				}

				sizeDifference := len(newReference) - len(reference)

				index += sizeDifference

				// replace
				formula = leftSubstring + newReference + rightSubstring

				haveValidReference = false
				referenceStartIndex = index + 1
				referenceEndIndex = index + 1
			}

		} else if unicode.IsLetter(character) || character == '$' || character == ':' {

			referenceEndIndex = index + 1

		} else if unicode.IsDigit(character) {

			// check if next character is digit or :
			if referenceEndIndex-referenceStartIndex > 0 {
				// non zero reference is built up, append digit
				referenceEndIndex = index
				haveValidReference = true

			} else {
				referenceStartIndex = index + 1
				referenceEndIndex = index + 1
				haveValidReference = false
			}
		} else {
			referenceStartIndex = index + 1
			referenceEndIndex = index + 1
			haveValidReference = false
		}

		index++
		if index >= len(formula) && !haveValidReference {
			break
		}
	}

	return formula
}
