package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func (c *Client) pythonInterpreter() {

	/// PYTHON COMMUNICATION SPAWNING PROTOCOL
	pythonCommand := "python3"

	if runtime.GOOS == "windows" {
		// in Windows runtime Python 3 doesn't use 3 in the name of the executable
		pythonCommand = "python"

		// pypy test
		// pythonCommand = "pypy3-c"
	}

	pythonCmd := exec.Command(pythonCommand, "-u", "python/init.py")

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	pythonCmd.Stdout = writer

	var eb bytes.Buffer
	errwriter := bufio.NewWriter(&eb)
	pythonCmd.Stderr = errwriter

	pythonIn, _ := pythonCmd.StdinPipe()
	defer func() {
		fmt.Println("Killing Python user session")
		pythonIn.Close()
	}()
	// pythonOut, _ := pythonCmd.StdoutPipe()

	pythonCmd.Start()

	bufferSize := b.Len()
	errorBufferSize := eb.Len()
	timer := time.NewTicker(time.Millisecond * 50)

	for {
		// read and clean buffer

		select {
		case command := <-c.commands:

			if command == "CLOSE" {
				fmt.Println("CLOSE received")
				return
			}

			pythonIn.Write([]byte(command + "\n\n"))

		case <-timer.C:
			// fmt.Println(b.Len())
			// detect whether Python process has outputted new data
			if bufferSize != b.Len() {

				oldSize := bufferSize
				bufferSize = b.Len()

				// only change bufferSize if ending is #PONG#
				// print string from b that is size of newBufferLength that starts at oldSize
				newString := string(b.Bytes()[oldSize:bufferSize])
				// newString = newString[:len(newString)]

				// check first 7 chars in substring
				if len(newString) > 6 && newString[:7] == "#PARSE#" {

					// could receive double JSON message
					commands := strings.Split(newString, "#PARSE#")

					// remove first element
					commands = commands[1:]

					for _, e := range commands {
						c.actions <- []byte(e)
					}

				} else if len(newString) > 5 && newString[:6] == "#DATA#" {

					// data receive request
					cellRangeString := newString[6:]

					cells := cellRangeToCells(cellRangeString)

					var commandBuf bytes.Buffer

					for _, e := range cells {

						valueDv := c.grid.data[e]
						value := convertToString(valueDv).DataString
						// for each cell get data
						commandBuf.WriteString("sheet_data[\"")
						commandBuf.WriteString(e)
						commandBuf.WriteString("\"] = ")

						if valueDv.ValueType == DynamicValueTypeString {
							commandBuf.WriteString("\"")
							commandBuf.WriteString(value)
							commandBuf.WriteString("\"")
						} else {
							commandBuf.WriteString(value)
						}

						commandBuf.WriteString("\n")
					}
					// empty line to finish command
					commandBuf.WriteString("\n")
					pythonIn.Write(commandBuf.Bytes())

				} else {
					// fmt.Println(newString)

					if len(newString) > 0 {
						jsonData := []string{"INTERPRETER"}
						jsonData = append(jsonData, newString)
						json, _ := json.Marshal(jsonData)
						c.send <- json
					}
				}

			}
			if eb.Len() != errorBufferSize {

				oldSize := errorBufferSize
				errorBufferSize = eb.Len()

				errorString := string(eb.Bytes()[oldSize:errorBufferSize])

				jsonData := []string{"INTERPRETER"}
				jsonData = append(jsonData, errorString)
				json, _ := json.Marshal(jsonData)
				c.send <- json

				// fmt.Println(errorString)
			}
		}

	}
}
