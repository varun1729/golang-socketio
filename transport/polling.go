package transport

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
)

const (
	PlDefaultPingInterval   = 30 * time.Second
	PlDefaultPingTimeout    = 60 * time.Second
	PlDefaultReceiveTimeout = 60 * time.Second
	PlDefaultSendTimeout    = 60 * time.Second
	StopMessage             = "stop"
	UpgradedMessage         = "upgrade"
)

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	Transport *PollingTransport
	eventsIn  chan string
	eventsOut chan string
	errors    chan string
	sid       string
}

// Realisation interface function, wait for incoming message
// from http request
func (plc *PollingConnection) GetMessage() (string, error) {
	select {
	case <-time.After(plc.Transport.ReceiveTimeout):
		logging.Log().Debug("Receive time out")
		return "", errors.New("Receive time out")
	case msg := <-plc.eventsIn:
		logging.Log().Debug("GetMessage: ", msg)
		if msg == protocol.MessageClose {
			logging.Log().Debug("send message 1 to eventsOut")
			return "", errors.New("Close connection")
		}
		return msg, nil
	}
}

// Realisation interface function, write message to channel
// and to PollingWriter
func (plc *PollingConnection) WriteMessage(message string) error {
	logging.Log().Debug("WriteMessage ", message)
	plc.eventsOut <- message
	logging.Log().Debug("Writed Message to eventsOut", message)
	select {
	case <-time.After(plc.Transport.SendTimeout):
		return errors.New("Write time out")
	case errString := <-plc.errors:
		if errString != "0" {
			logging.Log().Debug("errors write msg ", errString)
			return errors.New(errString)
		}
	}
	return nil
}

func (plc *PollingConnection) Close() {
	logging.Log().Debug("PollingConnection close ", plc.sid)
	plc.WriteMessage("6")
	plc.Transport.sessions.Delete(plc.sid)
}

func (plc *PollingConnection) PingParams() (time.Duration, time.Duration) {
	return plc.Transport.PingInterval, plc.Transport.PingTimeout
}

// sessionMap describes sessions needed for identifying polling connections with socket.io connections
type sessionMap struct {
	sync.Mutex
	sessions map[string]*PollingConnection
}

// Set sets sid to polling connection tr
func (s *sessionMap) Set(sid string, tr *PollingConnection) {
	logging.Log().Debug("sid: ", sid)
	s.Lock()
	defer s.Unlock()
	s.sessions[sid] = tr
}

// Delete sid from polling connection tr
func (s *sessionMap) Delete(sid string) {
	logging.Log().Debug("polling delete sid: ", sid)
	s.Lock()
	defer s.Unlock()
	delete(s.sessions, sid)
}

// Get returns polling connection if if exists, and bool existence flag
func (s *sessionMap) Get(sid string) (*PollingConnection, bool) {
	s.Lock()
	defer s.Unlock()
	tr, exists := s.sessions[sid]
	return tr, exists
}

type PollingTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	Headers  http.Header
	sessions sessionMap
}

func (plt *PollingTransport) Connect(url string) (Connection, error) {
	return nil, nil
}

// Create new PollingConnection
func (plt *PollingTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	return &PollingConnection{
		Transport: plt,
		eventsIn:  make(chan string),
		eventsOut: make(chan string),
		errors:    make(chan string),
	}, nil
}

func (plt *PollingTransport) SetSid(sid string, conn Connection) {
	plt.sessions.Set(sid, conn.(*PollingConnection))
	conn.(*PollingConnection).sid = sid
}

// Serve is for receiving messages from client, simple decoding also here
func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sid")
	conn, exists := plt.sessions.Get(sessionId)
	if !exists {
		return
	}

	switch r.Method {
	case http.MethodGet:
		logging.Log().Debug("get method")
		conn.PollingWriter(w, r)
	case http.MethodPost:
		bodyBytes, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			logging.Log().Debug("error in PollingTransport.Serve():", err)
			return
		}

		bodyString := string(bodyBytes)
		logging.Log().Debug("post mseg before split: ", bodyString)
		index := strings.Index(bodyString, ":")
		body := bodyString[index+1:]

		setHeaders(w)

		logging.Log().Debug("post mseg: ", body)
		w.Write([]byte("ok"))
		logging.Log().Debug("post response writed ")
		conn.eventsIn <- body
		logging.Log().Debug("body to eventsIn ")
	}
}

// Returns polling transport with default params
func GetDefaultPollingTransport() *PollingTransport {
	return &PollingTransport{
		PingInterval:   PlDefaultPingInterval,
		PingTimeout:    PlDefaultPingTimeout,
		ReceiveTimeout: PlDefaultReceiveTimeout,
		SendTimeout:    PlDefaultSendTimeout,
		sessions: sessionMap{
			Mutex:    sync.Mutex{},
			sessions: map[string]*PollingConnection{},
		},
		Headers: nil,
	}
}

// PollingWriter for write polling answer
func (plc *PollingConnection) PollingWriter(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	select {
	case <-time.After(plc.Transport.SendTimeout):
		logging.Log().Debug("timeout message to write ")
		plc.errors <- "0"
	case events := <-plc.eventsOut:
		logging.Log().Debug("post message to write ", events)
		events = strconv.Itoa(len(events)) + ":" + events
		if events == "1:6" {
			logging.Log().Debug("writing message 1:6")

			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
				return
			}

			conn, buffer, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			defer conn.Close()

			buffer.WriteString("HTTP/1.1 200 OK\r\n" +
				"Cache-Control: no-cache, private\r\n" +
				"Content-Length: 3\r\n" +
				"Date: Mon, 24 Nov 2016 10:21:21 GMT\r\n\r\n")
			buffer.WriteString("1:6")
			buffer.Flush()
			logging.Log().Debug("hijack return")
			plc.errors <- "0"
			plc.eventsIn <- StopMessage
		} else {
			_, err := w.Write([]byte(events))

			logging.Log().Debug("writed message ", events)

			if err != nil {
				logging.Log().Debug("err write message ", err)
				plc.errors <- err.Error()
				return
			}
			plc.errors <- "0"
		}
	}
}

func setHeaders(w http.ResponseWriter) {
	// We are going to return JSON no matter what:
	w.Header().Set("Content-Type", "application/json")
	// Don't cache response:
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1
	w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0
	w.Header().Set("Expires", "0")                                         // Proxies
}
