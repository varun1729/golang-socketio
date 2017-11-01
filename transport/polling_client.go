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
)

type OpenSequence struct {
	Sid          string        `json:"sid"`
	Upgrades     []string      `json:"upgrades"`
	PingInterval time.Duration `json:"pingInterval"`
	PingTimeout  time.Duration `json:"pingTimeout"`
}

type PollingClientConnection struct {
	transport *PollingClientTransport
	client    *http.Client
	url       string
	sid       string
}

func (plc *PollingClientConnection) GetMessage() (string, error) {
	logging.Log().Debug("Get request sended")

	resp, err := plc.client.Get(plc.url)
	if err != nil {
		logging.Log().Debug("error in get client: ", err)
		return "", err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error read resp body: ", err)
		return "", err
	}

	bodyString := string(bodyBytes)
	logging.Log().Debug("bodyString: ", bodyString)
	index := strings.Index(bodyString, ":")

	body := bodyString[index+1:]
	return body, nil
}

func (plc *PollingClientConnection) WriteMessage(message string) error {
	msgToWrite := strconv.Itoa(len(message)) + ":" + message
	logging.Log().Debug("write msg: ", msgToWrite)
	var jsonStr = []byte(msgToWrite)

	resp, err := plc.client.Post(plc.url, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		logging.Log().Debug("error in post client: ", err)
		return err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error read resp post body: ", err)
		return err
	}

	resp.Body.Close()
	bodyString := string(bodyBytes)
	if bodyString != "ok" {
		return errors.New("message post not ok")
	}

	return nil
}

func (plc *PollingClientConnection) Close() {
	plc.WriteMessage("1")
}

func (plc *PollingClientConnection) PingParams() (time.Duration, time.Duration) {
	return plc.transport.PingInterval, plc.transport.PingTimeout
}

type PollingClientTransport struct {
	PingInterval   time.Duration
	PingTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	Headers  http.Header
	sessions sessionMap
}

func (plt *PollingClientTransport) HandleConnection(w http.ResponseWriter, r *http.Request) (Connection, error) {
	return nil, nil
}
func (plt *PollingClientTransport) Serve(w http.ResponseWriter, r *http.Request) {}
func (plt *PollingClientTransport) SetSid(sid string, conn Connection)           {}

// Connecting to server, 3 http requests for connecting sequence
func (plt *PollingClientTransport) Connect(url string) (Connection, error) {
	plc := &PollingClientConnection{
		transport: plt,
		client:    &http.Client{},
		url:       url,
	}

	resp, err := plc.client.Get(plc.url)
	if err != nil {
		logging.Log().Debug("error in get client: ", err)
		return nil, err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log().Debug("error read resp body: ", err)
		return nil, err
	}

	resp.Body.Close()
	bodyString := string(bodyBytes)
	logging.Log().Debug("bodyString: ", bodyString)

	index := strings.Index(bodyString, ":")
	body := bodyString[index+1:]
	if string(body[0]) == "0" {
		bodyBytes2 := []byte(body[1:])
		var openSequence OpenSequence

		if err := json.Unmarshal(bodyBytes2, &openSequence); err != nil {
			logging.Log().Debug("read err: ", err)
			return nil, err
		}

		plc.url = plc.url + "&sid=" + openSequence.Sid
		logging.Log().Debug("plc.url ", plc.url)
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
		logging.Log().Debug("error read resp body: ", err)
		return nil, err
	}

	resp.Body.Close()
	bodyString = string(bodyBytes)
	logging.Log().Debug("bodyString: ", bodyString)
	index = strings.Index(bodyString, ":")
	body = bodyString[index+1:]

	if body == "40" {
		return plc, nil
	}

	return nil, errors.New("Not open message answer")
}

// Returns polling transport with default params
func GetDefaultPollingClientTransport() *PollingClientTransport {
	return &PollingClientTransport{
		PingInterval:   PlDefaultPingInterval,
		PingTimeout:    PlDefaultPingTimeout,
		ReceiveTimeout: PlDefaultReceiveTimeout,
		SendTimeout:    PlDefaultSendTimeout,

		Headers: nil,
	}
}
