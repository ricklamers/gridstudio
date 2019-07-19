package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/NYTimes/gziphandler"
)

var addr = flag.String("addr", ":8080", "http service address")
var mode = flag.String("mode", "server", "program run mode")
var rootDirectory = flag.String("root", "/home/userdata/workspace-TESTUUID/", "root directory for user files")

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

func main() {

	flag.Parse()

	fmt.Println("Root directory:" + *rootDirectory)

	// get user directory /home/userdata/worspace-UUID and serve all dynamic data from this

	// PROFILING //
	// if *cpuprofile != "" {

	// 	f, err := os.Create(*cpuprofile)
	// 	if err != nil {
	// 		log.Fatal("could not create CPU profile: ", err)
	// 	}

	// 	if err := pprof.StartCPUProfile(f); err != nil {
	// 		log.Fatal("could not start CPU profile: ", err)
	// 	}
	// 	defer pprof.StopCPUProfile()
	// }
	// END PROFILING //

	// initialize map
	parseInit()

	if *mode == "server" {
		runServer()
	} else if *mode == "testing" {
		runTests()
	}

	// PROFILING //
	// if *memprofile != "" {
	// 	f, err := os.Create(*memprofile)
	// 	if err != nil {
	// 		log.Fatal("could not create memory profile: ", err)
	// 	}
	// 	runtime.GC() // get up-to-date statistics
	// 	if err := pprof.WriteHeapProfile(f); err != nil {
	// 		log.Fatal("could not write memory profile: ", err)
	// 	}
	// 	f.Close()
	// }
	// END PROFILING //

}

func runServer() {
	fs := http.FileServer(http.Dir("static"))

	// withoutGz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	w.Header().Set("Content-Type", "text/plain")
	// 	io.WriteString(w, "Hello, World")
	// })

	withGz := gziphandler.GzipHandler(fs)

	http.Handle("/", withGz)

	http.HandleFunc("/upcheck", func(w http.ResponseWriter, r *http.Request) {

	})

	http.HandleFunc("/uploadFile", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("upload method:", r.Method)

		if r.Method == "POST" {
			r.ParseMultipartForm(32 << 20)

			file, handler, err := r.FormFile("file")

			if err != nil {
				fmt.Println(err)
				return
			}

			defer file.Close()

			fmt.Fprintf(w, "%v", handler.Header)

			f, err := os.OpenFile(*rootDirectory+"userdata/"+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)

			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			io.Copy(f, file)
		}
	})

	mainThreadChannel := make(chan string)

	hub := newHub(mainThreadChannel, *rootDirectory)

	go hub.run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {

		fmt.Println("/ws http request received")

		serveWs(hub, w, r)
	})

	http.HandleFunc("/fell-idle-check", func(w http.ResponseWriter, r *http.Request) {

		w.Write([]byte(strconv.Itoa(int(hub.inactiveTime / time.Second))))

	})

	fmt.Println("Go server listening on port " + *addr)
	srv := startHttpServer(*addr)

Loop:
	for {
		select {
		case message := <-mainThreadChannel:
			if message == "EXIT" {
				break Loop
			}
		}
	}

	if err := srv.Shutdown(nil); err != nil {
		panic(err) // failure/timeout shutting down the server gracefully
	}
}

func startHttpServer(addr string) *http.Server {
	srv := &http.Server{Addr: addr}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
			log.Printf("Httpserver: ListenAndServe() error: %s", err)
		}
	}()

	// returning reference so caller can call Shutdown()
	return srv
}

func assert(value string, expected string, testID *int) {
	if value == expected {
		fmt.Printf("[Test %d] Success. V: %s E: %s\n", *testID, value, expected)
	} else {
		fmt.Printf("\x1b[31;1m[Test %d] Failed. V: %s E: %s\x1b[0m\n", *testID, value, expected)
	}
	// increment test number
	*testID++
}
