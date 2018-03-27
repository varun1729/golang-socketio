package gosocketio

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
	"github.com/mtfelian/golang-socketio/transport"
)

const (
	queueBufferSize = 500
)

// header represents engine.io header to send or receive
type header struct {
	Sid          string   `json:"sid"`
	Upgrades     []string `json:"upgrades"`
	PingInterval int      `json:"pingInterval"`
	PingTimeout  int      `json:"pingTimeout"`
}

// Channel represents socket.io connection
// use IsAlive to check that handler is still working
// use Dial to connect via the chosen transport
// use In and Out channels for message exchange
// Close message means channel is closed
// ping is automatic
type Channel struct {
	conn transport.Connection

	out      chan string
	stub     chan string
	upgraded chan string
	header   header

	alive     bool
	aliveLock sync.Mutex

	ack ackProcessor

	server        *Server
	ip            string
	requestHeader http.Header
}

// initChannel initializes Channel
func (c *Channel) initChannel() {
	c.out, c.stub, c.upgraded = make(chan string, queueBufferSize), make(chan string), make(chan string)
	c.ack.resultWaiters = make(map[int](chan string))
	c.alive = true
}

// Id returns an ID of the current socket connection
func (c *Channel) Id() string { return c.header.Sid }

// IsAlive checks that Channel is still alive
func (c *Channel) IsAlive() bool {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	return c.alive
}

// Close the client (Channel) connection
func (c *Channel) Close() error { return closeChannel(c, &c.server.methods) }

// Stub closes the polling client (Channel) connection at socket.io upgrade
func (c *Channel) Stub() error { return closeChannel(c, nil) }

// closeChannel
func closeChannel(c *Channel, m *methods) error {
	switch c.conn.(type) {
	case *transport.PollingConnection:
		logging.Log().Debug("closeChannel() type: PollingConnection")
	case *transport.WebsocketConnection:
		logging.Log().Debug("closeChannel() type: WebsocketConnection")
	}

	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()

	if !c.alive { // already closed
		return nil
	}

	c.conn.Close()
	c.alive = false

	// clean outloop
	for len(c.out) > 0 {
		<-c.out
	}

	if m != nil { // close
		c.out <- protocol.MessageClose
		m.callLoopEvent(c, OnDisconnection)
	} else { // stub at transport upgrade
		c.out <- protocol.MessageStub
	}

	overfloodedLock.Lock()
	delete(overflooded, c)
	overfloodedLock.Unlock()

	return nil
}

//incoming messages loop, puts incoming messages to In channel
func inLoop(c *Channel, m *methods) error {
	for {
		pkg, err := c.conn.GetMessage()
		if err != nil {
			logging.Log().Debug("c.conn.GetMessage err ", err, " pkg: ", pkg)
			return closeChannel(c, m)
		}

		if pkg == transport.StopMessage {
			logging.Log().Debug("inLoop StopMessage")
			return nil
		}

		msg, err := protocol.Decode(pkg)
		if err != nil {
			logging.Log().Debug("Decoding err: ", err, " pkg: ", pkg)
			closeChannel(c, m)
			return err
		}

		switch msg.Type {
		case protocol.MessageTypeOpen:
			logging.Log().Debug("protocol.MessageTypeOpen: ", msg)
			if err := json.Unmarshal([]byte(msg.Source[1:]), &c.header); err != nil {
				closeChannel(c, m)
			}
			m.callLoopEvent(c, OnConnection)
		case protocol.MessageTypePing:
			logging.Log().Debug("get MessageTypePing ", msg, " source ", msg.Source)
			if msg.Source == protocol.MessagePingProbe {
				logging.Log().Debug("get 2probe")
				c.out <- protocol.MessagePongProbe
				c.upgraded <- transport.UpgradedMessage
			} else {
				c.out <- protocol.MessagePong
			}
		case protocol.MessageTypeUpgrade:
		case protocol.MessageTypeBlank:
		case protocol.MessageTypePong:
		default:
			go m.processIncomingEvent(c, msg)
		}
	}

	return nil
}

var overflooded = make(map[*Channel]struct{})
var overfloodedLock sync.Mutex

func AmountOfOverflooded() int64 {
	overfloodedLock.Lock()
	defer overfloodedLock.Unlock()

	return int64(len(overflooded))
}

// outgoing messages loop, sends messages from channel to socket
func outLoop(c *Channel, m *methods) error {
	for {
		outBufferLen := len(c.out)
		logging.Log().Debug("outBufferLen: ", outBufferLen)
		if outBufferLen >= queueBufferSize-1 {
			logging.Log().Debug("outBufferLen >= queueBufferSize-1")
			return closeChannel(c, m)
		} else if outBufferLen > int(queueBufferSize/2) {
			overfloodedLock.Lock()
			overflooded[c] = struct{}{}
			overfloodedLock.Unlock()
		} else {
			overfloodedLock.Lock()
			delete(overflooded, c)
			overfloodedLock.Unlock()
		}

		msg := <-c.out

		if msg == protocol.MessageClose || msg == protocol.MessageStub {
			return nil
		}

		if err := c.conn.WriteMessage(msg); err != nil {
			return closeChannel(c, m)
		}
	}
	return nil
}

// Pinger sends ping messages for keeping connection alive
func pinger(c *Channel) {
	for {
		interval, _ := c.conn.PingParams()
		time.Sleep(interval)
		if !c.IsAlive() {
			return
		}

		c.out <- protocol.MessagePing
	}
}

// Pauses for send http requests
func pollingClientListener(c *Channel, m *methods) {
	//time.Sleep(time.Second)
	m.callLoopEvent(c, OnConnection)
}
