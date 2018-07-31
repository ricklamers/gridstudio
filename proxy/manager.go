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

	_ "github.com/go-sql-driver/mysql"
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

	fmt.Println("checkQuickHTTPResponse returned: " + strconv.Itoa(resp.StatusCode))

	return false
}

func checkIdleInstances(usersessions map[string]dockermanager.DockerSession, usedports map[int]bool) {

	// first check if

	for uuid, ds := range usersessions {

		// check if docker instance idle too long

		client := http.Client{
			Timeout: time.Millisecond * 150,
		}

		resp, err1 := client.Get("http://127.0.0.1:" + strconv.Itoa(ds.Port) + "/fell-idle-check")
		if err1 != nil {
			fmt.Println(err1)

			// docker instance unavailable, attempt destroy
			destructSession(uuid, usersessions, usedports)

		} else {

			body, err2 := ioutil.ReadAll(resp.Body)
			if err2 != nil {
				fmt.Println(err2)
			}

			bs := string(body)

			timeIdle, err3 := strconv.Atoi(bs)
			if err3 != nil {
				fmt.Println(err3)
			}

			fmt.Println("Session " + uuid + " is idle for " + strconv.Itoa(timeIdle) + " seconds.")

			// when idle for two minutes kill session

			if timeIdle > 120 {
				fmt.Println("Session " + uuid + " fell idle. Destructing Docker session")
				destructSession(uuid, usersessions, usedports)
			}

		}

	}

}

type WorkspaceRow struct {
	ID      int    `json:"id"`
	Owner   int    `json:"owner"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Created string `json:"created"`
	Shared  int    `json:"shared"`
}

type User struct {
	ID    int
	Email string
}

func getUser(r *http.Request, db *sql.DB) User {

	cookieEmail, err1 := r.Cookie("email")
	cookieToken, err2 := r.Cookie("token")

	user := User{}

	user.ID = -1

	if err1 != nil || err2 != nil {
		return user
	}

	ownerQuery, err := db.Query("SELECT id,email FROM users WHERE email = ? AND token = ?", cookieEmail.Value, cookieToken.Value)
	if err != nil {
		log.Fatal(err)
	}
	defer ownerQuery.Close()

	if ownerQuery.Next() {

		ownerQuery.Scan(&user.ID, &user.Email)

		return user
	} else {
		return user
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

func destructSession(uuid string, usersessions map[string]dockermanager.DockerSession, usedports map[int]bool) {

	if _, ok := usersessions[uuid]; !ok {
		fmt.Println("Tried destroying session " + uuid + ", but sessions not in active usersessions.")
	} else {
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

	}

	printActiveUsers(usersessions)

}

func idleChecking() {

	ticker := time.NewTicker(time.Second * 120)

	for {
		select {
		case <-ticker.C:
			resp, err := http.Get("http://127.0.0.1/check-idle-instances")

			fmt.Println("Checking idle instances, response: " + strconv.Itoa(resp.StatusCode))

			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func main() {

	// build a map that holds all user sessions

	// form: UID key and int as port of active docker client
	var startPort = 4000
	var usedports map[int]bool
	var usersessions map[string]dockermanager.DockerSession
	usersessions = make(map[string]dockermanager.DockerSession)
	usedports = make(map[int]bool)

	db, err := sql.Open("mysql", "root:manneomanneo@tcp(192.168.178.110:3306)/grid")
	if err != nil {
		log.Fatal(err)
	}

	// db, err := sql.Open("sqlite3", "db/manager.db")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer db.Close()

	// kill all docker instances
	fmt.Println("Killing all running docker instances...")

	killDockerInstances := exec.Command("/bin/sh", "-c", "sudo docker kill $(docker ps -q)")
	killDockerInstances.Start()

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
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "static/home/index.html")
		} else {
			http.NotFound(w, r)
		}
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

	go idleChecking()

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

	http.HandleFunc("/workspace-change-share", func(w http.ResponseWriter, r *http.Request) {
		cookieEmail, err1 := r.Cookie("email")
		cookieToken, err2 := r.Cookie("token")

		if err1 == nil && err2 == nil && checkLoggedIn(cookieEmail.Value, cookieToken.Value, db) {

			r.ParseForm()
			id := r.Form.Get("workspaceId")
			share, errAtoi := strconv.Atoi(r.Form.Get("shared"))

			user := getUser(r, db)

			if errAtoi != nil {
				fmt.Println("Could not changing sharing setting for workspace, could not parse share")
				return
			}

			_, err := db.Exec("UPDATE workspaces SET shared=? WHERE id = ? AND owner = ?", share, id, user.ID)
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

			user := getUser(r, db)

			rows, err := db.Query("SELECT id, owner, slug, name, created, shared FROM workspaces WHERE owner = ?", user.ID)
			defer rows.Close()

			var (
				id      int
				owner   int
				slug    string
				name    string
				created string
				shared  int
			)

			for rows.Next() {
				err := rows.Scan(&id, &owner, &slug, &name, &created, &shared)
				if err != nil {
					log.Fatal(err)
				}
				row := WorkspaceRow{ID: id, Owner: owner, Slug: slug, Name: name, Created: created, Shared: shared}

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

	http.HandleFunc("/check-idle-instances", func(w http.ResponseWriter, r *http.Request) {
		// trigger cron
		checkIdleInstances(usersessions, usedports)
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
		cookie := http.Cookie{Name: "session_uuid", Value: uuid, Expires: expiration, Path: "/"}
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

		user := getUser(r, db)

		uuidFromUrl := splitURL[2]

		_, err := db.Exec("DELETE FROM workspaces WHERE owner = ? AND slug = ?", user.ID, uuidFromUrl)
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

				requestingUser := getUser(r, db)

				newUuid := uuid.NewV4().String()
				newDirName := "userdata/workspace-" + newUuid

				// get name form DB
				rows, err := db.Query("SELECT name, shared, owner FROM workspaces WHERE slug = ? LIMIT 1", uuidFromUrl)
				defer rows.Close()
				if err != nil {
					fmt.Println(err)
				}
				var (
					name   string
					shared int
					owner  int
				)

				for rows.Next() {
					err := rows.Scan(&name, &shared, &owner)
					if err != nil {
						log.Fatal(err)
					}
				}

				if requestingUser.ID == owner || shared == 1 {

					newName := name + " (Copy)"

					created := time.Now().Format(time.RFC1123)
					// create database entry
					_, err2 := db.Exec("INSERT INTO workspaces (owner, slug, name, created, shared) VALUES (?,?,?,?, 0)", requestingUser.ID, newUuid, newName, created)
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
					http.Redirect(w, r, "/dashboard/?error=Sharing is disabled for this workspace.", 302)
				}

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

		user := getUser(r, db)

		if creatingNew {

			fmt.Println("No uuid found, creating new Docker instance...")

			// create database entry
			created := time.Now().Format(time.RFC1123)
			_, err := db.Exec("INSERT INTO workspaces (owner, slug, name, created, shared) VALUES (?,?,?,?,0)", user.ID, uuidString, "Untitled", created)
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

		} else {
			fmt.Println("Opening UUID: " + uuidString)
		}

		// check if Docker instance is running for UUID uuidString
		// if not, create usersessions

		var ds dockermanager.DockerSession

		if _, ok := usersessions[uuidString]; !ok {

			ds = dockermanager.DockerSession{Port: getFreePort(usedports, startPort)}
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

			t := time.Now()
			logFilename := user.Email + "-" + t.Format("2006-01-02 15-04-05") + ".txt"
			outfile, err := os.Create("logs/dockerlogs/" + logFilename)
			if err != nil {
				panic(err)
			}
			// defer outfile.Close()

			dockerCmd.Stdout = outfile
			dockerCmd.Stderr = outfile
			fmt.Println("[Spawn] Tried creating docker instance")

			dockerCmd.Start()

		} else {
			ds = usersessions[uuidString]
		}

		// start listen loop
		for {

			time.Sleep(time.Second / 2)

			if checkQuickHTTPResponse("http://127.0.0.1:" + strconv.Itoa(ds.Port) + "/upcheck") {

				http.Redirect(w, r, "/workspace/"+uuidString+"/", 302)

				return
			}

		}

	})

	http.HandleFunc("/destruct/", func(w http.ResponseWriter, r *http.Request) {

		splitUrl := strings.Split(r.URL.Path, "/")

		// fmt.Println(splitUrl)

		if len(splitUrl) < 3 {
			http.Redirect(w, r, "/dashboard/", 302)
			return
		}

		uuid := splitUrl[2]

		destructSession(uuid, usersessions, usedports)

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
			// fmt.Println("HTTP proxy: " + "http://127.0.0.1:" + strconv.Itoa(httpRedirPort) + requestString)

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

			if err == nil {

				for h, val := range resp.Header {
					w.Header().Set(h, strings.Join(val, ","))
				}

				w.WriteHeader(resp.StatusCode)

				backendBody, _ := ioutil.ReadAll(resp.Body)

				w.Write(backendBody)

			} else {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			// fmt.Println("Send request to " + base.String() + " from " + r.UserAgent())

			defer resp.Body.Close()

		}

	})

	fmt.Println("Listening on port: " + strconv.Itoa(httpPort))

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(httpPort), nil))

}
