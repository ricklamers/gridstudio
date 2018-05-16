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

type Grid struct {
	Data        map[string]DynamicValue
	DirtyCells  map[string]DynamicValue
	RowCount    int
	ColumnCount int
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

		// message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))

		messageString := string(message)

		if len(messageString) > 100 {
			fmt.Println("Received WS message: " + messageString[:100] + "... [truncated]")
		} else {
			fmt.Println("Received WS message: " + messageString)
		}

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

func computeDirtyCells(grid *Grid) []string {

	changedRefs := []string{}

	// for every DV in dirtyCells clean up the DependInTemp list with refs not in DirtyCells
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

		// compute thisDv and update all DependOn values
		for ref, inSet := range *dv.DependOutTemp {
			if inSet {

				// only delete dirty dependencies for cells marked in dirtycells
				if _, ok := (grid.DirtyCells)[ref]; ok {
					delete(*(grid.DirtyCells)[ref].DependInTemp, index)
				}

			}
		}

		// NOTE!!! for now compare, check later if computationally feasible

		// DO COMPUTE HERE
		// stringBefore := convertToString((*grid)[index])

		originalDv := (grid.Data)[index]

		// originalIsString := false
		// if originalDv.ValueType == DynamicValueTypeString {
		// originalIsString = true
		// }

		newDv := originalDv

		// re-compute only non explosive formulas and not marked for non-recompute
		if originalDv.ValueType != DynamicValueTypeExplosiveFormula {

			originalDv.ValueType = DynamicValueTypeFormula
			newDv = parse(originalDv, grid, index)

			newDv.DataFormula = originalDv.DataFormula
			newDv.DependIn = originalDv.DependIn
			newDv.DependOut = originalDv.DependOut

			(grid.Data)[index] = newDv

			changedRefs = append(changedRefs, index)

		}

		// do always send (also explosive formulas)
		// restore state after compute
		// stringAfter := convertToString(newDv)

		// adjusting to client needs here

		// details: originalIsString is maintained because parse() affects the original Dv's ValueType
		// formulaString := "=" + newDv.DataFormula

		// if originalIsString {
		// formulaString = newDv.DataString
		// }

		delete(grid.DirtyCells, index)

	}

	return changedRefs
}

func sendCells(cellsToSend *[][]string, c *Client) {

	jsonData := []string{"SET"}

	// send all dirty cells
	for _, e := range *cellsToSend {
		jsonData = append(jsonData, e[0], e[1], e[2])
	}

	json, _ := json.Marshal(jsonData)
	c.send <- json

}

func sendSheetSize(c *Client, grid *Grid) {
	jsonData := []string{"SHEETSIZE", strconv.Itoa(grid.RowCount), strconv.Itoa(grid.ColumnCount)}
	json, _ := json.Marshal(jsonData)
	c.send <- json
}

func sendDirtyOrInvalidate(changedCells []string, grid *Grid, c *Client) {
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

func sendCellsInRange(cellRange string, grid *Grid, c *Client) {

	cells := cellRangeToCells(cellRange)

	cellsToSend := [][]string{}

	for _, refString := range cells {

		if dv, ok := grid.Data[refString]; ok {
			// cell to string
			stringAfter := convertToString(dv)
			cellsToSend = append(cellsToSend, []string{refString, stringAfter.DataString, "=" + dv.DataFormula})
		}
	}

	sendCells(&cellsToSend, c)
}

func sendCellsByRefs(refs []string, grid *Grid, c *Client) {

	cellsToSend := [][]string{}

	for _, refString := range refs {

		if dv, ok := grid.Data[refString]; ok {
			// cell to string
			stringAfter := convertToString(dv)
			cellsToSend = append(cellsToSend, []string{refString, stringAfter.DataString, "=" + dv.DataFormula})
		}
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

func generateCSV(grid *Grid) string {

	// determine number of columns
	numberOfColumns := 0
	numberOfRows := 0

	for r := 0; r < grid.RowCount; r++ {
		for c := 0; c < grid.ColumnCount; c++ {

			cell := grid.Data[doubleIndexToStringRef(r, c)]

			cellFormula := cell.DataFormula

			cellFormula = strings.Replace(cellFormula, "\"", "", -1)

			// if len(cellFormula) > 0 {
			// 	fmt.Println(cellFormula)
			// }

			if len(cellFormula) != 0 && c >= numberOfColumns {
				numberOfColumns = c + 1
			}

			if len(cellFormula) != 0 && r >= numberOfRows {
				numberOfRows = r + 1
			}
		}
	}

	// fmt.Println("Detected minimum rectangle: " + strconv.Itoa(numberOfRows) + "x" + strconv.Itoa(numberOfColumns))

	buffer := bytes.Buffer{}

	w := csv.NewWriter(&buffer)

	for r := 0; r < numberOfRows; r++ {

		var record []string

		for c := 0; c < numberOfColumns; c++ {

			cell := grid.Data[doubleIndexToStringRef(r, c)]

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
	reference string
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
		reference := indexesToReference(r, sortColumnIndex)
		sortGridItemArray = append(sortGridItemArray, SortGridItem{reference: reference, dv: grid.Data[reference]})
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

	newGrid := make(map[string]DynamicValue)

	currentRow := lowerRow

	for _, sortGridItem := range sortGridItemArray {

		newRef := indexesToReference(currentRow, sortColumnIndex)
		newGrid[newRef] = grid.Data[sortGridItem.reference]

		oldRowIndex := getReferenceRowIndex(sortGridItem.reference)

		for _, nonSortingColumnIndex := range nonSortingColumns {
			oldRef := indexesToReference(oldRowIndex, nonSortingColumnIndex)
			newRef := indexesToReference(currentRow, nonSortingColumnIndex)
			newGrid[newRef] = grid.Data[oldRef]
		}

		currentRow++
	}

	// then finally assign newGrid to grid
	for k, v := range newGrid {
		grid.Data[k] = v
	}
}

func copySourceToDestination(sourceRange string, destinationRange string, grid *Grid) []string {

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
	destinationMapping := make(map[string]string)
	// todo create destinationMapping
	// possible: only write for 1 to 1 mapping to check if bug free, then extend for case 2 & 3
	sourceCells := cellRangeToCells(sourceRange)
	destinationCells := cellRangeToCells(destinationRange)

	finalDestinationCells := []string{}

	if len(sourceCells) == len(destinationCells) {
		for key, value := range sourceCells {
			destinationMapping[destinationCells[key]] = value
		}
	} else {

		sourceRowStart := getReferenceRowIndex(sourceCells[0])
		sourceColumnStart := getReferenceColumnIndex(sourceCells[0])

		sourceRowEnd := getReferenceRowIndex(sourceCells[len(sourceCells)-1])
		sourceColumnEnd := getReferenceColumnIndex(sourceCells[len(sourceCells)-1])

		destinationRowStart := getReferenceRowIndex(destinationCells[0])
		destinationColumnStart := getReferenceColumnIndex(destinationCells[0])

		destinationRowEnd := getReferenceRowIndex(destinationCells[len(destinationCells)-1])
		destinationColumnEnd := getReferenceColumnIndex(destinationCells[len(destinationCells)-1])

		// start looping over all destination cells and co-iterate on source cells
		// whenever sourceCells run out, loop back around

		if len(sourceCells) < len(destinationCells) {

			sRow := sourceRowStart
			sColumn := sourceColumnStart

			for dColumn := destinationColumnStart; dColumn <= destinationColumnEnd; dColumn++ {
				for dRow := destinationRowStart; dRow <= destinationRowEnd; dRow++ {

					destinationRef := indexesToReference(dRow, dColumn)
					sourceRef := indexesToReference(sRow, sColumn)

					destinationMapping[destinationRef] = sourceRef

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

					destinationRef := indexesToReference(dRow, dColumn)
					sourceRef := indexesToReference(sRow, sColumn)

					destinationMapping[destinationRef] = sourceRef

					// fmt.Println(destinationRef + "->" + sourceRef)

					dRow++
				}
				dRow = destinationRowStart
				dColumn++
			}

		}
	}

	newDvs := make(map[string]DynamicValue)

	for destinationRef, sourceRef := range destinationMapping {

		finalDestinationCells = append(finalDestinationCells, destinationRef)

		referenceMapping := make(map[string]string)

		destinationRefRow := getReferenceRowIndex(destinationRef)
		destinationRefColumn := getReferenceColumnIndex(destinationRef)

		sourceFormula := grid.Data[sourceRef].DataFormula
		sourceReferences := findReferences(sourceFormula)

		for reference, _ := range sourceReferences {
			rowDifference, columnDifference, fixedRow, fixedColumn := getReferenceDifference(reference, sourceRef)

			newRefRow := destinationRefRow + rowDifference
			if newRefRow < 1 {
				newRefRow = 1
			}
			if newRefRow > grid.RowCount {
				newRefRow = grid.RowCount
			}

			newRefColumn := destinationRefColumn + columnDifference
			if newRefColumn < 1 {
				newRefColumn = 1
			}
			if newRefColumn > grid.ColumnCount {
				newRefColumn = grid.ColumnCount
			}

			if fixedRow {
				newRefRow = getReferenceRowIndex(reference)
			}
			if fixedColumn {
				newRefColumn = getReferenceColumnIndex(reference)
			}

			newRelativeReference := indexesToReferenceWithFixed(newRefRow, newRefColumn, fixedRow, fixedColumn)
			referenceMapping[reference] = newRelativeReference
		}

		newFormula := replaceReferencesInFormula(sourceFormula, referenceMapping)

		destinationDv := getDataFromRef(destinationRef, grid)
		destinationDv.DataFormula = newFormula

		newDvs[destinationRef] = destinationDv

	}

	for ref, dv := range newDvs {
		grid.Data[ref] = setDependencies(ref, dv, grid)
	}
	// for each cell mapping, copy contents after substituting the references
	// all cells in destination should be added to dirty

	return finalDestinationCells

}

func getReferenceDifference(reference string, sourceReference string) (int, int, bool, bool) {

	rowIndex := getReferenceRowIndex(reference)
	columnIndex := getReferenceColumnIndex(reference)

	sourceRowIndex := getReferenceRowIndex(sourceReference)
	sourceColumnIndex := getReferenceColumnIndex(sourceReference)

	rowDifference := rowIndex - sourceRowIndex

	fixedRow := false
	fixedColumn := false

	columnDifference := columnIndex - sourceColumnIndex

	if strings.Contains(reference[1:], "$") {
		// dollar sign present and not in front
		fixedRow = true
	}

	if reference[0:1] == "$" {
		// dollar sign in front
		fixedColumn = true
	}

	return rowDifference, columnDifference, fixedRow, fixedColumn
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

func clearCell(ref string, grid *Grid) {

	OriginalDependOut := grid.Data[ref].DependOut

	dv := DynamicValue{
		ValueType:   DynamicValueTypeString,
		DataFormula: "",
	}

	NewDependIn := make(map[string]bool)
	dv.DependIn = &NewDependIn       // new dependin (new formula)
	dv.DependOut = OriginalDependOut // dependout remain

	grid.Data[ref] = setDependencies(ref, dv, grid)
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

		} else if unicode.IsLetter(character) || character == '$' {

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
		if index >= len(formula) && !haveValidReference {
			break
		}
	}

	return formula
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)

	var grid Grid

	// if Grid serialized file exists try to load that
	sheetFile := "/home/user/sheet.serialized"
	if _, err := os.Stat(sheetFile); os.IsNotExist(err) {

		// initialize the datastructure for the matrix
		columnCount := 26
		rowCount := 10000

		grid = Grid{Data: make(map[string]DynamicValue), DirtyCells: make(map[string]DynamicValue), RowCount: rowCount, ColumnCount: columnCount}

		cellCount := 1

		for x := 1; x <= columnCount; x++ {
			for y := 1; y <= rowCount; y++ {
				dv := makeDv("")

				// DEBUG: fill with incrementing numbers
				// dv.ValueType = DynamicValueTypeInteger
				// dv.DataInteger = int32(cellCount)
				// dv.DataFormula = strconv.Itoa(cellCount)

				grid.Data[indexToLetters(x)+strconv.Itoa(y)] = dv

				cellCount++
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

				references := cellRangeToCells(parsed[2])

				switch parsed[1] {
				case "SETSINGLE":

					var formula string
					if len(parsed[3]) > 0 {
						formula = parsed[3][1:]
						formula = referencesToUpperCase(formula)
					} else {
						formula = parsed[3]
					}

					// parsed[3] contains the value (formula)
					newDvs := make(map[string]DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])

					incrementAmount := 0 // start at index 0

					// first add all to grid
					for _, ref := range references {

						OriginalDependOut := grid.Data[ref].DependOut

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
						grid.Data[ref] = dv

						incrementAmount++

					}

					// then setDependencies for all
					for ref, dv := range newDvs {
						grid.Data[ref] = setDependencies(ref, dv, &grid)
					}

					// now compute all dirty
					changedCells := computeDirtyCells(&grid)
					sendDirtyOrInvalidate(changedCells, &grid, c)

				case "SETLIST":

					// Note: SETLIST doesn't support formula insert, only raw data. E.g. numbers or strings

					// values are all values from parsed[3] on
					values := parsed[3:]

					// parsed[3] contains the value (formula)
					// newDvs := make(map[string]DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])

					// first add all to grid
					valuesIndex := 0
					for _, ref := range references {

						OriginalDependOut := grid.Data[ref].DependOut

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

						grid.Data[ref] = parsedDv

						valuesIndex++

					}

					invalidateView(&grid, c)

					// then setDependencies for all

					// even though all values, has to be ran for all new values because fields might depend on new input data
					// for ref, dv := range newDvs {
					// 	grid.Data[ref] = setDependencies(ref, dv, &grid)
					// }

					// now compute all dirty
					// computeAndSend(&grid, c)

				}
			case "GET":

				sendCellsInRange(parsed[1], &grid, c)

			case "GET-FILE":

				getFile(parsed[1], c)

			case "SET-FILE":

				setFile(parsed[1], parsed[2], c)

			case "GET-DIRECTORY":

				getDirectory(parsed[1], c)

			case "COPY":

				sourceRange := parsed[1]
				destinationRange := parsed[2]

				copySourceToDestination(sourceRange, destinationRange, &grid)

				changedCells := computeDirtyCells(&grid)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "CUT":

				sourceRange := parsed[1]
				destinationRange := parsed[2]

				// clear difference between sourceRange and destinationRange
				sourceCells := cellRangeToCells(sourceRange)
				destinationCells := copySourceToDestination(sourceRange, destinationRange, &grid)

				// clear sourceCells that are not in destination
				for _, ref := range sourceCells {
					if !contains(destinationCells, ref) {
						// clear cell
						clearCell(ref, &grid)
					}
				}

				changedCells := computeDirtyCells(&grid)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "SET":

				// check if formula or normal entry
				if len(parsed[2]) > 0 && parsed[2][0:1] == "=" {

					// TODO: regex check if input is legal

					// for SET commands with formula values update formula to uppercase any references
					formula := parsed[2][1:]
					formula = referencesToUpperCase(formula)

					if !isValidFormula(formula) {

						OriginalDependOut := grid.Data[parsed[1]].DependOut

						dv := DynamicValue{
							ValueType:   DynamicValueTypeString,
							DataFormula: "\"Error in formula: " + formula + "\"",
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						grid.Data[parsed[1]] = setDependencies(parsed[1], dv, &grid)

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
							dv = parse(dv, &grid, parsed[1])

							// don't need dependend information for parsing, hence assign after parse
							NewDependIn := make(map[string]bool)

							dv.DependIn = &NewDependIn                      // new dependin (new formula)
							dv.DependOut = OriginalDependOut                // dependout remain
							dv.ValueType = DynamicValueTypeExplosiveFormula // shouldn't be necessary, is return type of olsExplosive()
							dv.DataFormula = formula                        // re-assigning of formula is usually saved for computeDirty but this will be skipped there

							// add OLS cell to dirty (which needs DependInTemp etc)
							grid.Data[parsed[1]] = setDependencies(parsed[1], dv, &grid)

							// dependencies will be fulfilled for all cells created by explosion

						} else {
							// set value for cells
							// cut off = for parsing

							// original Dependends
							OriginalDependOut := grid.Data[parsed[1]].DependOut

							dv := DynamicValue{
								ValueType:   DynamicValueTypeFormula,
								DataFormula: formula,
							}

							NewDependIn := make(map[string]bool)
							dv.DependIn = &NewDependIn       // new dependin (new formula)
							dv.DependOut = OriginalDependOut // dependout remain

							grid.Data[parsed[1]] = setDependencies(parsed[1], dv, &grid)
						}

					}

				} else {

					// else enter as string
					// if user enters non string value, client is reponsible for adding the equals sign.
					// Anything without it won't be parsed as formula.

					OriginalDependOut := grid.Data[parsed[1]].DependOut

					dv := DynamicValue{
						ValueType:   DynamicValueTypeString,
						DataString:  parsed[2],
						DataFormula: "\"" + parsed[2] + "\""}

					DependIn := make(map[string]bool)

					dv.DependIn = &DependIn
					dv.DependOut = OriginalDependOut

					newDv := setDependencies(parsed[1], dv, &grid)
					newDv.ValueType = DynamicValueTypeString
					grid.Data[parsed[1]] = newDv

				}

				changedCells := computeDirtyCells(&grid)
				sendDirtyOrInvalidate(changedCells, &grid, c)

			case "CSV":
				fmt.Println("Received CSV! Size: " + strconv.Itoa(len(parsed[1])))

				// TODO: grow the grid to minimum size
				minColumnSize := 0

				// replace \r to \n
				csvString := strings.Replace(parsed[1], "\r", "\n", -1)
				csvStringReader := strings.NewReader(csvString)
				reader := csv.NewReader(csvStringReader)

				reader.Comma = ','
				lineCount := 0

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

						newDv.DataFormula = inputString

						// if not number, escape with quotes
						if !numberOnlyFilter.MatchString(inputString) {
							newDv.ValueType = DynamicValueTypeString
							newDv.DataString = inputString
						} else {
							newDv.ValueType = DynamicValueTypeFloat
							floatValue, err := strconv.ParseFloat(inputString, 64)

							if err != nil {
								fmt.Println("Error parsing number: ")
								fmt.Println(err)
							}

							newDv.DataFloat = floatValue
						}

						oldDv := grid.Data[cellIndex]
						if oldDv.DependOut != nil {
							newDv.DependOut = oldDv.DependOut // regain external dependencies, in case of oldDv
						}

						// this will add it to dirtyCells for re-compute
						// grid.Data[cellIndex] = setDependencies(cellIndex, newDv, &grid)
						grid.Data[cellIndex] = newDv

					}
					// fmt.Println()
					lineCount++

				}

				minRowSize := lineCount

				newRowCount := grid.RowCount
				newColumnCount := grid.ColumnCount

				if minRowSize > grid.RowCount {
					newRowCount = minRowSize
				}
				if minColumnSize > grid.ColumnCount {
					newColumnCount = minColumnSize
				}

				grid.RowCount = newRowCount
				grid.ColumnCount = newColumnCount

				sendSheetSize(c, &grid)

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
