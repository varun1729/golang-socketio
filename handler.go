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

// systemEventHandler function for internal event processing
type systemEventHandler func(c *Channel)

// methods abstracts a mapping of a event names to handler functions
type methods struct {
	eventHandlers      map[string]*handler // maps event name to handler function representation
	eventHandlersMutex sync.RWMutex

	onConnection    systemEventHandler
	onDisconnection systemEventHandler
}

// initEvents initializes events mapping
func (m *methods) initEvents() { m.eventHandlers = make(map[string]*handler) }

// On registers message processing function and binds it to the given event name
func (m *methods) On(name string, f interface{}) error {
	c, err := newHandler(f)
	if err != nil {
		return err
	}

	m.eventHandlersMutex.Lock()
	m.eventHandlers[name] = c
	m.eventHandlersMutex.Unlock()

	return nil
}

// findEvent returns a handler representation for the given event name
// the second parameter is true if such event found.
func (m *methods) findEvent(name string) (*handler, bool) {
	m.eventHandlersMutex.RLock()
	f, ok := m.eventHandlers[name]
	m.eventHandlersMutex.RUnlock()
	return f, ok
}

func (m *methods) callLoopEvent(c *Channel, event string) {
	if m.onConnection != nil && event == OnConnection {
		logging.Log().Debug("callLoopEvent(): OnConnection event")
		m.onConnection(c)
	}

	if m.onDisconnection != nil && event == OnDisconnection {
		m.onDisconnection(c)
	}

	f, ok := m.findEvent(event)
	if !ok {
		logging.Log().Debug("callLoopEvent(): event not found")
		return
	}

	f.call(c, &struct{}{})
}

// processIncomingEvent checks incoming message
func (m *methods) processIncomingEvent(c *Channel, msg *protocol.Message) {
	logging.Log().Debug("processIncomingEvent(): ", msg)
	switch msg.Type {
	case protocol.MessageTypeEmit:
		logging.Log().Debug("processIncomingEvent() is finding event: ", msg.Event)
		f, ok := m.findEvent(msg.Event)
		if !ok {
			logging.Log().Debug("processIncomingEvent(): event not found")
			return
		}

		logging.Log().Debug("processIncomingEvent() found method: ", f)

		if !f.hasArgs {
			f.call(c, &struct{}{})
			return
		}

		data := f.arguments()
		logging.Log().Debug("processIncomingEvent(): f.arguments() returned ", data)

		if err := json.Unmarshal([]byte(msg.Args), &data); err != nil {
			logging.Log().Infof("Error processing message. msg.Args: %s, data: %v, err: %v", msg.Args, data, err)
			return
		}

		f.call(c, data)

	case protocol.MessageTypeAckRequest:
		logging.Log().Debug("processIncomingEvent(): ack request")
		f, ok := m.findEvent(msg.Event)
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
		logging.Log().Debug("processIncomingEvent(): ack response")
		ackC, err := c.ack.obtain(msg.AckId)
		if err == nil {
			ackC <- msg.Args
		}
	}
}
