// Package websocketproxy is a reverse proxy for WebSocket connections.
package websocketproxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"../sessionmanager"

	"github.com/gorilla/websocket"
)

var (
	// DefaultUpgrader specifies the parameters for upgrading an HTTP
	// connection to a WebSocket connection.
	DefaultUpgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	// DefaultDialer is a dialer with all fields set to the default zero values.
	DefaultDialer = websocket.DefaultDialer
)

var uuidRegex = regexp.MustCompile(`(?m)workspace\/([0-9a-z-]*)\/`)

// WebsocketProxy is an HTTP Handler that takes an incoming WebSocket
// connection and proxies it to another server.
type WebsocketProxy struct {
	// Director, if non-nil, is a function that may copy additional request
	// headers from the incoming WebSocket connection into the output headers
	// which will be forwarded to another server.
	Director func(incoming *http.Request, out http.Header)

	UserSessions map[string]sessionmanager.WorkspaceSession

	// Upgrader specifies the parameters for upgrading a incoming HTTP
	// connection to a WebSocket connection. If nil, DefaultUpgrader is used.
	Upgrader *websocket.Upgrader

	//  Dialer contains options for connecting to the backend WebSocket server.
	//  If nil, DefaultDialer is used.
	Dialer *websocket.Dialer
}

// ProxyHandler returns a new http.Handler interface that reverse proxies the
// request to the given target.
func ProxyHandler(UserSessions map[string]sessionmanager.WorkspaceSession) http.Handler {
	return NewProxy(UserSessions)
}

// NewProxy returns a new Websocket reverse proxy that rewrites the
// URL's to the scheme, host and base path provider in target.
func NewProxy(UserSessions map[string]sessionmanager.WorkspaceSession) *WebsocketProxy {
	return &WebsocketProxy{UserSessions: UserSessions}
}

// ServeHTTP implements the http.Handler that proxies WebSocket connections.
func (w *WebsocketProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// construct backendURL based on UserSessions
	uuidFromPath := ""

	matches := uuidRegex.FindStringSubmatch(req.URL.Path)
	if len(matches) >= 2 {
		uuidFromPath = matches[1]
	}

	if len(uuidFromPath) == 0 {
		fmt.Println("No session_uuid present for websocket initialize connect: " + uuidFromPath)
	}

	ws := w.UserSessions[uuidFromPath]

	if ws.Port == 0 {
		fmt.Println("No session present for given session_uuid: " + uuidFromPath)
	}

	websocketPort := ws.Port

	if strings.Contains(req.URL.String(), "terminals") {
		websocketPort = ws.TermPort
	}

	// Web socket URL request filtering/directing
	splitUrl := strings.Split(req.URL.Path, "/")

	if len(splitUrl) < 3 {
		return
	}
	uuid := splitUrl[2]

	requestString := req.RequestURI
	workspacePrefix := "workspace/" + uuid + "/"

	if strings.Contains(requestString, workspacePrefix) {
		requestString = strings.Replace(requestString, workspacePrefix, "", -1)
	}

	backendURL, err := url.Parse("ws://127.0.0.1:" + strconv.Itoa(websocketPort) + requestString)
	if err != nil {
		log.Fatal(err)
	}

	if backendURL == nil {
		log.Println("websocketproxy: backend URL is nil")
		http.Error(rw, "internal server error (code: 2)", http.StatusInternalServerError)
		return
	}

	dialer := w.Dialer
	if w.Dialer == nil {
		dialer = DefaultDialer
	}

	// Pass headers from the incoming request to the dialer to forward them to
	// the final destinations.
	requestHeader := http.Header{}
	if origin := req.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin", origin)
	}
	for _, prot := range req.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range req.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}

	// Pass X-Forwarded-For headers too, code below is a part of
	// httputil.ReverseProxy. See http://en.wikipedia.org/wiki/X-Forwarded-For
	// for more information
	// TODO: use RFC7239 http://tools.ietf.org/html/rfc7239
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	// Set the originating protocol of the incoming HTTP request. The SSL might
	// be terminated on our site and because we doing proxy adding this would
	// be helpful for applications on the backend.
	requestHeader.Set("X-Forwarded-Proto", "http")
	if req.TLS != nil {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	// Enable the director to copy any additional headers it desires for
	// forwarding to the remote server.
	if w.Director != nil {
		w.Director(req, requestHeader)
	}

	// Connect to the backend URL, also pass the headers we get from the requst
	// together with the Forwarded headers we prepared above.
	// TODO: support multiplexing on the same backend connection instead of
	// opening a new TCP connection time for each request. This should be
	// optional:
	// http://tools.ietf.org/html/draft-ietf-hybi-websocket-multiplexing-01
	connBackend, resp, err := dialer.Dial(backendURL.String(), requestHeader)
	if err != nil {
		log.Printf("websocketproxy: couldn't dial to remote backend url %s\n", err)
		return
	}
	defer connBackend.Close()

	upgrader := w.Upgrader
	if w.Upgrader == nil {
		upgrader = DefaultUpgrader
	}

	// Only pass those headers to the upgrader.
	upgradeHeader := http.Header{}
	if hdr := resp.Header.Get("Sec-Websocket-Protocol"); hdr != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", hdr)
	}
	if hdr := resp.Header.Get("Set-Cookie"); hdr != "" {
		upgradeHeader.Set("Set-Cookie", hdr)
	}

	// Now upgrade the existing incoming request to a WebSocket connection.
	// Also pass the header that we gathered from the Dial handshake.
	connPub, err := upgrader.Upgrade(rw, req, upgradeHeader)
	if err != nil {
		log.Printf("websocketproxy: couldn't upgrade %s\n", err)
		return
	}
	defer connPub.Close()

	errc := make(chan error, 2)

	replicateWebsocketConn := func(dst, src *websocket.Conn, dstName, srcName string) {
		var err error
		for {
			msgType, msg, err := src.ReadMessage()

			if err != nil {

				// Don't log ReadMessage errors. If read fails, just close proxy
				// log.Printf("websocketproxy: error when copying from %s to %s using ReadMessage: %v", srcName, dstName, err)

				if srcName == "client" {
					// send close to dst
					// fmt.Println("Send WS close to dst")
					dst.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, err.Error()), time.Time{})
				}

				break
			}
			err = dst.WriteMessage(msgType, msg)

			if err != nil {
				log.Printf("msgType: " + strconv.Itoa(msgType))
				log.Printf(string(msg))
				log.Printf("websocketproxy: error when copying from %s to %s using WriteMessage: %v", srcName, dstName, err)
				break
			} else {
				// log.Printf("websocketproxy: copying from %s to %s completed without error.", srcName, dstName)
			}
		}
		errc <- err
	}

	go replicateWebsocketConn(connPub, connBackend, "client", "backend")
	go replicateWebsocketConn(connBackend, connPub, "backend", "client")

	<-errc
}
