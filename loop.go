package gosocketio

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/geneva-lake/golang-socketio/protocol"
	"github.com/geneva-lake/golang-socketio/transport"
	"fmt"
)

const (
	queueBufferSize = 500
)

var (
	ErrorWrongHeader = errors.New("Wrong header")
)

/**
engine.io header to send or receive
*/
type Header struct {
	Sid          string   `json:"sid"`
	Upgrades     []string `json:"upgrades"`
	PingInterval int      `json:"pingInterval"`
	PingTimeout  int      `json:"pingTimeout"`
}

/**
socket.io connection handler

use IsAlive to check that handler is still working
use Dial to connect to websocket
use In and Out channels for message exchange
Close message means channel is closed
ping is automatic
*/
type Channel struct {
	conn transport.Connection

	out    chan string
	header Header

	alive     bool
	aliveLock sync.Mutex

	ack ackProcessor

	server        *Server
	ip            string
	requestHeader http.Header
}

/**
create channel, map, and set active
*/
func (c *Channel) initChannel() {
	//TODO: queueBufferSize from constant to server or client variable
	c.out = make(chan string, queueBufferSize)
	c.ack.resultWaiters = make(map[int](chan string))
	c.alive = true
}

/**
Get id of current socket connection
*/
func (c *Channel) Id() string {
	return c.header.Sid
}

/**
Checks that Channel is still alive
*/
func (c *Channel) IsAlive() bool {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	return c.alive
}

// Close закрывает соединение для клиента (канала)
func (c *Channel) Close() error {
	fmt.Println("Close channel close")
	return CloseChannel(c, &c.server.methods, nil)
}

/**
Close channel
*/
func CloseChannel(c *Channel, m *methods, args ...interface{}) error {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	fmt.Println("Close channel begin")
	if !c.alive {
		//already closed
		return nil
	}

	c.conn.Close()
	c.alive = false
	fmt.Println("before clean outloop")
	//clean outloop
	for len(c.out) > 0 {
		<-c.out
	}
	fmt.Println("Close channel")
	c.out <- protocol.CloseMessage
	fmt.Println("Close to out channel")

	m.callLoopEvent(c, OnDisconnection)

	fmt.Println("OnDisconnection callLoopEvent")

	overfloodedLock.Lock()
	delete(overflooded, c)
	overfloodedLock.Unlock()

	fmt.Println("CloseChannel return")

	return nil
}

//incoming messages loop, puts incoming messages to In channel
func inLoop(c *Channel, m *methods) error {
	for {
		pkg, err := c.conn.GetMessage()
		if err != nil {
			fmt.Println("CloseChannel 1 ", err)
			return CloseChannel(c, m, err)
		}
		fmt.Println("inLoop pkg 2", pkg)
		msg, err := protocol.Decode(pkg)
		//fmt.Println("inLoop ", msg)
		if err != nil {
			fmt.Println("CloseChannel 2 ", err)
			CloseChannel(c, m, protocol.ErrorWrongPacket)
			return err
		}

		switch msg.Type {
		case protocol.MessageTypeOpen:
			if err := json.Unmarshal([]byte(msg.Source[1:]), &c.header); err != nil {
				fmt.Println("CloseChannel 3")
				CloseChannel(c, m, ErrorWrongHeader)
			}
			m.callLoopEvent(c, OnConnection)
		case protocol.MessageTypePing:
			c.out <- protocol.PongMessage
		case protocol.MessageTypePong:
		default:
			go m.processIncomingMessage(c, msg)
		}
	}
	return nil
}

var overflooded map[*Channel]struct{} = make(map[*Channel]struct{})
var overfloodedLock sync.Mutex

func AmountOfOverflooded() int64 {
	overfloodedLock.Lock()
	defer overfloodedLock.Unlock()

	return int64(len(overflooded))
}

/**
outgoing messages loop, sends messages from channel to socket
*/
func outLoop(c *Channel, m *methods) error {
	for {
		outBufferLen := len(c.out)
		if outBufferLen >= queueBufferSize-1 {
			fmt.Println("CloseChannel 5")
			return CloseChannel(c, m, ErrorSocketOverflood)
		} else if outBufferLen > int(queueBufferSize/2) {
			fmt.Println("outBufferLen > int(queueBufferSize/2)")
			overfloodedLock.Lock()
			overflooded[c] = struct{}{}
			overfloodedLock.Unlock()
		} else {
			fmt.Println("CloseChannel 5 else")
			overfloodedLock.Lock()
			delete(overflooded, c)
			overfloodedLock.Unlock()
		}

		msg := <-c.out
		fmt.Println("outLoop ", msg)
		if msg == protocol.CloseMessage {
			fmt.Println("outLoop CloseMessage", msg)
			return nil
		}
		err := c.conn.WriteMessage(msg)
		fmt.Println("outLoop msg writed", msg)
		if err != nil {
			fmt.Println("CloseChannel 6")
			return CloseChannel(c, m, err)
		}
	}
	return nil
}

/**
Pinger sends ping messages for keeping connection alive
*/
func pinger(c *Channel) {
	for {
		interval, _ := c.conn.PingParams()
		time.Sleep(interval)
		if !c.IsAlive() {
			return
		}

		c.out <- protocol.PingMessage
	}
}
