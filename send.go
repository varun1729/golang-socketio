package gosocketio

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
)

var (
	ErrorSendTimeout     = errors.New("timeout")
	ErrorSocketOverflood = errors.New("socket overflood")
)

// send message packet to the given channel c with payload
func (c *Channel) send(message *protocol.Message, payload interface{}) error {
	// preventing encoding/json "index out of range" panic
	defer func() {
		if r := recover(); r != nil {
			logging.Log().Warn("socket.io send panic: ", r)
		}
	}()

	if payload != nil {
		b, err := json.Marshal(&payload)
		if err != nil {
			return err
		}
		message.Args = string(b)
	}

	command, err := protocol.Encode(message)
	if err != nil {
		return err
	}

	if len(c.outC) == queueBufferSize {
		return ErrorSocketOverflood
	}

	c.outC <- command
	return nil
}

// Emit an asynchronous handler with the given name and payload
func (c *Channel) Emit(name string, payload interface{}) error {
	message := &protocol.Message{Type: protocol.MessageTypeEmit, Event: name}
	return c.send(message, payload)
}

// Ack a synchronous handler with the given name and payload and wait for/receive the response
func (c *Channel) Ack(name string, payload interface{}, timeout time.Duration) (string, error) {
	msg := &protocol.Message{Type: protocol.MessageTypeAckRequest, AckId: c.ack.nextId(), Event: name}

	ackC := make(chan string)
	c.ack.register(msg.AckId, ackC)

	if err := c.send(msg, payload); err != nil {
		c.ack.unregister(msg.AckId)
	}

	select {
	case result := <-ackC:
		return result, nil
	case <-time.After(timeout):
		c.ack.unregister(msg.AckId)
		return "", ErrorSendTimeout
	}
}
