package gosocketio

import (
	"strconv"

	"github.com/mtfelian/golang-socketio/transport"
)

const (
	webSocketProtocol       = "ws://"
	webSocketSecureProtocol = "wss://"
	socketioWebsocketUrl    = "/socket.io/?EIO=3&transport=websocket"

	pollingProtocol       = "http://"
	pollingSecureProtocol = "https://"
	socketioPollingUrl    = "/socket.io/?EIO=3&transport=polling"
)

// Socket.io client representation
type Client struct {
	methods
	Channel
}

// GetUrl returns an url for socket.io connection for wesocket transport
func GetUrl(host string, port int, secure bool) string {
	prefix := webSocketProtocol
	if secure {
		prefix = webSocketSecureProtocol
	}
	return prefix + host + ":" + strconv.Itoa(port) + socketioWebsocketUrl
}

// GetUrlPolling returns an url for socket.io connection for polling transport
func GetUrlPolling(host string, port int, secure bool) string {
	prefix := pollingProtocol
	if secure {
		prefix = pollingSecureProtocol
	}

	return prefix + host + ":" + strconv.Itoa(port) + socketioPollingUrl
}

// connect to host and initialise socket.io protocol
// The correct ws protocol url example:
// ws://myserver.com/socket.io/?EIO=3&transport=websocket
// You can use GetUrlByHost for generating correct url
func Dial(url string, tr transport.Transport) (*Client, error) {
	c := &Client{}
	c.initChannel()
	c.initMethods()

	var err error
	c.conn, err = tr.Connect(url)
	if err != nil {
		return nil, err
	}

	go inLoop(&c.Channel, &c.methods)
	go outLoop(&c.Channel, &c.methods)
	go pinger(&c.Channel)

	switch tr.(type) {
	case *transport.PollingClientTransport:
		//time.Sleep(1 * time.Second)
		go pollingClientListener(&c.Channel, &c.methods)
	}

	return c, nil
}

// Close client connection
func (c *Client) Close() {
	CloseChannel(&c.Channel, &c.methods)
}
