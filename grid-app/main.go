package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/NYTimes/gziphandler"
)

var addr = flag.String("addr", ":8080", "http service address")

var grid map[string]cell

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

func main() {

	// initialize map
	// grid = map[string]cell{}

	// x := 0
	// for {
	// 	set("0:"+strconv.Itoa(x), cell{CellType: CellTypeFloat, DataFloat: 100})

	// 	if x == 100 {
	// 		break
	// 	}
	// 	x = x + 1
	// }

	// fmt.Println(get("0:" + strconv.Itoa(x)))

	// PROFILING //
	// flag.Parse()
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

	flag.Parse()

	parseInit()

	// var input string

	// if len(os.Args) == 2 && os.Args[1] == "test" {

	// testIndex := 1

	// assert(convertToString(parse(createDv("A2"))).DataString, "2", &testIndex)
	// assert(convertToString(parse(createDv("A2+A3/A5"))).DataString, "2.6", &testIndex)

	// } else {

	// no commands start server
	// if len(os.Args) < 2 {
	// input = "AVERAGE(2,SUM(2,4))"

	// } else {
	// single command start server
	// input = os.Args[1]

	// var dv DynamicValue

	// dv = parse(createDv(input))
	// fmt.Println(convertToString(dv).DataString)
	// }

	// }

	fs := http.FileServer(http.Dir("static"))

	// withoutGz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	w.Header().Set("Content-Type", "text/plain")
	// 	io.WriteString(w, "Hello, World")
	// })

	withGz := gziphandler.GzipHandler(fs)

	http.Handle("/", withGz)

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

			f, err := os.OpenFile("/home/user/"+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)

			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			io.Copy(f, file)
		}
	})

	hub := newHub()
	go hub.run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {

		fmt.Println("/ws http request received")

		serveWs(hub, w, r)
	})

	fmt.Println("Go server listening on port " + *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))

	// for i := 0; i < 1000000; i++ {
	// 	dv = parse(DynamicValue{ValueType: DynamicValueTypeFormula, DataString: "MATH.C(\"E\")^(2*A5)"})
	// }

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

// func createDv(formula string) DynamicValue {
// 	return parse(DynamicValue{ValueType: DynamicValueTypeFormula, DataFormula: formula})
// }

func assert(value string, expected string, testID *int) {
	if value == expected {
		fmt.Printf("[Test %d] Success. V: %s E: %s\n", *testID, value, expected)
	} else {
		fmt.Printf("\x1b[31;1m[Test %d] Failed. V: %s E: %s\x1b[0m\n", *testID, value, expected)
	}
	// increment test number
	*testID++
}

func set(index string, value cell) {
	grid[index] = value
}
func get(index string) cell {
	return grid[index]
}
