package main

import (
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/csimplestring/go-csv/detector"
)

type Reference struct {
	String     string
	SheetIndex int8
}

type ReferenceRange struct {
	String     string
	SheetIndex int8
}

type SheetSize struct {
	RowCount    int
	ColumnCount int
}

type Grid struct {
	Data                map[string]*DynamicValue
	DirtyCells          map[string]bool
	ActiveSheet         int8
	SheetNames          map[string]int8
	PerformanceCounting map[string]int
	SheetList           []string
	SheetSizes          []SheetSize
	PythonResultChannel chan string
	PythonClient        chan string
}

func copyToDirty(index string, grid *Grid) {

	// only add
	if _, ok := grid.DirtyCells[index]; !ok {
		grid.DirtyCells[index] = true

		for ref, inSet := range getDataByNormalRef(index, grid).DependOut {
			if inSet {
				copyToDirty(ref, grid)
			}
		}
	} else {
		fmt.Println("Notice: tried to add to dirty twice (" + index + ")")
	}

}

func gridInstance(c *Client) {

	var grid Grid

	defaultColumnCount := 15
	defaultRowCount := 100

	// if Grid serialized file exists try to load that
	sheetFile := c.hub.rootDirectory + "sheetdata/sheet.serialized"
	if _, err := os.Stat(sheetFile); os.IsNotExist(err) {

		// initialize the datastructure for the matrix
		columnCount := defaultColumnCount
		rowCount := defaultRowCount

		sheetSizes := []SheetSize{SheetSize{RowCount: rowCount, ColumnCount: columnCount}, SheetSize{RowCount: rowCount, ColumnCount: columnCount}}

		// For now make this a two way mapping for ordered loops and O(1) access times -- aware of redundancy of state which could cause problems
		sheetNames := make(map[string]int8)
		sheetNames["Sheet1"] = 0
		sheetNames["Sheet2"] = 1

		sheetList := []string{"Sheet1", "Sheet2"}

		grid = Grid{Data: make(map[string]*DynamicValue), PerformanceCounting: make(map[string]int), DirtyCells: make(map[string]bool), ActiveSheet: 0, SheetNames: sheetNames, SheetList: sheetList, SheetSizes: sheetSizes}

		cellCount := 1

		for sheet := 0; sheet <= len(sheetList); sheet++ {
			for x := 1; x <= columnCount; x++ {
				for y := 1; y <= rowCount; y++ {
					dv := makeDv("")
					dv.SheetIndex = int8(sheet)

					// DEBUG: fill with incrementing numbers
					// dv.ValueType = DynamicValueTypeInteger
					// dv.DataInteger = int32(cellCount)
					// dv.DataFormula = strconv.Itoa(cellCount)

					grid.Data[strconv.Itoa(sheet)+"!"+indexToLetters(x)+strconv.Itoa(y)] = dv

					cellCount++
				}
			}
		}

		fmt.Printf("Initialized client with grid of length: %d\n", len(grid.Data))

	} else {

		gridData, err := ioutil.ReadFile(sheetFile)
		if err != nil {
			fmt.Println(err)
		}
		grid = FromGOB64(gridData)

		fmt.Println("Loaded Grid struct from sheet.serialized")

	}

	sendSheets(c, &grid)

	grid.PythonResultChannel = make(chan string, 256)
	grid.PythonClient = c.commands

	c.grid = &grid

	for {
		select {
		case actions, ok := <-c.actions:

			if !ok {
				log.Fatal("Something wrong with channel")
			}

			res := StringJSON{}
			err := json.Unmarshal(actions, &res)
			if err != nil {

				fmt.Println("Error decoding JSON string: ", err)
			}

			parsed := res.Arguments

			if len(actions) > 100 {
				fmt.Println("Received WS in Client actions: " + string(actions[:100]) + "... [truncated]")
			} else {
				fmt.Println("Received WS in Client actions: " + string(actions))
			}

			switch parsed[0] {
			case "RANGE":

				// send value for cell(s)
				// c.send <- []byte(convertToString(grid[parsed[1]]).DataString)

				// for range compute/assign first

				// parsed[2] contains the range ref

				// 1. add all elements to grid

				// 2. then compute setDependencies on all added elements

				// 3. then single call to computeDirty cells

				references := cellRangeToCells(ReferenceRange{String: parsed[2], SheetIndex: getIndexFromString(parsed[3])})

				switch parsed[1] {
				case "SETSINGLE":

					var formula string
					if len(parsed[4]) > 0 {
						formula = parsed[4][1:]
						// formula = referencesToUpperCase(formula)
					} else {
						formula = parsed[4]
					}

					// parsed[3] contains the value (formula)
					newDvs := make(map[Reference]*DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])
					incrementAmount := 0 // start at index 0

					// first add all to grid
					for _, ref := range references {

						thisReference := ref

						dv := getDataFromRef(thisReference, &grid)

						if !isValidFormula(formula) {
							dv.ValueType = DynamicValueTypeString
							dv.DataFormula = "\"Error in formula: " + formula + "\""
						} else {
							dv.ValueType = DynamicValueTypeFormula
							dv.DataFormula = formula
						}

						newDvs[ref] = dv
						incrementAmount++

					}

					for ref, dv := range newDvs {
						setDataByRef(ref, setDependencies(ref, dv, &grid), &grid)
					}

					// now compute all dirty
					changedCells := computeDirtyCells(&grid, c)
					sendDirtyOrInvalidate(changedCells, &grid, c)

				case "SETLIST":

					// Note: SETLIST doesn't support formula insert, only raw data. E.g. numbers or strings

					// values are all values from parsed[3] on
					values := parsed[4:]

					// parsed[3] contains the value (formula)
					// newDvs := make(map[string]DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])

					// first add all to grid
					valuesIndex := 0
					newDvs := make(map[Reference]*DynamicValue)

					for _, ref := range references {

						if checkDataPresenceFromRef(ref, &grid) {
							dv := getDataFromRef(ref, &grid)

							if !isValidFormula(values[valuesIndex]) {
								dv.ValueType = DynamicValueTypeString
								dv.DataFormula = "\"Error in formula: " + values[valuesIndex] + "\""
							} else {

								dv.ValueType = DynamicValueTypeFormula
								dv.DataFormula = values[valuesIndex]
							}

							newDvs[ref] = dv

							valuesIndex++
							if valuesIndex > len(values)-1 {
								break
							}

						} else {
							fmt.Println("Tried writing to cell: " + getMapIndexFromReference(ref) + " which doesn't exist.")
						}

					}

					for ref, dv := range newDvs {
						setDataByRef(ref, setDependencies(ref, dv, &grid), &grid)
					}

					computeDirtyCells(&grid, c)
					invalidateView(&grid, c)

					// then setDependencies for all

					// even though all values, has to be ran for all new values because fields might depend on new input data
					// for ref, dv := range newDvs {
					// 	grid.Data[ref] = setDependencies(ref, dv, &grid)
					// }

					// now compute all dirty
					// computeAndSend(&grid, c)

				}
			case "EXIT":

				c.hub.mainThreadChannel <- "EXIT"

			case "GET":

				sendCellsInRange(ReferenceRange{String: parsed[1], SheetIndex: getIndexFromString(parsed[2])}, &grid, c)

			case "SWITCHSHEET":

				grid.ActiveSheet = getIndexFromString(parsed[1])

			case "JUMPCELL":

				currentCell := parsed[1]
				direction := parsed[2]
				sheetIndex := getIndexFromString(parsed[3])

				findJumpCell(Reference{String: currentCell, SheetIndex: sheetIndex}, direction, &grid, c)

			case "MAXCOLUMNWIDTH":

				columnIndex := getIntFromString(parsed[1])
				sheetIndex := getIndexFromString(parsed[2])

				findMaxColumnWidth(columnIndex, sheetIndex, &grid, c)

			case "GET-FILE":

				getFile(parsed[1], c)

			case "SET-FILE":

				setFile(parsed[1], parsed[2], c)

			case "GET-DIRECTORY":

				getDirectory(parsed[1], c)

			case "ADDSHEET":

				sheetName := parsed[1]

				sheetIndex := int8(len(grid.SheetList))

				grid.SheetNames[sheetName] = int8(sheetIndex)
				grid.SheetList = append(grid.SheetList, sheetName)
				grid.SheetSizes = append(grid.SheetSizes, SheetSize{RowCount: defaultRowCount, ColumnCount: defaultColumnCount})

				// populate new sheet
				for x := 1; x <= defaultColumnCount; x++ {
					for y := 1; y <= defaultRowCount; y++ {
						dv := makeDv("")
						dv.SheetIndex = sheetIndex
						grid.Data[strconv.Itoa(int(sheetIndex))+"!"+indexToLetters(x)+strconv.Itoa(y)] = dv
					}
				}

				sendSheets(c, &grid)

			case "TESTCALLBACK-PING":

				jsonData := []string{"TESTCALLBACK-PONG"}
				json, _ := json.Marshal(jsonData)
				c.send <- json

			case "REMOVESHEET":

				sheetIndex := getIndexFromString(parsed[1])
				removeSheet(sheetIndex, &grid)
				sendSheets(c, &grid)

			case "COPY":

				start := time.Now() // debug

				sourceRange := ReferenceRange{parsed[1], getIndexFromString(parsed[2])}
				destinationRange := ReferenceRange{parsed[3], getIndexFromString(parsed[4])}

				copySourceToDestination(sourceRange, destinationRange, &grid, false)

				elapsed := time.Since(start)                           // debug
				log.Printf("copySourceToDestination took %s", elapsed) // debug

				start = time.Now() // debug

				changedCells := computeDirtyCells(&grid, c)

				elapsed = time.Since(start)                      // debug
				log.Printf("computeDirtyCells took %s", elapsed) // debug

				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "COPYASVALUE":

				sourceRange := ReferenceRange{parsed[1], getIndexFromString(parsed[2])}
				destinationRange := ReferenceRange{parsed[3], getIndexFromString(parsed[4])}

				copyByValue(sourceRange, destinationRange, &grid)

				changedCells := computeDirtyCells(&grid, c)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "CUTASVALUE":

				sourceRange := ReferenceRange{parsed[1], getIndexFromString(parsed[2])}
				destinationRange := ReferenceRange{parsed[3], getIndexFromString(parsed[4])}

				cutByValue(sourceRange, destinationRange, &grid)

				changedCells := computeDirtyCells(&grid, c)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "INSERTROWCOL":

				insertType := parsed[1]
				direction := parsed[2]
				reference := parsed[3]

				insertRowColumn(insertType, direction, reference, c, &grid)

				invalidateView(&grid, c)

			case "DELETEROW":

				referenceString := parsed[1]

				rowIndex := getReferenceRowIndex(referenceString)
				// columnIndex := getReferenceColumnIndex(referenceString)

				cutFromRangeString := indexesToReferenceString(rowIndex+1, 1) + ":" + indexesToReferenceString(grid.SheetSizes[grid.ActiveSheet].RowCount, grid.SheetSizes[grid.ActiveSheet].ColumnCount)

				cutToRangeString := indexesToReferenceString(rowIndex, 1) + ":" + indexesToReferenceString(rowIndex, 1)

				cutFromRange := ReferenceRange{String: cutFromRangeString, SheetIndex: grid.ActiveSheet}
				cutToRange := ReferenceRange{String: cutToRangeString, SheetIndex: grid.ActiveSheet}

				// clear everything in row of reference
				clearCells := cellRangeToCells(ReferenceRange{String: indexesToReferenceString(rowIndex, 1) + ":" + indexesToReferenceString(rowIndex, grid.SheetSizes[grid.ActiveSheet].ColumnCount)})

				for _, k := range clearCells {
					clearCell(k, &grid)
				}

				// move everything below reference up
				cutCells(cutFromRange, cutToRange, &grid, c)

				invalidateView(&grid, c)

			case "DELETECOLUMN":

				referenceString := parsed[1]

				// rowIndex := getReferenceRowIndex(referenceString)
				columnIndex := getReferenceColumnIndex(referenceString)

				cutFromRangeString := indexesToReferenceString(1, columnIndex+1) + ":" + indexesToReferenceString(grid.SheetSizes[grid.ActiveSheet].RowCount, grid.SheetSizes[grid.ActiveSheet].ColumnCount)

				cutToRangeString := indexesToReferenceString(1, columnIndex) + ":" + indexesToReferenceString(1, columnIndex)

				cutFromRange := ReferenceRange{String: cutFromRangeString, SheetIndex: grid.ActiveSheet}
				cutToRange := ReferenceRange{String: cutToRangeString, SheetIndex: grid.ActiveSheet}

				// clear everything in row of reference
				clearCells := cellRangeToCells(ReferenceRange{String: indexesToReferenceString(1, columnIndex) + ":" + indexesToReferenceString(grid.SheetSizes[grid.ActiveSheet].RowCount, columnIndex)})

				for _, k := range clearCells {
					clearCell(k, &grid)
				}

				// move everything below reference up
				cutCells(cutFromRange, cutToRange, &grid, c)

				invalidateView(&grid, c)

			case "CUT":

				sourceRange := parsed[1]
				sourceRangeSheetInt, err := strconv.Atoi(parsed[2])
				if err != nil {
					log.Fatal(err)
				}
				sourceRangeSheetIndex := int8(sourceRangeSheetInt)

				destinationRange := parsed[3]
				destinationRangeSheetInt, err := strconv.Atoi(parsed[4])
				if err != nil {
					log.Fatal(err)
				}
				destinationRangeSheetIndex := int8(destinationRangeSheetInt)

				// clear difference between sourceRange and destinationRange
				changedCells := cutCells(
					ReferenceRange{String: sourceRange, SheetIndex: sourceRangeSheetIndex},
					ReferenceRange{String: destinationRange, SheetIndex: destinationRangeSheetIndex}, &grid, c)

				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "SET":

				// check if formula or normal entry
				if len(parsed[2]) > 0 && parsed[2][0:1] == "=" {

					// TODO: regex check if input is legal

					// for SET commands with formula values update formula to uppercase any references
					formula := parsed[2][1:]
					// formula = referencesToUpperCase(formula)

					if !isValidFormula(formula) {

						sheetIndex := getIndexFromString(parsed[3])
						reference := Reference{String: parsed[1], SheetIndex: sheetIndex}

						dv := getDataFromRef(reference, &grid)
						dv.ValueType = DynamicValueTypeString
						dv.DataFormula = "\"Error in formula: " + formula + "\""

						dv.DependIn = make(map[string]bool) // new dependin (new formula)

						setDataByRef(reference, setDependencies(reference, dv, &grid), &grid)

					} else {

						// check for explosive formulas
						isExplosive := isExplosiveFormula(formula)

						if isExplosive {

							// original Dependends can stay on
							reference := Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}
							dv := getDataFromRef(reference, &grid)

							dv.ValueType = DynamicValueTypeExplosiveFormula
							dv.DataFormula = formula

							// Dependencies are not required, since this cell won't depend on anything given that it's explosive

							// parse explosive formula (also, explosive formulas cannot be nested)
							newDv := parse(dv, &grid, reference)

							// don't need dependend information for parsing, hence assign after parse
							newDv.DependIn = make(map[string]bool)             // new dependin (new formula)
							newDv.DependOut = dv.DependOut                     // dependout remain
							newDv.ValueType = DynamicValueTypeExplosiveFormula // shouldn't be necessary, is return type of olsExplosive()
							newDv.DataFormula = formula                        // re-assigning of formula is usually saved for computeDirty but this will be skipped there

							// add OLS cell to dirty (which needs DependInTemp etc)
							setDataByRef(reference, setDependencies(reference, newDv, &grid), &grid)

							// dependencies will be fulfilled for all cells created by explosion

						} else {
							// set value for cells
							// cut off = for parsing

							// original Dependends
							thisReference := Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}

							dv := getDataFromRef(thisReference, &grid)

							dv.ValueType = DynamicValueTypeFormula
							dv.DataFormula = formula

							setDataByRef(thisReference, setDependencies(thisReference, dv, &grid), &grid)
						}

					}

				} else {

					// else enter as string
					// if user enters non string value, client is reponsible for adding the equals sign.
					// Anything without it won't be parsed as formula.
					reference := Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}

					dv := getDataFromRef(reference, &grid)

					// escape double quotes
					formulaString := strings.Replace(parsed[2], "\"", "\\\"", -1)

					dv.ValueType = DynamicValueTypeString
					dv.DataString = parsed[2]
					dv.DataFormula = "\"" + formulaString + "\""

					// if input is empty string, set formula to empty string without quotes
					if len(parsed[2]) == 0 {
						dv.DataFormula = ""
					}

					newDv := setDependencies(reference, dv, &grid)
					newDv.ValueType = DynamicValueTypeString

					setDataByRef(reference, newDv, &grid)

				}

				changedCells := computeDirtyCells(&grid, c)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "SETSIZE":

				newRowCount, _ := strconv.Atoi(parsed[1])
				newColumnCount, _ := strconv.Atoi(parsed[2])
				sheetIndex := getIndexFromString(parsed[3])

				changeSheetSize(newRowCount, newColumnCount, sheetIndex, c, &grid)

			case "CSV":
				fmt.Println("Received CSV! Size: " + strconv.Itoa(len(parsed[1])))

				// TODO: grow the grid to minimum size
				minColumnSize := 0

				// replace \r\n to \n
				csvString := strings.Replace(parsed[1], "\r\n", "\n", -1)

				// replace \r to \n
				csvString = strings.Replace(csvString, "\r", "\n", -1)
				csvStringReader := strings.NewReader(csvString)
				reader := csv.NewReader(csvStringReader)

				detector := detector.New()
				delimiters := detector.DetectDelimiter(csvStringReader, '"')

				// reset for CSV reader
				csvStringReader.Reset(csvString)

				if len(delimiters) > 0 {
					reader.Comma = []rune(delimiters[0])[0]
				} else {
					reader.Comma = ','
				}

				lineCount := 0

				newDvs := make(map[Reference]*DynamicValue)

				lines := [][]string{}

				for {
					// read just one record, but we could ReadAll() as well
					record, err := reader.Read()
					// end-of-file is fitted into err
					if err == io.EOF {
						break
					} else if err != nil {
						fmt.Println("Error:", err)
						return
					}
					// record is an array of string so is directly printable
					// fmt.Println("Record", lineCount, "is", record, "and has", len(record), "fields")

					lines = append(lines, record)

					if len(record) > minColumnSize {
						minColumnSize = len(record)
					}

					// fmt.Println()
					lineCount++
				}

				minRowSize := lineCount

				newRowCount := grid.SheetSizes[grid.ActiveSheet].RowCount
				newColumnCount := grid.SheetSizes[grid.ActiveSheet].ColumnCount

				if minRowSize > grid.SheetSizes[grid.ActiveSheet].RowCount {
					newRowCount = minRowSize
				}
				if minColumnSize > grid.SheetSizes[grid.ActiveSheet].ColumnCount {
					newColumnCount = minColumnSize
				}

				changeSheetSize(newRowCount, newColumnCount, grid.ActiveSheet, c, &grid)

				lineCount = 0
				for _, line := range lines {

					for i := 0; i < len(line); i++ {

						// for now load CSV file to upper left cell, starting at A1
						cellIndex := indexToLetters(i+1) + strconv.Itoa(lineCount+1)

						inputString := strings.TrimSpace(line[i])

						reference := Reference{String: cellIndex, SheetIndex: grid.ActiveSheet}

						newDv := getDataFromRef(reference, &grid)

						// if not number, escape with quotes
						if !numberOnlyFilter.MatchString(inputString) {
							newDv.ValueType = DynamicValueTypeString
							newDv.DataString = inputString

							escapedStringValue := strings.Replace(inputString, "\"", "\\\"", -1)

							newDv.DataFormula = "\"" + escapedStringValue + "\""

						} else {
							newDv.ValueType = DynamicValueTypeFloat
							newDv.DataFormula = inputString

							floatValue, err := strconv.ParseFloat(inputString, 64)

							if err != nil {
								fmt.Println("Error parsing number: ")
								fmt.Println(err)
							}

							newDv.DataFloat = floatValue
						}

						// this will add it to dirtyCells for re-compute
						newDvs[reference] = newDv
					}

					lineCount++
				}

				for ref, dv := range newDvs {
					setDataByRef(ref, setDependencies(ref, dv, &grid), &grid)
				}

				changedCells := computeDirtyCells(&grid, c)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "EXPORT-CSV":

				fmt.Println("Generating CSV...")

				csvString := generateCSV(&grid)

				fmt.Println("Generating of CSV completed.")

				jsonData := []string{"EXPORT-CSV", csvString}

				json, err := json.Marshal(jsonData)

				if err != nil {
					fmt.Println(err)
				}

				c.send <- json

			case "SAVE":
				fmt.Println("Saving workspace...")

				serializedGrid := ToGOB64(grid)

				err := ioutil.WriteFile(c.hub.rootDirectory+"sheetdata/sheet.serialized", serializedGrid, 0644)

				if err != nil {
					fmt.Println(err)
				}

				c.send <- []byte("[\"SAVED\"]")
			case "SORT":
				sortRange(parsed[1], parsed[2], parsed[3], &grid) // direction (ASC,DESC), range ("A1:B20"), column ("B")
				computeDirtyCells(&grid, c)
				invalidateView(&grid, c)
			}
		}
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

		if quoteDepth == 0 {
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
					// skipNextChar = true
				}

				continue
			}
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

			if inReference && r == '$' && dollarInRow {
				return false
			}

			if inReference && !dollarInRow && r == '$' {
				dollarInRow = true
			}

			if inReference && (r == ':' && []rune(previousChar)[0] != ':') {
				inReference = false
				validReference = false
				referenceFoundRange = true
				foundReferenceMark = false
				dollarInColumn = false
				dollarInRow = false
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
				operatorAllowed = true
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

			if inReference && referenceFoundRange && !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == '$') {

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

func getReferenceRangeFromMapIndex(standardRangeReference string) ReferenceRange {

	referenceParts := strings.Split(standardRangeReference, "!")
	sheetIndex, err := strconv.Atoi(referenceParts[0])

	if err != nil {
		log.Fatal(err)
	}

	return ReferenceRange{String: referenceParts[1], SheetIndex: int8(sheetIndex)}
}

func getReferenceFromMapIndex(standardReference string) Reference {

	referenceParts := strings.Split(standardReference, "!")
	sheetIndex, err := strconv.Atoi(referenceParts[0])

	if err != nil {
		log.Fatal(err)
	}

	return Reference{String: referenceParts[1], SheetIndex: int8(sheetIndex)}
}

func getMapIndexFromReference(reference Reference) string {
	stringRef := strings.Replace(reference.String, "$", "", -1)

	splittedString := strings.Split(stringRef, "!")
	lastPart := splittedString[len(splittedString)-1]

	stringRef = strconv.Itoa(int(reference.SheetIndex)) + "!" + lastPart

	return stringRef
}

func computeDirtyCells(grid *Grid, c *Client) []Reference {

	changedRefs := []Reference{}

	indicateProgress := false
	progressTotal := len(grid.DirtyCells)
	// communicate long computations (at arbitrary boundry 1000):
	if progressTotal > 1000 {
		indicateProgress = true
	}

	/// initialize DependInTemp and DependOutTemp for resolving
	for key, _ := range grid.DirtyCells {

		thisDv := getDataByNormalRef(key, grid)

		thisDv.DependInTemp = make(map[string]bool)
		thisDv.DependOutTemp = make(map[string]bool)

		for ref, inSet := range thisDv.DependIn {
			thisDv.DependInTemp[ref] = inSet
		}
		for ref, inSet := range thisDv.DependOut {
			thisDv.DependOutTemp[ref] = inSet
		}

	}

	// for every DV in dirtyCells clean up the DependInTemp list with refs not in DirtyCells

	// When a cell is not in DirtyCells but IS in the DependInTemp of a cell, it needs to be removed from it since it needs to have zero DependInTemp before it can be evaluated

	noDependInDirtyCells := make(map[string]bool)

	for stringRef, _ := range grid.DirtyCells {

		thisDv := getDataByNormalRef(stringRef, grid)

		for stringRefInner := range thisDv.DependInTemp {

			// if ref is not in dirty cells, remove from depend in
			if _, ok := (grid.DirtyCells)[stringRefInner]; !ok {
				delete(thisDv.DependInTemp, stringRefInner)
			}

		}

		// if after this removal DependInTemp is 0, add to list to be processed
		if len(thisDv.DependInTemp) == 0 {
			noDependInDirtyCells[stringRef] = true
		}

	}

	for len((grid.DirtyCells)) != 0 {

		var dv *DynamicValue
		var index string

		// send progress indicator
		if indicateProgress {

			if len(grid.DirtyCells)%1000 == 0 || len(grid.DirtyCells) == 1 {
				progress := float64(progressTotal-len(grid.DirtyCells)+1) / float64(progressTotal)
				c.send <- []byte("[\"PROGRESSINDICATOR\", " + strconv.FormatFloat(progress, 'E', -1, 64) + "]")
			}

		}

		// remove all DependIn that are not in dirty cells (since not dirty, can use existing values)
		// This step is done in computeDirtyCells because at this point
		// we are certain whether cells are dirty or not

		if len(noDependInDirtyCells) == 0 {
			fmt.Println("Error: should have an element in noDependInDirtyCells when DirtyCells is not empty")

			// circular dependency error in all dirty cells
			for key, _ := range grid.DirtyCells {
				thisDv := getDataByNormalRef(key, grid)
				thisDv.DataString = "\"Circular reference: " + thisDv.DataFormula + "\""
				thisDv.ValueType = DynamicValueTypeString
				changedRefs = append(changedRefs, getReferenceFromMapIndex(key))
			}

			break
		}

		// take first element in noDependInDirtyCells
		for k := range noDependInDirtyCells {
			index = k
			dv = getDataByNormalRef(index, grid)
			break
		}

		// compute thisDv and update all DependOn values
		if dv.DependOutTemp != nil {
			for ref, inSet := range dv.DependOutTemp {
				if inSet {

					// only delete dirty dependencies for cells marked in dirtycells
					if _, ok := (grid.DirtyCells)[ref]; ok {
						delete(getDataByNormalRef(ref, grid).DependInTemp, index)

						if len(getDataByNormalRef(ref, grid).DependInTemp) == 0 {

							// TO VALIDATE: is the below a problem?
							// check to see if every time when trying to add to noDependInDirtyCells it is not contained in it
							// log.Fatal("while removing DependInTemp " + ref + " was already in noDependInDirtyCells")
							noDependInDirtyCells[ref] = true
						}
					}

				}
			}
		}

		// NOTE!!! for now compare, check later if computationally feasible

		// DO COMPUTE HERE
		// stringBefore := convertToString((*grid)[index])

		originalDv := getDataByNormalRef(index, grid)

		// originalIsString := false
		// if originalDv.ValueType == DynamicValueTypeString {
		// originalIsString = true
		// }

		newDv := originalDv

		// re-compute only non explosive formulas and not marked for non-recompute
		if originalDv.ValueType != DynamicValueTypeExplosiveFormula {

			originalDv.ValueType = DynamicValueTypeFormula
			currentReference := getReferenceFromMapIndex(index)
			newDv = parse(originalDv, grid, currentReference)

			newDv.DataFormula = originalDv.DataFormula
			newDv.DependIn = originalDv.DependIn
			newDv.DependOut = originalDv.DependOut
			newDv.SheetIndex = originalDv.SheetIndex

			setDataByRef(currentReference, newDv, grid)

			changedRefs = append(changedRefs, currentReference)

		}

		delete(grid.DirtyCells, index)
		delete(noDependInDirtyCells, index)

	}

	return changedRefs
}

func sendCells(cellsToSend *[][]string, c *Client) {

	jsonData := []string{"SET"}

	// send all dirty cells
	for _, e := range *cellsToSend {
		jsonData = append(jsonData, e[0], e[1], e[2], e[3])
	}

	json, _ := json.Marshal(jsonData)
	c.send <- json

}

func sendSheets(c *Client, grid *Grid) {
	jsonData := []string{"SETSHEETS"}

	for key, value := range grid.SheetList {
		jsonData = append(jsonData, value)
		jsonData = append(jsonData, strconv.Itoa(grid.SheetSizes[key].RowCount))
		jsonData = append(jsonData, strconv.Itoa(grid.SheetSizes[key].ColumnCount))
	}

	json, _ := json.Marshal(jsonData)
	c.send <- json
}

func sendSheetSize(c *Client, sheetIndex int8, grid *Grid) {
	jsonData := []string{"SHEETSIZE", strconv.Itoa(grid.SheetSizes[sheetIndex].RowCount), strconv.Itoa(grid.SheetSizes[sheetIndex].ColumnCount), strconv.Itoa(int(sheetIndex))}
	json, _ := json.Marshal(jsonData)
	c.send <- json
}

func sendDirtyOrInvalidate(changedCells []Reference, grid *Grid, c *Client) {
	// magic number to speed up cell updating
	if len(changedCells) < 100 {
		sendCellsByRefs(changedCells, grid, c)
	} else {
		invalidateView(grid, c)
	}
}

func invalidateView(grid *Grid, c *Client) {

	jsonData := []string{"VIEW-INVALIDATED"}
	json, _ := json.Marshal(jsonData)
	c.send <- json
}

func sendCellsInRange(cellRange ReferenceRange, grid *Grid, c *Client) {

	cells := cellRangeToCells(cellRange)

	cellsToSend := [][]string{}

	for _, reference := range cells {

		dv := getDataFromRef(reference, grid)

		if dv != nil {
			stringAfter := convertToString(dv)
			cellsToSend = append(cellsToSend, []string{relativeReferenceString(reference), stringAfter.DataString, "=" + dv.DataFormula, strconv.Itoa(int(dv.SheetIndex))})
		}

		// cell to string
	}

	sendCells(&cellsToSend, c)
}

func relativeReferenceString(reference Reference) string {
	stringRef := reference.String
	splittedString := strings.Split(stringRef, "!")
	lastPart := splittedString[len(splittedString)-1]
	return lastPart
}

func sendCellsByRefs(refs []Reference, grid *Grid, c *Client) {

	cellsToSend := [][]string{}

	for _, reference := range refs {

		dv := getDataFromRef(reference, grid)
		// cell to string
		stringAfter := convertToString(dv)
		cellsToSend = append(cellsToSend, []string{relativeReferenceString(reference), stringAfter.DataString, "=" + dv.DataFormula, strconv.Itoa(int(dv.SheetIndex))})
	}

	sendCells(&cellsToSend, c)
}

// go binary encoder
func ToGOB64(grid Grid) []byte {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(grid)
	if err != nil {
		fmt.Println(`failed gob Encode`, err)
	}
	return b.Bytes()
}

// go binary decoder
func FromGOB64(binary []byte) Grid {
	grid := Grid{}
	r := bytes.NewReader(binary)
	d := gob.NewDecoder(r)
	err := d.Decode(&grid)
	if err != nil {
		fmt.Println(`failed gob Decode`, err)
	}
	return grid
}

func determineMinimumRectangle(startRow int, startColumn int, sheetIndex int8, grid *Grid) (int, int) {

	maximumRow := startRow
	maximumColumn := startColumn

	for r := startRow; r <= grid.SheetSizes[sheetIndex].RowCount; r++ {
		for c := startColumn; c <= grid.SheetSizes[sheetIndex].ColumnCount; c++ {

			cell := getDataFromRef(Reference{String: indexesToReferenceString(r, c), SheetIndex: sheetIndex}, grid)

			cellFormula := cell.DataFormula

			cellFormula = strings.Replace(cellFormula, "\"", "", -1)

			if len(cellFormula) != 0 && c >= maximumColumn {
				maximumColumn = c
			}

			if len(cellFormula) != 0 && r >= maximumRow {
				maximumRow = r
			}
		}
	}
	return maximumRow, maximumColumn
}

func generateCSV(grid *Grid) string {

	// determine number of columns
	numberOfRows, numberOfColumns := determineMinimumRectangle(1, 1, grid.ActiveSheet, grid)

	// fmt.Println("Detected minimum rectangle: " + strconv.Itoa(numberOfRows) + "x" + strconv.Itoa(numberOfColumns))

	buffer := bytes.Buffer{}

	w := csv.NewWriter(&buffer)

	for r := 1; r <= numberOfRows; r++ {

		var record []string

		for c := 1; c <= numberOfColumns; c++ {

			cell := getDataFromRef(Reference{String: indexesToReferenceString(r, c), SheetIndex: grid.ActiveSheet}, grid)

			// fmt.Println("Ref: " + doubleIndexToStringRef(r, c))
			// fmt.Println("cell.DataFormula: " + cell.DataFormula)

			stringDv := convertToString(cell)

			// fmt.Println("stringDv.DataString: " + stringDv.DataString)
			record = append(record, stringDv.DataString)
		}

		if err := w.Write(record); err != nil {
			log.Fatalln("error writing record to csv:", err)
		}
	}

	w.Flush()

	return buffer.String()
}

func doubleIndexToStringRef(row int, col int) string {
	return indexToLetters(col+1) + strconv.Itoa(row+1)
}

type SortGridItem struct {
	reference Reference
	dv        *DynamicValue
}

type GridItemSorter struct {
	gridItems []SortGridItem
	by        func(p1, p2 *SortGridItem) bool // Closure used in the Less method.
}

func (s GridItemSorter) Swap(i, j int) {
	s.gridItems[i], s.gridItems[j] = s.gridItems[j], s.gridItems[i]
}
func (s GridItemSorter) Len() int {
	return len(s.gridItems)
}
func (s GridItemSorter) Less(i, j int) bool {
	return s.by(&s.gridItems[i], &s.gridItems[j])
}

func cellRangeBoundaries(cellRange string) (int, int, int, int) {
	cells := strings.Split(cellRange, ":")

	lowerRow := getReferenceRowIndex(cells[0])
	upperRow := getReferenceRowIndex(cells[1])

	lowerColumn := getReferenceColumnIndex(cells[0])
	upperColumn := getReferenceColumnIndex(cells[1])

	return lowerRow, lowerColumn, upperRow, upperColumn
}

func compareDvsBigger(dv1 *DynamicValue, dv2 *DynamicValue) bool {
	if dv1.ValueType == DynamicValueTypeString && dv2.ValueType == DynamicValueTypeString {
		return dv1.DataString > dv2.DataString
	} else if dv1.ValueType == DynamicValueTypeString && dv2.ValueType == DynamicValueTypeFloat {
		return true
	} else if dv1.ValueType == DynamicValueTypeFloat && dv2.ValueType == DynamicValueTypeString {
		return false
	} else if dv1.ValueType == DynamicValueTypeFloat && dv2.ValueType == DynamicValueTypeFloat {
		return dv1.DataFloat > dv2.DataFloat
	} else if dv1.ValueType == DynamicValueTypeBool && dv2.ValueType == DynamicValueTypeBool {
		return dv1.DataBool == true && dv2.DataBool == false
	} else {
		return false
	}
}

func compareDvsSmaller(dv1 *DynamicValue, dv2 *DynamicValue) bool {
	if dv1.ValueType == DynamicValueTypeString && dv2.ValueType == DynamicValueTypeString {
		return dv1.DataString < dv2.DataString
	} else if dv1.ValueType == DynamicValueTypeString && dv2.ValueType == DynamicValueTypeFloat {
		return false
	} else if dv1.ValueType == DynamicValueTypeFloat && dv2.ValueType == DynamicValueTypeString {
		return true
	} else if dv1.ValueType == DynamicValueTypeFloat && dv2.ValueType == DynamicValueTypeFloat {
		return dv1.DataFloat < dv2.DataFloat
	} else if dv1.ValueType == DynamicValueTypeBool && dv2.ValueType == DynamicValueTypeBool {
		return dv1.DataBool == false && dv2.DataBool == true
	} else {
		return false
	}
}

func sortRange(direction string, cellRange string, sortColumn string, grid *Grid) {

	// references := cellRangeToCells(cellRange)

	// get lowerRow, lower column and upper row and upper column from cellRange
	lowerRow, lowerColumn, upperRow, upperColumn := cellRangeBoundaries(cellRange)

	sortColumnIndex := getReferenceColumnIndex(sortColumn)

	nonSortingColumns := []int{}

	for c := lowerColumn; c <= upperColumn; c++ {
		if c != sortColumnIndex {
			nonSortingColumns = append(nonSortingColumns, c)
		}
	}

	// get key value pair of column to be sorted [("A1", "2.12"), ("A2","1.5"), ("A3", "2.9")]
	var sortGridItemArray []SortGridItem

	for r := lowerRow; r <= upperRow; r++ {
		reference := Reference{String: indexesToReferenceString(r, sortColumnIndex), SheetIndex: grid.ActiveSheet}
		sortGridItemArray = append(sortGridItemArray, SortGridItem{reference: reference, dv: getDataFromRef(reference, grid)})
	}

	// sort struct {reference: "A1", stringValue: "2.12"}
	// sort this key value pair through string sorting
	var sortByReference func(p1, p2 *SortGridItem) bool

	if direction == "ASC" {
		sortByReference = func(p1, p2 *SortGridItem) bool {
			return compareDvsSmaller(p1.dv, p2.dv)
		}
	} else {
		sortByReference = func(p1, p2 *SortGridItem) bool {
			return compareDvsBigger(p1.dv, p2.dv)
		}
	}

	gis := GridItemSorter{gridItems: sortGridItemArray, by: sortByReference}
	sort.Sort(gis)

	// use the sorted array to iterator through the rows and swap appropriately
	// e.g. after sorting [("A3", "2.9"), ("A1","2.12"), ("A2", "1.5")]
	// then on the first iteration the following swaps are performed (assuming A1:B3 was the sorting region)

	// newGrid (map)

	// newGrid["A1"] = grid["A3"]
	// otherColumns = ["B"] -- create from sorted[0].reference
	// newGrid["B1"] = grid["B3"]

	newGrid := make(map[Reference]*DynamicValue)

	currentRow := lowerRow

	for _, sortGridItem := range sortGridItemArray {

		newRef := Reference{String: indexesToReferenceString(currentRow, sortColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}

		oldRowIndex := getReferenceRowIndex(sortGridItem.reference.String)

		// update formula to update relative references
		sourceFormula := sortGridItem.dv.DataFormula
		newFormula := incrementFormula(sourceFormula, sortGridItem.reference, newRef, false, grid)

		newDv := getDataFromRef(sortGridItem.reference, grid)

		if sourceFormula != newFormula {
			newDv.DataFormula = newFormula
		}

		// DependIn will be constructed in setDependencies based on formula content

		// copy DependOuts from current newRef in Grid to newDv
		newRefDependOut := getDataFromRef(newRef, grid).DependOut
		newDv.DependOut = newRefDependOut

		newGrid[newRef] = newDv

		for _, nonSortingColumnIndex := range nonSortingColumns {
			oldRef := Reference{String: indexesToReferenceString(oldRowIndex, nonSortingColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}
			newRef := Reference{String: indexesToReferenceString(currentRow, nonSortingColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}

			oldDv := getDataFromRef(oldRef, grid)
			sourceFormula := oldDv.DataFormula
			newFormula := incrementFormula(sourceFormula, oldRef, newRef, false, grid)

			newDv := oldDv

			if sourceFormula != newFormula {
				newDv.DataFormula = newFormula
			}

			// copy DependOuts from current newRef in Grid to newDv
			newRefDependOut := getDataFromRef(newRef, grid).DependOut
			newDv.DependOut = newRefDependOut

			newGrid[newRef] = newDv
		}

		currentRow++
	}

	// then finally assign newGrid to grid
	for k, v := range newGrid {
		setDataByRef(k, setDependencies(k, v, grid), grid)
	}

}

func changeReferenceIndex(reference Reference, rowDifference int, columnDifference int, targetSheetIndex int8, grid *Grid) (Reference, bool) {

	crossedBounds := false

	referenceString := reference.String

	if rowDifference == 0 && columnDifference == 0 {
		return Reference{String: reference.String, SheetIndex: targetSheetIndex}, false
	}

	refRow := getReferenceRowIndex(referenceString)
	refColumn := getReferenceColumnIndex(referenceString)

	fixedRow, fixedColumn := getReferenceFixedBools(reference.String)

	if !fixedRow {
		refRow += rowDifference
	}

	if !fixedColumn {
		refColumn += columnDifference
	}

	// check bounds
	if refRow < 1 {
		refRow = 1
		crossedBounds = true
	}
	if refRow > grid.SheetSizes[grid.ActiveSheet].RowCount {
		refRow = grid.SheetSizes[grid.ActiveSheet].RowCount
		crossedBounds = true
	}

	if refColumn < 1 {
		refColumn = 1
		crossedBounds = true
	}
	if refColumn > grid.SheetSizes[grid.ActiveSheet].ColumnCount {
		refColumn = grid.SheetSizes[grid.ActiveSheet].ColumnCount
		crossedBounds = true
	}

	return Reference{String: indexesToReferenceWithFixed(refRow, refColumn, fixedRow, fixedColumn), SheetIndex: targetSheetIndex}, crossedBounds
}

func changeRangeReference(rangeReference ReferenceRange, rowDifference int, columnDifference int, targetSheetIndex int8, grid *Grid) ReferenceRange {

	rangeReferenceString := rangeReference.String
	rangeReferences := strings.Split(rangeReferenceString, ":")
	rangeStartReference := Reference{String: rangeReferences[0], SheetIndex: rangeReference.SheetIndex}
	rangeEndReference := Reference{String: rangeReferences[1], SheetIndex: rangeReference.SheetIndex}

	// increment references
	rangeStartReference, _ = changeReferenceIndex(rangeStartReference, rowDifference, columnDifference, rangeStartReference.SheetIndex, grid)
	rangeEndReference, _ = changeReferenceIndex(rangeEndReference, rowDifference, columnDifference, rangeEndReference.SheetIndex, grid)

	rangeReferenceString = rangeStartReference.String + ":" + rangeEndReference.String

	return ReferenceRange{String: rangeReferenceString, SheetIndex: targetSheetIndex}
}

func incrementFormula(sourceFormula string, sourceRef Reference, destinationRef Reference, isCut bool, grid *Grid) string {

	referenceMapping := make(map[Reference]Reference)
	referenceRangeMapping := make(map[ReferenceRange]ReferenceRange)

	sourceReferences := findReferences(sourceFormula, sourceRef.SheetIndex, false, grid)
	sourceRanges := findRanges(sourceFormula, sourceRef.SheetIndex, grid)

	newFormula := sourceFormula

	// find and replace the references and rangeReferences in the sourceCell itself
	for reference := range sourceReferences {

		rowDifference, columnDifference := getReferenceStringDifference(destinationRef.String, sourceRef.String)

		// when cutting, only the targetSheet is updated, so no difference in row or Column
		targetSheetIndex := destinationRef.SheetIndex
		if reference.SheetIndex != sourceRef.SheetIndex {
			targetSheetIndex = reference.SheetIndex
		}

		if isCut {
			rowDifference = 0
			columnDifference = 0

			if reference.SheetIndex != sourceRef.SheetIndex {
				targetSheetIndex = reference.SheetIndex
			} else {
				targetSheetIndex = sourceRef.SheetIndex
			}
		}

		referenceMapping[reference], _ = changeReferenceIndex(reference, rowDifference, columnDifference, targetSheetIndex, grid)
	}

	for _, rangeReference := range sourceRanges {

		rowDifference, columnDifference := getReferenceStringDifference(destinationRef.String, sourceRef.String)

		// when cutting, only the targetSheet is updated, so no difference in row or Column
		targetSheetIndex := destinationRef.SheetIndex
		if rangeReference.SheetIndex != sourceRef.SheetIndex {
			targetSheetIndex = rangeReference.SheetIndex
		}

		if isCut {
			rowDifference = 0
			columnDifference = 0

			if rangeReference.SheetIndex != sourceRef.SheetIndex {
				targetSheetIndex = rangeReference.SheetIndex
			} else {
				targetSheetIndex = sourceRef.SheetIndex
			}
		}

		newRangeReference := changeRangeReference(rangeReference, rowDifference, columnDifference, targetSheetIndex, grid)
		referenceRangeMapping[rangeReference] = newRangeReference
	}

	if len(sourceReferences) > 0 {
		newFormula = replaceReferencesInFormula(sourceFormula, sourceRef.SheetIndex, destinationRef.SheetIndex, referenceMapping, grid)
	}

	if len(sourceRanges) > 0 {
		newFormula = replaceReferenceRangesInFormula(newFormula, sourceRef.SheetIndex, destinationRef.SheetIndex, referenceRangeMapping, grid)
	}

	return newFormula
}

func removeSheet(sheetIndex int8, grid *Grid) {

	for currentSheetIndex := int(sheetIndex + 1); currentSheetIndex < len(grid.SheetList); currentSheetIndex++ {

		for column := 1; column < grid.SheetSizes[currentSheetIndex].ColumnCount; column++ {

			for row := 1; row < grid.SheetSizes[currentSheetIndex].RowCount; row++ {

				indexString := strconv.Itoa(currentSheetIndex) + "!" + indexesToReferenceString(row, column)
				newIndexString := strconv.Itoa(currentSheetIndex-1) + "!" + indexesToReferenceString(row, column)

				toBeMovedDv := grid.Data[indexString]
				toBeMovedDv.SheetIndex = int8(currentSheetIndex - 1)
				grid.Data[newIndexString] = toBeMovedDv

			}

		}
	}

	// change grid.SheetList, grid.SheetNames, grid.SheetSizes
	delete(grid.SheetNames, grid.SheetList[sheetIndex])

	grid.SheetList = append(grid.SheetList[0:sheetIndex], grid.SheetList[sheetIndex+1:]...)
	grid.SheetSizes = append(grid.SheetSizes[0:sheetIndex], grid.SheetSizes[sheetIndex+1:]...)

	index := 0
	for _, sheetName := range grid.SheetList {
		grid.SheetNames[sheetName] = int8(index)
		index++
	}
}

func sourceToDestinationMapping(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid) ([]Reference, []Reference, []Reference) {
	// case 1: sourceRange is smaller then destinationRange
	// solution: repeat but only if it fits exactly in destinationRange
	// case 2: sourceRange is bigger then destinationRange
	// solution: copy everything from source to destination starting at destinationRange cell 1

	// mapping: determine which cell's contents will go where

	// e.g. copy A1:A3 to B1:B3 then B1<-A1, B2<-A2, B3<-A3
	// e.g. copy A3 to B1:B3 then (edge case - when source is one cell, map that cell to each cell in destinationRange)
	// so, B1<-A3, B2<-A3, B3<-A3

	// key is the destinationCell, the value is the cell is should take the value from
	destinationMapping := []Reference{}

	// todo create destinationMapping
	// possible: only write for 1 to 1 mapping to check if bug free, then extend for case 2 & 3
	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := cellRangeToCells(destinationRange)

	// if len(sourceCells) == len(destinationCells) {
	// 	for key, value := range sourceCells {

	// 		destinationRef := Reference{String: indexesToReferenceString(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
	// 		sourceRef := Reference{String: indexesToReferenceString(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

	// 		destinationMapping[destinationCells[key]] = value
	// 	}

	sourceRowStart := getReferenceRowIndex(sourceCells[0].String)
	sourceColumnStart := getReferenceColumnIndex(sourceCells[0].String)

	sourceRowEnd := getReferenceRowIndex(sourceCells[len(sourceCells)-1].String)
	sourceColumnEnd := getReferenceColumnIndex(sourceCells[len(sourceCells)-1].String)

	destinationRowStart := getReferenceRowIndex(destinationCells[0].String)
	destinationColumnStart := getReferenceColumnIndex(destinationCells[0].String)

	destinationRowEnd := getReferenceRowIndex(destinationCells[len(destinationCells)-1].String)
	destinationColumnEnd := getReferenceColumnIndex(destinationCells[len(destinationCells)-1].String)

	// start looping over all destination cells and co-iterate on source cells
	// whenever sourceCells run out, loop back around

	if len(sourceCells) < len(destinationCells) {

		sRow := sourceRowStart
		sColumn := sourceColumnStart

		for dColumn := destinationColumnStart; dColumn <= destinationColumnEnd; dColumn++ {
			for dRow := destinationRowStart; dRow <= destinationRowEnd; dRow++ {

				destinationRef := Reference{String: indexesToReferenceString(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
				sourceRef := Reference{String: indexesToReferenceString(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

				if !(dRow > grid.SheetSizes[destinationRange.SheetIndex].RowCount || dColumn > grid.SheetSizes[destinationRange.SheetIndex].ColumnCount) {
					destinationMapping = append(destinationMapping, destinationRef)
					destinationMapping = append(destinationMapping, sourceRef)
				}

				if sRow < sourceRowEnd {
					sRow++
				} else {
					sRow = sourceRowStart
				}
			}

			// always start row again for new column
			sRow = sourceRowStart

			if sColumn < sourceColumnEnd {
				sColumn++
			} else {
				sColumn = sourceColumnStart
			}
		}

	} else {

		dRow := destinationRowStart
		dColumn := destinationColumnStart

		for sColumn := sourceColumnStart; sColumn <= sourceColumnEnd; sColumn++ {
			for sRow := sourceRowStart; sRow <= sourceRowEnd; sRow++ {

				destinationRef := Reference{String: indexesToReferenceString(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
				sourceRef := Reference{String: indexesToReferenceString(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

				if !(dRow > grid.SheetSizes[destinationRange.SheetIndex].RowCount || dColumn > grid.SheetSizes[destinationRange.SheetIndex].ColumnCount) {
					destinationMapping = append(destinationMapping, destinationRef)
					destinationMapping = append(destinationMapping, sourceRef)
				}

				// fmt.Println(destinationRef + "->" + sourceRef)

				dRow++
			}
			dRow = destinationRowStart
			dColumn++
		}

	}

	return destinationMapping, sourceCells, destinationCells
}

func cutByValue(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid) []Reference {

	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := copyByValue(sourceRange, destinationRange, grid)

	// clear sourceCells that are not in destination
	for _, ref := range sourceCells {
		if !containsReferences(destinationCells, ref) {
			// clear cell
			clearCell(ref, grid)
		}
	}

	return destinationCells
}

func copyByValue(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid) []Reference {

	destinationMapping, _, destinationCells := sourceToDestinationMapping(sourceRange, destinationRange, grid)

	newDvs := make(map[Reference]*DynamicValue)

	k := 0
	for k < len(destinationMapping) {
		destinationRef := destinationMapping[k]
		sourceRef := destinationMapping[k+1]

		sourceDv := getDataFromRef(sourceRef, grid)
		destinationDv := getDataFromRef(destinationRef, grid)

		destinationDv.ValueType = sourceDv.ValueType
		destinationDv.DataBool = sourceDv.DataBool
		destinationDv.DataFloat = sourceDv.DataFloat
		destinationDv.DataString = sourceDv.DataString

		if destinationDv.ValueType == DynamicValueTypeString {
			destinationDv.DataFormula = "\"" + sourceDv.DataString + "\""
		} else if destinationDv.ValueType == DynamicValueTypeFloat {
			destinationDv.DataFormula = strconv.FormatFloat(sourceDv.DataFloat, 'f', -1, 64)
		} else {
			destinationDv.DataFormula = "TRUE"
			if !sourceDv.DataBool {
				destinationDv.DataFormula = "FALSE"
			}
		}

		newDvs[destinationRef] = destinationDv

		k += 2
	}

	for reference, dv := range newDvs {
		setDataByRef(reference, setDependencies(reference, dv, grid), grid)
	}

	return destinationCells
}

func copySourceToDestination(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid, isCut bool) []Reference {

	destinationMapping, sourceCells, _ := sourceToDestinationMapping(sourceRange, destinationRange, grid)

	finalDestinationCells := []Reference{}
	newDvs := make(map[Reference]*DynamicValue)
	requiresUpdates := make(map[Reference]*DynamicValue)

	rangesToCheck := make(map[Reference][]ReferenceRange)

	var operationRowDifference int
	var operationColumnDifference int
	var operationSourceSheet int8
	var operationTargetSheet int8

	haveOperationDifference := false

	k := 0
	for k < len(destinationMapping) {

		destinationRef := destinationMapping[k]
		sourceRef := destinationMapping[k+1]

		if !haveOperationDifference {
			operationRowDifference, operationColumnDifference = getReferenceStringDifference(destinationRef.String, sourceRef.String)
			haveOperationDifference = true
			operationSourceSheet = sourceRange.SheetIndex
			operationTargetSheet = destinationRef.SheetIndex
		}

		finalDestinationCells = append(finalDestinationCells, destinationRef)

		sourceDv := getDataFromRef(sourceRef, grid)
		sourceFormula := sourceDv.DataFormula
		sourceRanges := findRanges(sourceFormula, sourceDv.SheetIndex, grid)
		newFormula := incrementFormula(sourceFormula, sourceRef, destinationRef, isCut, grid)

		previousDv := getDataFromRef(destinationRef, grid)
		destinationDv := makeDv(newFormula)
		destinationDv.DependOut = previousDv.DependOut

		newDvs[destinationRef] = destinationDv

		if isCut {

			// when cutting cells, make sure that refences in DependOut are also appropriately incremented
			for ref := range getDataFromRef(sourceRef, grid).DependOut {

				thisReference := getReferenceFromMapIndex(ref)

				originalDv := getDvAndRefForCopyModify(thisReference, operationRowDifference, operationColumnDifference, operationSourceSheet, operationTargetSheet, newDvs, grid)

				originalFormula := originalDv.DataFormula
				outgoingDvReferences := findReferences(originalFormula, originalDv.SheetIndex, false, grid)

				if !containsReferences(sourceCells, thisReference) {
					outgoingRanges := findRanges(originalFormula, thisReference.SheetIndex, grid)
					if len(outgoingRanges) > 0 {
						rangesToCheck[thisReference] = outgoingRanges
					}
				}

				referenceMapping := make(map[Reference]Reference)

				for reference := range outgoingDvReferences {

					if reference == sourceRef {
						referenceMapping[reference], _ = changeReferenceIndex(reference, operationRowDifference, operationColumnDifference, destinationRef.SheetIndex, grid)
					}

				}

				newFormula := replaceReferencesInFormula(originalFormula, originalDv.SheetIndex, thisReference.SheetIndex, referenceMapping, grid)

				newDependOutDv := makeDv(newFormula)
				newDependOutDv.DependOut = originalDv.DependOut
				putDvForCopyModify(thisReference, newDependOutDv, operationRowDifference, operationColumnDifference, operationSourceSheet, operationTargetSheet, newDvs, requiresUpdates, grid)

			}

			// check for ranges in the cell that is moved itself
			if len(sourceRanges) > 0 {
				rangesToCheck[destinationRef] = sourceRanges
			}

		}

		k = k + 2
	}

	// check whether DependOut ranges
	for outgoingRef, outgoingRanges := range rangesToCheck {

		// check for each outgoingRange whether it is matched in full
		// check whether in newDv or grid

		outgoingReference := outgoingRef

		outgoingRefDv := getDvAndRefForCopyModify(outgoingReference, 0, 0, operationSourceSheet, outgoingRef.SheetIndex, newDvs, grid)
		outgoingRefFormula := outgoingRefDv.DataFormula

		replacedFormula := false

		referenceRangeMapping := make(map[ReferenceRange]ReferenceRange)

		for _, rangeReference := range outgoingRanges {

			// since cut/copy blocks are square, if first and last element in outgoingRange are in sourceRef they can be considered to all be in there
			rangeReferences := strings.Split(rangeReference.String, ":")
			rangeStartReference := Reference{String: rangeReferences[0], SheetIndex: rangeReference.SheetIndex}
			rangeEndReference := Reference{String: rangeReferences[1], SheetIndex: rangeReference.SheetIndex}

			if containsReferences(sourceCells, rangeStartReference) && containsReferences(sourceCells, rangeEndReference) {

				replacedFormula = true

				// whole range is in here, increment outgoingRef's range (could be replacing more outgoingRanges)
				referenceRangeMapping[rangeReference] = changeRangeReference(rangeReference, operationRowDifference, operationColumnDifference, operationTargetSheet, grid)

			}

		}

		if replacedFormula {

			outgoingRefFormula = replaceReferenceRangesInFormula(outgoingRefFormula, sourceCells[0].SheetIndex, outgoingRefDv.SheetIndex, referenceRangeMapping, grid)

			newDv := makeDv(outgoingRefFormula)
			newDv.DependOut = outgoingRefDv.DependOut

			putDvForCopyModify(outgoingReference, newDv, 0, 0, operationSourceSheet, outgoingRef.SheetIndex, newDvs, requiresUpdates, grid)

		}

	}

	for reference, dv := range newDvs {
		setDataByRef(reference, setDependencies(reference, dv, grid), grid)
	}

	for reference, _ := range requiresUpdates {
		setDataByRef(reference, setDependencies(reference, getDataFromRef(reference, grid), grid), grid)
	}

	// for each cell mapping, copy contents after substituting the references
	// all cells in destination should be added to dirty

	return finalDestinationCells

}

func getDvAndRefForCopyModify(reference Reference, diffRow int, diffCol int, operationSourceSheet int8, operationTargetSheet int8, newDvs map[Reference]*DynamicValue, grid *Grid) *DynamicValue {

	newlyMappedRef, crossedBounds := changeReferenceIndex(reference, diffRow, diffCol, operationTargetSheet, grid)

	if _, ok := newDvs[newlyMappedRef]; ok && !crossedBounds && reference.SheetIndex == operationSourceSheet {
		return newDvs[newlyMappedRef]
	} else {
		return getDataFromRef(reference, grid)
	}
}

func putDvForCopyModify(reference Reference, dv *DynamicValue, diffRow int, diffCol int, operationSourceSheet int8, operationTargetSheet int8, newDvs map[Reference]*DynamicValue, requiresUpdates map[Reference]*DynamicValue, grid *Grid) {
	newlyMappedRef, crossedBounds := changeReferenceIndex(reference, diffRow, diffCol, operationTargetSheet, grid)

	if _, ok := newDvs[newlyMappedRef]; ok && !crossedBounds && reference.SheetIndex == operationSourceSheet {
		newDvs[newlyMappedRef] = dv
	} else {
		setDataByRef(reference, dv, grid)
		requiresUpdates[reference] = dv
	}
}

func getReferenceFixedBools(reference string) (bool, bool) {

	fixedRow := false
	fixedColumn := false

	if strings.Contains(reference[1:], "$") {
		// dollar sign present and not in front
		fixedRow = true
	}

	if reference[0:1] == "$" {
		// dollar sign in front
		fixedColumn = true
	}

	return fixedRow, fixedColumn
}

func getReferenceStringDifference(reference string, sourceReference string) (int, int) {

	rowIndex := getReferenceRowIndex(reference)
	columnIndex := getReferenceColumnIndex(reference)

	sourceRowIndex := getReferenceRowIndex(sourceReference)
	sourceColumnIndex := getReferenceColumnIndex(sourceReference)

	rowDifference := rowIndex - sourceRowIndex

	columnDifference := columnIndex - sourceColumnIndex

	return rowDifference, columnDifference
}

func findMaxColumnWidth(columnIndex int, sheetIndex int8, grid *Grid, c *Client) {

	maxRow := grid.SheetSizes[sheetIndex].RowCount
	currentRowIndex := 1

	maxLengthFound := -1
	maxIndexNormalRef := ""
	maxRowIndex := 1

	for {
		normalRef := strconv.Itoa(int(sheetIndex)) + "!" + indexesToReferenceString(currentRowIndex, columnIndex)
		dv := convertToString(getDataByNormalRef(normalRef, grid))

		if len(dv.DataString) > maxLengthFound {
			maxLengthFound = len(dv.DataString)
			maxIndexNormalRef = normalRef
			maxRowIndex = currentRowIndex
		}

		currentRowIndex += 1

		if currentRowIndex > maxRow {
			break
		}
	}

	// make sure client has maxlen ref
	sendCellsByRefs([]Reference{getReferenceFromMapIndex(maxIndexNormalRef)}, grid, c)

	jsonData := []string{"MAXCOLUMNWIDTH", strconv.Itoa(maxRowIndex), strconv.Itoa(columnIndex), strconv.Itoa(int(sheetIndex)), strconv.Itoa(maxLengthFound)}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json

}
func findJumpCell(startCell Reference, direction string, grid *Grid, c *Client) {

	// find jump cell based on startCell
	startCellRow := getReferenceRowIndex(startCell.String)
	startCellColumn := getReferenceColumnIndex(startCell.String)

	// check whether cell is empty
	startCellEmpty := isCellEmpty(getDataFromRef(startCell, grid))

	horizontalIncrement := 0
	verticalIncrement := 0

	if direction == "up" {
		verticalIncrement = -1
	} else if direction == "down" {
		verticalIncrement = 1
	} else if direction == "left" {
		horizontalIncrement = -1
	} else if direction == "right" {
		horizontalIncrement = 1
	}

	currentCellRow := startCellRow
	currentCellColumn := startCellColumn

	isFirstCellCheck := true

	for {
		currentCellRow += verticalIncrement
		currentCellColumn += horizontalIncrement

		if currentCellRow > grid.SheetSizes[grid.ActiveSheet].RowCount || currentCellRow < 1 {
			break
		}
		if currentCellColumn > grid.SheetSizes[grid.ActiveSheet].ColumnCount || currentCellColumn < 1 {
			break
		}

		thisCellEmpty := isCellEmpty(getDataFromRef(Reference{String: indexesToReferenceString(currentCellRow, currentCellColumn), SheetIndex: startCell.SheetIndex}, grid))

		if isFirstCellCheck && thisCellEmpty && !startCellEmpty {
			// if first cell check is empty cell and this cell is non-empty find first non-empty cell
			startCellEmpty = !startCellEmpty
		}

		if !startCellEmpty && thisCellEmpty {

			break
		}
		if startCellEmpty && !thisCellEmpty {

			currentCellRow += verticalIncrement
			currentCellColumn += horizontalIncrement

			break
		}

		isFirstCellCheck = false
	}

	// reverse one step
	currentCellRow -= verticalIncrement
	currentCellColumn -= horizontalIncrement

	newCell := indexesToReferenceString(currentCellRow, currentCellColumn)

	jsonData := []string{"JUMPCELL", relativeReferenceString(startCell), direction, newCell}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json
}

func getIntFromString(intString string) int {
	intValue, err := strconv.Atoi(intString)
	if err != nil {
		log.Fatal(err)
	}
	return intValue
}

func getIndexFromString(sheetIndexString string) int8 {
	sheetIndexInt, err := strconv.Atoi(sheetIndexString)
	sheetIndex := int8(sheetIndexInt)

	if err != nil {
		log.Fatal(err)
	}
	return sheetIndex
}

func clearCell(ref Reference, grid *Grid) {

	dv := getDataFromRef(ref, grid)

	dv.ValueType = DynamicValueTypeString
	dv.DataFormula = ""
	dv.DataString = ""

	setDataByRef(ref, setDependencies(ref, dv, grid), grid)
}

func replaceReferenceStringInFormula(formula string, referenceMap map[string]string) string {
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
	inSingleQuote := false

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

		} else if inSingleQuote {

			if character == '\'' {
				// exit quote section
				inSingleQuote = false
			}
			referenceEndIndex = index

		} else if character == '"' {
			inQuoteSection = true
		} else if haveValidReference {

			if character == ':' || character == '!' || character == '\'' {

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

		} else if unicode.IsLetter(character) || character == '$' || character == ':' || character == '!' || character == '\'' {

			if character == '\'' {
				inSingleQuote = true
			}

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

func replaceReferencesInFormula(formula string, sourceIndex int8, targetIndex int8, referenceMap map[Reference]Reference, grid *Grid) string {
	referenceStrings := findReferenceStrings(formula)

	stringReferenceMap := make(map[string]string)

	for _, referenceString := range referenceStrings {
		if !strings.Contains(referenceString, ":") {
			reference := getReferenceFromString(referenceString, sourceIndex, grid)
			stringReferenceMap[referenceString] = referenceToRelativeString(referenceMap[reference], targetIndex, grid)
		}
	}

	return replaceReferenceStringInFormula(formula, stringReferenceMap)
}

func replaceReferenceRangesInFormula(formula string, sourceIndex int8, targetIndex int8, referenceRangeMap map[ReferenceRange]ReferenceRange, grid *Grid) string {
	referenceStrings := findReferenceStrings(formula)

	stringReferenceMap := make(map[string]string)

	for _, referenceString := range referenceStrings {
		if strings.Contains(referenceString, ":") {
			referenceRange := getRangeReferenceFromString(referenceString, sourceIndex, grid)
			stringReferenceMap[referenceString] = referenceRangeToRelativeString(referenceRangeMap[referenceRange], targetIndex, grid)
		}
	}

	return replaceReferenceStringInFormula(formula, stringReferenceMap)
}

func changeSheetSize(newRowCount int, newColumnCount int, sheetIndex int8, c *Client, grid *Grid) {

	if newRowCount > grid.SheetSizes[sheetIndex].RowCount || newColumnCount > grid.SheetSizes[sheetIndex].ColumnCount {

		// add missing row/column cells
		for currentColumn := 0; currentColumn <= newColumnCount; currentColumn++ {
			for currentRow := 0; currentRow <= newRowCount; currentRow++ {
				if currentColumn > grid.SheetSizes[sheetIndex].ColumnCount || currentRow > grid.SheetSizes[sheetIndex].RowCount {

					reference := Reference{String: indexesToReferenceString(currentRow, currentColumn), SheetIndex: sheetIndex}

					if !checkDataPresenceFromRef(reference, grid) {
						setDataByRef(reference, makeEmptyDv(), grid)
					}
				}
			}
		}
	}

	grid.SheetSizes[sheetIndex].RowCount = newRowCount
	grid.SheetSizes[sheetIndex].ColumnCount = newColumnCount

	sendSheetSize(c, sheetIndex, grid)
}

func insertRowColumn(insertType string, direction string, reference string, c *Client, grid *Grid) {

	if insertType == "COLUMN" {

		changeSheetSize(grid.SheetSizes[grid.ActiveSheet].RowCount, grid.SheetSizes[grid.ActiveSheet].ColumnCount+1, grid.ActiveSheet, c, grid)

		baseColumn := getReferenceColumnIndex(reference)

		if direction == "RIGHT" {
			baseColumn++
		}

		maximumRow, maximumColumn := determineMinimumRectangle(1, baseColumn, grid.ActiveSheet, grid)

		topLeftRef := indexesToReferenceString(1, baseColumn)
		bottomRightRef := indexesToReferenceString(maximumRow, maximumColumn)

		newTopLeftRef := indexesToReferenceString(1, baseColumn+1)
		newBottomRightRef := indexesToReferenceString(maximumRow, maximumColumn+1)

		cutCells(ReferenceRange{String: topLeftRef + ":" + bottomRightRef, SheetIndex: grid.ActiveSheet},
			ReferenceRange{String: newTopLeftRef + ":" + newBottomRightRef, SheetIndex: grid.ActiveSheet}, grid, c)

	} else if insertType == "ROW" {

		changeSheetSize(grid.SheetSizes[grid.ActiveSheet].RowCount+1, grid.SheetSizes[grid.ActiveSheet].ColumnCount, grid.ActiveSheet, c, grid)

		baseRow := getReferenceRowIndex(reference)

		if direction == "BELOW" {
			baseRow++
		}

		maximumRow, maximumColumn := determineMinimumRectangle(baseRow, 1, grid.ActiveSheet, grid)

		topLeftRef := indexesToReferenceString(baseRow, 1)
		bottomRightRef := indexesToReferenceString(maximumRow, maximumColumn)

		newTopLeftRef := indexesToReferenceString(baseRow+1, 1)
		newBottomRightRef := indexesToReferenceString(maximumRow+1, maximumColumn)

		cutCells(ReferenceRange{String: topLeftRef + ":" + bottomRightRef, SheetIndex: grid.ActiveSheet},
			ReferenceRange{String: newTopLeftRef + ":" + newBottomRightRef, SheetIndex: grid.ActiveSheet}, grid, c)

	}

}

func cutCells(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid, c *Client) []Reference {

	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := copySourceToDestination(sourceRange, destinationRange, grid, true)

	// clear sourceCells that are not in destination
	for _, ref := range sourceCells {
		if !containsReferences(destinationCells, ref) {
			// clear cell
			clearCell(ref, grid)
		}
	}

	changedCells := computeDirtyCells(grid, c)

	return changedCells
}
