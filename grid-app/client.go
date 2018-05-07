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
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

func computeDirtyCells(grid *Grid, cellsToSend *[][]string) {

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

		originalIsString := false
		if originalDv.ValueType == DynamicValueTypeString {
			originalIsString = true
		}

		newDv := originalDv

		// re-compute only non explosive formulas and not marked for non-recompute
		if originalDv.ValueType != DynamicValueTypeExplosiveFormula {

			originalDv.ValueType = DynamicValueTypeFormula
			newDv = parse(originalDv, grid, index)

			newDv.DataFormula = originalDv.DataFormula
			newDv.DependIn = originalDv.DependIn
			newDv.DependOut = originalDv.DependOut

			(grid.Data)[index] = newDv

		}

		// do always send (also explosive formulas)
		// restore state after compute
		stringAfter := convertToString(newDv)

		// adjusting to client needs here

		// details: originalIsString is maintained because parse() affects the original Dv's ValueType
		formulaString := "=" + newDv.DataFormula

		if originalIsString {
			formulaString = newDv.DataString
		}

		*cellsToSend = append(*cellsToSend, []string{index, stringAfter.DataString, formulaString})

		delete(grid.DirtyCells, index)

	}
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

func invalidateView(grid *Grid, c *Client) {

	jsonData := []string{"VIEW-INVALIDATED"}
	json, _ := json.Marshal(jsonData)
	c.send <- json
}

func computeAndSend(grid *Grid, c *Client) {

	cellsToSend := [][]string{}

	// compute dirty cells as a result of adding a cell above
	computeDirtyCells(grid, &cellsToSend)

	sendCells(&cellsToSend, c)

}

func sendCellsInRange(cellRange string, grid *Grid, c *Client) {

	cells := cellRangeToCells(cellRange)

	cellsToSend := [][]string{}

	for _, refString := range cells {
		dv := grid.Data[refString]

		// cell to string
		stringAfter := convertToString(dv)

		cellsToSend = append(cellsToSend, []string{refString, stringAfter.DataString, "=" + dv.DataFormula})
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

	var buffer bytes.Buffer

	for r := 0; r < numberOfRows; r++ {
		for c := 0; c < numberOfColumns; c++ {

			cell := grid.Data[doubleIndexToStringRef(r, c)]

			// fmt.Println("Ref: " + doubleIndexToStringRef(r, c))
			// fmt.Println("cell.DataFormula: " + cell.DataFormula)

			stringDv := convertToString(cell)

			// fmt.Println("stringDv.DataString: " + stringDv.DataString)

			if c+1 == numberOfColumns {
				buffer.WriteString(stringDv.DataString)
				buffer.WriteString("\r\n")
			} else {
				buffer.WriteString(stringDv.DataString)
				buffer.WriteString(",")
			}
		}
	}

	return buffer.String()
}

func doubleIndexToStringRef(row int, col int) string {
	return indexToLetters(col+1) + strconv.Itoa(row+1)
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

					formula := parsed[3][1:]
					formula = referencesToUpperCase(formula)

					// parsed[3] contains the value (formula)
					newDvs := make(map[string]DynamicValue)

					// starting row

					// get row of first reference
					// initRow := getReferenceRowIndex(references[0])

					incrementAmount := 0 // start at index 0

					// first add all to grid
					for _, ref := range references {

						OriginalDependOut := grid.Data[ref].DependOut

						dv := DynamicValue{
							ValueType:   DynamicValueTypeFormula,
							DataFormula: formula,
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						// IMPORTANT AUTO (SINGLE) REFERENCE INCREMENT
						//dv.DataFormula = incrementSingleReferences(dv.DataFormula, incrementAmount)

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
					computeAndSend(&grid, c)
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

						dv := DynamicValue{
							ValueType:   DynamicValueTypeFormula,
							DataFormula: values[valuesIndex],
						}

						NewDependIn := make(map[string]bool)
						dv.DependIn = &NewDependIn       // new dependin (new formula)
						dv.DependOut = OriginalDependOut // dependout remain

						// IMPORTANT AUTO (SINGLE) REFERENCE INCREMENT
						//dv.DataFormula = incrementSingleReferences(dv.DataFormula, incrementAmount)

						// range auto reference manipulation, increment row automatically for references in this formula for each iteration
						// newDvs[ref] = dv

						// set to grid for access during setDependencies
						parsedDv := parse(dv, &grid, ref)
						parsedDv.DataFormula = values[valuesIndex]

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

			case "SET":

				// check if formula or normal entry
				if len(parsed[2]) > 0 && parsed[2][0:1] == "=" {

					// TODO: regex check if input is legal

					// for SET commands with formula values update formula to uppercase any references
					formula := parsed[2][1:]
					formula = referencesToUpperCase(formula)

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

				computeAndSend(&grid, c)
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
				computeAndSend(&grid, c)
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
