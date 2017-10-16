package transport

import (
	_ "errors"
	"io/ioutil"
	"net/http"
	"time"

	_ "encoding/json"
	"fmt"
	_ "io"
	"log"
	"strconv"

	"strings"
)

var counter int =0;
var pc int =0;

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	transport           *PollingTransport
	eventsIn            chan string
	eventsOut            chan string
	//IncomeMessageChan chan string
	SubscriptionHandler func(w http.ResponseWriter, r *http.Request)
}

func (plc *PollingConnection) GetMessage() (message string, err error) {
	msg := <-plc.eventsIn
	return msg, nil
}

func (plc *PollingConnection) PostMessage(message string)  {
	plc.eventsIn <- message
}

func (plc *PollingConnection) WriteMessage(message string) error {
	fmt.Println("plc WriteMessage before", message)
	plc.eventsOut <- message
	fmt.Println("plc WriteMessage ", message)
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

	sids     map[string]*PollingConnection

	//IncomeMessageChan chan string

	//PollingConnection *PollingConnection
}

func (plt *PollingTransport) Connect(url string) (conn Connection, err error) {
	return nil, nil
}

func (plt *PollingTransport) HandleConnection(
	w http.ResponseWriter, r *http.Request) (conn Connection, err error) {
	eventChan := make(chan string, 100)
	eventOutChan := make(chan string, 100)
	SubscriptionHandler := getLongPollSubscriptionHandler(eventOutChan, 20, true)

	plc := &PollingConnection{transport: plt, eventsIn: eventChan, eventsOut: eventOutChan, SubscriptionHandler: SubscriptionHandler}
	//plc.SubscriptionHandler(w, r)
	//plt.PollingConnection = plc

	return plc, nil
}

func (plt *PollingTransport) SetSid(sid string, conn Connection) {
	plt.sids[sid] = conn.(*PollingConnection)
}

/**
Websocket connection do not require any additional processing
*/
func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sid")
	conn, ok := plt.sids[sessionId]
	switch r.Method {
	case "GET":
		//plt.get(w, r)
		if ok {
			conn.SubscriptionHandler(w, r)
		}
	case "POST":
		//plt.post(w, r)
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		bodyString := string(bodyBytes)
		fmt.Println("post ", bodyString)
		index := strings.Index(bodyString, ":")
		body := bodyString[index+1:len(bodyString)]
		fmt.Println("post body", body)
		//if pc == 0 {
		//	w.Write([]byte("2:40"))
		//} else {

		conn.eventsIn<-body
		w.Write([]byte("ok"))
	}

	//plt.PollingConnection.SubscriptionHandler(w, r)


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
	// XHR Polling

	//w.Header().Set("Content-Type", "text/plain; charset=UTF-8")

	//w.Write([]byte("97:0{\"sid\":\"il0ZOYhV67tK6MQ2AAAC\",\"upgrades\":[\"websocket\"],\"pingInterval\":25000,\"pingTimeout\":60000}2:40"))
	fmt.Println("get")

}

func (plt *PollingTransport) post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	//bodyBytes, _ := ioutil.ReadAll(r.Body)
	//bodyString := string(bodyBytes)
	//plt.IncomeMessageChan <- bodyString
	fmt.Println("post")
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
		sids:     make(map[string]*PollingConnection),

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
		//timeout, err := strconv.Atoi(r.URL.Query().Get("timeout"))

		timeout := 10

		fmt.Println("ollSubscriptionHandler, timeout ")

		if loggingEnabled {
			log.Println("Handling HTTP request at ", r.URL)
		}
		// We are going to return json no matter what:
		w.Header().Set("Content-Type", "application/json")
		// Don't cache response:
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
		w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
		w.Header().Set("Expires", "0")                                         // Proxies.
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		//w.Header().Set("Set-Cookie", "io=roenPVdL73Bj_LKMAAAE; Path=/; HttpOnly")

		if r.Method=="POST" {
			bodyBytes, _ := ioutil.ReadAll(r.Body)
			bodyString := string(bodyBytes)
			fmt.Println("post ", bodyString)
			//if pc == 0 {
			//	w.Write([]byte("2:40"))
			//} else {
			w.Write([]byte("ok"))
			//}
			//if bodyString=="14:40/socket.io/," {
			//	w.Write([]byte("ok"))
			//	return
			//}
			return
		}

		// if err != nil || timeout > maxTimeoutSeconds || timeout < 1 {
		// 	if loggingEnabled {
		// 		log.Printf("Error: Invalid timeout param.  Must be 1-%d. Got: %q.\n",
		// 			maxTimeoutSeconds, r.URL.Query().Get("timeout"))

		// 	}
		// 	io.WriteString(w, fmt.Sprintf("{\"error\": \"Invalid timeout arg.  Must be 1-%d.\"}", maxTimeoutSeconds))
		// 	return
		// }
		// category := r.URL.Query().Get("category")
		// if len(category) == 0 || len(category) > 1024 {
		// 	if loggingEnabled {
		// 		log.Printf("Error: Invalid subscription category, must be 1-1024 characters long.\n")
		// 	}
		// 	io.WriteString(w, "{\"error\": \"Invalid subscription category, must be 1-1024 characters long.\"}")
		// 	return
		// }
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
		//disconnectNotify := w.(http.CloseNotifier).CloseNotify()

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
				//fmt.Println("EventsIn timeout", timeout)
			case events := <-EventsIn:
				fmt.Println("EventsIn ", events)
				// Consume event.  Subscription manager will automatically discard
				// this client's channel upon sending event
				// NOTE: event is actually []Event
				// if jsonData, err := json.Marshal(events); err == nil {
				// 	io.WriteString(w, string(jsonData))
				// } else {
				// 	io.WriteString(w, "{\"error\": \"json marshaller failed\"}")
				// }

				events = strconv.Itoa(len(events)) + ":" + events
				//time.Sleep(2 * time.Second)
				w.Write([]byte(events))


//if counter==0 {
//	//io.WriteString(w, events+"40")
//	w.Write([]byte("97:0{\"sid\":\"il0ZOYhV67tK6MQ2AAAC\",\"upgrades\":[\"websocket\"],\"pingInterval\":25000,\"pingTimeout\":60000}"))
//	//w.Write([]byte("86:0{\"sid\":\"aaYsu0y-42QFC1AoAAAB\",\"upgrades\":[],\"pingInterval\":30000,\"pingTimeout\":60000}"))
//counter++
//	} else if counter==1 {
//	w.Write([]byte("2:40"))//("33:44/socket.io/,\"Invalid namespace\""))
//	counter++
//} else {
//	time.Sleep(26 * time.Second)
//	w.Write([]byte("1:3"))
//}



				//w.Write([]byte("0{\"sid\":\"O624xTOe2HpOooI0RZdy\",\"upgrades\":[],\"pingInterval\":30000,\"pingTimeout\":60000}40"))

				//w.Write([]byte(events))
				fmt.Println("EventsIn writed ", events)
				//case <-disconnectNotify:
				// Client connection closed before any events occurred and before
				// the timeout was exceeded.  Tell manager to forget about this
				// client.
				//clientTimeouts <- subscription.clientCategoryPair
				return
			}

	}
}
