package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	fmt.Println("Get request sended")
	resp, err := plc.client.Get(plc.url)
	if err != nil {
		fmt.Println("error in get client: ", err)
		return "", err
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error read resp body: ", err)
		return "", err
	}
	bodyString := string(bodyBytes)
	fmt.Println("bodyString: ", bodyString)
	index := strings.Index(bodyString, ":")
	body := bodyString[index+1:]
	return body, nil
}

func (plc *PollingClientConnection) WriteMessage(message string) error {
	msgToWrite := strconv.Itoa(len(message)) + ":" + message
	fmt.Println("write msg: ", msgToWrite)
	var jsonStr = []byte(msgToWrite)
	resp, err := plc.client.Post(plc.url, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		fmt.Println("error in post client: ", err)
		return err
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error read resp post body: ", err)
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

func (plt *PollingClientTransport) Connect(url string) (Connection, error) {
	plc := &PollingClientConnection{
		transport: plt,
		client:    &http.Client{},
		url:       url,
	}

	resp, err := plc.client.Get(plc.url)
	if err != nil {
		fmt.Println("error in get client: ", err)
		return nil, err
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error read resp body: ", err)
		return nil, err
	}
	resp.Body.Close()
	bodyString := string(bodyBytes)
	fmt.Println("bodyString: ", bodyString)
	index := strings.Index(bodyString, ":")
	body := bodyString[index+1:]
	if string(body[0]) == "0" {
		bodyBytes2 := []byte(body[1:])
		var openSequence OpenSequence
		json.Unmarshal(bodyBytes2, &openSequence)
		if err != nil {
			log.Println("read err", err)
			return nil, err
		}
		plc.url = plc.url + "&sid=" + openSequence.Sid
		fmt.Println("plc.url ", plc.url)
	} else {
		return nil, errors.New("Not opensequence answer")
	}

	resp, err = plc.client.Get(plc.url)
	if err != nil {
		fmt.Println("error in get client: ", err)
		return nil, err
	}
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error read resp body: ", err)
		return nil, err
	}
	resp.Body.Close()
	bodyString = string(bodyBytes)
	fmt.Println("bodyString: ", bodyString)
	index = strings.Index(bodyString, ":")
	body = bodyString[index+1:]

	if body == "40" {
		return plc, nil
	} else {
		return nil, errors.New("Not open message answer")
	}
}

/**
Returns polling transport with default params
*/
func GetDefaultPollingClientTransport() *PollingClientTransport {
	return &PollingClientTransport{
		PingInterval:   PlDefaultPingInterval,
		PingTimeout:    PlDefaultPingTimeout,
		ReceiveTimeout: PlDefaultReceiveTimeout,
		SendTimeout:    PlDefaultSendTimeout,

		Headers: nil,
	}
}
