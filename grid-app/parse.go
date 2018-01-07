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

	matrix "github.com/skelterjohn/go.matrix"
)

const DynamicValueTypeFormula int8 = 0
const DynamicValueTypeReference int8 = 1
const DynamicValueTypeInteger int8 = 2
const DynamicValueTypeFloat int8 = 3
const DynamicValueTypeString int8 = 4
const DynamicValueTypeBool int8 = 5
const DynamicValueTypeExplosiveFormula int8 = 6
const DynamicValueTypeOperator int8 = 7

type DynamicValue struct {
	ValueType     int8
	DataFloat     float64
	DataInteger   int32
	DataString    string
	DataBool      bool
	DataFormula   string
	DependIn      *map[string]bool
	DependOut     *map[string]bool
	DependInTemp  *map[string]bool
	DependOutTemp *map[string]bool
}

var numberOnlyReg *regexp.Regexp
var numberOnlyFilter *regexp.Regexp

var availableOperators []string
var maxOperatorSize int
var operatorsBroken []map[string]int
var breakChars []string

func makeDv(formula string) DynamicValue {
	dv := DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: formula}

	DependIn := make(map[string]bool)
	DependOut := make(map[string]bool)

	dv.DependIn = &DependIn
	dv.DependOut = &DependOut

	return dv
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

func getData(ref1 DynamicValue, grid *Grid) DynamicValue {
	return (grid.data)[ref1.DataString]
}

func findInMap(amap map[int]string, value string) bool {
	for _, e := range amap {
		if e == value {
			return true
		}
	}
	return false
}

func incrementSingleReferences(formula string, incrementAmount int) string {

	// strategy

	// create a hash map of old vs new references, after scanning the formula, do mass replace action (deals with increment in reference length issue)
	references := make(map[string]string)

	// loop over string, if double quote is found, ignore input for references,
	quoteLevel := 0
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

		if quoteLevel == 0 {

			// assume everything not in quotes is a reference
			// if we find open brace it must be a function name
			if char == "(" {
				buf.Reset()
				continue
			} else if contains(breakChars, char) {

				if buf.Len() > 0 {
					// found space, previous is reference

					// check whether singular or plural reference
					reference := buf.String()

					if !strings.Contains(reference, ":") {
						// ignore the plural references (for now)

						// increment reference
						addToReferenceReplaceMap(reference, incrementAmount, &references)
					}

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

		reference := buf.String()

		if !strings.Contains(reference, ":") {
			// ignore the plural references (for now)

			// increment reference
			addToReferenceReplaceMap(reference, incrementAmount, &references)
		}
	}

	// parse references
	for oldReference, newReference := range references {
		formula = strings.Replace(formula, oldReference, newReference, -1)
	}

	return formula
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

func findReferences(formula string) map[string]bool {

	references := []string{}

	// loop over string, if double quote is found, ignore input for references,
	quoteLevel := 0
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

		if quoteLevel == 0 {

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

	// expand references when necessary
	for k, reference := range references {

		if strings.Contains(reference, ":") {

			// remove from references

			// split operation (if 1, or 0 just replace with empty, if bigger replace)
			if len(references) > 1 {
				references = append(references[:k-1], references[k+1:]...)
			} else {
				references = []string{}
			}

			// split
			references = append(references, cellRangeToCells(reference)...)
		}
	}

	finalMap := make(map[string]bool)

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

func cellRangeToCells(reference string) []string {
	references := []string{}

	cells := strings.Split(reference, ":")

	cell1Row := getReferenceRowIndex(cells[0])
	cell2Row := getReferenceRowIndex(cells[1])

	cell1Column := getReferenceColumnIndex(cells[0])
	cell2Column := getReferenceColumnIndex(cells[1])

	for x := cell1Column; x <= cell2Column; x++ {
		for y := cell1Row; y <= cell2Row; y++ {
			references = append(references, indexToLetters(x)+strconv.Itoa(y))
		}
	}

	return references
}

func setDependencies(index string, dv DynamicValue, grid *Grid) DynamicValue {

	var references map[string]bool
	// explosiveFormulas never have dependencies
	if dv.ValueType == DynamicValueTypeExplosiveFormula {
		references = make(map[string]bool)
	} else {
		references = findReferences(dv.DataFormula)
	}

	for ref, inSet := range references {
		if inSet {
			if ref == index {
				// cell is dependent on self
				log.Fatal("Circular reference error")
			}
			(*dv.DependIn)[ref] = true

			(*(grid.data)[ref].DependOut)[index] = true

			// copy
			copyToDirty((grid.data)[ref], ref, grid)

		}
	}

	// always add self to dirty (after setting references DependIn for self - loop above)
	copyToDirty(dv, index, grid)

	// mark all cells dirty that depend on this cell
	for ref, inSet := range *dv.DependOut {
		if inSet {

			dv := (grid.data)[ref]
			copyToDirty(dv, ref, grid)
		}
	}

	return dv
}

func copyToDirty(dv DynamicValue, index string, grid *Grid) {

	// only add if not already in
	if _, ok := (grid.dirtyCells)[index]; !ok {

		// copy the DependIn/DependOut maps to retain original
		DependInTemp := make(map[string]bool)
		DependOutTemp := make(map[string]bool)

		dv.DependInTemp = &DependInTemp
		dv.DependOutTemp = &DependOutTemp

		// copy in
		(grid.dirtyCells)[index] = dv
	}

	// always copy dependencies
	for ref, inSet := range *dv.DependIn {
		(*(grid.dirtyCells)[index].DependInTemp)[ref] = inSet
	}
	for ref, inSet := range *dv.DependOut {
		(*(grid.dirtyCells)[index].DependOutTemp)[ref] = inSet

		// if outgoing dependency not in dirtCells, add it now
		if _, ok := (grid.dirtyCells)[ref]; !ok {
			copyToDirty((grid.data)[ref], ref, grid)
		}

	}

}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func parse(formula DynamicValue, grid *Grid, targetRef string) DynamicValue {

	if formula.ValueType == DynamicValueTypeFloat || formula.ValueType == DynamicValueTypeInteger {
		return formula
	}
	if formula.ValueType == DynamicValueTypeFormula && len(formula.DataFormula) == 0 {
		formula.DataString = ""
		formula.ValueType = DynamicValueTypeString
		return formula
	}

	elements := []DynamicValue{}
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
							elements = append(elements, DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: strings.TrimSpace(buffer.String())})

							// add operator to elements
							elements = append(elements, DynamicValue{ValueType: DynamicValueTypeOperator, DataFormula: identifiedOperator})

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

	elements = append(elements, DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: strings.TrimSpace(buffer.String())})

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

			arguments = append(arguments, strings.TrimSpace(buffer.String()))

			var argumentFormulas []DynamicValue

			argumentFormulas = []DynamicValue{}

			for _, e := range arguments {
				argumentFormulas = append(argumentFormulas, parse(DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: e}, grid, targetRef))
			}

			return executeCommand(command, argumentFormulas, grid, targetRef)

		} else {

			if strings.Index(singleElement.DataFormula, ":") != -1 {

				cells := strings.Split(singleElement.DataFormula, ":")

				if !((numberOnlyFilter.MatchString(cells[0]) && numberOnlyFilter.MatchString(cells[1])) ||
					(!numberOnlyFilter.MatchString(cells[0]) && !numberOnlyFilter.MatchString(cells[1]))) {

					log.Fatal("Wrong reference specifier")

				} else {
					return DynamicValue{ValueType: DynamicValueTypeReference, DataString: singleElement.DataFormula}
				}

			} else if singleElement.DataFormula[0:1] == "\"" && singleElement.DataFormula[len(singleElement.DataFormula)-1:len(singleElement.DataFormula)] == "\"" {

				return DynamicValue{ValueType: DynamicValueTypeString, DataString: singleElement.DataFormula[1 : len(singleElement.DataFormula)-1]}

			} else if numberOnlyFilter.MatchString(singleElement.DataFormula) {

				floatValue, err := strconv.ParseFloat(singleElement.DataFormula, 64)
				if err != nil {
					log.Fatal(err)
				}

				return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: float64(floatValue)}

			} else {

				singleElement.ValueType = DynamicValueTypeReference
				singleElement.DataString = singleElement.DataFormula

				return getData(singleElement, grid)

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
						if (LHS.ValueType == DynamicValueTypeInteger || LHS.ValueType == DynamicValueTypeFloat) &&
							(RHS.ValueType == DynamicValueTypeInteger || RHS.ValueType == DynamicValueTypeFloat) {

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
					elements = append(elements[:operatorLocation-1], result)
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
	} else if A.ValueType == DynamicValueTypeInteger {
		return A.DataInteger > 0
	} else if A.ValueType == DynamicValueTypeString {
		return len(A.DataString) > 0
	} else {
		return false
	}
}

func booleanCompare(LHS DynamicValue, RHS DynamicValue, operator string) DynamicValue {

	// for now, cast all values to float for comparison

	var result DynamicValue

	var A float64
	var B float64

	// boolean operators
	if LHS.ValueType == DynamicValueTypeBool {
		A = 0
		if LHS.DataBool {
			A = 1
		}
	}
	if LHS.ValueType == DynamicValueTypeFloat {
		A = LHS.DataFloat
	}
	if LHS.ValueType == DynamicValueTypeInteger {
		A = float64(LHS.DataInteger)
	}

	// parse RHS dynamically
	if RHS.ValueType == DynamicValueTypeBool {
		B = 0
		if RHS.DataBool {
			B = 1
		}
	}
	if RHS.ValueType == DynamicValueTypeFloat {
		B = RHS.DataFloat
	}
	if RHS.ValueType == DynamicValueTypeInteger {
		B = float64(RHS.DataInteger)
	}

	switch operator {
	case ">":
		result.DataBool = A > B
		result.ValueType = DynamicValueTypeBool
	case "<":
		result.DataBool = A < B
		result.ValueType = DynamicValueTypeBool
	case ">=":
		result.DataBool = A >= B
		result.ValueType = DynamicValueTypeBool
	case "<=":
		result.DataBool = A <= B
		result.ValueType = DynamicValueTypeBool
	case "==":
		result.DataBool = A == B
		result.ValueType = DynamicValueTypeBool
	case "!=":
		result.DataBool = A != B
		result.ValueType = DynamicValueTypeBool
	case "<>":
		result.DataBool = A != B
		result.ValueType = DynamicValueTypeBool
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

func indexOfOperator(ds []DynamicValue, s string) int {
	for k, e := range ds {
		if e.DataFormula == s {
			return k
		}
	}
	return -1
}

func findFirstOperatorOccurence(elements []DynamicValue, operatorSet []string) int {

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

func convertToString(dv DynamicValue) DynamicValue {

	if dv.ValueType == DynamicValueTypeBool {
		dv.DataString = "false"
		if dv.DataBool {
			dv.DataString = "true"
		}
	} else if dv.ValueType == DynamicValueTypeFloat {
		dv.DataString = strconv.FormatFloat(float64(dv.DataFloat), 'f', -1, 64)

		// 10 arbitrarily chosen but covers most situations
		if len(dv.DataString) > 10 {
			dv.DataString = strconv.FormatFloat(float64(dv.DataFloat), 'f', 10, 64)
		}

	} else if dv.ValueType == DynamicValueTypeInteger {
		dv.DataString = strconv.Itoa(int(dv.DataInteger))
	}

	return dv
}

func convertToFloat(dv DynamicValue) DynamicValue {

	if !(dv.ValueType == DynamicValueTypeBool ||
		dv.ValueType == DynamicValueTypeFloat ||
		dv.ValueType == DynamicValueTypeInteger ||
		dv.ValueType == DynamicValueTypeString) {

		fmt.Println("Can't convert any other type to float")

		return DynamicValue{ValueType: DynamicValueTypeFloat}
	}

	if dv.ValueType == DynamicValueTypeInteger {
		dv.DataFloat = float64(dv.DataInteger)
	}

	if dv.ValueType == DynamicValueTypeString {

		// first remove all whitespace from string
		strippedString := strings.TrimSpace(dv.DataString)

		value, err := strconv.ParseFloat(strippedString, 64)
		if err != nil {
			fmt.Println("Can't make number from " + dv.DataString)

			return DynamicValue{ValueType: DynamicValueTypeFloat}
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

func average(arguments []DynamicValue, grid *Grid) DynamicValue {

	var total float64
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {
			dvs := getDataRange(dv, grid)
			dv = average(dvs, grid)
		} else {
			dv = convertToFloat(dv)
		}

		total += dv.DataFloat
	}

	return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: total / float64(len(arguments))}
}

func getDataRange(dv DynamicValue, grid *Grid) []DynamicValue {

	if dv.ValueType == DynamicValueTypeReference {

		// get range
		cells := strings.Split(dv.DataString, ":")

		column1 := getReferenceColumnIndex(cells[0])
		row1 := getReferenceRowIndex(cells[0])

		column2 := getReferenceColumnIndex(cells[1])
		row2 := getReferenceRowIndex(cells[1])

		dvs := []DynamicValue{}

		for x := column1; x <= column2; x++ {
			for y := row1; y <= row2; y++ {
				dvs = append(dvs, (grid.data)[indexToLetters(x)+strconv.Itoa(y)])
			}
		}

		return dvs

	} else {
		log.Fatal("Tried to get range of non-range DV")
	}

	return []DynamicValue{DynamicValue{}}
}

func getReferenceColumnIndex(ref string) int {
	return lettersToIndex(numberOnlyReg.FindAllString(ref, -1)[0])
}
func getReferenceRowIndex(ref string) int {
	row, _ := strconv.Atoi(numberOnlyReg.ReplaceAllString(ref, ""))
	return row
}

func count(arguments []DynamicValue, grid *Grid) DynamicValue {

	var countValue int32
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {
			dvs := getDataRange(dv, grid)
			dv = count(dvs, grid)
			countValue += dv.DataInteger
		} else {
			dv = convertToString(dv)
		}

		if len(dv.DataString) > 0 {
			countValue++
		}
	}

	return DynamicValue{ValueType: DynamicValueTypeInteger, DataInteger: countValue}
}

func sum(arguments []DynamicValue, grid *Grid) DynamicValue {

	var total float64
	for _, dv := range arguments {

		// check if argument is range
		if dv.ValueType == DynamicValueTypeReference {
			dvs := getDataRange(dv, grid)
			dv = sum(dvs, grid)
		} else {
			dv = convertToFloat(dv)
		}

		total += dv.DataFloat
	}

	return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: total}
}

func ifFunc(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 3 {
		log.Fatal("IF function requires 3 arguments")
	}
	if arguments[0].ValueType != DynamicValueTypeBool {
		log.Fatal("IF 1st parameter must be boolean")
	}

	if arguments[0].DataBool {
		return arguments[1]
	} else {
		return arguments[2]
	}

}

func mathConstant(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 1 {
		log.Fatal("MATH.C only takes one argument")
	}

	switch constant := arguments[0].DataString; constant {
	case "e", "E":
		return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.E}
	case "π", "pi", "PI", "Pi":
		return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.Pi}
	}

	// couldn't find constant (didn't return before)
	log.Fatal("constant requested not found" + arguments[0].DataString)

	// will never be reached, will error before
	return DynamicValue{}
}

func sqrt(arguments []DynamicValue) DynamicValue {
	if len(arguments) != 1 {
		log.Fatal("SQRT only takes one argument")
	}

	floatDv := convertToFloat(arguments[0])

	return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: math.Sqrt(floatDv.DataFloat)}
}

func concatenate(arguments []DynamicValue) DynamicValue {
	var buff bytes.Buffer

	for _, e := range arguments {
		if e.ValueType != DynamicValueTypeString {
			e = convertToString(e)
		}
		buff.WriteString(e.DataString)
	}
	return DynamicValue{ValueType: DynamicValueTypeString, DataString: buff.String()}
}
func number(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 1 {
		log.Fatal("NUMBER only supports one argument")
	}

	return convertToFloat(arguments[0])
}

func floor(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 1 {
		log.Fatal("LEN only supports one argument")
	}

	dv := convertToFloat(arguments[0])

	dv.DataFloat = math.Floor(dv.DataFloat)

	dv.DataInteger = int32(dv.DataFloat)
	dv.ValueType = DynamicValueTypeInteger

	return dv
}

func ceil(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 1 {
		log.Fatal("LEN only supports one argument")
	}

	dv := convertToFloat(arguments[0])

	dv.DataFloat = math.Ceil(dv.DataFloat)

	dv.DataInteger = int32(dv.DataFloat)
	dv.ValueType = DynamicValueTypeInteger

	return dv
}

func length(arguments []DynamicValue) DynamicValue {

	if len(arguments) != 1 {
		log.Fatal("LEN only supports one argument")
	}

	stringValue := convertToString(arguments[0]).DataString

	return DynamicValue{ValueType: DynamicValueTypeInteger, DataInteger: int32(len(stringValue))}
}

func random() DynamicValue {
	return DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: rand.Float64()}
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

func olsExplosive(arguments []DynamicValue, grid *Grid, targetRef string) DynamicValue {

	// TESTING
	// set the cell below and right to this cell for testing
	targetCellColumn := getReferenceColumnIndex(targetRef)
	targetCellRow := getReferenceRowIndex(targetRef)

	// Algorithm for OLS

	// For explosive values convert existing cells (e.g.) set them to proper new value

	// first determine number of independents
	independentsCount := len(arguments) - 1 // minus Y

	// get data for x's
	xDataSets := [][]float64{}

	var dataSize int

	for x := 0; x < independentsCount; x++ {

		thisXDVs := getDataRange(arguments[x+1], grid) // plus one, x ranges start at index 1

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

	yDVs := getDataRange(arguments[0], grid)

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
		explosionSetValue(thisIndex, DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: yPredict}, grid)
	}
	// residuals
	for key, residual := range residuals {
		thisIndex := indexToLetters(targetCellColumn+2) + strconv.Itoa(targetCellRow+key+1) // new index is below the targetRef (two because labels)
		explosionSetValue(thisIndex, DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: residual}, grid)
	}

	// co-efficients
	for i := 0; i < independentsCount+1; i++ { // also beta 1 for intercept

		coefficientLabelIndex := indexToLetters(targetCellColumn+3) + strconv.Itoa(targetCellRow+i)
		explosionSetValue(coefficientLabelIndex, DynamicValue{ValueType: DynamicValueTypeString, DataString: "beta " + strconv.Itoa(i+1)}, grid)

		coefficientIndex := indexToLetters(targetCellColumn+4) + strconv.Itoa(targetCellRow+i)
		explosionSetValue(coefficientIndex, DynamicValue{ValueType: DynamicValueTypeFloat, DataFloat: B.Get(i, 0)}, grid)
	}

	// labels
	yPredictsLabelIndex := indexToLetters(targetCellColumn+1) + strconv.Itoa(targetCellRow)
	explosionSetValue(yPredictsLabelIndex, DynamicValue{ValueType: DynamicValueTypeString, DataString: "y hat"}, grid)

	residualsLabelIndex := indexToLetters(targetCellColumn+2) + strconv.Itoa(targetCellRow)
	explosionSetValue(residualsLabelIndex, DynamicValue{ValueType: DynamicValueTypeString, DataString: "residuals"}, grid)

	olsDv := grid.data[targetRef]
	olsDv.DataString = "OLS Regression"

	// OLS also returns a DynamicValue itself
	return olsDv
}

func explosionSetValue(index string, dataDv DynamicValue, grid *Grid) {

	OriginalDependOut := grid.data[index].DependOut

	NewDependIn := make(map[string]bool)
	dataDv.DependIn = &NewDependIn       // new dependin (new formula)
	dataDv.DependOut = OriginalDependOut // dependout remain

	// TODO for now add formula so re-compute succeeds: later optimize for performance
	if dataDv.ValueType == DynamicValueTypeString {
		dataDv.DataFormula = "\"" + dataDv.DataString + "\""
	} else if dataDv.ValueType == DynamicValueTypeFloat {
		dataDv.DataFormula = strconv.FormatFloat(dataDv.DataFloat, 'f', -1, 64)
	} else if dataDv.ValueType == DynamicValueTypeInteger {
		dataDv.DataFormula = strconv.Itoa(int(dataDv.DataInteger))
	} else if dataDv.ValueType == DynamicValueTypeBool {
		dataDv.DataFormula = "false"
		if dataDv.DataBool {
			dataDv.DataFormula = "true"
		}
	}

	grid.data[index] = setDependencies(index, dataDv, grid)

}

func abs(arguments []DynamicValue) DynamicValue {
	if len(arguments) != 1 {
		fmt.Println("ABS requires 1 argument")
	}
	dv := arguments[0]
	dv = convertToFloat(dv)
	dv.DataFloat = math.Abs(dv.DataFloat)
	return dv
}
func executeCommand(command string, arguments []DynamicValue, grid *Grid, targetRef string) DynamicValue {

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
	case "OLS":
		return olsExplosive(arguments, grid, targetRef)
	}

	return DynamicValue{ValueType: DynamicValueTypeInteger, DataInteger: 0}
}
