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
func send(message *protocol.Message, c *Channel, payload interface{}) error {
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

	if len(c.out) == queueBufferSize {
		return ErrorSocketOverflood
	}

	c.out <- command
	return nil
}

// Emit an asynchronous event with the given name and payload
func (c *Channel) Emit(name string, payload interface{}) error {
	message := &protocol.Message{Type: protocol.MessageTypeEmit, Event: name}
	return send(message, c, payload)
}

// Ack a synchronous event with the given name and payload and wait for/receive the response
func (c *Channel) Ack(name string, payload interface{}, timeout time.Duration) (string, error) {
	msg := &protocol.Message{Type: protocol.MessageTypeAckRequest, AckId: c.ack.getNextId(), Event: name}

	waiter := make(chan string)
	c.ack.addWaiter(msg.AckId, waiter)

	if err := send(msg, c, payload); err != nil {
		c.ack.removeWaiter(msg.AckId)
	}

	select {
	case result := <-waiter:
		return result, nil
	case <-time.After(timeout):
		c.ack.removeWaiter(msg.AckId)
		return "", ErrorSendTimeout
	}
}
