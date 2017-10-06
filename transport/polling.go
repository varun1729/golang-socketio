package transport

import (
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/jcuga/golongpoll"
)

type PollingTransportParams struct {
	Headers http.Header
}

type PollingConnection struct {
	transport *PollingTransport
}

func (plc *PollingConnection) GetMessage() (message string, err error) {

	return nil, nil
}

func (plc *PollingConnection) WriteMessage(message string) error {

	return nil
}

func (plc *PollingConnection) Close() {
	plc.socket.Close()
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
}

func (plt *PollingTransport) Connect(url string) (conn Connection, err error) {
	return nil, nil
}

func (plt *PollingTransport) HandleConnection(
	w http.ResponseWriter, r *http.Request) (conn Connection, err error) {

	if r.Method != "GET" {
		http.Error(w, upgradeFailed+ErrorMethodNotAllowed.Error(), 503)
		return nil, ErrorMethodNotAllowed
	}

	return &PollingConnection{plt}, nil
}

/**
Websocket connection do not require any additional processing
*/
func (plt *PollingTransport) Serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if jsonp := r.URL.Query().Get("j"); jsonp != "" {
			buf := bytes.NewBuffer(nil)
			if err := c.Payload.FlushOut(buf); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=UTF-8")
			pl := template.JSEscapeString(buf.String())
			w.Write([]byte("___eio[" + jsonp + "](\""))
			w.Write([]byte(pl))
			w.Write([]byte("\");"))
			return
		}
		if c.supportBinary {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		}
		if err := c.Payload.FlushOut(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	case "POST":
		mime := r.Header.Get("Content-Type")
		supportBinary, err := mimeSupportBinary(mime)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Payload.FeedIn(r.Body, supportBinary); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("ok"))
		return
	default:
		http.Error(w, "invalid method", http.StatusBadRequest)
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

		Headers: nil,
	}
}

// GetPollingTransport returns websocket transport with additional params
// func GetPollingTransport(params PollingTransportParams) *PollingTransport {
// 	tr := GetDefaultPollingTransport()
// 	tr.Headers = params.Headers
// 	return tr
// }
