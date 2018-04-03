package transport

import (
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mtfelian/golang-socketio/logging"
)

const (
	upgradeFailed = "Upgrade failed: "

	wsDefaultPingInterval   = 30 * time.Second
	wsDefaultPingTimeout    = 60 * time.Second
	wsDefaultReceiveTimeout = 60 * time.Second
	wsDefaultSendTimeout    = 60 * time.Second
	wsDefaultBufferSize     = 1024 * 32
)

// WebsocketTransportParams is a parameters for getting non-default websocket transport
type WebsocketTransportParams struct{ Headers http.Header }

var (
	errBinaryMessage     = errors.New("binary messages are not supported")
	errBadBuffer         = errors.New("buffer error")
	errPacketWrong       = errors.New("wrong packet type error")
	errMethodNotAllowed  = errors.New("method not allowed")
	errHttpUpgradeFailed = errors.New("http upgrade failed")
)

// WebsocketConnection represents websocket connection
type WebsocketConnection struct {
	socket    *websocket.Conn
	transport *WebsocketTransport
}

// GetMessage from the connection
func (wsc *WebsocketConnection) GetMessage() (string, error) {
	logging.Log().Debug("GetMessage ws begin")
	wsc.socket.SetReadDeadline(time.Now().Add(wsc.transport.ReceiveTimeout))

	msgType, reader, err := wsc.socket.NextReader()
	if err != nil {
		logging.Log().Debug("ws reading err ", err)
		return "", err
	}

	// supports only text messages exchange
	if msgType != websocket.TextMessage {
		logging.Log().Debug("ws reading err ErrorBinaryMessage")
		return "", errBinaryMessage
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		logging.Log().Debug("ws reading err ErrorBadBuffer")
		return "", errBadBuffer
	}

	text := string(data)
	logging.Log().Debug("GetMessage ws text ", text)

	// empty messages are not allowed
	if len(text) == 0 {
		logging.Log().Debug("ws reading err ErrorPacketWrong")
		return "", errPacketWrong
	}

	return text, nil
}

// SetSid does nothing for the websocket transport, it's used only when transport changes (from)
func (wsc *WebsocketTransport) SetSid(string, Connection) {}

// WriteMessage message into a connection
func (wsc *WebsocketConnection) WriteMessage(message string) error {
	logging.Log().Debug("WriteMessage ws ", message)
	wsc.socket.SetWriteDeadline(time.Now().Add(wsc.transport.SendTimeout))

	writer, err := wsc.socket.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}

	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}

	return writer.Close()
}

// Close the connection
func (wsc *WebsocketConnection) Close() error {
	logging.Log().Debug("ws close")
	return wsc.socket.Close()
}

// PingParams returns ping params
func (wsc *WebsocketConnection) PingParams() (time.Duration, time.Duration) {
	return wsc.transport.PingInterval, wsc.transport.PingTimeout
}

// WebsocketTransport implements websocket transport
type WebsocketTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	BufferSize int
	Headers    http.Header
}

// Connect to the given url
func (wst *WebsocketTransport) Connect(url string) (Connection, error) {
	dialer := websocket.Dialer{}
	socket, _, err := dialer.Dial(url, wst.Headers)
	if err != nil {
		return nil, err
	}

	return &WebsocketConnection{socket, wst}, nil
}

// HandleConnection
func (wst *WebsocketTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	if r.Method != http.MethodGet {
		http.Error(w, upgradeFailed+errMethodNotAllowed.Error(), http.StatusServiceUnavailable)
		return nil, errMethodNotAllowed
	}

	socket, err := (&websocket.Upgrader{
		ReadBufferSize:  wst.BufferSize,
		WriteBufferSize: wst.BufferSize,
	}).Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, upgradeFailed+err.Error(), 503)
		return nil, errHttpUpgradeFailed
	}

	return &WebsocketConnection{socket, wst}, nil
}

// Serve does nothing here. Websocket connection does not require any additional processing
func (wst *WebsocketTransport) Serve(w http.ResponseWriter, r *http.Request) {}

// DefaultWebsocketTransport returns websocket connection with default params
func DefaultWebsocketTransport() *WebsocketTransport {
	return &WebsocketTransport{
		PingInterval:   wsDefaultPingInterval,
		PingTimeout:    wsDefaultPingTimeout,
		ReceiveTimeout: wsDefaultReceiveTimeout,
		SendTimeout:    wsDefaultSendTimeout,

		BufferSize: wsDefaultBufferSize,
		Headers:    nil,
	}
}

// NewWebsocketTransport returns websocket transport with given params
func NewWebsocketTransport(params WebsocketTransportParams) *WebsocketTransport {
	tr := DefaultWebsocketTransport()
	tr.Headers = params.Headers
	return tr
}
