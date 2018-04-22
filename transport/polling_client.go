package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
)

// PollingClientConnection represents XHR polling client connection
type PollingClientConnection struct {
	transport *PollingClientTransport
	client    *http.Client
	url       string
	sid       string
}

// GetMessage performs a GET request to wait for the following message
func (plc *PollingClientConnection) GetMessage() (string, error) {
	logging.Log().Debug("Get request sent")

	resp, err := plc.client.Get(plc.url)
	if err != nil {
		logging.Log().Debug("error from plc.client.Get():", err)
		return "", err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error in GetMessage, reading resp.Body:", err)
		return "", err
	}

	bodyString := string(bodyBytes)
	logging.Log().Debug("bodyString:", bodyString)
	index := strings.Index(bodyString, ":")

	body := bodyString[index+1:]
	return body, nil
}

// WriteMessage performs a POST request to send a message to server
func (plc *PollingClientConnection) WriteMessage(message string) error {
	msgToWrite := strconv.Itoa(len(message)) + ":" + message
	logging.Log().Debug("write msg: ", msgToWrite)
	var jsonStr = []byte(msgToWrite)

	resp, err := plc.client.Post(plc.url, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		logging.Log().Debug("error from pcl.Client.Post():", err)
		return err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error in WriteMessage(), reading resp.Body:", err)
		return err
	}

	resp.Body.Close()
	bodyString := string(bodyBytes)
	if bodyString != "ok" {
		return errors.New("message post result is not OK")
	}

	return nil
}

// Close the client connection gracefully
func (plc *PollingClientConnection) Close() error {
	return plc.WriteMessage(protocol.MessageClose)
}

// PingParams returns PingInterval and PingTimeout params
func (plc *PollingClientConnection) PingParams() (time.Duration, time.Duration) {
	return plc.transport.PingInterval, plc.transport.PingTimeout
}

// PollingClientTransport represents polling client transport parameters
type PollingClientTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	Headers  http.Header
	sessions sessions
}

// HandleConnection for the polling client is a placeholder
func (plt *PollingClientTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	return nil, nil
}

// Serve for the polling client is a placeholder
func (plt *PollingClientTransport) Serve(w http.ResponseWriter, r *http.Request) {}

// SetSid for the polling client is a placeholder
func (plt *PollingClientTransport) SetSid(sid string, conn Connection) {}

// openSequence represents a connection open sequence parameters
type openSequence struct {
	Sid          string        `json:"sid"`
	Upgrades     []string      `json:"upgrades"`
	PingInterval time.Duration `json:"pingInterval"`
	PingTimeout  time.Duration `json:"pingTimeout"`
}

// Connect to server, perform 3 HTTP requests in connecting sequence
func (plt *PollingClientTransport) Connect(url string) (Connection, error) {
	plc := &PollingClientConnection{transport: plt, client: &http.Client{}, url: url}

	resp, err := plc.client.Get(plc.url)
	if err != nil {
		logging.Log().Debug("error from plc.client.Get:", err)
		return nil, err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error from Connect() reading resp.Body:", err)
		return nil, err
	}

	resp.Body.Close()
	bodyString := string(bodyBytes)
	logging.Log().Debug("bodyString:", bodyString)

	index := strings.Index(bodyString, ":")
	body := bodyString[index+1:]
	if string(body[0]) == protocol.MessageOpen {
		bodyBytes2 := []byte(body[1:])
		var openSequence openSequence

		if err := json.Unmarshal(bodyBytes2, &openSequence); err != nil {
			logging.Log().Debug("json.Unmarshal() err in Connect():", err)
			return nil, err
		}

		plc.url += "&sid=" + openSequence.Sid
		logging.Log().Debug("plc.url:", plc.url)
	} else {
		return nil, errors.New("Not opensequence answer")
	}

	resp, err = plc.client.Get(plc.url)
	if err != nil {
		logging.Log().Debug("error in get client: ", err)
		return nil, err
	}

	bodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log()
		logging.Log().Debug("error read resp body: ", err)
		return nil, err
	}

	resp.Body.Close()
	bodyString = string(bodyBytes)
	logging.Log().Debug("bodyString: ", bodyString)
	index = strings.Index(bodyString, ":")
	body = bodyString[index+1:]

	if body == protocol.MessageEmpty {
		return plc, nil
	}

	return nil, errors.New("Not open message answer")
}

// DefaultPollingClientTransport returns client polling transport with default params
func DefaultPollingClientTransport() *PollingClientTransport {
	return &PollingClientTransport{
		PingInterval:   PlDefaultPingInterval,
		PingTimeout:    PlDefaultPingTimeout,
		ReceiveTimeout: PlDefaultReceiveTimeout,
		SendTimeout:    PlDefaultSendTimeout,
	}
}
