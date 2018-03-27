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

// send message packet to socket
func send(message *protocol.Message, c *Channel, payload interface{}) error {
	// preventing json/encoding "index out of range" panic
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

// Emit the event with given name and payload
func (c *Channel) Emit(name string, payload interface{}) error {
	msg := &protocol.Message{
		Type:  protocol.MessageTypeEmit,
		Event: name,
	}

	return send(msg, c, payload)
}

// Create ack packet based on given data and send it and receive response
func (c *Channel) Ack(method string, payload interface{}, timeout time.Duration) (string, error) {
	msg := &protocol.Message{
		Type:  protocol.MessageTypeAckRequest,
		AckId: c.ack.getNextId(),
		Event: method,
	}

	waiter := make(chan string)
	c.ack.addWaiter(msg.AckId, waiter)

	err := send(msg, c, payload)
	if err != nil {
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
