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

// inLoop is an incoming messages loop
func (c *Channel) inLoop(m *methods) error {
	for {
		message, err := c.conn.GetMessage()
		if err != nil {
			logging.Log().Debugf("inLoop(), c.conn.GetMessage() err: %v, message: %s", err, message)
			return closeChannel(c, m)
		}

		if message == transport.StopMessage {
			logging.Log().Debug("inLoop(): StopMessage")
			return nil
		}

		decodedMessage, err := protocol.Decode(message)
		if err != nil {
			logging.Log().Debugf("inLoop() decoding err: %v, message: %s", err, message)
			closeChannel(c, m)
			return err
		}

		switch decodedMessage.Type {
		case protocol.MessageTypeOpen:
			logging.Log().Debugf("inLoop(), protocol.MessageTypeOpen: %+v", decodedMessage)
			if err := json.Unmarshal([]byte(decodedMessage.Source[1:]), &c.header); err != nil {
				closeChannel(c, m)
			}
			m.callLoopEvent(c, OnConnection)

		case protocol.MessageTypePing:
			logging.Log().Debugf("inLoop(), protocol.MessageTypePing: %+v", decodedMessage)
			if decodedMessage.Source == protocol.MessagePingProbe {
				logging.Log().Debugf("inLoop(), got %s", decodedMessage.Source)
				c.out <- protocol.MessagePongProbe
				c.upgraded <- transport.UpgradedMessage
			} else {
				c.out <- protocol.MessagePong
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
var overfloodedLock sync.Mutex

// OverfloodedChannelsCount returns an amount of overflooding channels
func OverfloodedChannelsCount() int64 {
	overfloodedLock.Lock()
	defer overfloodedLock.Unlock()
	return int64(len(overflooded))
}

// outLoop is an outgoing messages loop, sends messages from channel to socket
func (c *Channel) outLoop(m *methods) error {
	for {
		outBufferLen := len(c.out)
		logging.Log().Debug("outLoop(), outBufferLen: ", outBufferLen)
		switch {
		case outBufferLen >= queueBufferSize-1:
			logging.Log().Debug("outLoop(), outBufferLen >= queueBufferSize-1")
			return closeChannel(c, m)
		case outBufferLen > int(queueBufferSize/2):
			overfloodedLock.Lock()
			overflooded[c] = struct{}{}
			overfloodedLock.Unlock()
		default:
			overfloodedLock.Lock()
			delete(overflooded, c)
			overfloodedLock.Unlock()
		}

		msg := <-c.out

		if msg == protocol.MessageClose || msg == protocol.MessageStub {
			return nil
		}

		if err := c.conn.WriteMessage(msg); err != nil {
			logging.Log().Debug("outLoop(), failed to c.conn.WriteMessage(), err: ", err)
			return closeChannel(c, m)
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

		c.out <- protocol.MessagePing
	}
}
