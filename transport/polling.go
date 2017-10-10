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
	msg <- plc.transport.IncomeMessageChan
	return msg, nil
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

	IncomeMessageChan chan string
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
		plt.get(w, r)
	case "POST":
		plt.post(w, r)
	}
}

func (plt *PollingTransport) get(w http.ResponseWriter, r *http.Request) {
	if !plt.getLocker.TryLock() {
		http.Error(w, "overlay get", http.StatusBadRequest)
		return
	}
	if plt.getState() != stateNormal {
		http.Error(w, "closed", http.StatusBadRequest)
		return
	}

	defer func() {
		if plt.getState() == stateClosing {
			if plt.postLocker.TryLock() {
				plt.setState(stateClosed)
				plt.callback.OnClose(p)
				plt.postLocker.Unlock()
			}
		}
		plt.getLocker.Unlock()
	}()

	<-plt.sendChan

	// XHR Polling
	if plt.encoder.IsString() {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	plt.encoder.EncodeTo(w)

}

func (plt *PollingTransport) post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	plt.IncomeMessageChan <- r.Body
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
