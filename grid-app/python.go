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
			if err == io.EOF {
				stdoutPipe.Close()
				break
			}
			if err != nil {
				fmt.Println(err)
				break
			}
			buffer = buffer[0:n]

			bufferString += string(buffer)

			if strings.HasSuffix(bufferString, "\n") {

				// detect whether Python process has outputted new data
				errorString := bufferString

				jsonData := []string{"INTERPRETER"}
				jsonData = append(jsonData, "[error]"+errorString)
				json, _ := json.Marshal(jsonData)
				c.send <- json

				bufferString = ""
			}

		}

	}
}

func streamPythonOut(stdoutPipe io.ReadCloser, pythonIn io.WriteCloser, c *Client, closeChannel chan string) {
	buffer := make([]byte, 100, 1000)

	bufferString := ""

	for {

		select {
		case command := <-closeChannel:
			if command == "CLOSE" {
				return
			}

		default:
			for {
				n, err := stdoutPipe.Read(buffer)
				if err == io.EOF {
					stdoutPipe.Close()
					break
				}
				buffer = buffer[0:n]

				bufferString += string(buffer)

				if strings.HasSuffix(bufferString, "\n") {

					// detect whether Python process has outputted new data
					if len(buffer) > 0 {

						newString := bufferString

						// only change bufferSize if ending is #PONG#
						// print string from b that is size of newBufferLength that starts at oldSize

						// check first 7 chars in substring
						if len(newString) > 6 && newString[:7] == "#PARSE#" {

							// could receive double JSON message
							commands := strings.Split(newString, "#PARSE#")

							// remove first element
							commands = commands[1:]

							for _, e := range commands {
								c.actions <- []byte(e)
							}

						} else if len(newString) > 6 && newString[:7] == "#IMAGE#" {
							commands := strings.Split(newString, "#IMAGE#")

							// remove first element
							commands = commands[1:2]

							// send JSON bytes directly to client websocket connection
							for _, e := range commands {
								c.send <- []byte(e)
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

							if len(newString) > 0 {
								jsonData := []string{"INTERPRETER"}
								jsonData = append(jsonData, newString)
								json, _ := json.Marshal(jsonData)
								c.send <- json
							}
						}

					}

					bufferString = ""
				}

			}
		}
	}
}

func (c *Client) pythonInterpreter() {

	/// PYTHON COMMUNICATION SPAWNING PROTOCOL
	pythonCommand := "python3"

	if runtime.GOOS == "windows" {
		// in Windows runtime Python 3 doesn't use 3 in the name of the executable
		pythonCommand = "python"
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

			fmt.Println("Write for Python interpreter received: " + command)

			pythonIn.Write([]byte(command + "\n\n"))

		}

	}
}
