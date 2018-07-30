package main

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	matrix "github.com/skelterjohn/go.matrix"
)

const DynamicValueTypeFormula int8 = 0
const DynamicValueTypeReference int8 = 1
const DynamicValueTypeFloat int8 = 3
const DynamicValueTypeString int8 = 4
const DynamicValueTypeBool int8 = 5
const DynamicValueTypeExplosiveFormula int8 = 6
const DynamicValueTypeOperator int8 = 7

type DynamicValue struct {
	ValueType     int8
	DataFloat     float64
	DataString    string
	DataBool      bool
	DataFormula   string
	SheetIndex    int8
	DependIn      map[string]bool
	DependOut     map[string]bool
	DependInTemp  map[string]bool
	DependOutTemp map[string]bool
}

var numberOnlyReg *regexp.Regexp
var numberOnlyFilter *regexp.Regexp

var availableOperators []string
var maxOperatorSize int
var operatorsBroken []map[string]int
var breakChars []string

func makeEmptyDv() *DynamicValue {
	dv := DynamicValue{}

	dv.DependIn = make(map[string]bool)
	dv.DependOut = make(map[string]bool)

	return &dv
}

func makeDv(formula string) *DynamicValue {
	dv := DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: formula}

	dv.DependIn = make(map[string]bool)
	dv.DependOut = make(map[string]bool)

	return &dv
}

func parseInit() {

	availableOperators = []string{"^", "*", "/", "+", "-", ">", "<", ">=", "<=", "==", "<>", "!="}
	breakChars = []string{" ", ")", ",", "*", "/", "+", "-", ">", "<", "=", "^"}

	// loop over availableOperators twice: once to find maxOperatorSize to initialize hashmaps, once to fill hashmaps
	for _, e := range availableOperators {
		if len(e) > maxOperatorSize {
			maxOperatorSize = len(e)
		}
	}

	// create empty hashmap array
	operatorsBroken = []map[string]int{}

	// initialize hashmaps
	for i := 0; i < maxOperatorSize; i++ {
		operatorsBroken = append(operatorsBroken, make(map[string]int))
	}

	for _, e := range availableOperators {

		// determine target hashmap
		index := len(e)

		// assign hashmap value
		operatorsBroken[index-1][e] = 1

	}

	numberOnlyReg, _ = regexp.Compile("[^0-9]+")
	numberOnlyFilter, _ = regexp.Compile(`^-?[0-9]\d*(\.\d+)?$`)
}

func getData(ref1 DynamicValue, grid *Grid) *DynamicValue {
	return (grid.Data)[ref1.DataString]
}

func getDataFromRef(reference Reference, grid *Grid) *DynamicValue {
	return grid.Data[getMapIndexFromReference(reference)]
}
func checkDataPresenceFromRef(reference Reference, grid *Grid) bool {
	_, ok := grid.Data[getMapIndexFromReference(reference)]
	return ok
}

func getReferenceFromString(formula string, sheetIndex int8, grid *Grid) Reference {
	if !strings.Contains(formula, "!") {
		return Reference{String: formula, SheetIndex: sheetIndex}
	} else {
		splittedFormula := strings.Split(formula, "!")
		sheetName := strings.Replace(splittedFormula[0], "'", "", -1)
		return Reference{String: splittedFormula[1], SheetIndex: grid.SheetNames[sheetName]}
	}
}
func getRangeReferenceFromString(formula string, sheetIndex int8, grid *Grid) ReferenceRange {
	if !strings.Contains(formula, "!") {
		return ReferenceRange{String: formula, SheetIndex: sheetIndex}
	} else {
		splittedFormula := strings.Split(formula, "!")
		sheetName := strings.Replace(splittedFormula[0], "'", "", -1)
		return ReferenceRange{String: splittedFormula[1], SheetIndex: grid.SheetNames[sheetName]}
	}
}

func referenceRangeToRelativeString(referenceRange ReferenceRange, sheetIndex int8, grid *Grid) string {
	if referenceRange.SheetIndex == sheetIndex {
		return referenceRange.String
	} else {
		return getPrefixFromSheetName(grid.SheetList[referenceRange.SheetIndex]) + "!" + referenceRange.String
	}
}

func referenceToRelativeString(reference Reference, sheetIndex int8, grid *Grid) string {
	if reference.SheetIndex == sheetIndex {
		return reference.String
	} else {
		return getPrefixFromSheetName(grid.SheetList[reference.SheetIndex]) + "!" + reference.String
	}
}

func getPrefixFromSheetName(sheetName string) string {
	if strings.Contains(sheetName, " ") {
		return "'" + sheetName + "'"
	} else {
		return sheetName
	}
}

func checkIfRefExists(reference Reference, grid *Grid) bool {
	if _, ok := grid.Data[getMapIndexFromReference(reference)]; ok {
		return true
	}
	return false
}

func getDataByNormalRef(ref string, grid *Grid) *DynamicValue {
	return grid.Data[ref]
}

func setDataByRef(reference Reference, dv *DynamicValue, grid *Grid) {
	dv.SheetIndex = reference.SheetIndex
	mapIndex := getMapIndexFromReference(reference)
	grid.Data[mapIndex] = dv
}

func findInMap(amap map[int]string, value string) bool {
	for _, e := range amap {
		if e == value {
			return true
		}
	}
	return false
}

func addToReferenceReplaceMap(reference string, incrementAmount int, references *map[string]string) {

	// exclude from increment any reference with $ in it
	if !strings.Contains(reference, "$") {
		referenceRow := getReferenceRowIndex(reference)
		referenceColumn := getReferenceColumnIndex(reference)

		referenceRow = referenceRow + incrementAmount

		newReference := indexToLetters(referenceColumn) + strconv.Itoa(referenceRow)
		(*references)[reference] = newReference
	}

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

		if c == '\'' && singleQuoteLevel == 0 {
			singleQuoteLevel++
		} else if c == '\'' && singleQuoteLevel == 1 {
			singleQuoteLevel--
		}

		if quoteLevel == 0 && singleQuoteLevel == 1 {
			buf.WriteString(char)
		}

		if quoteLevel == 0 && singleQuoteLevel == 0 {

			// assume everything not in quotes is a reference
			// if we find open brace it must be a function name
			if char == "(" {
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

				if unicode.IsDigit(c) || char == "." { // check for both numbers and . character

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

func findRanges(formula string, sheetIndex int8, grid *Grid) []ReferenceRange {

	rangeReferences := []ReferenceRange{}
	referenceStrings := findReferenceStrings(formula)

	// expand references when necessary
	for _, referenceString := range referenceStrings {
		if strings.Contains(referenceString, ":") {
			rangeReferences = append(rangeReferences, getRangeReferenceFromString(referenceString, sheetIndex, grid))
		}
	}

	return rangeReferences
}

func findReferences(formula string, sheetIndex int8, includeRanges bool, grid *Grid) map[Reference]bool {

	referenceStrings := findReferenceStrings(formula)

	references := []Reference{}

	// expand references when necessary
	for _, referenceString := range referenceStrings {

		if strings.Contains(referenceString, ":") {

			if includeRanges {
				references = append(references, cellRangeToCells(getRangeReferenceFromString(referenceString, sheetIndex, grid))...)
			}
		} else {

			references = append(references, getReferenceFromString(referenceString, sheetIndex, grid))
		}

	}

	finalMap := make(map[Reference]bool)

	for _, reference := range references {
		finalMap[reference] = true
	}

	return finalMap
}

func referencesToUpperCase(formula string) string {

	// loop over string, if double quote is found, ignore input for references,
	quoteLevel := 0
	previousChar := ""

	var buf bytes.Buffer

	for k, c := range formula {

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

		if quoteLevel == 0 {

			// assume everything not in quotes is a reference
			// if we find open brace it must be a function name
			if char == "(" {
				buf.Reset()
				continue
			} else if contains(breakChars, char) {

				if buf.Len() > 0 {
					// found space, previous is references

					// buf is reference, replace buf with uppercase version
					formula = formula[:k-buf.Len()] + strings.ToUpper(buf.String()) + formula[k:]
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
		formula = formula[:len(formula)-buf.Len()] + strings.ToUpper(buf.String()) + formula[len(formula):]
	}

	return formula
}

func cellRangeToCells(referenceRange ReferenceRange) []Reference {
	references := []Reference{}

	cell1Row, cell1Column, cell2Row, cell2Column := cellRangeBoundaries(referenceRange.String)

	// illegal argument, cell1Row should always be lower
	if cell1Row > cell2Row {
		return references
	}
	// illegal argument, cell1Column should always be lower
	if cell1Column > cell2Column {
		return references
	}

	// all equals means just one cell, example: A1:A2
	// if cell1Row == cell2Row && cell1Column == cell2Column {
	// 	references = append(references, indexToLetters(cell1Column)+strconv.Itoa(cell1Row))
	// 	return references
	// }

	for x := cell1Column; x <= cell2Column; x++ {
		for y := cell1Row; y <= cell2Row; y++ {
			references = append(references, Reference{String: indexToLetters(x) + strconv.Itoa(y), SheetIndex: referenceRange.SheetIndex})
		}
	}

	return references
}

func setDependencies(reference Reference, dv *DynamicValue, grid *Grid) *DynamicValue {

	standardIndex := getMapIndexFromReference(reference)
	var references map[Reference]bool
	// explosiveFormulas never have dependencies
	if dv.ValueType == DynamicValueTypeExplosiveFormula {
		references = make(map[Reference]bool)
	} else {
		references = findReferences(dv.DataFormula, reference.SheetIndex, true, grid)
	}

	// every cell that this depended on needs to get removed
	referenceDv := getDataFromRef(reference, grid)

	for ref := range referenceDv.DependIn {
		delete(getDataByNormalRef(ref, grid).DependOut, standardIndex)
	}

	// always clear incoming references, if they still exist
	dv.DependIn = make(map[string]bool)

	for thisRef, inSet := range references {

		// when findReferences is called and a reference is not in grid.Data[] the reference is invalid
		if checkIfRefExists(thisRef, grid) {
			thisDv := getDataFromRef(thisRef, grid)

			// for dependency checking get rid of dollar signs in references
			thisDvStandardRef := getMapIndexFromReference(thisRef)

			if inSet {
				// if thisRef == reference {
				// 	// cell is dependent on self
				// 	fmt.Println("Circular reference error!")
				// 	dv.ValueType = DynamicValueTypeString
				// 	dv.DataFormula = "\"#Error, circular reference: " + dv.DataFormula + "\""
				// } else {

				// }

				dv.DependIn[thisDvStandardRef] = true
				thisDv.DependOut[standardIndex] = true

				// copy
				// copyToDirty(thisDvStandardRef, grid)
			}

		} else {
			dv.DataFormula = "\"#REF: " + referenceToRelativeString(thisRef, reference.SheetIndex, grid) + "\""
		}

	}

	// always add self to dirty (after setting references DependIn for self - loop above)
	copyToDirty(standardIndex, grid)

	// // mark all cells dirty that depend on this cell
	// for ref, inSet := range dv.DependOut {
	// 	if inSet {

	// 		dv := (grid.Data)[ref]
	// 		copyToDirty(dv, ref, grid)
	// 	}
	// }

	return dv
}

func containsReferences(s []Reference, e Reference) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func copyDv(dv *DynamicValue) *DynamicValue {
	newDv := DynamicValue{}
	newDv.DataBool = dv.DataBool
	newDv.DataFloat = dv.DataFloat
	newDv.DataString = dv.DataString
	newDv.DataFormula = dv.DataFormula
	newDv.SheetIndex = dv.SheetIndex
	newDv.ValueType = dv.ValueType
	return &newDv
}

// func copyToDv(sourceDv *DynamicValue, targetDv *DynamicValue) {
// 	// note: DependOut and DependIn are unmodified - need to be done by setDependencies
// 	targetDv.DataBool = sourceDv.DataBool
// 	targetDv.DataFloat = sourceDv.DataFloat
// 	targetDv.DataString = sourceDv.DataString
// 	targetDv.DataFormula = sourceDv.DataFormula
// 	targetDv.SheetIndex = sourceDv.SheetIndex
// 	targetDv.ValueType = sourceDv.ValueType
// }

func parse(formula *DynamicValue, grid *Grid, targetRef Reference) *DynamicValue {

	if formula.ValueType == DynamicValueTypeFloat {
		return formula
	}
	if formula.ValueType == DynamicValueTypeFormula && len(formula.DataFormula) == 0 {
		formula.DataString = ""
		formula.ValueType = DynamicValueTypeString
		return formula
	}

	elements := []*DynamicValue{}
	operatorsFound := []string{}
	parenDepth := 0
	quoteDepth := 0
	previousChar := ""
	var buffer bytes.Buffer

	// TODO: instead of writing to buffer keep track of indexes for efficiency
	skipCharacters := 0

	for k, r := range formula.DataFormula {

		c := string(r)

		if skipCharacters > 0 {

			skipCharacters--
			continue

		} else {

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
						log.Fatal("Invalid input string")
					}

				} else if parenDepth == 0 {

					identifiedOperator := ""

					// never check first operator (could be minus sign)

					currentTrimmedBuffer := strings.TrimSpace(buffer.String())
					// first char and equals - or previous element is operator and this is minus

					// first check is for first character equals minus sign,
					// second check is for a minus sign directly after an operator to indicate that the following term is negative
					if !((k == 0 && c == "-") ||
						(c == "-" && len(elements) > 0 &&
							elements[len(elements)-1].ValueType == DynamicValueTypeOperator &&
							len(currentTrimmedBuffer) == 0)) {

						for i := 0; i < maxOperatorSize; i++ {

							// condition 1: never check beyond length of original formula
							// condition 2: identfy whether operator is in hashmapped operator data structure for high performance

							// first check for operators of size 1, then for operators of size 2, etc
							if (k+i+1) <= len(formula.DataFormula)-1 && operatorsBroken[i][formula.DataFormula[k:k+i+1]] == 1 {
								identifiedOperator = formula.DataFormula[k : k+i+1]
							}

						}

						if len(identifiedOperator) > 0 {

							// add to elements everything in buffer before operator
							newDv := DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeFormula, DataFormula: strings.TrimSpace(buffer.String())}
							elements = append(elements, &newDv)

							newDv2 := DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeOperator, DataFormula: identifiedOperator}
							// add operator to elements
							elements = append(elements, &newDv2)

							skipCharacters = len(identifiedOperator) - 1

							// append operator to found operators, if not already in there
							if !contains(operatorsFound, identifiedOperator) {
								operatorsFound = append(operatorsFound, identifiedOperator)
							}

							// reset buffer
							buffer.Reset()

							continue

						}
					}
				}
			}

			buffer.WriteString(c)
		}

		previousChar = c

	}

	newDv := DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeFormula, DataFormula: strings.TrimSpace(buffer.String())}
	elements = append(elements, &newDv)

	// if first and last element in elements are parens, remove parens
	for k, e := range elements {
		if e.ValueType == DynamicValueTypeFormula {
			if len(e.DataFormula) > 0 {
				if e.DataFormula[0:1] == "(" && e.DataFormula[len(e.DataFormula)-1:len(e.DataFormula)] == ")" {
					e.DataFormula = e.DataFormula[1 : len(e.DataFormula)-1]
					elements[k] = e

					// after remove braces, parse this element
					elements[k] = parse(e, grid, targetRef)

					// for single elements return at this point
					if len(elements) == 1 {
						return elements[k]
					}
				}
			}
		}
	}

	if len(elements) == 1 {

		singleElement := elements[0]

		if isFunction(singleElement.DataFormula) {

			parenIndex := strings.Index(singleElement.DataFormula, "(")
			command := singleElement.DataFormula[0:parenIndex]

			// modify arguments function
			argumentString := singleElement.DataFormula[parenIndex+1 : len(singleElement.DataFormula)-1]
			var arguments []string

			level := 0
			quoteLevel := 0
			var buffer bytes.Buffer

			for _, r := range argumentString {

				c := string(r)

				// valid delimeters
				// go down level: " (
				// go up level: " )

				if c == "(" {
					level++
				} else if c == ")" {
					level--
				} else if c == "\"" && quoteLevel == 0 {
					level++
					quoteLevel++
				} else if c == "\"" && quoteLevel == 1 {
					level--
					quoteLevel--
				}

				if level == 0 {
					if c == "," {
						arguments = append(arguments, strings.TrimSpace(buffer.String()))
						buffer.Reset()
						continue
					}
				}

				buffer.WriteString(c)

			}

			bufferRemainder := strings.TrimSpace(buffer.String())
			if len(bufferRemainder) > 0 {
				arguments = append(arguments, bufferRemainder)
			}

			argumentFormulas := []*DynamicValue{}

			for _, e := range arguments {
				newDv := DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeFormula, DataFormula: e}
				argumentFormulas = append(argumentFormulas, parse(&newDv, grid, targetRef))
			}

			return executeCommand(command, argumentFormulas, grid, targetRef)

		} else {

			if singleElement.DataFormula[0:1] == "\"" && singleElement.DataFormula[len(singleElement.DataFormula)-1:len(singleElement.DataFormula)] == "\"" {

				stringValue := singleElement.DataFormula[1 : len(singleElement.DataFormula)-1]

				// unescape double quote
				stringValue = strings.Replace(stringValue, "\\\"", "\"", -1)

				return &DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeString, DataString: stringValue}

			} else if strings.Index(singleElement.DataFormula, ":") != -1 {

				cells := strings.Split(singleElement.DataFormula, ":")

				if !((numberOnlyFilter.MatchString(cells[0]) && numberOnlyFilter.MatchString(cells[1])) ||
					(!numberOnlyFilter.MatchString(cells[0]) && !numberOnlyFilter.MatchString(cells[1]))) {

					log.Fatal("Wrong reference specifier")

				} else {
					return &DynamicValue{ValueType: DynamicValueTypeReference, SheetIndex: targetRef.SheetIndex, DataString: singleElement.DataFormula}
				}

			} else if numberOnlyFilter.MatchString(singleElement.DataFormula) {

				floatValue, err := strconv.ParseFloat(singleElement.DataFormula, 64)
				if err != nil {
					log.Fatal(err)
				}

				return &DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeFloat, DataFloat: float64(floatValue)}

			} else if singleElement.DataFormula == "FALSE" || singleElement.DataFormula == "TRUE" {
				newDv := DynamicValue{SheetIndex: targetRef.SheetIndex, ValueType: DynamicValueTypeBool, DataBool: false}
				if singleElement.DataFormula == "TRUE" {
					newDv.DataBool = true
				}
				return &newDv
			} else {

				// when references contain dollar signs, remove them here
				newDv := copyDv(getDataFromRef(getReferenceFromString(singleElement.DataFormula, targetRef.SheetIndex, grid), grid))
				return newDv

			}

		}

	} else {

		// parse operators until top elements per formula reduced to 1
		var operatorSets [][]string

		operatorSets = [][]string{{"^"}, {"*", "/"}, {"+", "-"}, {">", "<", ">=", "<=", "==", "<>", "!="}}

		for _, operatorSet := range operatorSets {

			// more efficient would be to compare each element in operatorsFound to elements in operatorSet
			operatorInSet := anyOperatorsInOperatorSet(operatorSet, operatorsFound)

			if operatorInSet {
				operatorLocation := findFirstOperatorOccurence(elements, operatorSet)

				if operatorLocation == 0 {
					log.Fatal("Operator can never be the first input in an array")
				}

				for operatorLocation != -1 {

					LHS := parse(elements[operatorLocation-1], grid, targetRef)
					RHS := parse(elements[operatorLocation+1], grid, targetRef)

					var result DynamicValue

					// todo implement operators for all possible data type combinations

					// for now implement int and float as all float (* might actually be necessary for good performance (should compare int and float multiplication for large n in Golang))

					operator := elements[operatorLocation].DataFormula

					// if boolean operatorSet run boolean_compare
					if contains(operatorSet, "<") {
						result = booleanCompare(LHS, RHS, elements[operatorLocation].DataFormula)
					} else {
						// otherwise run numeric operator evaluation
						if LHS.ValueType == DynamicValueTypeFloat && RHS.ValueType == DynamicValueTypeFloat {

							// convert LHS and RHS to float
							LHS = convertToFloat(LHS)
							RHS = convertToFloat(RHS)

							result = DynamicValue{ValueType: DynamicValueTypeFloat}

							switch operator {
							case "*":
								result.DataFloat = LHS.DataFloat * RHS.DataFloat
							case "/":
								result.DataFloat = LHS.DataFloat / RHS.DataFloat
							case "+":
								result.DataFloat = LHS.DataFloat + RHS.DataFloat
							case "-":
								result.DataFloat = LHS.DataFloat - RHS.DataFloat
							case "^":
								result.DataFloat = float64(math.Pow(float64(LHS.DataFloat), float64(RHS.DataFloat)))
							}

						}
					}

					arrayEnd := elements[operatorLocation+2:]
					elements = append(elements[:operatorLocation-1], &result)
					elements = append(elements, arrayEnd...)

					operatorLocation = findFirstOperatorOccurence(elements, operatorSet)

				}

			}

		}

		if len(elements) != 1 {
			log.Fatal("Elements should be fully merged to 1 element")
		}

		return elements[0]

	}

	return formula
}

func anyOperatorsInOperatorSet(operatorSet []string, operatorsFound []string) bool {

	// if no operators were found it can never be in the set
	if len(operatorsFound) == 0 {
		return false
	}
	for _, operatorInSet := range operatorSet {
		for _, operatorFound := range operatorsFound {
			if operatorInSet == operatorFound {
				return true
			}
		}
	}
	return false
}

func dynamicToBool(A DynamicValue) bool {
	if A.ValueType == DynamicValueTypeBool {
		return A.DataBool
	} else if A.ValueType == DynamicValueTypeFloat {
		return A.DataFloat > 0
	} else if A.ValueType == DynamicValueTypeString {
		return len(A.DataString) > 0
	} else {
		return false
	}
}

func booleanCompare(LHS *DynamicValue, RHS *DynamicValue, operator string) DynamicValue {

	// for now, cast all values to float for comparison

	var result DynamicValue
	result.ValueType = DynamicValueTypeBool

	// var A float64
	// var B float64

	// // boolean operators
	// if LHS.ValueType == DynamicValueTypeBool {
	// 	A = 0
	// 	if LHS.DataBool {
	// 		A = 1
	// 	}
	// }
	// if LHS.ValueType == DynamicValueTypeFloat {
	// 	A = LHS.DataFloat
	// }
	// if LHS.ValueType == DynamicValueTypeInteger {
	// 	A = float64(LHS.DataInteger)
	// }

	// // parse RHS dynamically
	// if RHS.ValueType == DynamicValueTypeBool {
	// 	B = 0
	// 	if RHS.DataBool {
	// 		B = 1
	// 	}
	// }
	// if RHS.ValueType == DynamicValueTypeFloat {
	// 	B = RHS.DataFloat
	// }
	// if RHS.ValueType == DynamicValueTypeInteger {
	// 	B = float64(RHS.DataInteger)
	// }

	switch operator {
	case ">":
		result.DataBool = compareDvsBigger(LHS, RHS)
	case "<":
		result.DataBool = compareDvsSmaller(LHS, RHS)
	case ">=":
		result.DataBool = compareDvsBigger(LHS, RHS) || (!compareDvsSmaller(LHS, RHS) && !compareDvsBigger(LHS, RHS))
	case "<=":
		result.DataBool = compareDvsSmaller(LHS, RHS) || (!compareDvsSmaller(LHS, RHS) && !compareDvsBigger(LHS, RHS))
	case "==":
		result.DataBool = (!compareDvsSmaller(LHS, RHS) && !compareDvsBigger(LHS, RHS))
	case "!=":
		result.DataBool = compareDvsSmaller(LHS, RHS) || compareDvsBigger(LHS, RHS)
	case "<>":
		result.DataBool = compareDvsSmaller(LHS, RHS) || compareDvsBigger(LHS, RHS)
	}

	return result
}

func filter(src []string) (res []string) {
	for _, s := range src {
		newStr := strings.Join(res, " ")
		if !strings.Contains(newStr, s) {
			res = append(res, s)
		}
	}
	return
}

func intersections(section1, section2 []string) (intersection []string) {
	str1 := strings.Join(filter(section1), " ")
	for _, s := range filter(section2) {
		if strings.Contains(str1, s) {
			intersection = append(intersection, s)
		}
	}
	return
}

func indexOfOperator(ds []*DynamicValue, s string) int {
	for k, e := range ds {
		if e.DataFormula == s {
			return k
		}
	}
	return -1
}

func findFirstOperatorOccurence(elements []*DynamicValue, operatorSet []string) int {

	operatorLocation := math.MaxInt32

	for d := 0; d < len(operatorSet); d++ {

		index := indexOfOperator(elements, operatorSet[d])
		if index != -1 && index < operatorLocation {
			operatorLocation = index
		}
	}

	if operatorLocation == math.MaxInt32 {
		return -1
	}

	return operatorLocation
}

func stringToInteger(s string) int32 {

	numberString := numberOnlyReg.ReplaceAllString(s, "")

	number, _ := strconv.Atoi(numberString)

	return int32(number)
}

func isFunction(formulaString string) bool {

	for k, s := range formulaString {

		char := string(s)

		if char == "(" {

			if k > 0 {
				if formulaString[len(formulaString)-1] == ')' {
					return true
				}
			}
		}

		if !(s >= 65 && s <= 90 || s >= 97 && s <= 122) {
			return false
		}
	}

	return false
}

func toChar(i int) rune {
	return rune('A' - 1 + i)
}

func indexesToReferenceString(row int, col int) string {
	return indexToLetters(col) + strconv.Itoa(row)
}

func indexesToReferenceWithFixed(row int, col int, fixedRow bool, fixedColumn bool) string {
	firstPrefix := ""
	if fixedColumn {
		firstPrefix = "$"
	}

	secondPrefix := ""
	if fixedRow {
		secondPrefix = "$"
	}

	return firstPrefix + indexToLetters(col) + secondPrefix + strconv.Itoa(row)
}

func indexToLetters(index int) string {

	base := float64(26)

	// start at the base that is bigger and work your way down
	floatIndex := float64(index)
	leftOver := floatIndex

	columns := []int{}

	for leftOver > 0 {
		remainder := math.Mod(leftOver, base)

		if remainder == 0 {
			remainder = base
		}

		columns = append([]int{int(remainder)}, columns...)
		leftOver = (leftOver - remainder) / base
	}

	var buff bytes.Buffer

	for _, e := range columns {
		buff.WriteRune(toChar(e))
	}

	return buff.String()

}

func lettersToIndex(letters string) int {

	columns := len(letters) - 1
	sum := 0
	base := 26

	for _, e := range letters {
		number := int(e-'0') - 16

		sum += number * int((math.Pow(float64(base), float64(columns))))

		columns--
	}

	return sum
}

func convertToBool(dv *DynamicValue) *DynamicValue {

	boolDv := DynamicValue{ValueType: DynamicValueTypeBool, DataBool: false}

	if dv.ValueType == DynamicValueTypeBool {
		return dv
	} else if dv.ValueType == DynamicValueTypeFloat {
		if dv.DataFloat != 0 {
			boolDv.DataBool = true
		}
	} else if dv.ValueType == DynamicValueTypeString {
		if len(dv.DataString) != 0 {
			boolDv.DataBool = true
		}
	}

	return &boolDv

}

func convertToString(dv *DynamicValue) *DynamicValue {

	if dv.ValueType == DynamicValueTypeBool {
		dv.DataString = "FALSE"
		if dv.DataBool {
			dv.DataString = "TRUE"
		}
	} else if dv.ValueType == DynamicValueTypeFloat {
		dv.DataString = strconv.FormatFloat(float64(dv.DataFloat), 'f', -1, 64)

		// 10 arbitrarily chosen but covers most situations
		// if len(dv.DataString) > 10 {
		// 	dv.DataString = strconv.FormatFloat(float64(dv.DataFloat), 'f', 10, 64)
		// }

	}

	return dv
}

func isCellEmpty(dv *DynamicValue) bool {
	return len(dv.DataFormula) == 0
}

func convertToFloat(dv *DynamicValue) *DynamicValue {

	if !(dv.ValueType == DynamicValueTypeBool ||
		dv.ValueType == DynamicValueTypeFloat ||
		dv.ValueType == DynamicValueTypeString) {

		fmt.Println("Can't convert any other type to float")

		return &DynamicValue{ValueType: DynamicValueTypeFloat}
	}

	if dv.ValueType == DynamicValueTypeString {

		// first remove all whitespace from string
		strippedString := strings.TrimSpace(dv.DataString)

		value, err := strconv.ParseFloat(strippedString, 64)
		if err != nil {
			fmt.Println("Can't make number from " + dv.DataString)

			return &DynamicValue{ValueType: DynamicValueTypeFloat}
		}
		dv.DataFloat = value
	}
	if dv.ValueType == DynamicValueTypeBool {
		dv.DataFloat = 0
		if dv.DataBool {
			dv.DataFloat = 1
		}
	}

	dv.ValueType = DynamicValueTypeFloat

	return dv

}

func average(arguments []*DynamicValue, grid *Grid) *DynamicValue {

	var total float64
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {
			dvs := getDvsFromReferenceRange(getRangeReferenceFromString(dv.DataString, dv.SheetIndex, grid), grid)
			dv = average(dvs, grid)
		} else {
			dv = convertToFloat(dv)
		}

		total += dv.DataFloat
	}

	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: total / float64(len(arguments))}
}

func getDvsFromReferenceRange(referenceRange ReferenceRange, grid *Grid) []*DynamicValue {

	references := getReferencesFromRange(referenceRange)

	dvs := []*DynamicValue{}

	for _, ref := range references {
		dvs = append(dvs, getDataFromRef(ref, grid))
	}

	return dvs
}

func getReferencesFromRange(referenceRange ReferenceRange) []Reference {

	// get range
	cells := strings.Split(referenceRange.String, ":")

	column1 := getReferenceColumnIndex(cells[0])
	row1 := getReferenceRowIndex(cells[0])

	column2 := getReferenceColumnIndex(cells[1])
	row2 := getReferenceRowIndex(cells[1])

	references := []Reference{}

	for x := column1; x <= column2; x++ {
		for y := row1; y <= row2; y++ {
			references = append(references, Reference{String: indexToLetters(x) + strconv.Itoa(y), SheetIndex: referenceRange.SheetIndex})
		}
	}

	return references

}

func getReferenceColumnIndex(ref string) int {
	ref = strings.Replace(ref, "$", "", -1)
	return lettersToIndex(numberOnlyReg.FindAllString(ref, -1)[0])
}
func getReferenceRowIndex(ref string) int {
	ref = strings.Replace(ref, "$", "", -1)
	row, _ := strconv.Atoi(numberOnlyReg.ReplaceAllString(ref, ""))
	return row
}

func count(arguments []*DynamicValue, grid *Grid) *DynamicValue {

	var countValue float64
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {
			dvs := getDvsFromReferenceRange(getRangeReferenceFromString(dv.DataString, dv.SheetIndex, grid), grid)
			dv = count(dvs, grid)
			countValue += dv.DataFloat
		} else {
			dv = convertToString(dv)
		}

		if len(dv.DataString) > 0 {
			countValue++
		}
	}

	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: countValue}
}

func sum(arguments []*DynamicValue, grid *Grid) *DynamicValue {

	var total float64
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {

			var dvs []*DynamicValue

			if strings.Contains(dv.DataString, ":") {
				rangeRef := getRangeReferenceFromString(dv.DataString, dv.SheetIndex, grid)
				dvs = getDvsFromReferenceRange(rangeRef, grid)
			} else {
				dvs = []*DynamicValue{getDataFromRef(getReferenceFromString(dv.DataString, dv.SheetIndex, grid), grid)}
			}

			dv = sum(dvs, grid)

		} else {
			dv = convertToFloat(dv)
		}

		total += dv.DataFloat
	}

	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: total}
}

func ifFunc(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 3 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "IF requires 3 params"}
	}
	if arguments[0].ValueType != DynamicValueTypeBool {
		arguments[0] = convertToBool(arguments[0])
	}

	if arguments[0].DataBool {
		return arguments[1]
	} else {
		return arguments[2]
	}

}

func mathConstant(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "MATH.C only takes one argument"}
	}

	switch constant := arguments[0].DataString; constant {
	case "e", "E":
		return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.E}
	case "π", "pi", "PI", "Pi":
		return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.Pi}
	}

	// couldn't find constant (didn't return before)
	return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "constant requested not found: " + arguments[0].DataString}

}

func sqrt(arguments []*DynamicValue) *DynamicValue {
	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "SQRT only takes one argument"}
	}

	floatDv := convertToFloat(arguments[0])

	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.Sqrt(floatDv.DataFloat)}
}

func concatenate(arguments []*DynamicValue) *DynamicValue {
	var buff bytes.Buffer

	for _, e := range arguments {
		if e.ValueType != DynamicValueTypeString {
			e = convertToString(e)
		}
		buff.WriteString(e.DataString)
	}
	return &DynamicValue{ValueType: DynamicValueTypeString, DataString: buff.String()}
}
func number(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "NUMBER only supports one argument"}
	}

	return convertToFloat(arguments[0])
}

func floor(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "FLOOR only supports one argument"}
	}

	dv := convertToFloat(arguments[0])

	dv.DataFloat = math.Floor(dv.DataFloat)
	dv.ValueType = DynamicValueTypeFloat

	return dv
}

func ceil(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "CEIL only supports one argument"}
	}

	dv := convertToFloat(arguments[0])

	dv.DataFloat = math.Ceil(dv.DataFloat)
	return dv
}

func length(arguments []*DynamicValue) *DynamicValue {

	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "LEN only supports one argument"}
	}

	stringValue := convertToString(arguments[0]).DataString

	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: float64(len(stringValue))}
}

func random() *DynamicValue {
	return &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: rand.Float64()}
}

func isExplosiveFormula(formula string) bool {

	explosiveFormulas := []string{"OLS"}

	for _, explosiveFormula := range explosiveFormulas {
		if strings.Contains(formula, explosiveFormula+"(") {
			return true
		}
	}
	return false
}

func olsExplosive(arguments []*DynamicValue, grid *Grid, targetRef Reference) *DynamicValue {

	// TESTING
	// set the cell below and right to this cell for testing
	targetCellColumn := getReferenceColumnIndex(targetRef.String)
	targetCellRow := getReferenceRowIndex(targetRef.String)

	// Algorithm for OLS

	// For explosive values convert existing cells (e.g.) set them to proper new value

	// first determine number of independents
	independentsCount := len(arguments) - 1 // minus Y

	// get data for x's
	xDataSets := [][]float64{}

	var dataSize int

	for x := 0; x < independentsCount; x++ {

		thisXDVs := getDvsFromReferenceRange(getRangeReferenceFromString(arguments[x+1].DataString, targetRef.SheetIndex, grid), grid) // plus one, x ranges start at index 1

		// set dataSize based on getDataRange
		dataSize = len(thisXDVs)

		row := []float64{}

		for _, dv := range thisXDVs {

			// convert to float
			dv := convertToFloat(dv)
			row = append(row, dv.DataFloat)

		}
		xDataSets = append(xDataSets, row)

	}

	// add the ones for the constant
	ones := []float64{}
	for x := 0; x < dataSize; x++ {
		ones = append(ones, 1)
	}

	// prepend xDataSets
	xDataSets = append([][]float64{ones}, xDataSets...)

	// get the y values
	yDataSet := []float64{}

	yDVs := getDvsFromReferenceRange(getRangeReferenceFromString(arguments[0].DataString, targetRef.SheetIndex, grid), grid)

	for _, dv := range yDVs {
		dv := convertToFloat(dv)
		yDataSet = append(yDataSet, dv.DataFloat)
	}

	// have xDataSets and yDataSets do matrix algebra
	Y := matrix.MakeDenseMatrixStacked([][]float64{yDataSet}).Transpose()
	// fmt.Println(Y)

	// stacked needs transpose
	X := matrix.MakeDenseMatrixStacked(xDataSets).Transpose()
	// fmt.Println(X)

	Xt := X.Transpose()
	XtX, _ := Xt.Times(X)
	XtY, _ := Xt.Times(Y)
	XtXi, _ := XtX.DenseMatrix().Inverse()
	B, _ := XtXi.Times(XtY)

	yPredicts := []float64{}
	residuals := []float64{}

	// loop over elements
	for i := 0; i < dataSize; i++ {

		// loop over independents
		yPredict := B.Get(0, 0)
		for x := 1; x <= independentsCount; x++ {
			yPredict += B.Get(x, 0) * xDataSets[x][i]
		}
		yPredicts = append(yPredicts, yPredict)

		residual := yDataSet[i] - yPredict
		residuals = append(residuals, residual)

	}

	// FOR NOW: compute and DV setting seperated, could be merged later for performance

	// now y_predicts, residuals and betas are known output cells

	// y_predicts
	for key, yPredict := range yPredicts {
		thisIndex := indexToLetters(targetCellColumn+1) + strconv.Itoa(targetCellRow+key+1) // new index is below the targetRef (two because labels)
		explosionSetValue(Reference{String: thisIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: yPredict}, grid)
	}
	// residuals
	for key, residual := range residuals {
		thisIndex := indexToLetters(targetCellColumn+2) + strconv.Itoa(targetCellRow+key+1) // new index is below the targetRef (two because labels)
		explosionSetValue(Reference{String: thisIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: residual}, grid)
	}

	// co-efficients
	for i := 0; i < independentsCount+1; i++ { // also beta 1 for intercept

		coefficientLabelIndex := indexToLetters(targetCellColumn+3) + strconv.Itoa(targetCellRow+i)
		explosionSetValue(Reference{String: coefficientLabelIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeString, DataString: "beta " + strconv.Itoa(i+1)}, grid)

		coefficientIndex := indexToLetters(targetCellColumn+4) + strconv.Itoa(targetCellRow+i)
		explosionSetValue(Reference{String: coefficientIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: B.Get(i, 0)}, grid)
	}

	// labels
	yPredictsLabelIndex := indexToLetters(targetCellColumn+1) + strconv.Itoa(targetCellRow)
	explosionSetValue(Reference{String: yPredictsLabelIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeString, DataString: "y hat"}, grid)

	residualsLabelIndex := indexToLetters(targetCellColumn+2) + strconv.Itoa(targetCellRow)
	explosionSetValue(Reference{String: residualsLabelIndex, SheetIndex: targetRef.SheetIndex}, &DynamicValue{ValueType: DynamicValueTypeString, DataString: "residuals"}, grid)

	olsDv := getDataFromRef(targetRef, grid)
	olsDv.DataString = "OLS Regression"

	// OLS also returns a DynamicValue itself
	return olsDv
}

func explosionSetValue(ref Reference, dataDv *DynamicValue, grid *Grid) {

	OriginalDependOut := getDataFromRef(ref, grid).DependOut

	dataDv.DependIn = make(map[string]bool) // new dependin (new formula)
	dataDv.DependOut = OriginalDependOut    // dependout remain

	// TODO for now add formula so re-compute succeeds: later optimize for performance
	if dataDv.ValueType == DynamicValueTypeString {
		dataDv.DataFormula = "\"" + dataDv.DataString + "\""
	} else if dataDv.ValueType == DynamicValueTypeFloat {
		dataDv.DataFormula = strconv.FormatFloat(dataDv.DataFloat, 'f', -1, 64)
	} else if dataDv.ValueType == DynamicValueTypeBool {
		dataDv.DataFormula = "false"
		if dataDv.DataBool {
			dataDv.DataFormula = "true"
		}
	}

	setDataByRef(ref, setDependencies(ref, dataDv, grid), grid)
}

func vlookup(arguments []*DynamicValue, grid *Grid, targetRef Reference) *DynamicValue {
	if len(arguments) != 3 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "VLOOKUP only supports 3 arguments"}
	}

	stringSearchValue := convertToString(arguments[0])

	vlookupRange := getRangeReferenceFromString(arguments[1].DataString, targetRef.SheetIndex, grid)

	// now take only the first column
	rangeReferences := strings.Split(vlookupRange.String, ":")
	firstReference := rangeReferences[0]
	secondReference := rangeReferences[1]

	searchRangeColumn := getReferenceColumnIndex(firstReference)
	searchRangeStartRow := getReferenceRowIndex(firstReference)
	searchRangeEndRow := getReferenceRowIndex(secondReference)

	searchRange := ReferenceRange{String: indexesToReferenceString(searchRangeStartRow, searchRangeColumn) + ":" + indexesToReferenceString(searchRangeEndRow, searchRangeColumn), SheetIndex: vlookupRange.SheetIndex}

	rangeDvs := getDvsFromReferenceRange(searchRange, grid)

	returnColumnIndex := int(arguments[2].DataFloat) - 1

	for index, dv := range rangeDvs {
		checkStringValue := convertToString(dv)

		if checkStringValue.DataString == stringSearchValue.DataString {

			stringMapReference := strconv.Itoa(int(vlookupRange.SheetIndex)) + "!" + indexesToReferenceString(searchRangeStartRow+index, searchRangeColumn+returnColumnIndex)

			return copyDv(getDataByNormalRef(stringMapReference, grid))
		}
	}
	notFoundDv := makeEmptyDv()
	notFoundDv.DataString = "#NOTFOUND"
	return notFoundDv
}

func abs(arguments []*DynamicValue) *DynamicValue {
	if len(arguments) != 1 {
		return &DynamicValue{ValueType: DynamicValueTypeString, DataString: "ABS only supports one argument"}
	}
	dv := arguments[0]
	dv = convertToFloat(dv)
	dv.DataFloat = math.Abs(dv.DataFloat)
	return dv
}
func executeCommand(command string, arguments []*DynamicValue, grid *Grid, targetRef Reference) *DynamicValue {

	switch command := command; command {
	case "SUM":
		return sum(arguments, grid)
	case "AVERAGE":
		return average(arguments, grid)
	case "IF":
		return ifFunc(arguments)
	case "MATHC":
		return mathConstant(arguments)
	case "SQRT":
		return sqrt(arguments)
	case "CONCATENATE", "CONCAT":
		return concatenate(arguments)
	case "NUMBER":
		return number(arguments)
	case "LEN":
		return length(arguments)
	case "COUNT":
		return count(arguments, grid)
	case "RAND":
		return random()
	case "FLOOR":
		return floor(arguments)
	case "CEIL":
		return ceil(arguments)
	case "ABS":
		return abs(arguments)
	case "VLOOKUP":
		return vlookup(arguments, grid, targetRef)
	case "OLS":
		return olsExplosive(arguments, grid, targetRef)
	default:

		argumentStrings := []string{}

		for _, dv := range arguments {
			stringDv := convertToString(dv)
			argumentStrings = append(argumentStrings, stringDv.DataString)
		}

		// send command to Python
		if len(arguments) != 0 {
			grid.PythonClient <- "parseCall(\"" + command + "\", \"" + strings.Join(argumentStrings, "\",\"") + "\")"
		} else {
			grid.PythonClient <- "parseCall(\"" + command + "\")"
		}
		// fmt.Println("Posted message to Python CMD")

		// loop until result is back
		for {
			select {
			case pythonResult := <-grid.PythonResultChannel:
				// fmt.Println("Received message from Python to return parse()")
				newDv := DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: pythonResult}
				return parse(&newDv, grid, targetRef)
			}
		}

	}

}
