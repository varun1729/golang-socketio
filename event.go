package gosocketio

import (
	"encoding/json"
	"reflect"
	"sync"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
)

const (
	OnConnection    = "connection"
	OnDisconnection = "disconnection"
	OnError         = "error"
)

// systemEventHandler function for internal handler processing
type systemEventHandler func(c *Channel)

// event abstracts a mapping of a handler names to handler functions
type event struct {
	handlers   map[string]*handler // maps handler name to handler function representation
	handlersMu sync.RWMutex

	onConnection    systemEventHandler
	onDisconnection systemEventHandler
}

// init initializes events mapping
func (e *event) init() { e.handlers = make(map[string]*handler) }

// On registers message processing function and binds it to the given event name
func (e *event) On(name string, f interface{}) error {
	c, err := newHandler(f)
	if err != nil {
		return err
	}

	e.handlersMu.Lock()
	e.handlers[name] = c
	e.handlersMu.Unlock()

	return nil
}

// findEvent returns a handler representation for the given event name
// the second parameter is true if such event found.
func (e *event) findEvent(name string) (*handler, bool) {
	e.handlersMu.RLock()
	f, ok := e.handlers[name]
	e.handlersMu.RUnlock()
	return f, ok
}

func (e *event) callLoopEvent(c *Channel, event string) {
	if e.onConnection != nil && event == OnConnection {
		logging.Log().Debug("callLoopEvent(): OnConnection handler")
		e.onConnection(c)
	}

	if e.onDisconnection != nil && event == OnDisconnection {
		e.onDisconnection(c)
	}

	f, ok := e.findEvent(event)
	if !ok {
		logging.Log().Debug("callLoopEvent(): handler not found")
		return
	}

	f.call(c, &struct{}{})
}

// processIncomingEvent checks incoming message
func (e *event) processIncomingEvent(c *Channel, msg *protocol.Message) {
	logging.Log().Debug("processIncomingEvent() fired with:", msg)
	switch msg.Type {
	case protocol.MessageTypeEmit:
		logging.Log().Debug("processIncomingEvent() is finding handler for msg.Event:", msg.Event)
		f, ok := e.findEvent(msg.Event)
		if !ok {
			logging.Log().Debug("processIncomingEvent(): handler not found")
			return
		}

		logging.Log().Debug("processIncomingEvent() found handler:", f)

		if !f.hasArgs {
			f.call(c, &struct{}{})
			return
		}

		data := f.arguments()
		logging.Log().Debug("processIncomingEvent(), f.arguments() returned:", data)

		if err := json.Unmarshal([]byte(msg.Args), &data); err != nil {
			logging.Log().Infof("processIncomingEvent() failed to json.Unmaeshal(). msg.Args: %s, data: %v, err: %v",
				msg.Args, data, err)
			return
		}

		f.call(c, data)

	case protocol.MessageTypeAckRequest:
		logging.Log().Debug("processIncomingEvent() ack request")
		f, ok := e.findEvent(msg.Event)
		if !ok || !f.out {
			return
		}

		var result []reflect.Value
		if f.hasArgs {
			// data type should be defined for Unmarshal()
			data := f.arguments()
			if err := json.Unmarshal([]byte(msg.Args), &data); err != nil {
				return
			}
			result = f.call(c, data)
		} else {
			result = f.call(c, &struct{}{})
		}

		ackResponse := &protocol.Message{
			Type:  protocol.MessageTypeAckResponse,
			AckId: msg.AckId,
		}

		c.send(ackResponse, result[0].Interface())

	case protocol.MessageTypeAckResponse:
		logging.Log().Debug("processIncomingEvent() ack response")
		ackC, err := c.ack.obtain(msg.AckId)
		if err == nil {
			ackC <- msg.Args
		}
	}
}
