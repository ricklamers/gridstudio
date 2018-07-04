package main

import b64 "encoding/base64"

import (
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"./detector"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second

	pongWait = 60 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = math.MaxInt64
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type StringJSON struct {
	Arguments []string `json:"arguments"`
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	actions chan []byte

	commands chan string

	grid *Grid
}

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
	Data                map[string]DynamicValue
	DirtyCells          map[string]DynamicValue
	ActiveSheet         int8
	SheetNames          map[string]int8
	PerformanceCounting map[string]int
	SheetList           []string
	SheetSizes          []SheetSize
	PythonResultChannel chan string
	PythonClient        chan string
}

// readPump pumps messages from the websocket connection to the hub (?)
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		fmt.Println("Closed readPump")
		c.commands <- "CLOSE"
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}

		messageString := string(message)

		// if len(messageString) > 100 {
		// 	fmt.Println("Received WS message: " + messageString[:100] + "... [truncated]")
		// } else {
		// 	fmt.Println("Received WS message: " + messageString)
		// }

		// check if command or code
		if messageString[:7] == "#PARSE#" {
			c.commands <- messageString[7:]
		} else {
			c.actions <- message
		}

		// c.hub.broadcast <- message // send message to hub
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

func computeDirtyCells(grid *Grid) []Reference {

	changedRefs := []Reference{}

	// for every DV in dirtyCells clean up the DependInTemp list with refs not in DirtyCells

	// EXPLANATION: this code removes DependInTemp's that don't need to be recomputed, however, checking this might be more expensive than just including all dependintemp's

	for _, thisDv := range grid.DirtyCells {

		for ref := range *thisDv.DependInTemp {

			// if ref is not in dirty cells, remove from depend in
			if _, ok := (grid.DirtyCells)[ref]; !ok {
				delete(*thisDv.DependInTemp, ref)
			}
		}

	}

	for len((grid.DirtyCells)) != 0 {

		var dv DynamicValue
		var index string

		// remove all DependIn that are not in dirty cells (since not dirty, can use existing values)
		// This step is done in computeDirtyCells because at this point
		// we are certain whether cells are dirty or not

		for ref, thisDv := range grid.DirtyCells {

			if len(*thisDv.DependInTemp) == 0 {
				dv = thisDv
				index = ref
				break
			}
		}

		if index == "" {
			break
		}

		// compute thisDv and update all DependOn values
		if dv.DependOutTemp != nil {
			for ref, inSet := range *dv.DependOutTemp {
				if inSet {

					// only delete dirty dependencies for cells marked in dirtycells
					if _, ok := (grid.DirtyCells)[ref]; ok {
						delete(*(grid.DirtyCells)[ref].DependInTemp, index)
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
			newDv = parse(originalDv, grid, getReferenceFromMapIndex(index))

			newDv.DataFormula = originalDv.DataFormula
			newDv.DependIn = originalDv.DependIn
			newDv.DependOut = originalDv.DependOut
			newDv.SheetIndex = originalDv.SheetIndex

			newReference := getReferenceFromMapIndex(index)
			setDataByRef(newReference, newDv, grid)

			changedRefs = append(changedRefs, newReference)

		}

		delete(grid.DirtyCells, index)

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

		// cell to string
		stringAfter := convertToString(dv)
		cellsToSend = append(cellsToSend, []string{relativeReferenceString(reference), stringAfter.DataString, "=" + dv.DataFormula, strconv.Itoa(int(dv.SheetIndex))})
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

			cell := getDataFromRef(Reference{String: indexesToReference(r, c), SheetIndex: sheetIndex}, grid)

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

			cell := getDataFromRef(Reference{String: indexesToReference(r, c), SheetIndex: grid.ActiveSheet}, grid)

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
	dv        DynamicValue
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

func compareDvsBigger(dv1 DynamicValue, dv2 DynamicValue) bool {
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

func compareDvsSmaller(dv1 DynamicValue, dv2 DynamicValue) bool {
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
		reference := Reference{String: indexesToReference(r, sortColumnIndex), SheetIndex: grid.ActiveSheet}
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

	newGrid := make(map[Reference]DynamicValue)

	currentRow := lowerRow

	for _, sortGridItem := range sortGridItemArray {

		newRef := Reference{String: indexesToReference(currentRow, sortColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}

		oldRowIndex := getReferenceRowIndex(sortGridItem.reference.String)

		// update formula to update relative references
		sourceFormula := sortGridItem.dv.DataFormula
		newFormula := incrementFormula(sourceFormula, sortGridItem.reference, newRef, false, grid)

		newDv := getDataFromRef(sortGridItem.reference, grid)

		if sourceFormula != newFormula {
			newDv.DataFormula = newFormula
		}

		newGrid[newRef] = newDv

		for _, nonSortingColumnIndex := range nonSortingColumns {
			oldRef := Reference{String: indexesToReference(oldRowIndex, nonSortingColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}
			newRef := Reference{String: indexesToReference(currentRow, nonSortingColumnIndex), SheetIndex: sortGridItem.reference.SheetIndex}

			oldDv := getDataFromRef(oldRef, grid)
			sourceFormula := oldDv.DataFormula
			newFormula := incrementFormula(sourceFormula, oldRef, newRef, false, grid)

			newDv := oldDv

			if sourceFormula != newFormula {
				newDv.DataFormula = newFormula
			}

			newGrid[newRef] = newDv
		}

		currentRow++
	}

	// then finally assign newGrid to grid
	for k, v := range newGrid {
		setDataByRef(k, setDependencies(k, v, grid), grid)
	}

}

func changeReferenceIndex(reference Reference, rowDifference int, columnDifference int, targetSheetIndex int8, grid *Grid) Reference {

	referenceString := reference.String

	if rowDifference == 0 && columnDifference == 0 {
		return Reference{String: reference.String, SheetIndex: targetSheetIndex}
	}

	refRow := getReferenceRowIndex(referenceString)
	refColumn := getReferenceColumnIndex(referenceString)

	refRow += rowDifference
	refColumn += columnDifference

	// check bounds
	if refRow < 1 {
		refRow = 1
	}
	if refRow > grid.SheetSizes[grid.ActiveSheet].RowCount {
		refRow = grid.SheetSizes[grid.ActiveSheet].RowCount
	}

	if refColumn < 1 {
		refColumn = 1
	}
	if refColumn > grid.SheetSizes[grid.ActiveSheet].ColumnCount {
		refColumn = grid.SheetSizes[grid.ActiveSheet].ColumnCount
	}

	fixedRow, fixedColumn := getReferenceFixedBools(reference.String)

	return Reference{String: indexesToReferenceWithFixed(refRow, refColumn, fixedRow, fixedColumn), SheetIndex: targetSheetIndex}
}

func changeRangeReference(rangeReference ReferenceRange, rowDifference int, columnDifference int, targetSheetIndex int8, grid *Grid) ReferenceRange {

	rangeReferenceString := rangeReference.String
	rangeReferences := strings.Split(rangeReferenceString, ":")
	rangeStartReference := Reference{String: rangeReferences[0], SheetIndex: rangeReference.SheetIndex}
	rangeEndReference := Reference{String: rangeReferences[1], SheetIndex: rangeReference.SheetIndex}

	rangeReferenceString = changeReferenceIndex(rangeStartReference, rowDifference, columnDifference, rangeStartReference.SheetIndex, grid).String + ":" + changeReferenceIndex(rangeEndReference, rowDifference, columnDifference, rangeEndReference.SheetIndex, grid).String

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

		fixedRow, fixedColumn := getReferenceFixedBools(reference.String)

		if fixedRow {
			rowDifference = 0
		}
		if fixedColumn {
			columnDifference = 0
		}

		referenceMapping[reference] = changeReferenceIndex(reference, rowDifference, columnDifference, targetSheetIndex, grid)
	}

	for _, rangeReference := range sourceRanges {

		rowDifference, columnDifference := getReferenceStringDifference(destinationRef.String, sourceRef.String)

		// when cutting, only the targetSheet is updated, so no difference in row or Column
		targetSheetIndex := destinationRef.SheetIndex
		if isCut {
			rowDifference = 0
			columnDifference = 0

			if rangeReference.SheetIndex != sourceRef.SheetIndex {
				targetSheetIndex = rangeReference.SheetIndex
			} else {
				targetSheetIndex = sourceRef.SheetIndex
			}
		}

		fixedRow, fixedColumn := getReferenceFixedBools(rangeReference.String)

		if fixedRow {
			rowDifference = 0
		}
		if fixedColumn {
			columnDifference = 0
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

				indexString := strconv.Itoa(currentSheetIndex) + "!" + indexesToReference(row, column)
				newIndexString := strconv.Itoa(currentSheetIndex-1) + "!" + indexesToReference(row, column)

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

func copySourceToDestination(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid, isCut bool) []Reference {

	// case 1: sourceRange and destinationRange have equal size
	// solution: copy every cell to every destinationRange cell
	// case 2: sourceRange is smaller then destinationRange
	// solution: repeat but only if it fits exactly in destinationRange
	// case 3: sourceRange is bigger then destinationRange
	// solution: copy everything from source to destination starting at destinationRange cell 1

	// mapping: determine which cell's contents will go where

	// e.g. copy A1:A3 to B1:B3 then B1<-A1, B2<-A2, B3<-A3
	// e.g. copy A3 to B1:B3 then (edge case - when source is one cell, map that cell to each cell in destinationRange)
	// so, B1<-A3, B2<-A3, B3<-A3

	// key is the destinationCell, the value is the cell is should take the value from
	destinationMapping := make(map[Reference]Reference)
	// todo create destinationMapping
	// possible: only write for 1 to 1 mapping to check if bug free, then extend for case 2 & 3
	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := cellRangeToCells(destinationRange)

	finalDestinationCells := []Reference{}

	// if len(sourceCells) == len(destinationCells) {
	// 	for key, value := range sourceCells {

	// 		destinationRef := Reference{String: indexesToReference(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
	// 		sourceRef := Reference{String: indexesToReference(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

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

				destinationRef := Reference{String: indexesToReference(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
				sourceRef := Reference{String: indexesToReference(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

				if !(dRow > grid.SheetSizes[destinationRange.SheetIndex].RowCount || dColumn > grid.SheetSizes[destinationRange.SheetIndex].ColumnCount) {
					destinationMapping[destinationRef] = sourceRef
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

				destinationRef := Reference{String: indexesToReference(dRow, dColumn), SheetIndex: destinationRange.SheetIndex}
				sourceRef := Reference{String: indexesToReference(sRow, sColumn), SheetIndex: sourceRange.SheetIndex}

				if !(dRow > grid.SheetSizes[destinationRange.SheetIndex].RowCount || dColumn > grid.SheetSizes[destinationRange.SheetIndex].ColumnCount) {
					destinationMapping[destinationRef] = sourceRef
				}

				// fmt.Println(destinationRef + "->" + sourceRef)

				dRow++
			}
			dRow = destinationRowStart
			dColumn++
		}

	}

	newDvs := make(map[Reference]DynamicValue)

	rangesToCheck := make(map[Reference][]ReferenceRange)

	var operationRowDifference int
	var operationColumnDifference int
	var operationTargetSheet int8

	haveOperationDifference := false

	for destinationRef, sourceRef := range destinationMapping {

		if !haveOperationDifference {
			operationRowDifference, operationColumnDifference = getReferenceStringDifference(destinationRef.String, sourceRef.String)
			haveOperationDifference = true
			operationTargetSheet = destinationRef.SheetIndex
		}

		finalDestinationCells = append(finalDestinationCells, destinationRef)

		sourceFormula := getDataFromRef(sourceRef, grid).DataFormula
		sourceRanges := findRanges(sourceFormula, sourceRef.SheetIndex, grid)
		newFormula := incrementFormula(sourceFormula, sourceRef, destinationRef, isCut, grid)

		previousDv := getDataFromRef(destinationRef, grid)
		destinationDv := makeDv(newFormula)
		destinationDv.DependOut = previousDv.DependOut

		newDvs[destinationRef] = destinationDv

		if isCut {

			// when cutting cells, make sure that single refences in DependOut are also appropriately incremeted
			for ref := range *getDataFromRef(sourceRef, grid).DependOut {

				thisReference := getReferenceFromMapIndex(ref)

				// if DependOut not in source (will not be moved/deleted)
				originalDv := getDvAndRefForCopyModify(getReferenceFromMapIndex(ref), operationRowDifference, operationColumnDifference, destinationRef.SheetIndex, newDvs, grid)
				originalFormula := originalDv.DataFormula

				outgoingDvReferences := findReferences(originalFormula, originalDv.SheetIndex, false, grid)

				if !containsReferences(sourceCells, thisReference) {
					outgoingRanges := findRanges(originalFormula, thisReference.SheetIndex, grid)
					rangesToCheck[thisReference] = outgoingRanges
				}

				referenceMapping := make(map[Reference]Reference)

				for reference := range outgoingDvReferences {

					if reference == sourceRef {
						referenceMapping[reference] = changeReferenceIndex(reference, operationRowDifference, operationColumnDifference, destinationRef.SheetIndex, grid)
					}

				}

				newFormula := replaceReferencesInFormula(originalFormula, originalDv.SheetIndex, destinationRef.SheetIndex, referenceMapping, grid)

				newDependOutDv := makeDv(newFormula)
				newDependOutDv.DependOut = originalDv.DependOut
				putDvForCopyModify(getReferenceFromMapIndex(ref), newDependOutDv, operationRowDifference, operationColumnDifference, destinationRef.SheetIndex, newDvs, grid)

			}

			// check for ranges in the cell that is moved itself
			if len(sourceRanges) > 0 {
				rangesToCheck[destinationRef] = sourceRanges
			}

		}

	}

	// check whether DependOut ranges
	for outgoingRef, outgoingRanges := range rangesToCheck {

		// check for each outgoingRange whether it is matched in full
		// check whether in newDv or grid

		outgoingReference := outgoingRef

		outgoingRefDv := getDvAndRefForCopyModify(outgoingReference, 0, 0, outgoingRef.SheetIndex, newDvs, grid)
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

			putDvForCopyModify(outgoingReference, newDv, 0, 0, outgoingRef.SheetIndex, newDvs, grid)

		}

	}

	for reference, dv := range newDvs {
		setDataByRef(reference, setDependencies(reference, dv, grid), grid)
	}

	// for each cell mapping, copy contents after substituting the references
	// all cells in destination should be added to dirty

	return finalDestinationCells

}

func getDvAndRefForCopyModify(reference Reference, diffRow int, diffCol int, targetSheetIndex int8, newDvs map[Reference]DynamicValue, grid *Grid) DynamicValue {

	newlyMappedRef := changeReferenceIndex(reference, diffRow, diffCol, targetSheetIndex, grid)

	if _, ok := newDvs[newlyMappedRef]; ok {
		return newDvs[newlyMappedRef]
	} else {
		return getDataFromRef(reference, grid)
	}
}

func putDvForCopyModify(reference Reference, dv DynamicValue, diffRow int, diffCol int, targetSheetIndex int8, newDvs map[Reference]DynamicValue, grid *Grid) {
	newlyMappedRef := changeReferenceIndex(reference, diffRow, diffCol, targetSheetIndex, grid)

	if _, ok := newDvs[newlyMappedRef]; ok {
		newDvs[newlyMappedRef] = dv
	} else {
		setDataByRef(reference, setDependencies(reference, dv, grid), grid)
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

func setFile(path string, dataString string, c *Client) {

	errFile := ioutil.WriteFile(path, []byte(dataString), 0644)
	if errFile != nil {
		fmt.Println("Error calling setFile for path: " + path)
		fmt.Print(errFile)
		return
	}
}

func getFile(path string, c *Client) {

	b, err := ioutil.ReadFile(path) // just pass the file name
	if err != nil {
		fmt.Println("Error calling getFile for path: " + path)
		fmt.Print(err)
		return
	}

	sEnc := b64.StdEncoding.EncodeToString(b)

	jsonData := []string{"GET-FILE", path, sEnc}

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

		thisCellEmpty := isCellEmpty(getDataFromRef(Reference{String: indexesToReference(currentCellRow, currentCellColumn), SheetIndex: startCell.SheetIndex}, grid))

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

	newCell := indexesToReference(currentCellRow, currentCellColumn)

	jsonData := []string{"JUMPCELL", relativeReferenceString(startCell), direction, newCell}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json
}

func getDirectory(path string, c *Client) {

	path = strings.TrimRight(path, "/")

	if len(path) == 0 {
		path = "/"
	}

	jsonData := []string{"GET-DIRECTORY"}

	levelUp := ""

	if len(path) > 0 {
		pathComponents := strings.Split(path, "/")
		pathComponents = pathComponents[:len(pathComponents)-1]
		levelUp = strings.Join(pathComponents, "/")

		if levelUp == "" {
			levelUp = "/"
		}
	}

	jsonData = append(jsonData, "directory", "..", levelUp)

	files, err := ioutil.ReadDir(path)
	if err != nil {
		fmt.Println(err)

		// directory doesn't exist
		jsonData = []string{"GET-DIRECTORY", "INVALIDPATH"}

	}

	filePath := path
	if path == "/" {
		filePath = ""
	}

	for _, f := range files {
		fileType := "file"
		if f.IsDir() {
			fileType = "directory"
		}

		jsonData = append(jsonData, fileType, f.Name(), filePath+"/"+f.Name())
	}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json
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

	OriginalDependOut := getDataFromRef(ref, grid).DependOut

	dv := DynamicValue{
		ValueType:   DynamicValueTypeString,
		DataFormula: "",
	}

	NewDependIn := make(map[string]bool)
	dv.DependIn = &NewDependIn       // new dependin (new formula)
	dv.DependOut = OriginalDependOut // dependout remain

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

					reference := Reference{String: indexesToReference(currentRow, currentColumn), SheetIndex: sheetIndex}

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

		topLeftRef := indexesToReference(1, baseColumn)
		bottomRightRef := indexesToReference(maximumRow, maximumColumn)

		newTopLeftRef := indexesToReference(1, baseColumn+1)
		newBottomRightRef := indexesToReference(maximumRow, maximumColumn+1)

		cutCells(ReferenceRange{String: topLeftRef + ":" + bottomRightRef, SheetIndex: grid.ActiveSheet},
			ReferenceRange{String: newTopLeftRef + ":" + newBottomRightRef, SheetIndex: grid.ActiveSheet}, grid)

	} else if insertType == "ROW" {

		changeSheetSize(grid.SheetSizes[grid.ActiveSheet].RowCount+1, grid.SheetSizes[grid.ActiveSheet].ColumnCount, grid.ActiveSheet, c, grid)

		baseRow := getReferenceRowIndex(reference)

		if direction == "BELOW" {
			baseRow++
		}

		maximumRow, maximumColumn := determineMinimumRectangle(baseRow, 1, grid.ActiveSheet, grid)

		topLeftRef := indexesToReference(baseRow, 1)
		bottomRightRef := indexesToReference(maximumRow, maximumColumn)

		newTopLeftRef := indexesToReference(baseRow+1, 1)
		newBottomRightRef := indexesToReference(maximumRow+1, maximumColumn)

		cutCells(ReferenceRange{String: topLeftRef + ":" + bottomRightRef, SheetIndex: grid.ActiveSheet},
			ReferenceRange{String: newTopLeftRef + ":" + newBottomRightRef, SheetIndex: grid.ActiveSheet}, grid)

	}

}

func cutCells(sourceRange ReferenceRange, destinationRange ReferenceRange, grid *Grid) []Reference {

	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := copySourceToDestination(sourceRange, destinationRange, grid, true)

	// clear sourceCells that are not in destination
	for _, ref := range sourceCells {
		if !containsReferences(destinationCells, ref) {
			// clear cell
			clearCell(ref, grid)
		}
	}

	changedCells := computeDirtyCells(grid)

	return changedCells
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)

	var grid Grid

	defaultColumnCount := 10
	defaultRowCount := 100

	// if Grid serialized file exists try to load that
	sheetFile := "/home/user/sheet.serialized"
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

		grid = Grid{Data: make(map[string]DynamicValue), PerformanceCounting: make(map[string]int), DirtyCells: make(map[string]DynamicValue), ActiveSheet: 0, SheetNames: sheetNames, SheetList: sheetList, SheetSizes: sheetSizes}

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

	defer func() {
		ticker.Stop()
		c.conn.Close()

		fmt.Println("Closed writePump")
	}()

	for {
		select {
		case actions, ok := <-c.actions:

			if !ok {
				log.Fatal("Something wrong with channel")
			}

			res := StringJSON{}
			json.Unmarshal(actions, &res)

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
					newDvs := make(map[Reference]DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])
					incrementAmount := 0 // start at index 0

					// first add all to grid
					for _, ref := range references {

						thisReference := ref

						OriginalDependOut := getDataFromRef(thisReference, &grid).DependOut

						var dv DynamicValue

						if !isValidFormula(formula) {
							dv = DynamicValue{
								ValueType:   DynamicValueTypeString,
								DataFormula: "\"Error in formula: " + formula + "\"",
							}
						} else {
							dv = DynamicValue{
								ValueType:   DynamicValueTypeFormula,
								DataFormula: formula,
							}
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						// range auto reference manipulation, increment row automatically for references in this formula for each iteration
						newDvs[ref] = dv

						// set to grid for access during setDependencies
						setDataByRef(thisReference, dv, &grid)

						incrementAmount++

					}

					for ref, dv := range newDvs {
						setDataByRef(ref, setDependencies(ref, dv, &grid), &grid)
					}

					// now compute all dirty
					changedCells := computeDirtyCells(&grid)
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
					for _, ref := range references {

						OriginalDependOut := getDataFromRef(ref, &grid).DependOut

						var dv DynamicValue

						if !isValidFormula(values[valuesIndex]) {
							dv = DynamicValue{
								ValueType:   DynamicValueTypeString,
								DataFormula: "\"Error in formula: " + values[valuesIndex] + "\"",
							}
						} else {
							dv = DynamicValue{
								ValueType:   DynamicValueTypeFormula,
								DataFormula: values[valuesIndex],
							}
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						// range auto reference manipulation, increment row automatically for references in this formula for each iteration
						// newDvs[ref] = dv

						// set to grid for access during setDependencies
						parsedDv := parse(dv, &grid, ref)
						parsedDv.DataFormula = values[valuesIndex]
						parsedDv.DependIn = &NewDependIn
						parsedDv.DependOut = OriginalDependOut

						setDataByRef(ref, parsedDv, &grid)

						valuesIndex++

						// add all OriginalDependOut to dirty
						for key, _ := range *OriginalDependOut {
							copyToDirty(grid.Data[key], key, &grid)
						}

					}

					computeDirtyCells(&grid)

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

				changedCells := computeDirtyCells(&grid)

				elapsed = time.Since(start)                      // debug
				log.Printf("computeDirtyCells took %s", elapsed) // debug

				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "INSERTROWCOL":

				insertType := parsed[1]
				direction := parsed[2]
				reference := parsed[3]

				insertRowColumn(insertType, direction, reference, c, &grid)

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
					ReferenceRange{String: destinationRange, SheetIndex: destinationRangeSheetIndex}, &grid)

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

						OriginalDependOut := getDataFromRef(reference, &grid).DependOut

						dv := DynamicValue{
							ValueType:   DynamicValueTypeString,
							DataFormula: "\"Error in formula: " + formula + "\"",
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						setDataByRef(reference, setDependencies(reference, dv, &grid), &grid)

					} else {

						// check for explosive formulas
						isExplosive := isExplosiveFormula(formula)

						if isExplosive {

							// original Dependends can stay on
							OriginalDependOut := grid.Data[parsed[1]].DependOut

							dv := DynamicValue{
								ValueType:   DynamicValueTypeExplosiveFormula,
								DataFormula: formula,
							}

							// Dependencies are not required, since this cell won't depend on anything given that it's explosive

							// parse explosive formula (also, explosive formulas cannot be nested)
							dv = parse(dv, &grid, Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])})

							// don't need dependend information for parsing, hence assign after parse
							NewDependIn := make(map[string]bool)

							dv.DependIn = &NewDependIn                      // new dependin (new formula)
							dv.DependOut = OriginalDependOut                // dependout remain
							dv.ValueType = DynamicValueTypeExplosiveFormula // shouldn't be necessary, is return type of olsExplosive()
							dv.DataFormula = formula                        // re-assigning of formula is usually saved for computeDirty but this will be skipped there

							// add OLS cell to dirty (which needs DependInTemp etc)
							grid.Data[parsed[1]] = setDependencies(Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}, dv, &grid)

							// dependencies will be fulfilled for all cells created by explosion

						} else {
							// set value for cells
							// cut off = for parsing

							// original Dependends
							thisReference := Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}

							OriginalDependOut := getDataFromRef(thisReference, &grid).DependOut

							dv := DynamicValue{
								ValueType:   DynamicValueTypeFormula,
								DataFormula: formula,
							}

							NewDependIn := make(map[string]bool)
							dv.DependIn = &NewDependIn       // new dependin (new formula)
							dv.DependOut = OriginalDependOut // dependout remain

							setDataByRef(thisReference, setDependencies(thisReference, dv, &grid), &grid)
						}

					}

				} else {

					// else enter as string
					// if user enters non string value, client is reponsible for adding the equals sign.
					// Anything without it won't be parsed as formula.
					reference := Reference{String: parsed[1], SheetIndex: getIndexFromString(parsed[3])}

					OriginalDependOut := getDataFromRef(reference, &grid).DependOut

					// escape double quotes
					formulaString := strings.Replace(parsed[2], "\"", "\\\"", -1)

					dv := DynamicValue{
						ValueType:   DynamicValueTypeString,
						DataString:  parsed[2],
						DataFormula: "\"" + formulaString + "\""}

					// if input is empty string, set formula to empty string without quotes
					if len(parsed[2]) == 0 {
						dv.DataFormula = ""
					}

					DependIn := make(map[string]bool)

					dv.DependIn = &DependIn
					dv.DependOut = OriginalDependOut

					newDv := setDependencies(reference, dv, &grid)
					newDv.ValueType = DynamicValueTypeString

					setDataByRef(reference, newDv, &grid)

				}

				changedCells := computeDirtyCells(&grid)
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

				newDvs := make(map[Reference]DynamicValue)

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

					if minColumnSize == 0 {
						minColumnSize = len(record)
					}

					// and we can iterate on top of that
					for i := 0; i < len(record); i++ {
						// fmt.Println(" ", record[i])

						// for now load CSV file to upper left cell, starting at A1
						cellIndex := indexToLetters(i+1) + strconv.Itoa(lineCount+1)

						inputString := record[i]

						newDv := makeEmptyDv()

						// if not number, escape with quotes
						if !numberOnlyFilter.MatchString(inputString) {
							newDv.ValueType = DynamicValueTypeString
							newDv.DataString = inputString
							newDv.DataFormula = "\"" + inputString + "\""

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

						reference := Reference{String: cellIndex, SheetIndex: grid.ActiveSheet}
						oldDv := getDataFromRef(reference, &grid)
						if oldDv.DependOut != nil {
							newDv.DependOut = oldDv.DependOut // regain external dependencies, in case of oldDv
						}

						// this will add it to dirtyCells for re-compute
						newDvs[reference] = newDv

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

				for ref, dv := range newDvs {
					setDataByRef(ref, setDependencies(ref, dv, &grid), &grid)
				}

				changedCells := computeDirtyCells(&grid)
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

				err := ioutil.WriteFile("/home/user/sheet.serialized", serializedGrid, 0644)

				if err != nil {
					fmt.Println(err)
				}

				c.send <- []byte("[\"SAVED\"]")
			case "SORT":
				sortRange(parsed[1], parsed[2], parsed[3], &grid) // direction (ASC,DESC), range ("A1:B20"), column ("B")
				computeDirtyCells(&grid)
				invalidateView(&grid, c)
			}

		case message, ok := <-c.send:

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)

			if err != nil {
				return
			}

			w.Write(message)

			// add queued chat messages to the current websocket message.
			n := len(c.send)

			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				fmt.Println("errored on sending pingmessage to client")
				fmt.Println(err)
				return
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), actions: make(chan []byte, 256), commands: make(chan string, 256)}
	client.hub.register <- client
	fmt.Println("Client connected!")

	// Allow new connection of memory referenced by the caller by doing all the work in new goroutines.
	go client.writePump()
	go client.readPump()
	go client.pythonInterpreter()

}
