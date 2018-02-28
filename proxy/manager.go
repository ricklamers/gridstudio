package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koding/websocketproxy"
	"github.com/twinj/uuid"
)

const wsBase = 5000
const termBase = -1000

func wsProxy(port int) {

	base, err := url.Parse("ws://127.0.0.1:" + strconv.Itoa(port) + "/ws")
	fmt.Println("WS base: " + "ws://127.0.0.1:" + strconv.Itoa(port) + "/ws")

	if err != nil {
		log.Fatal(err)
	}

	wsPort := wsBase + port

	fmt.Println("WS Listening on port: " + strconv.Itoa(wsPort))

	wsp := websocketproxy.NewProxy(base)
	wsp.Upgrader = &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	errWS := http.ListenAndServe(":"+strconv.Itoa(wsPort), wsp)
	if errWS != nil {
		log.Fatal(errWS)
	}
}

type DockerSession struct {
	port int
}

func getFreePort(usedports map[int]bool, startPort int) int {
	currentPort := startPort

	for {
		if val, ok := usedports[currentPort]; ok {
			if val == false {
				// port is defined in map, but true, hence free
				return currentPort
			}
		} else {
			// port is not defined in map, hence free
			return currentPort
		}

		currentPort++
	}
}

func printActiveUsers(usersessions map[string]DockerSession) {
	fmt.Printf("%d user sessions active.\n", len(usersessions))
}

func main() {

	// build a map that holds all user sessions

	// form: UID key and int as port of active docker client
	var startPort = 4000
	var usedports map[int]bool
	var usedwsports map[int]bool
	var usersessions map[string]DockerSession
	usersessions = make(map[string]DockerSession)
	usedports = make(map[int]bool)
	usedwsports = make(map[int]bool)

	// index.html to initialize
	httpClient := http.Client{}

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/dashboard/", fs)

	http.HandleFunc("/create-debug", func(w http.ResponseWriter, r *http.Request) {

		// start user session and set cookie
		uuid := uuid.NewV4().String()

		ds := DockerSession{port: getFreePort(usedports, startPort)}

		// set usedports for assigned port
		usedports[ds.port] = true
		usersessions[uuid] = ds

		// set cookie to UUID
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "session_uuid", Value: uuid, Expires: expiration}
		http.SetCookie(w, &cookie)

		// ws_port to 4000 for debug
		cookieWs := http.Cookie{Name: "ws_port", Value: "4000", Expires: expiration}
		http.SetCookie(w, &cookieWs)

		// redirect to app
		http.Redirect(w, r, "/", 302)
	})

	http.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {

		// start user session and set cookie
		uuid := uuid.NewV4().String()

		ds := DockerSession{port: getFreePort(usedports, startPort)}

		// set usedports for assigned port
		usedports[ds.port] = true

		usersessions[uuid] = ds

		// set cookie to UUID
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "session_uuid", Value: uuid, Expires: expiration, Path: "/"}
		http.SetCookie(w, &cookie)

		cookieWs := http.Cookie{Name: "ws_port", Value: strconv.Itoa(ds.port + wsBase), Expires: expiration, Path: "/"}
		http.SetCookie(w, &cookieWs)

		cookieTerm := http.Cookie{Name: "term_port", Value: strconv.Itoa(ds.port + termBase), Expires: expiration, Path: "/"}
		http.SetCookie(w, &cookieTerm)

		// initialize ws Proxy
		// TODO: Create way to kill wsProxy (channels?)
		if _, ok := usedwsports[wsBase+ds.port]; !ok {
			go wsProxy(ds.port)
			usedwsports[wsBase+ds.port] = true
		} else {
			fmt.Println("WS Port already initialized for port: " + strconv.Itoa(wsBase+ds.port))
		}

		// log
		fmt.Println("Create users session with UUID: " + uuid + ".")

		printActiveUsers(usersessions)

		var dockerCmd *exec.Cmd

		// start docker instance based on OS
		if runtime.GOOS == "windows" {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.port), "--rm=true", "-v", "C:\\Users\\Rick\\workspace\\grid-docker\\grid-app:/home/source", "-p", strconv.Itoa(ds.port)+":8080", "-p", strconv.Itoa(termBase+ds.port)+":3000", "goserver")
		} else {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.port), "--rm=true", "-v", "/Users/rick/workspace/grid-docker/grid-app:/home/source", "-p", strconv.Itoa(ds.port)+":8080", "-p", strconv.Itoa(termBase+ds.port)+":3000", "goserver")
		}

		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		dockerCmd.Start()

		fmt.Printf("[Spawn] Tried creating docker instance")

		// redirect to app
		time.Sleep(time.Second * 6)
		http.Redirect(w, r, "/", 302)
	})

	http.HandleFunc("/destruct", func(w http.ResponseWriter, r *http.Request) {

		uuidCookie, err := r.Cookie("session_uuid")
		if err != nil {
			log.Fatal(err)
		}

		uuid := uuidCookie.Value

		ds := usersessions[uuid]

		// set usedports for assigned port
		usedports[ds.port] = false

		// delete from user sessions
		delete(usersessions, uuid)

		// kill Docker instance
		dockerCmd := exec.Command("docker", "kill", "grid"+strconv.Itoa(ds.port))
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		dockerCmd.Start()

		fmt.Println("Destruct users session with UUID: " + uuid + ".")

		printActiveUsers(usersessions)

	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		// append port based on UUID
		uuidCookie, err := r.Cookie("session_uuid")
		if err != nil {
			fmt.Println(err)

			http.Redirect(w, r, "/dashboard/", 302)

			return
		}

		uuid := uuidCookie.Value

		fmt.Println("Following UUID requested at root: " + uuid)

		ds := usersessions[uuid]

		if ds.port == 0 {

			// if no cookie is set or found redirect
			http.Redirect(w, r, "/dashboard/", 302)

		} else {

			fmt.Println(r.RequestURI)

			base, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(ds.port) + r.RequestURI)
			if err != nil {
				log.Fatal(err)
			}

			body, err := ioutil.ReadAll(r.Body)

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// you can reassign the body if you need to parse it as multipart
			r.Body = ioutil.NopCloser(bytes.NewReader(body))

			proxyReq, err := http.NewRequest(r.Method, base.String(), bytes.NewReader(body))

			proxyReq.Header = make(http.Header)
			for h, val := range r.Header {
				proxyReq.Header[h] = val
			}

			resp, err := httpClient.Do(proxyReq)
			fmt.Println("Send request to " + base.String() + "from" + r.UserAgent())

			for h, val := range resp.Header {
				w.Header().Set(h, strings.Join(val, ","))
			}

			w.WriteHeader(resp.StatusCode)

			backendBody, _ := ioutil.ReadAll(resp.Body)

			w.Write(backendBody)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()

		}

	})

	log.Fatal(http.ListenAndServe(":8080", nil))

}
