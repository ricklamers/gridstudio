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
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/twinj/uuid"
)

const termBase = -1000
const httpPort = 80
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

func checkQuickHTTPResponse(requestURL string) bool {

	timeout := time.Duration(150 * time.Millisecond)

	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(requestURL)
	if err != nil {
		return false
	}

	if resp.StatusCode == 200 {
		return true
	}

	return false
}

type WorkspaceRow struct {
	ID    int    `json:"id"`
	Owner int    `json:"owner"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
}

func getUserId(r *http.Request, db *sql.DB) int {

	cookieEmail, err1 := r.Cookie("email")
	cookieToken, err2 := r.Cookie("token")

	if err1 != nil || err2 != nil {
		return -1
	}

	ownerQuery, err := db.Query("SELECT id FROM users WHERE email = ? AND token = ?", cookieEmail.Value, cookieToken.Value)
	if err != nil {
		log.Fatal(err)
	}
	defer ownerQuery.Close()

	if ownerQuery.Next() {

		var userId int
		ownerQuery.Scan(&userId)

		return userId
	} else {
		return -1
	}
}

func checkLoggedIn(email string, token string, db *sql.DB) bool {

	rows, err := db.Query("SELECT id FROM users WHERE email = ? AND token = ?", email, token)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

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

	// db, err := sql.Open("mysql", "root:manneomanneo@/grid")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	db, err := sql.Open("sqlite3", "db/manager.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

			fmt.Println("Hashed PW: " + passwordHashed)

			// check token validity
			rows, err := db.Query("SELECT id FROM users WHERE email = ? AND password = ?", email, passwordHashed)
			if err != nil {
				log.Fatal(err)
			}

			if rows.Next() {

				rows.Close()

				fmt.Println("Login, found user matching hashed PW.")

				// log in
				expiration := time.Now().Add(365 * 24 * time.Hour)

				key := RandStringBytes(32)

				token := string(key)
				tokenCookie := http.Cookie{Name: "token", Value: token, Expires: expiration}
				http.SetCookie(w, &tokenCookie)

				emailCookie := http.Cookie{Name: "email", Value: email, Expires: expiration}
				http.SetCookie(w, &emailCookie)

				// update token in sql
				_, err := db.Exec("UPDATE users SET token = ? WHERE email = ?", token, email)
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

	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {

		fileString := "static/home" + strings.Replace(r.URL.Path, "static/", "", -1)

		// fmt.Println("Serving static file: " + fileString)

		http.ServeFile(w, r, fileString)

	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/home/index.html")
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

	http.HandleFunc("/workspace-change-name", func(w http.ResponseWriter, r *http.Request) {
		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 == nil && err2 == nil && checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {

			r.ParseForm()
			id := r.Form.Get("workspaceId")
			newName := r.Form.Get("workspaceNewName")

			_, err := db.Exec("UPDATE workspaces SET name=? WHERE id = ?", newName, id)
			if err != nil {
				fmt.Println(err)
			}

		}
	})

	http.HandleFunc("/get-workspace-details", func(w http.ResponseWriter, r *http.Request) {

		workspaces := []WorkspaceRow{}

		r.ParseForm()
		requestedSlug := r.Form.Get("workspaceSlug")

		rows, err := db.Query("SELECT id, owner, slug, name FROM workspaces WHERE slug = ?", requestedSlug)
		if err != nil {
			fmt.Println(err)
		}
		defer rows.Close()

		var (
			id    int
			owner int
			slug  string
			name  string
		)

		for rows.Next() {
			err := rows.Scan(&id, &owner, &slug, &name)
			if err != nil {
				log.Fatal(err)
			}
			row := WorkspaceRow{ID: id, Owner: owner, Slug: slug, Name: name}

			workspaces = append(workspaces, row)
		}

		if len(workspaces) > 0 {
			js, err := json.Marshal(workspaces[0])

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(js)
		} else {
			http.NotFound(w, r)
		}

	})

	http.HandleFunc("/get-workspaces", func(w http.ResponseWriter, r *http.Request) {

		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 == nil && err2 == nil && checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {

			workspaces := []WorkspaceRow{}

			userId := getUserId(r, db)

			rows, err := db.Query("SELECT id, owner, slug, name FROM workspaces WHERE owner = ?", userId)
			defer rows.Close()

			var (
				id    int
				owner int
				slug  string
				name  string
			)

			for rows.Next() {
				err := rows.Scan(&id, &owner, &slug, &name)
				if err != nil {
					log.Fatal(err)
				}
				row := WorkspaceRow{ID: id, Owner: owner, Slug: slug, Name: name}

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

	http.HandleFunc("/create-debug/", func(w http.ResponseWriter, r *http.Request) {

		// start user session and set cookie
		splitURL := strings.Split(r.URL.Path, "/")

		if len(splitURL) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		uuid := splitURL[2]

		ds := dockermanager.DockerSession{Port: getFreePort(usedports, startPort)}

		// set usedports for assigned port
		usedports[ds.Port] = true
		usersessions[uuid] = ds

		// set cookie to UUID
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "session_uuid", Value: uuid, Expires: expiration}
		http.SetCookie(w, &cookie)

		// redirect to app
		http.Redirect(w, r, "/workspace/"+uuid+"/", 302)
	})

	http.HandleFunc("/remove/", func(w http.ResponseWriter, r *http.Request) {
		splitURL := strings.Split(r.URL.Path, "/")

		if len(splitURL) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		userID := getUserId(r, db)

		uuidFromUrl := splitURL[2]

		_, err := db.Exec("DELETE FROM workspaces WHERE owner = ? AND slug = ?", userID, uuidFromUrl)
		if err != nil {
			fmt.Println(err)
		}

		dirName := "userdata/workspace-" + uuidFromUrl

		if _, err := os.Stat(dirName); !os.IsNotExist(err) {
			removeCommand := "rm -rf " + dirName
			fmt.Println(removeCommand)
			chownCommand := exec.Command("/bin/sh", "-c", removeCommand)
			chownCommand.Start()
		}

		http.Redirect(w, r, "/dashboard/", 302)

	})

	http.HandleFunc("/copy/", func(w http.ResponseWriter, r *http.Request) {

		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 != nil || err2 != nil {
			http.Redirect(w, r, "/login", 302)
		} else if checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {

			splitURL := strings.Split(r.URL.Path, "/")

			if len(splitURL) < 3 {
				http.Redirect(w, r, "/dashboard/", 302)
				return
			}

			uuidFromUrl := splitURL[2]

			var dirName string

			dirName = "userdata/workspace-" + uuidFromUrl

			fmt.Println(dirName)

			if _, err := os.Stat(dirName); !os.IsNotExist(err) {
				// path/to/whatever does not exist

				userID := getUserId(r, db)

				newUuid := uuid.NewV4().String()
				newDirName := "userdata/workspace-" + newUuid

				// get name form DB
				rows, err := db.Query("SELECT name FROM workspaces WHERE slug = ?", uuidFromUrl)
				defer rows.Close()
				if err != nil {
					fmt.Println(err)
				}
				var (
					name    string
					oldName string
				)

				for rows.Next() {
					err := rows.Scan(&name)
					if err != nil {
						log.Fatal(err)
					}
					oldName = name
				}

				fmt.Println(oldName)

				newName := oldName + " (Copy)"

				// create database entry
				_, err2 := db.Exec("INSERT INTO workspaces (owner, slug, name) VALUES (?,?,?)", userID, newUuid, newName)
				if err2 != nil {
					fmt.Println(err2)
				}

				// copy directory
				copyCommand := "cp -a -p " + dirName + "/. " + newDirName
				fmt.Println(copyCommand)
				chownCommand := exec.Command("/bin/sh", "-c", copyCommand)
				chownCommand.Start()

				// redirect to initialize
				// http.Redirect(w, r, "/initialize?uuid="+newUuid, 302)
				http.Redirect(w, r, "/dashboard/", 302)

			} else {
				http.Redirect(w, r, "/dashboard/", 302)
			}

		} else {
			http.Redirect(w, r, "/dashboard/", 302)
		}

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
			userID := getUserId(r, db)

			// create database entry
			_, err := db.Exec("INSERT INTO workspaces (owner, slug, name) VALUES (?,?,?)", userID, uuidString, "Untitled")
			if err != nil {
				fmt.Println(err)
			}

			// create files
			os.MkdirAll(dirName, 0777)
			os.MkdirAll(dirName+"/sheetdata", 0777)
			os.MkdirAll(dirName+"/userfolder", 0777)

			// chownCommand := exec.Command("/bin/sh", "-c", "chown ricklamers:staff "+dirName+"; "+"chown ricklamers:staff "+dirName+"/sheetdata; "+"chown ricklamers:staff "+dirName+"/userfolder; "+"chmod 0666 "+dirName+"; "+"chmod 0666 "+dirName+"/sheetdata; "+"chmod 0666 "+dirName+"/userfolder;")
			chownCommand := exec.Command("/bin/sh", "-c", "chmod 0777 "+dirName+"; "+"chmod 0777 "+dirName+"/sheetdata; "+"chmod 0777 "+dirName+"/userfolder;")
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
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true",
				"-v", "/home/rick/workspace/grid-docker/grid-app:/home/source",
				"-v", "/home/rick/workspace/grid-docker/proxy/userdata/workspace-"+uuidString+"/userfolder:/home/user",
				"-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "-d=false", "goserver")
		} else if runtime.GOOS == "windows" {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true",
				"-v", "C:\\Users\\Rick\\workspace\\grid-docker\\grid-app:/home/source",
				"-v", "C:\\Users\\Rick\\workspace\\grid-docker\\proxy\\userdata\\workspace-"+uuidString+"\\userfolder:/home/user",
				"-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "goserver")
		} else {
			dockerCmd = exec.Command("docker", "run", "--name=grid"+strconv.Itoa(ds.Port), "--rm=true", "-v", "/Users/ricklamers/workspace/grid-docker/proxy/userdata/workspace-"+uuidString+"/userfolder:/home/user", "-v", "/Users/ricklamers/workspace/grid-docker/grid-app:/home/source", "-p", strconv.Itoa(ds.Port)+":8080", "-p", strconv.Itoa(termBase+ds.Port)+":3000", "-d=false", "goserver")
		}

		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		fmt.Printf("[Spawn] Tried creating docker instance")

		dockerCmd.Start()

		// start listen loop
		for {

			time.Sleep(time.Second / 2)

			if checkQuickHTTPResponse("http://127.0.0.1:" + strconv.Itoa(ds.Port) + "/upcheck") {
				if !creatingNew {
					// // copy files to docker container
					// copyCmds := []string{"cp", "/home/rick/workspace/grid-docker/proxy/" + dirName + "/userfolder/.", "grid" + strconv.Itoa(ds.Port) + ":/home/user/"}
					// dockerCopyCmd := exec.Command("docker", copyCmds...)

					// fmt.Println(copyCmds)
					// dockerCopyCmd.Run()
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

		// fmt.Println(splitUrl)

		if len(splitUrl) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		uuid := splitUrl[2]

		// fmt.Println("Following UUID requested at root: " + uuid)

		ds := usersessions[uuid]

		if ds.Port == 0 {

			// if no uuid session is found redirect to dashboard
			http.Redirect(w, r, "/dashboard/", 302)
			return

		} else {

			httpRedirPort := ds.Port

			workspacePrefix := "workspace/" + uuid + "/"
			requestString := r.RequestURI

			// fmt.Println("requestString (before replace): " + requestString)

			if strings.Contains(requestString, "/terminals") {
				httpRedirPort = ds.TermPort
			}

			if strings.Contains(requestString, workspacePrefix) {
				requestString = strings.Replace(requestString, workspacePrefix, "", -1)
			}

			// fmt.Println("workspacePrefix: " + workspacePrefix)
			// fmt.Println("requestString (after replace): " + requestString)

			fmt.Println("HTTP proxy: " + "http://127.0.0.1:" + strconv.Itoa(httpRedirPort) + requestString)

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
			// fmt.Println("Send request to " + base.String() + " from " + r.UserAgent())

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
