package main

import (
	"fmt"
	"unicode"
)

func main() {
	// formula := "A1 + A2 + \"hello A1\""
	formula := "A2*A1*A2/A1*A1 + \"hello 😭 A1\""

	//fmt.Println(rune(formula[0]) == 'A')

	referenceMap := make(map[string]string)

	referenceMap["A1"] = "B1"
	referenceMap["A2"] = "B2"

	fmt.Println(replaceReferencesInFormula(formula, referenceMap))
}

func replaceReferencesInFormula(formula string, referenceMap map[string]string) string {

	// take into account replacements that elongate the string while in the loop
	// e.g. A9 => A10, after replacing the index should be incremented by one (use IsDigit from unicode package)

	// loop through formula string and only replace references in the map that are
	index := 0

	referenceStartIndex := 0
	referenceEndIndex := 0
	haveValidReference := false
	inQuoteSection := false

	for {

		// get character
		character := rune(formula[index])

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

			if unicode.IsDigit(character) {
				// append digit to valid reference
				referenceEndIndex = index
			} else {
				// replace reference
				leftSubstring := formula[:referenceStartIndex]
				rightSubstring := formula[referenceEndIndex+1:]

				reference := formula[referenceStartIndex : referenceEndIndex+1]
				newReference := referenceMap[reference]

				sizeDifference := len(reference) - len(newReference)

				index += sizeDifference

				// replace
				formula = leftSubstring + newReference + rightSubstring

				haveValidReference = false
				referenceStartIndex = index + 1
				referenceEndIndex = index + 1
			}

		} else if unicode.IsLetter(character) {

			referenceEndIndex = index + 1

		} else if unicode.IsDigit(character) {
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
		if index >= len(formula) {
			break
		}
	}

	return formula
}
