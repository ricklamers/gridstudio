package main

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

var availableOperators []string

var breakChars []string

func main() {
	// formula := "A1 + A2 + \"hello A1\""
	// formula := "A2*A1*A2/A1*A1 + \"hello 😭 A1\""
	// formula := "A$2*$A$1*$A2/A1*A1 + \"hello 😭 A1\""
	debug := false

	// debug = true

	availableOperators = []string{"^", "*", "/", "+", "-", ">", "<", ">=", "<=", "==", "<>", "!="}
	breakChars = []string{" ", ")", ",", "*", "/", "+", "-", ">", "<", "=", "^"}

	if !debug {
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
		test("SUM(A1 ^ 10, 1, 1.05))", false)
		test("SUM(A1 ^ 10, 1, 1.05)", true)
		test("SUM(A1 ^ 10, 1, A1.05)", false)
		test("A.01", false)
		test("A10+0.01", true)
		test("A10+A", false)
		test("$A$10+$A1+A$2", true)
		test("$$A1", false)
		test("A$$1", false)
		test("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)
		test("0!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", false)
		test("10+-10/10--10", true)
		test("", true)
		test("10+-10/10---10", false)
		test("A10+(-10)", true)
		test("A10+(--10)", false)
		test("A1*-5", true)
		test("*5", false)

	} else {
		// test("SUM(A1:10, 10)", false)

		// test("((A1 + A10) - (1))", true)

		// test("SUM(A1:A10, 10)", true)
		// test("$A$10+$A1+A$2", true)

		// referenceMap := make(map[string]string)

		// referenceMap["A1:B10"] = "B1:C10"
		// referenceMap["A1"] = "B1"

		// replaceReferencesInFormula("SUM( A1:B10 ) + A1", referenceMap)
		// references := findReferenceStrings("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100")

		// for _, value := range references {
		// 	fmt.Println(value)
		// }

		test("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)

		// test("A1 * A20 + 0.2 - \"abc\"", true)
		// test("'0'!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)
		// test("0!A5 + 'Blad 2'!A10 + A10 - Blad15!$A$100", true)

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
	singleQuoteDepth := 0
	skipNextChar := false
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
	foundReferenceMark := false
	inOperator := false
	inFunction := false
	operatorAllowed := false

	var buffer bytes.Buffer

	// skipCharacters := 0
	formulaRunes := []rune{}

	for _, r := range formula {
		formulaRunes = append(formulaRunes, r)
	}

	for k := range formulaRunes {

		if skipNextChar {
			skipNextChar = false
			continue
		}

		// fmt.Println("char index: " + strconv.Itoa(k))
		r := formulaRunes[k]

		nextR := ' '

		if len(formulaRunes) > k+1 {
			nextR = formulaRunes[k+1]
		}

		c := string(r)

		// check for quotes
		if c == "\"" && quoteDepth == 0 && previousChar != "\\" {

			quoteDepth++
			continue

		} else if c == "\"" && quoteDepth == 1 && previousChar != "\\" {

			quoteDepth--
			continue
		}

		if c == "'" && singleQuoteDepth == 0 {

			singleQuoteDepth++

			continue

		} else if c == "'" && singleQuoteDepth == 1 {

			singleQuoteDepth--

			// should be followed by dollar or letter
			if nextR != '!' {
				return false
			} else {
				inReference = true

				// skip next char !
				skipNextChar = true
			}

			continue
		}

		if quoteDepth == 0 && singleQuoteDepth == 0 {

			if c == "(" {

				parenDepth++

				operatorAllowed = false

				inOperator = false

				operatorsFound = append(operatorsFound, currentOperator)
				currentOperator = ""

			} else if c == ")" {

				parenDepth--

				if parenDepth < 0 {
					return false
				}

			}

			/* reference checking */
			if inReference && r == '!' && !foundReferenceMark {
				foundReferenceMark = true
				validReference = false
				inReference = false
				continue
			}

			if r == '!' && foundReferenceMark {
				return false
			}

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
				foundReferenceMark = false
			}

			if inReference && validReference && unicode.IsLetter(r) {
				return false
			}

			/* function checking */
			if inReference && !validReference && r == '(' {
				inFunction = true
				inReference = false
				foundReferenceMark = false
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
				foundReferenceMark = false
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
				foundReferenceMark = false
				validReference = false
				dollarInColumn = false
				dollarInRow = false
				operatorAllowed = true
			}

			if !inReference && !inDecimal && unicode.IsDigit(r) {
				inNumber = true

				operatorsFound = append(operatorsFound, currentOperator)
				currentOperator = ""

				if inOperator {
					inOperator = false
					operatorAllowed = false
				}
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

			if (inNumber || inDecimal) && !(unicode.IsLetter(r) || unicode.IsDigit(r)) && r != '.' {
				inNumber = false
				inDecimal = false

				// number end
				operatorAllowed = true
			}

			if !inNumber && r == '-' && unicode.IsDigit(nextR) {
				inNumber = true

				operatorsFound = append(operatorsFound, currentOperator)
				currentOperator = ""

				if inOperator {
					inOperator = false
				}

			} else if inOperator && r == '-' && currentOperator == "-" {
				return false
			} else if !inReference && r == '-' && !inNumber && !inOperator && !unicode.IsDigit(nextR) {
				currentOperator += c
				inOperator = true

				if !operatorAllowed {
					return false
				}

			} else if !inReference && !inFunction && !inDecimal && !(unicode.IsDigit(r) || unicode.IsLetter(r)) && r != ' ' && r != '(' && r != ')' && r != ',' && r != '$' {
				// if not in reference and operator is not space
				currentOperator += c
				inOperator = true

				if !operatorAllowed {
					return false
				}
			}

			if inOperator && (unicode.IsDigit(r) || unicode.IsLetter(r) || r == ' ') {
				inOperator = false
				operatorAllowed = false
				operatorsFound = append(operatorsFound, currentOperator)
				currentOperator = ""
			}

		}

		buffer.WriteString(c)

		previousChar = c

	}

	for _, operator := range operatorsFound {
		if !contains(availableOperators, operator) && operator != "" {
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

func findReferenceStrings(formula string) []string {

	references := []string{}

	// loop over string, if double quote is found, ignore input for references,
	quoteLevel := 0
	singleQuoteLevel := 0
	previousChar := ""

	var buf bytes.Buffer

	for _, c := range formula {
		char := string(c)

		if char == "\"" && previousChar != "\\" {
			if quoteLevel == 0 {
				quoteLevel++
			} else {
				buf.Reset()
				quoteLevel--
				continue
			}
		}

		if quoteLevel == 0 && char == "'" && singleQuoteLevel == 0 {
			singleQuoteLevel++
		} else if quoteLevel == 0 && singleQuoteLevel == 1 && char == "'" {
			singleQuoteLevel--
		}

		if quoteLevel == 0 {

			// assume everything not in quotes is a reference
			// if we find open brace it must be a function name
			if singleQuoteLevel == 1 {
				// continue regularly when singleQuoteLevel is 1, full buf with WriteString
			} else if char == "(" {
				buf.Reset()
				continue
			} else if contains(breakChars, char) {

				if buf.Len() > 0 {
					// found space, previous is references
					references = append(references, buf.String())
					buf.Reset()
				}
				// never add break char to reference
				continue
			}

			// edge case for number, if first char of buffer is number, can't be reference
			if buf.Len() == 0 {

				_, err := strconv.Atoi(char)

				if err == nil || char == "." { // check for both numbers and . character

					// found number can't be reference
					buf.Reset()
					continue
				}
			}

			buf.WriteString(char)

		}

		previousChar = char

	}

	// at the end also add as reference
	if buf.Len() > 0 {
		references = append(references, buf.String())
	}

	// filter references for known keywords such as TRUE/FALSE
	for k, reference := range references {
		if len(references) > 0 {
			if reference == "TRUE" || reference == "FALSE" {
				references = append(references[:k], references[k+1:]...)
			}
		}
	}

	return references
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
