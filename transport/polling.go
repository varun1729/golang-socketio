package transport

import (
	"errors"
	"fmt"
	"io/ioutil"
	_"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geneva-lake/golang-socketio/protocol"
)

const (
	PlDefaultPingInterval   = 30 * time.Second
	PlDefaultPingTimeout    = 60 * time.Second
	PlDefaultReceiveTimeout = 60 * time.Second
	PlDefaultSendTimeout    = 60 * time.Second
	StopMessage = "stop"
)

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	transport *PollingTransport
	eventsIn  chan string
	eventsOut chan string
	errors    chan string
}

func (plc *PollingConnection) GetMessage() (string, error) {
	select {
	case <-time.After(plc.transport.ReceiveTimeout):
		return "", errors.New("Receive time out")
	case msg := <-plc.eventsIn:
		fmt.Println("GetMessage: ", msg)
		if msg == protocol.CloseMessage {
			//plc.eventsOut <- protocol.CloseMessage
			fmt.Println("send message 1 to eventsOut")
			return "", errors.New("Close connection")
		}
		return msg, nil
	}
}

func (plc *PollingConnection) WriteMessage(message string) error {
	fmt.Println("WriteMessage ", message)
	plc.eventsOut <- message
	select {
	case <-time.After(plc.transport.SendTimeout):
		return errors.New("Write time out")
	case errString := <-plc.errors:
		if errString != "0" {
			return errors.New(errString)
		}
	}
	return nil
}

func (plc *PollingConnection) Close() {
	plc.WriteMessage("1")
}

func (plc *PollingConnection) PingParams() (time.Duration, time.Duration) {
	return plc.transport.PingInterval, plc.transport.PingTimeout
}

// sessionMap describes sessions needed for identifying polling connections with socket.io connections
type sessionMap struct {
	sync.Mutex
	sessions map[string]*PollingConnection
}

// Set sets sid to polling connection tr
func (s *sessionMap) Set(sid string, tr *PollingConnection) {
	s.Lock()
	defer s.Unlock()
	s.sessions[sid] = tr
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

func (plt *PollingTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	eventChan := make(chan string, 100)
	eventOutChan := make(chan string, 100)
	plc := &PollingConnection{
		transport: plt,
		eventsIn:  eventChan,
		eventsOut: eventOutChan,
		errors:    make(chan string),
	}

	return plc, nil
}

func (plt *PollingTransport) SetSid(sid string, conn Connection) {
	plt.sessions.Set(sid, conn.(*PollingConnection))
}

func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sid")
	conn, exists := plt.sessions.Get(sessionId)
	switch r.Method {
	case http.MethodGet:
		if !exists {
			return
		}
		fmt.Println("get method")
		conn.PollingWriter(w, r)
	case http.MethodPost:
		bodyBytes, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			fmt.Println("error in PollingTransport.Serve():", err)
			return
		}
		bodyString := string(bodyBytes)
		index := strings.Index(bodyString, ":")
		body := bodyString[index+1:]
		setHeaders(w)
		fmt.Println("post mseg: ", body)
		w.Write([]byte("ok"))
		fmt.Println("post response writed ")
		conn.eventsIn <- body
		fmt.Println("body to eventsIn ")
	}
}

/**
Returns polling transport with default params
*/
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

func (plc *PollingConnection) PollingWriter(w http.ResponseWriter, r *http.Request) {
	//setHeaders(w)
	select {
	case <-time.After(plc.transport.SendTimeout):
		fmt.Println("timeout message to write ")
		//_, err := w.Write([]byte("1:6"))
		//if err != nil {
		//	fmt.Println("timeout message to write err", err)
		//	plc.errors <- err.Error()
		//	return
		//}
		plc.errors <- "0"
	case events := <-plc.eventsOut:
		fmt.Println("get message to write ", events)
		if events == "1" {
			fmt.Println("writing message 1")
		}
		if events == "send received" {
			fmt.Println("send received write")
		}
		events = strconv.Itoa(len(events)) + ":" + events
		if events == "1:1" {
			fmt.Println("writing message 1:1")
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Don't forget to close the connection:
			 defer conn.Close()
			bufrw.WriteString("1:1")
			bufrw.Flush()
			//s, err := bufrw.ReadString('\n')
			////if err != nil {
			////	log.Printf("error reading string: %v", err)
			////	return
			////}
			//fmt.Fprintf(bufrw, "You said: %q\nBye.\n", s)
			//bufrw.Flush()
			fmt.Println("hijack return")
			plc.errors <- "0"
		} else {
			_, err := w.Write([]byte(events))
			if events == "1" {
				fmt.Println("writed message 1")
			}
			if err != nil {
				fmt.Println("err write message ", err)
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
	w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
	w.Header().Set("Expires", "0")                                         // Proxies.
}
