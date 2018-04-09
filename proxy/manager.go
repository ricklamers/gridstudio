package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"time"

	"database/sql"

	"./dockermanager"
	"./websocketproxy"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/twinj/uuid"
)

const termBase = -1000
const httpPort = 8080
const wsPort = 443
const constPasswordSalt = "GY=B[+inIGy,W5@U%kwP/wWrw%4uQ?6|8P$]9{X=-XY:LO6*1cG@P-+`<s=+TL#N"

var src = rand.NewSource(time.Now().UnixNano())

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func wsProxy(wsPort int, usersessions map[string]dockermanager.DockerSession) {

	// base, err := url.Parse("ws://127.0.0.1:" + strconv.Itoa(port) + "/ws")
	// fmt.Println("WS base: " + "ws://127.0.0.1:" + strconv.Itoa(port) + "/ws")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	fmt.Println("WS Listening on port: " + strconv.Itoa(wsPort))

	wsp := websocketproxy.NewProxy(usersessions)
	wsp.Upgrader = &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	errWS := http.ListenAndServe(":"+strconv.Itoa(wsPort), wsp)
	if errWS != nil {
		log.Fatal(errWS)
	}
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

func printActiveUsers(usersessions map[string]dockermanager.DockerSession) {
	fmt.Printf("%d user sessions active.\n", len(usersessions))
}

func checkQuickHttpResponse(requestUrl string) bool {

	timeout := time.Duration(150 * time.Millisecond)

	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(requestUrl)
	if err != nil {
		return false
	}

	if resp.StatusCode == 200 {
		return true
	}

	return false
}

type WorkspaceRow struct {
	Id    int    `json:"id"`
	Owner int    `json:"owner"`
	Slug  string `json:"slug"`
}

func getUserId(r *http.Request, db *sql.DB) int {

	cookieEmail, err1 := r.Cookie("email")
	cookieToken, err2 := r.Cookie("token")

	if err1 != nil || err2 != nil {
		return -1
	}

	owner_query, err := db.Query("SELECT id FROM users WHERE email = ? AND token = ?", cookieEmail.Value, cookieToken.Value)
	if err != nil {
		log.Fatal(err)
	}

	if owner_query.Next() {

		var user_id int
		owner_query.Scan(&user_id)

		return user_id
	} else {
		return -1
	}
}

func checkLoggedIn(email string, token string, db *sql.DB) bool {

	rows, err := db.Query("SELECT id FROM users WHERE email = ? AND token = ?", email, token)

	if err != nil {
		log.Fatal(err)
	}

	if rows.Next() {
		return true
	}

	return false
}
func renderTemplate(file string) string {
	files := []string{file}

	files = append([]string{"static/dashboard/header.html"}, files...)
	files = append(files, "static/dashboard/footer.html")

	return concatHtmlFiles(files)
}

func RandStringBytes(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func concatHtmlFiles(files []string) string {

	buf := bytes.NewBuffer(nil)
	for _, filename := range files {
		f, _ := os.Open(filename) // Error handling elided for brevity.
		io.Copy(buf, f)           // Error handling elided for brevity.
		f.Close()
	}
	return string(buf.Bytes())
}

func main() {

	// build a map that holds all user sessions

	// form: UID key and int as port of active docker client
	var startPort = 4000
	var usedports map[int]bool
	var usersessions map[string]dockermanager.DockerSession
	usersessions = make(map[string]dockermanager.DockerSession)
	usedports = make(map[int]bool)

	db, err := sql.Open("mysql", "root:manneomanneo@/grid")
	if err != nil {
		log.Fatal(err)
	}

	// index.html to initialize
	httpClient := http.Client{}

	http.HandleFunc("/dashboard/static/", func(w http.ResponseWriter, r *http.Request) {

		fileString := "static/dashboard" + strings.Replace(r.URL.Path, "dashboard/static/", "", -1)

		// fmt.Println("Serving static file: " + fileString)

		http.ServeFile(w, r, fileString)

	})

	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		expiration := time.Now().Add(time.Hour * -1)

		tokenCookie := http.Cookie{Name: "token", Value: "", Expires: expiration}
		http.SetCookie(w, &tokenCookie)

		emailCookie := http.Cookie{Name: "email", Value: "", Expires: expiration}
		http.SetCookie(w, &emailCookie)

		http.Redirect(w, r, "/login", 302)
	})

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {

		if r.Method == "POST" {
			// handle post to set cookie token
			r.ParseForm()
			email := r.Form.Get("email")
			password := r.Form.Get("password")

			h := sha1.New()
			io.WriteString(h, password+constPasswordSalt)
			passwordHashed := base64.URLEncoding.EncodeToString(h.Sum(nil))

			// fmt.Println("Hashed PW: " + passwordHashed)

			// check token validity
			rows, err := db.Query("SELECT id FROM users WHERE email = ? AND password = ?", email, passwordHashed)

			if err != nil {
				log.Fatal(err)
			}

			if rows.Next() {
				// log in
				expiration := time.Now().Add(365 * 24 * time.Hour)

				key := RandStringBytes(32)

				token := string(key)
				tokenCookie := http.Cookie{Name: "token", Value: token, Expires: expiration}
				http.SetCookie(w, &tokenCookie)

				emailCookie := http.Cookie{Name: "email", Value: email, Expires: expiration}
				http.SetCookie(w, &emailCookie)

				// update token in sql
				_, err := db.Query("UPDATE users SET token = ? WHERE email = ?", token, email)

				if err != nil {
					log.Fatal(err)
				}

				http.Redirect(w, r, "/dashboard/", 302)

			} else {
				http.Redirect(w, r, "/login?error=incorrect-login", 302)
			}
		}

		io.WriteString(w, renderTemplate("static/dashboard/login.html"))

	})

	http.HandleFunc("/dashboard/", func(w http.ResponseWriter, r *http.Request) {

		// handle authorization
		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 != nil || err2 != nil {
			http.Redirect(w, r, "/login", 302)
		} else {

			// check token

			if checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {

				// load dashboard
				io.WriteString(w, renderTemplate("static/dashboard/index.html"))

			} else {
				http.Redirect(w, r, "/login", 302)
			}
		}

	})

	// setup WS proxy for Go server and Terminal
	go wsProxy(wsPort, usersessions)

	http.HandleFunc("/get-workspaces", func(w http.ResponseWriter, r *http.Request) {

		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 == nil && err2 == nil && checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {
			workspaces := []WorkspaceRow{}

			userId := getUserId(r, db)

			rows, err := db.Query("SELECT id, owner, slug FROM workspaces WHERE owner = ?", userId)

			var (
				id    int
				owner int
				slug  string
			)

			for rows.Next() {
				err := rows.Scan(&id, &owner, &slug)
				if err != nil {
					log.Fatal(err)
				}
				row := WorkspaceRow{Id: id, Owner: owner, Slug: slug}

				workspaces = append(workspaces, row)
			}

			js, err := json.Marshal(workspaces)

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(js)

		}

	})

	http.HandleFunc("/create-debug", func(w http.ResponseWriter, r *http.Request) {

		// start user session and set cookie
		uuid := uuid.NewV4().String()

		ds := dockermanager.DockerSession{Port: getFreePort(usedports, startPort)}

		// set usedports for assigned port
		usedports[ds.Port] = true
		usersessions[uuid] = ds

		// set cookie to UUID
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "session_uuid", Value: uuid, Expires: expiration}
		http.SetCookie(w, &cookie)

		// ws_port to 4000 for debug
		// cookieWs := http.Cookie{Name: "ws_port", Value: "4000", Expires: expiration}
		// http.SetCookie(w, &cookieWs)

		// redirect to app
		http.Redirect(w, r, "/", 302)
	})

	http.HandleFunc("/initialize", func(w http.ResponseWriter, r *http.Request) {

		r.ParseForm()

		var uuidString string
		creatingNew := false

		if len(r.Form.Get("uuid")) == 0 {
			// start user session and set cookie
			uuidString = uuid.NewV4().String()
			creatingNew = true
		} else {
			uuidString = r.Form.Get("uuid")
		}

		var dirName string

		dirName = "userdata/workspace-" + uuidString

		if creatingNew {
			userId := getUserId(r, db)

			// create database entry
			db.Query("INSERT INTO workspaces (owner, slug) VALUES (?,?)", userId, uuidString)

			// create files
			os.MkdirAll(dirName, 0750)
			os.MkdirAll(dirName+"/sheetdata", 0750)
			os.MkdirAll(dirName+"/userfolder", 0750)

			chownCommand := exec.Command("/bin/sh", "-c", "chown rick:rick "+dirName+"; "+"chown rick:rick "+dirName+"/sheetdata; "+"chown rick:rick "+dirName+"/userfolder; "+"chmod 0750 "+dirName+"; "+"chmod 0750 "+dirName+"/sheetdata; "+"chmod 0750 "+dirName+"/userfolder;")
			chownCommand.Start()

		}

		ds := dockermanager.DockerSession{Port: getFreePort(usedports, startPort)}
		ds.TermPort = ds.Port + termBase

		// set usedports for assigned port
		usedports[ds.Port] = true

		usersessions[uuidString] = ds

		// set cookie to UUID
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "session_uuid", Value: uuidString, Expires: expiration, Path: "/"}
		http.SetCookie(w, &cookie)

		// log
		fmt.Println("Create users session with UUID: " + uuidString + ".")

		printActiveUsers(usersessions)

		var dockerCmd *exec.Cmd

		// start docker instance based on OS
		if runtime.GOOS == "linux" {
			// TODO: add GPU docker - big change - will be done later
			// dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true", "-v", "/home/rick/workspace/grid-docker/grid-app:/home/source", "-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "--device=/dev/nvidia0:/dev/nvidia0", "--device=/dev/nvidiactl:/dev/nvidiactl", "--device=/dev/nvidia-uvm:/dev/nvidia-uvm", "--device=/dev/nvidia-modeset:/dev/nvidia-modeset", "goserver")
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true", "-v", "/home/rick/workspace/grid-docker/grid-app:/home/source", "-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "-d=true", "goserver")
		} else if runtime.GOOS == "windows" {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true", "-v", "C:\\Users\\Rick\\workspace\\grid-docker\\grid-app:/home/source", "-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "goserver")
		} else {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true", "-v", "/home/rick/workspace/grid-docker/grid-app:/home/source", "-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "goserver")
		}

		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		fmt.Printf("[Spawn] Tried creating docker instance")

		dockerCmd.Start()

		// start listen loop
		for {

			time.Sleep(time.Second / 2)

			if checkQuickHttpResponse("http://127.0.0.1:" + strconv.Itoa(ds.Port) + "/upcheck") {
				if !creatingNew {
					// copy files to docker container
					copyCmds := []string{"cp", "/home/rick/workspace/grid-docker/proxy/" + dirName + "/userfolder/.", "grid" + strconv.Itoa(ds.Port) + ":/home/user/"}
					dockerCopyCmd := exec.Command("docker", copyCmds...)

					fmt.Println(copyCmds)
					dockerCopyCmd.Run()
				}

				http.Redirect(w, r, "/workspace/"+uuidString+"/", 302)

				return
			}

		}

	})

	http.HandleFunc("/destruct/", func(w http.ResponseWriter, r *http.Request) {

		splitUrl := strings.Split(r.URL.Path, "/")

		fmt.Println(splitUrl)

		if len(splitUrl) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		uuid := splitUrl[2]

		ds := usersessions[uuid]

		// set usedports for assigned port
		usedports[ds.Port] = false

		// delete from user sessions
		delete(usersessions, uuid)

		// kill Docker instance
		dockerCmd := exec.Command("docker", "kill", "grid"+strconv.Itoa(ds.Port))
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		dockerCmd.Start()

		fmt.Println("Destruct users session with UUID: " + uuid + ".")

		printActiveUsers(usersessions)

		http.Redirect(w, r, "/dashboard/", 302)

	})

	http.HandleFunc("/workspace/", func(w http.ResponseWriter, r *http.Request) {

		// append port based on UUID
		splitUrl := strings.Split(r.URL.Path, "/")

		fmt.Println(splitUrl)

		if len(splitUrl) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		uuid := splitUrl[2]

		fmt.Println("Following UUID requested at root: " + uuid)

		ds := usersessions[uuid]

		if ds.Port == 0 {

			// if no uuid session is found redirect to dashboard
			http.Redirect(w, r, "/dashboard/", 302)
			return

		} else {

			httpRedirPort := ds.Port

			workspacePrefix := "workspace/" + uuid + "/"
			requestString := r.RequestURI

			fmt.Println("requestString (before replace): " + requestString)

			if strings.Contains(requestString, "/terminals") {
				httpRedirPort = ds.TermPort
			}

			if strings.Contains(requestString, workspacePrefix) {
				requestString = strings.Replace(requestString, workspacePrefix, "", -1)
			}

			fmt.Println("workspacePrefix: " + workspacePrefix)
			fmt.Println("requestString (after replace): " + requestString)

			base, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(httpRedirPort) + requestString)
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
			fmt.Println("Send request to " + base.String() + " from " + r.UserAgent())

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

	fmt.Println("Listening on port: " + strconv.Itoa(httpPort))

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(httpPort), nil))

}
