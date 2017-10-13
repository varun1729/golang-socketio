package transport

import (
	_"errors"
	"io/ioutil"
	"net/http"
	"time"

	"strconv"
	"log"
	"io"
	"fmt"
	"encoding/json"
)

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	transport           *PollingTransport
	eventsIn            chan string
	SubscriptionHandler func(w http.ResponseWriter, r *http.Request)
}

func (plc *PollingConnection) GetMessage() (message string, err error) {
	msg := <- plc.transport.IncomeMessageChan
	return msg, nil
}

func (plc *PollingConnection) WriteMessage(message string) error {
	plc.eventsIn <- message
	return nil
}

func (plc *PollingConnection) Close() {
	//plc.socket.Close()
}

func (plc *PollingConnection) PingParams() (interval, timeout time.Duration) {
	return plc.transport.PingInterval, plc.transport.PingTimeout
}

type PollingTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	BufferSize int

	Headers http.Header

	IncomeMessageChan chan string
}

func (plt *PollingTransport) Connect(url string) (conn Connection, err error) {
	return nil, nil
}

func (plt *PollingTransport) HandleConnection(
	w http.ResponseWriter, r *http.Request) (conn Connection, err error) {

	return &PollingConnection{transport: plt}, nil
}

/**
Websocket connection do not require any additional processing
*/
func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		plt.get(w, r)
	case "POST":
		plt.post(w, r)
	}
}

func (plt *PollingTransport) get(w http.ResponseWriter, r *http.Request) {
	//if !plt.getLocker.TryLock() {
	//	http.Error(w, "overlay get", http.StatusBadRequest)
	//	return
	//}
	//if plt.getState() != stateNormal {
	//	http.Error(w, "closed", http.StatusBadRequest)
	//	return
	//}
	//
	//defer func() {
	//	if plt.getState() == stateClosing {
	//		if plt.postLocker.TryLock() {
	//			plt.setState(stateClosed)
	//			plt.callback.OnClose(p)
	//			plt.postLocker.Unlock()
	//		}
	//	}
	//	plt.getLocker.Unlock()
	//}()
	//
	//<-plt.sendChan
	//
	//// XHR Polling
	//if plt.encoder.IsString() {
	//	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	//} else {
	//	w.Header().Set("Content-Type", "application/octet-stream")
	//}
	//plt.encoder.EncodeTo(w)

}

func (plt *PollingTransport) post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	bodyString := string(bodyBytes)
	plt.IncomeMessageChan <- bodyString
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

		Headers: nil,
	}
}

// GetPollingTransport returns websocket transport with additional params
// func GetPollingTransport(params PollingTransportParams) *PollingTransport {
// 	tr := GetDefaultPollingTransport()
// 	tr.Headers = params.Headers
// 	return tr
// }

// get web handler that has closure around sub chanel and clientTimeout channnel
func getLongPollSubscriptionHandler(EventsIn chan string, maxTimeoutSeconds int, loggingEnabled bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		timeout, err := strconv.Atoi(r.URL.Query().Get("timeout"))
		if loggingEnabled {
			log.Println("Handling HTTP request at ", r.URL)
		}
		// We are going to return json no matter what:
		w.Header().Set("Content-Type", "application/json")
		// Don't cache response:
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
		w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
		w.Header().Set("Expires", "0")                                         // Proxies.
		if err != nil || timeout > maxTimeoutSeconds || timeout < 1 {
			if loggingEnabled {
				log.Printf("Error: Invalid timeout param.  Must be 1-%d. Got: %q.\n",
					maxTimeoutSeconds, r.URL.Query().Get("timeout"))
			}
			io.WriteString(w, fmt.Sprintf("{\"error\": \"Invalid timeout arg.  Must be 1-%d.\"}", maxTimeoutSeconds))
			return
		}
		category := r.URL.Query().Get("category")
		if len(category) == 0 || len(category) > 1024 {
			if loggingEnabled {
				log.Printf("Error: Invalid subscription category, must be 1-1024 characters long.\n")
			}
			io.WriteString(w, "{\"error\": \"Invalid subscription category, must be 1-1024 characters long.\"}")
			return
		}
		// Default to only looking for current events
		//lastEventTime := time.Now()
		// since_time is string of milliseconds since epoch
		//lastEventTimeParam := r.URL.Query().Get("since_time")
		//if len(lastEventTimeParam) > 0 {
		//	// Client is requesting any event from given timestamp
		//	// parse time
		//	var parseError error
		//	lastEventTime, parseError = millisecondStringToTime(lastEventTimeParam)
		//	if parseError != nil {
		//		if loggingEnabled {
		//			log.Printf("Error parsing last_event_time arg. Parm Value: %s, Error: %s.\n",
		//				lastEventTimeParam, err)
		//		}
		//		io.WriteString(w, "{\"error\": \"Invalid last_event_time arg.\"}")
		//		return
		//	}
		//}
		//subscription, err := newclientSubscription(category, lastEventTime)
		//if err != nil {
		//	if loggingEnabled {
		//		log.Printf("Error creating new Subscription: %s.\n", err)
		//	}
		//	io.WriteString(w, "{\"error\": \"Error creating new Subscription.\"}")
		//	return
		//}
		//subscriptionRequests <- *subscription
		// Listens for connection close and un-register subscription in the
		// event that a client crashes or the connection goes down.  We don't
		// need to wait around to fulfill a subscription if no one is going to
		// receive it
		disconnectNotify := w.(http.CloseNotifier).CloseNotify()
		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			// Lets the subscription manager know it can discard this request's
			// channel.
			//clientTimeouts <- subscription.clientCategoryPair
			//timeout_resp := makeTimeoutResponse(time.Now())
			//if jsonData, err := json.Marshal(timeout_resp); err == nil {
			//	io.WriteString(w, string(jsonData))
			//} else {
			//	io.WriteString(w, "{\"error\": \"json marshaller failed\"}")
			//}
		case events := <- EventsIn:
			// Consume event.  Subscription manager will automatically discard
			// this client's channel upon sending event
			// NOTE: event is actually []Event
			if jsonData, err := json.Marshal(events); err == nil {
				io.WriteString(w, string(jsonData))
			} else {
				io.WriteString(w, "{\"error\": \"json marshaller failed\"}")
			}
		case <-disconnectNotify:
			// Client connection closed before any events occurred and before
			// the timeout was exceeded.  Tell manager to forget about this
			// client.
			//clientTimeouts <- subscription.clientCategoryPair
		}
	}
}
