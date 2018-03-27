package gosocketio

import (
	"strconv"

	_ "time"

	"github.com/mtfelian/golang-socketio/transport"
)

const (
	webSocketSchema       = "ws://"
	webSocketSecureSchema = "wss://"
	socketioWebsocketURL  = "/socket.io/?EIO=3&transport=websocket"

	pollingSchema       = "http://"
	pollingSecureSchema = "https://"
	socketioPollingURL  = "/socket.io/?EIO=3&transport=polling"
)

// Client represents socket.io client
type Client struct {
	methods
	Channel
}

// GetUrl returns an url for socket.io connection for websocket transport
func GetUrl(host string, port int, secure bool) string {
	prefix := webSocketSchema
	if secure {
		prefix = webSocketSecureSchema
	}
	return prefix + host + ":" + strconv.Itoa(port) + socketioWebsocketURL
}

// GetUrlPolling returns an url for socket.io connection for polling transport
func GetUrlPolling(host string, port int, secure bool) string {
	prefix := pollingSchema
	if secure {
		prefix = pollingSecureSchema
	}
	return prefix + host + ":" + strconv.Itoa(port) + socketioPollingURL
}

// Dial connects to host and initializes socket.io protocol
// The correct ws protocol url example:
// ws://myserver.com/socket.io/?EIO=3&transport=websocket
// Use GetUrlByHost to obtain the correct URL
func Dial(url string, tr transport.Transport) (*Client, error) {
	c := &Client{}
	c.initChannel()
	c.initEvents()

	var err error
	c.conn, err = tr.Connect(url)
	if err != nil {
		return nil, err
	}

	go (&c.Channel).inLoop(&c.methods)
	go (&c.Channel).outLoop(&c.methods)
	go (&c.Channel).pingLoop()

	switch tr.(type) {
	case *transport.PollingClientTransport:
		go (&c.methods).callLoopEvent(&c.Channel, OnConnection)
	}

	return c, nil
}

// Close client connection
func (c *Client) Close() { closeChannel(&c.Channel, &c.methods) }
