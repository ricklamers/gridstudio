package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

func streamPythonEOut(stdoutPipe io.ReadCloser, pythonIn io.WriteCloser, c *Client, closeChannel chan string) {
	buffer := make([]byte, 100, 1000)

	bufferString := ""

	for {

		select {
		case command := <-closeChannel:
			if command == "CLOSE" {
				return
			}

		default:
			n, err := stdoutPipe.Read(buffer)

			if err != nil {
				fmt.Println(err)
			}

			if err == io.EOF {
				stdoutPipe.Close()
				return
			}
			if err != nil {
				fmt.Println(err)
				break
			}
			buffer = buffer[0:n]

			bufferString += string(buffer)

			// fmt.Println("Error:" + string(buffer))

			if strings.HasSuffix(bufferString, "\n") {

				// detect whether Python process has outputted new data
				errorString := bufferString

				jsonData := []string{"INTERPRETER"}
				jsonData = append(jsonData, "[error]"+errorString)
				jsonString, _ := json.Marshal(jsonData)
				c.send <- jsonString

				jsonData2 := []string{"COMMANDCOMPLETE"}
				jsonString2, _ := json.Marshal(jsonData2)
				c.send <- jsonString2

				bufferString = ""
			}

		}

	}
}

func streamPythonOut(stdoutPipe io.ReadCloser, pythonIn io.WriteCloser, c *Client, closeChannel chan string) {
	buffer := make([]byte, 65536/2) // make read buffer smaller than Python stdout limit

	var bufferHolder bytes.Buffer

	for {

		select {
		case command := <-closeChannel:
			if command == "CLOSE" {
				return
			}

		default:
			for {
				n, err := stdoutPipe.Read(buffer)

				if err != nil {
					fmt.Println(err)
				}

				if err == io.EOF {
					stdoutPipe.Close()
					return
				}
				// n, err := io.ReadFull(stdoutPipe, buffer)

				// testing wether the stdoutPipe.Read max of 65536 was reached // recheck stdoutPipe again
				if n != len(buffer) {
					// error implies that the buffer could not be fully filled, hence some sort of end was found in stdoutPipe
					subbuffer := buffer[:n]

					// fmt.Println("Python out:" + string(subbuffer))

					bufferHolder.Write(subbuffer)

					parsePythonOutput(bufferHolder, pythonIn, c)

					bufferHolder.Reset()
				} else {
					bufferHolder.Write(buffer)
				}

				// try Read a second time, if it n == 0, no more bytes to be read and call to parsePythonOutput can be made
				// n2, err2 := stdoutPipe.Read(buffer)
				// if err2 == io.EOF {
				// 	stdoutPipe.Close()
				// 	break
				// }
				// buffer = buffer[0:n2]

				// bufferHolder.Write(buffer)

				// if n2 == 0 {
				// 	parsePythonOutput(bufferHolder, pythonIn, c)
				// }

			}
		}
	}
}

func parsePythonOutput(bufferHolder bytes.Buffer, pythonIn io.WriteCloser, c *Client) {

	if strings.HasSuffix(bufferHolder.String(), "#ENDPARSE#") {

		parseStrings := strings.Split(bufferHolder.String(), "#ENDPARSE#")

		for _, e := range parseStrings {

			// detect whether Python process has outputted new data
			if len(e) > 0 {

				newString := e

				// fmt.Println(newString)

				// only change bufferSize if ending is #PONG#
				// print string from b that is size of newBufferLength that starts at oldSize

				// check first 7 chars in substring

				if len(newString) > 16 && newString[:17] == "#COMMANDCOMPLETE#" {

					jsonData := []string{"COMMANDCOMPLETE"}
					json, _ := json.Marshal(jsonData)
					c.send <- json

				} else if len(newString) > 6 && newString[:7] == "#PARSE#" {

					c.actions <- []byte(newString[7:])

				} else if len(newString) > 6 && newString[:7] == "#IMAGE#" {
					commands := strings.Split(newString, "#IMAGE#")

					// remove first element
					commands = commands[1:2]

					// send JSON bytes directly to client websocket connection
					for _, e := range commands {
						c.send <- []byte(e)
					}

				} else if len(newString) > 15 && newString[:16] == "#PYTHONFUNCTION#" {

					commands := strings.Split(newString, "#PYTHONFUNCTION#")

					// remove first element
					commands = commands[1:2]

					// send back result in formula form (string escaped, numbers as literals)
					// c.actions <- []byte(commands[0])
					c.grid.PythonResultChannel <- commands[0]

				} else if len(newString) > 5 && newString[:6] == "#DATA#" {

					// data receive request
					cellRangeString := newString[6:]

					cells := cellRangeToCells(getReferenceRangeFromMapIndex(cellRangeString))

					var commandBuf bytes.Buffer

					for _, e := range cells {

						valueDv := getDataFromRef(e, c.grid)
						value := convertToString(valueDv).DataString
						// for each cell get data
						commandBuf.WriteString("sheet_data[\"")
						commandBuf.WriteString(getMapIndexFromReference(e))
						commandBuf.WriteString("\"] = ")

						if valueDv.ValueType == DynamicValueTypeString {
							commandBuf.WriteString("\"")

							escapedStringValue := strings.Replace(value, "\"", "\\\"", -1)
							commandBuf.WriteString(escapedStringValue)

							commandBuf.WriteString("\"")
						} else {
							if len(value) == 0 {
								commandBuf.WriteString("\"\"")
							} else {
								commandBuf.WriteString(value)
							}
						}

						commandBuf.WriteString("\n")
					}
					// empty line to finish command
					commandBuf.WriteString("\n")
					pythonIn.Write(commandBuf.Bytes())

				} else if len(newString) > 12 && newString[:13] == "#INTERPRETER#" {

					jsonData := []string{"INTERPRETER"}
					jsonData = append(jsonData, newString[13:])
					json, _ := json.Marshal(jsonData)
					c.send <- json
				}

			}

		}

	}
}

func (c *Client) pythonInterpreter() {

	/// PYTHON COMMUNICATION SPAWNING PROTOCOL
	pythonCommand := "python3.7"

	if runtime.GOOS == "windows" {
		// in Windows runtime Python 3 doesn't use 3 in the name of the executable
		pythonCommand = "python"
	} else if runtime.GOOS == "darwin" {
		pythonCommand = "python3"
	}

	pythonCmd := exec.Command(pythonCommand, "-u", "python/init.py")

	pythonIn, _ := pythonCmd.StdinPipe()
	defer func() {
		fmt.Println("Killing Python user session")
		pythonIn.Close()
	}()

	closeChannel := make(chan string)

	pythonOut, _ := pythonCmd.StdoutPipe()
	pythonEOut, _ := pythonCmd.StderrPipe()

	pythonCmd.Start()

	go streamPythonOut(pythonOut, pythonIn, c, closeChannel)
	go streamPythonEOut(pythonEOut, pythonIn, c, closeChannel)

	for {
		// read and clean buffer

		select {
		case command := <-c.commands:

			if command == "CLOSE" {
				closeChannel <- "CLOSE"
				fmt.Println("CLOSE received")
				return
			}

			// fmt.Println("Received")

			fmt.Println("Write for Python interpreter received: " + command)

			pythonIn.Write([]byte(command + "\n\n"))

		}

	}
}
