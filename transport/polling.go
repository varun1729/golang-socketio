package transport

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	transport           *PollingTransport
	eventsIn            chan string
	eventsOut           chan string
	SubscriptionHandler func(w http.ResponseWriter, r *http.Request)
}

func (plc *PollingConnection) GetMessage() (string, error) {
	msg := <-plc.eventsIn
	return msg, nil
}

func (plc *PollingConnection) PostMessage(message string) {
	plc.eventsIn <- message
}

func (plc *PollingConnection) WriteMessage(message string) error {
	plc.eventsOut <- message
	return nil
}

func (plc *PollingConnection) Close() {

}

func (plc *PollingConnection) PingParams() (time.Duration, time.Duration) {
	return plc.transport.PingInterval, plc.transport.PingTimeout
}

type PollingTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	BufferSize int

	Headers http.Header

	sids map[string]*PollingConnection
}

func (plt *PollingTransport) Connect(url string) (Connection, error) {
	return nil, nil
}

func (plt *PollingTransport) HandleConnection(
	w http.ResponseWriter, r *http.Request) (Connection, error) {
	eventChan := make(chan string, 100)
	eventOutChan := make(chan string, 100)
	SubscriptionHandler := getLongPollSubscriptionHandler(eventOutChan)

	plc := &PollingConnection{
		transport:           plt,
		eventsIn:            eventChan,
		eventsOut:           eventOutChan,
		SubscriptionHandler: SubscriptionHandler,
	}

	return plc, nil
}

func (plt *PollingTransport) SetSid(sid string, conn Connection) {
	plt.sids[sid] = conn.(*PollingConnection)
}

func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sid")
	conn, ok := plt.sids[sessionId]
	switch r.Method {
	case "GET":
		if ok {
			conn.SubscriptionHandler(w, r)
		}
	case "POST":
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		bodyString := string(bodyBytes)
		index := strings.Index(bodyString, ":")
		body := bodyString[index+1 : len(bodyString)]
		conn.eventsIn <- body
		w.Write([]byte("ok"))
	}
}

/**
Returns websocket connection with default params
*/
func GetDefaultPollingTransport() *PollingTransport {
	return &PollingTransport{
		PingInterval:   WsDefaultPingInterval,
		PingTimeout:    WsDefaultPingTimeout,
		ReceiveTimeout: WsDefaultReceiveTimeout,
		SendTimeout:    WsDefaultSendTimeout,
		BufferSize:     WsDefaultBufferSize,
		sids:           make(map[string]*PollingConnection),
		Headers:        nil,
	}
}

// get web handler that has closure around sub chanel and clientTimeout channnel
func getLongPollSubscriptionHandler(EventsIn chan string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		timeout := 30

		// We are going to return json no matter what:
		w.Header().Set("Content-Type", "application/json")

		// Don't cache response:
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
		w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
		w.Header().Set("Expires", "0")                                         // Proxies.
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			w.Write([]byte("1:3"))
		case events := <-EventsIn:
			events = strconv.Itoa(len(events)) + ":" + events
			w.Write([]byte(events))
			return
		}
	}
}
