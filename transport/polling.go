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
	noError                 = "0"
)

// PollingTransportParams represents XHR polling transport params
type PollingTransportParams struct {
	Headers http.Header
}

// PollingConnection represents a XHR polling connection
type PollingConnection struct {
	Transport  *PollingTransport
	eventsInC  chan string
	eventsOutC chan string
	errors     chan string
	sessionID  string
}

// GetMessage waits for incoming message from the connection
func (plc *PollingConnection) GetMessage() (string, error) {
	select {
	case <-time.After(plc.Transport.ReceiveTimeout):
		logging.Log().Debug("GetMessage timed out")
		return "", errors.New("GetMessage timed out")
	case msg := <-plc.eventsInC:
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
	logging.Log().Debug("WriteMessage:", message)
	plc.eventsOutC <- message
	logging.Log().Debug("Written Message to eventsOutC:", message)
	select {
	case <-time.After(plc.Transport.SendTimeout):
		return errors.New("WriteMessage timed out")
	case errString := <-plc.errors:
		if errString != noError {
			logging.Log().Debug("error writing msg:", errString)
			return errors.New(errString)
		}
	}
	return nil
}

// Close the polling connection and delete session
func (plc *PollingConnection) Close() error {
	logging.Log().Debug("PollingConnection close ", plc.sessionID)
	err := plc.WriteMessage("6")
	plc.Transport.sessions.Delete(plc.sessionID)
	return err
}

func (plc *PollingConnection) PingParams() (time.Duration, time.Duration) {
	return plc.Transport.PingInterval, plc.Transport.PingTimeout
}

// sessions describes sessions needed for identifying polling connections with socket.io connections
type sessions struct {
	sync.Mutex
	m map[string]*PollingConnection
}

// Set sets sessionID to the given connection
func (s *sessions) Set(sessionID string, conn *PollingConnection) {
	logging.Log().Debug("sid:", sessionID)
	s.Lock()
	defer s.Unlock()
	s.m[sessionID] = conn
}

// Delete the sessionID
func (s *sessions) Delete(sessionID string) {
	logging.Log().Debug("polling delete sessionID:", sessionID)
	s.Lock()
	defer s.Unlock()
	delete(s.m, sessionID)
}

// Get returns polling connection if it exists, and bool existence flag
func (s *sessions) Get(sessionID string) (*PollingConnection, bool) {
	s.Lock()
	defer s.Unlock()
	connection, exists := s.m[sessionID]
	return connection, exists
}

type PollingTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	Headers  http.Header
	sessions sessions
}

func (t *PollingTransport) Connect(url string) (Connection, error) {
	return nil, nil
}

// HandleConnection returns a pointer to a new PollingConnection
func (t *PollingTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	return &PollingConnection{
		Transport:  t,
		eventsInC:  make(chan string),
		eventsOutC: make(chan string),
		errors:     make(chan string),
	}, nil
}

func (t *PollingTransport) SetSid(sessionID string, conn Connection) {
	t.sessions.Set(sessionID, conn.(*PollingConnection))
	conn.(*PollingConnection).sessionID = sessionID
}

// Serve is for receiving messages from client, simple decoding also here
func (t *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sid")
	conn, exists := t.sessions.Get(sessionId)
	if !exists {
		return
	}

	switch r.Method {
	case http.MethodGet:
		logging.Log().Debug("GET method")
		conn.PollingWriter(w, r)
	case http.MethodPost:
		bodyBytes, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			logging.Log().Debug("error in PollingTransport.Serve():", err)
			return
		}

		bodyString := string(bodyBytes)
		logging.Log().Debug("POST msg before split: ", bodyString)
		index := strings.Index(bodyString, ":")
		body := bodyString[index+1:]

		setHeaders(w)

		logging.Log().Debug("POST msg:", body)
		w.Write([]byte("ok"))
		logging.Log().Debug("POST response written")
		conn.eventsInC <- body
		logging.Log().Debug("POST sent to eventsInC")
	}
}

// DefaultPollingTransport returns PollingTransport with default params
func DefaultPollingTransport() *PollingTransport {
	return &PollingTransport{
		PingInterval:   PlDefaultPingInterval,
		PingTimeout:    PlDefaultPingTimeout,
		ReceiveTimeout: PlDefaultReceiveTimeout,
		SendTimeout:    PlDefaultSendTimeout,
		sessions: sessions{
			Mutex: sync.Mutex{},
			m:     map[string]*PollingConnection{},
		},
		Headers: nil,
	}
}

// PollingWriter for writing polling answer
func (plc *PollingConnection) PollingWriter(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	select {
	case <-time.After(plc.Transport.SendTimeout):
		logging.Log().Debug("timed out PollingWriter")
		plc.errors <- noError
	case message := <-plc.eventsOutC:
		logging.Log().Debug("post message to write ", message)
		message = strconv.Itoa(len(message)) + ":" + message
		if message == protocol.MessageClose+":"+protocol.MessageBlank {
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
			buffer.WriteString(protocol.MessageClose + ":" + protocol.MessageBlank)
			buffer.Flush()
			logging.Log().Debug("hijack return")
			plc.errors <- noError
			plc.eventsInC <- StopMessage
		} else {
			_, err := w.Write([]byte(message))
			logging.Log().Debug("written message:", message)
			if err != nil {
				logging.Log().Debug("error writing message:", err)
				plc.errors <- err.Error()
				return
			}
			plc.errors <- noError
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
