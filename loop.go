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

// connectionHeader represents engine.io connection header
type connectionHeader struct {
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

	outC       chan string
	stubC      chan string
	upgradedC  chan string
	connHeader connectionHeader

	alive      bool
	aliveMutex sync.Mutex

	ack acks

	server  *Server
	address string
	header  http.Header
}

// init the Channel
func (c *Channel) init() {
	c.outC, c.stubC, c.upgradedC = make(chan string, queueBufferSize), make(chan string), make(chan string)
	c.ack.ackC = make(map[int](chan string))
	c.alive = true
}

// Id returns an ID of the current socket connection
func (c *Channel) Id() string { return c.connHeader.Sid }

// IsAlive checks that Channel is still alive
func (c *Channel) IsAlive() bool {
	c.aliveMutex.Lock()
	defer c.aliveMutex.Unlock()
	return c.alive
}

// Close the client (Channel) connection
func (c *Channel) Close() error { return c.close(c.server.event) }

// stub closes the polling client (Channel) connection at socket.io upgrade
func (c *Channel) stub() error { return c.close(nil) }

// close channel
func (c *Channel) close(m *event) error {
	switch c.conn.(type) {
	case *transport.PollingConnection:
		logging.Log().Debug("close() type: PollingConnection")
	case *transport.WebsocketConnection:
		logging.Log().Debug("close() type: WebsocketConnection")
	}

	c.aliveMutex.Lock()
	defer c.aliveMutex.Unlock()

	if !c.alive { // already closed
		return nil
	}

	c.conn.Close()
	c.alive = false

	// clean outloop
	for len(c.outC) > 0 {
		<-c.outC
	}

	if m != nil { // close
		c.outC <- protocol.MessageClose
		m.callLoopEvent(c, OnDisconnection)
	} else { // stub at transport upgrade
		c.outC <- protocol.MessageStub
	}

	overfloodedMutex.Lock()
	delete(overflooded, c)
	overfloodedMutex.Unlock()

	return nil
}

// inLoop is an incoming messages loop
func (c *Channel) inLoop(m *event) error {
	for {
		message, err := c.conn.GetMessage()
		if err != nil {
			logging.Log().Debugf("inLoop(), c.conn.GetMessage() err: %v, message: %s", err, message)
			return c.close(m)
		}

		if message == transport.StopMessage {
			logging.Log().Debug("inLoop(): StopMessage")
			return nil
		}

		decodedMessage, err := protocol.Decode(message)
		if err != nil {
			logging.Log().Debugf("inLoop() decoding err: %v, message: %s", err, message)
			c.close(m)
			return err
		}

		switch decodedMessage.Type {
		case protocol.MessageTypeOpen:
			logging.Log().Debugf("inLoop(), protocol.MessageTypeOpen: %+v", decodedMessage)
			if err := json.Unmarshal([]byte(decodedMessage.Source[1:]), &c.connHeader); err != nil {
				c.close(m)
			}
			m.callLoopEvent(c, OnConnection)

		case protocol.MessageTypePing:
			logging.Log().Debugf("inLoop(), protocol.MessageTypePing: %+v", decodedMessage)
			if decodedMessage.Source == protocol.MessagePingProbe {
				logging.Log().Debugf("inLoop(), got %s", decodedMessage.Source)
				c.outC <- protocol.MessagePongProbe
				c.upgradedC <- transport.UpgradedMessage
			} else {
				c.outC <- protocol.MessagePong
			}

		case protocol.MessageTypeUpgrade:
		case protocol.MessageTypeBlank:
		case protocol.MessageTypePong:
		default:
			go m.processIncomingEvent(c, decodedMessage)
		}
	}

	return nil
}

var overflooded = make(map[*Channel]struct{})
var overfloodedMutex sync.Mutex

// CountOverfloodingChannels returns an amount of overflooding channels
func CountOverfloodingChannels() int {
	overfloodedMutex.Lock()
	defer overfloodedMutex.Unlock()
	return len(overflooded)
}

// outLoop is an outgoing messages loop, sends messages from channel to socket
func (c *Channel) outLoop(m *event) error {
	for {
		outBufferLen := len(c.outC)
		logging.Log().Debug("outLoop(), outBufferLen: ", outBufferLen)
		switch {
		case outBufferLen >= queueBufferSize-1:
			logging.Log().Debug("outLoop(), outBufferLen >= queueBufferSize-1")
			return c.close(m)
		case outBufferLen > int(queueBufferSize/2):
			overfloodedMutex.Lock()
			overflooded[c] = struct{}{}
			overfloodedMutex.Unlock()
		default:
			overfloodedMutex.Lock()
			delete(overflooded, c)
			overfloodedMutex.Unlock()
		}

		msg := <-c.outC

		if msg == protocol.MessageClose || msg == protocol.MessageStub {
			return nil
		}

		if err := c.conn.WriteMessage(msg); err != nil {
			logging.Log().Debug("outLoop(), failed to c.conn.WriteMessage(), err: ", err)
			return c.close(m)
		}
	}
	return nil
}

// pingLoop sends ping messages for keeping connection alive
func (c *Channel) pingLoop() {
	for {
		interval, _ := c.conn.PingParams()
		time.Sleep(interval)
		if !c.IsAlive() {
			return
		}

		c.outC <- protocol.MessagePing
	}
}
