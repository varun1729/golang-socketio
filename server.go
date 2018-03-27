package gosocketio

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/mtfelian/golang-socketio/logging"
	"github.com/mtfelian/golang-socketio/protocol"
	"github.com/mtfelian/golang-socketio/transport"
)

const (
	HeaderForward = "X-Forwarded-For"
)

var (
	ErrorServerNotSet       = errors.New("server was not set")
	ErrorConnectionNotFound = errors.New("connection not found")
)

// Server represents a socket.io server instance
type Server struct {
	methods
	http.Handler

	channels      map[string]map[*Channel]struct{} // maps room name to map of channels to an empty struct
	rooms         map[*Channel]map[string]struct{} // maps channel to map of room names to an empty struct
	channelsMutex sync.RWMutex

	sids      map[string]*Channel // maps channel id to channel
	sidsMutex sync.RWMutex

	websocket *transport.WebsocketTransport
	polling   *transport.PollingTransport
}

// Ip returns an IP of the socket client
func (c *Channel) Ip() string {
	forward := c.RequestHeader().Get(HeaderForward)
	if forward != "" {
		return forward
	}
	return c.ip
}

// RequestHeader returns a connection request header
func (c *Channel) RequestHeader() http.Header { return c.requestHeader }

// GetChannel by it's sid
func (s *Server) GetChannel(sid string) (*Channel, error) {
	s.sidsMutex.RLock()
	defer s.sidsMutex.RUnlock()

	c, ok := s.sids[sid]
	if !ok {
		return nil, ErrorConnectionNotFound
	}

	return c, nil
}

// Join this channel to the given room
func (c *Channel) Join(room string) error {
	if c.server == nil {
		return ErrorServerNotSet
	}

	c.server.channelsMutex.Lock()
	defer c.server.channelsMutex.Unlock()

	if _, ok := c.server.channels[room]; !ok {
		c.server.channels[room] = make(map[*Channel]struct{})
	}

	if _, ok := c.server.rooms[c]; !ok {
		c.server.rooms[c] = make(map[string]struct{})
	}

	c.server.channels[room][c], c.server.rooms[c][room] = struct{}{}, struct{}{}
	return nil
}

// Leave the given room (remove channel from it)
func (c *Channel) Leave(room string) error {
	if c.server == nil {
		return ErrorServerNotSet
	}

	c.server.channelsMutex.Lock()
	defer c.server.channelsMutex.Unlock()

	if _, ok := c.server.channels[room]; ok {
		delete(c.server.channels[room], c)
		if len(c.server.channels[room]) == 0 {
			delete(c.server.channels, room)
		}
	}

	if _, ok := c.server.rooms[c]; ok {
		delete(c.server.rooms[c], room)
	}

	return nil
}

// Amount returns an amount of channels joined to the given room
func (c *Channel) Amount(room string) int {
	if c.server == nil {
		return 0
	}

	return c.server.Amount(room)
}

// Get amount of channels, joined to given room, using server
func (s *Server) Amount(room string) int {
	s.channelsMutex.RLock()
	defer s.channelsMutex.RUnlock()

	roomChannels, _ := s.channels[room]
	return len(roomChannels)
}

// Get list of channels, joined to given room, using channel
func (c *Channel) List(room string) []*Channel {
	if c.server == nil {
		return []*Channel{}
	}

	return c.server.List(room)
}

// Get list of channels, joined to given room, using server
func (s *Server) List(room string) []*Channel {
	s.channelsMutex.RLock()
	defer s.channelsMutex.RUnlock()

	roomChannels, ok := s.channels[room]
	if !ok {
		return []*Channel{}
	}

	i := 0
	roomChannelsCopy := make([]*Channel, len(roomChannels))
	for channel := range roomChannels {
		roomChannelsCopy[i] = channel
		i++
	}

	return roomChannelsCopy

}

func (c *Channel) BroadcastTo(room, method string, payload interface{}) {
	if c.server == nil {
		return
	}
	c.server.BroadcastTo(room, method, payload)
}

// Broadcast message to all room channels
func (s *Server) BroadcastTo(room, method string, payload interface{}) {
	s.channelsMutex.RLock()
	defer s.channelsMutex.RUnlock()

	roomChannels, ok := s.channels[room]
	if !ok {
		return
	}

	for cn := range roomChannels {
		if cn.IsAlive() {
			go cn.Emit(method, payload)
		}
	}
}

// Broadcast to all clients
func (s *Server) BroadcastToAll(method string, payload interface{}) {
	s.sidsMutex.RLock()
	defer s.sidsMutex.RUnlock()

	for _, cn := range s.sids {
		if cn.IsAlive() {
			go cn.Emit(method, payload)
		}
	}
}

// Generate new id for socket.io connection
func generateNewId(custom string) string {
	hash := fmt.Sprintf("%s %s %n %n", custom, time.Now(), rand.Uint32(), rand.Uint32())
	buf := bytes.NewBuffer(nil)
	sum := md5.Sum([]byte(hash))
	encoder := base64.NewEncoder(base64.URLEncoding, buf)
	encoder.Write(sum[:])
	encoder.Close()
	return buf.String()[:20]
}

// On connection system handler, store sid
func onConnectStore(c *Channel) {
	c.server.sidsMutex.Lock()
	defer c.server.sidsMutex.Unlock()
	c.server.sids[c.Id()] = c
}

// On disconnection system handler, clean joins and sid
func onDisconnectCleanup(c *Channel) {
	c.server.channelsMutex.Lock()
	defer c.server.channelsMutex.Unlock()

	byRoom, ok := c.server.rooms[c]
	if ok {
		for room := range byRoom {
			if curRoom, ok := c.server.channels[room]; ok {
				delete(curRoom, c)
				if len(curRoom) == 0 {
					delete(c.server.channels, room)
				}
			}
		}

		delete(c.server.rooms, c)
	}

	c.server.sidsMutex.Lock()
	defer c.server.sidsMutex.Unlock()

	delete(c.server.sids, c.Id())
}

func (s *Server) SendOpenSequence(c *Channel) {
	jsonHdr, err := json.Marshal(&c.header)
	if err != nil {
		panic(err)
	}
	c.out <- protocol.MustEncode(&protocol.Message{Type: protocol.MessageTypeOpen, Args: string(jsonHdr)})
	c.out <- protocol.MustEncode(&protocol.Message{Type: protocol.MessageTypeEmpty})
}

// Setup event loop for given connection
func (s *Server) SetupEventLoop(conn transport.Connection, remoteAddr string, requestHeader http.Header) {

	interval, timeout := conn.PingParams()
	hdr := header{
		Sid:          generateNewId(remoteAddr),
		Upgrades:     []string{"websocket"},
		PingInterval: int(interval / time.Millisecond),
		PingTimeout:  int(timeout / time.Millisecond),
	}

	c := &Channel{
		conn:          conn,
		ip:            remoteAddr,
		requestHeader: requestHeader,
		server:        s,
		header:        hdr,
	}
	c.initChannel()

	switch conn.(type) {
	case *transport.PollingConnection:
		conn.(*transport.PollingConnection).Transport.SetSid(hdr.Sid, conn)
	}

	s.SendOpenSequence(c)

	go c.inLoop(&s.methods)
	go c.outLoop(&s.methods)

	s.callLoopEvent(c, OnConnection)
}

// Setup event loop when upgrading connection
func (s *Server) SetupUpgradeEventLoop(conn transport.Connection, remoteAddr string, requestHeader http.Header, sid string) {
	logging.Log().Debug("SetupUpgradeEventLoop")

	cp, err := s.GetChannel(sid)
	if err != nil {
		logging.Log().Warnf("can't find channel for session: ", sid)
		return
	}

	logging.Log().Debug("SetupUpgradeEventLoop close")
	interval, timeout := conn.PingParams()
	hdr := header{
		Sid:          sid,
		Upgrades:     []string{},
		PingInterval: int(interval / time.Millisecond),
		PingTimeout:  int(timeout / time.Millisecond),
	}

	c := &Channel{
		conn:          conn,
		ip:            remoteAddr,
		requestHeader: requestHeader,
		server:        s,
		header:        hdr,
	}
	c.initChannel()
	logging.Log().Debug("SetupUpgradeEventLoop init channel")

	go c.inLoop(&s.methods)
	go c.outLoop(&s.methods)

	logging.Log().Debug("SetupUpgradeEventLoop go loops")
	onConnectStore(c)

	// synchronize stubbing polling channel with receiving "2probe" message
	<-c.upgraded
	cp.Stub()
}

// implements ServeHTTP function from http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("sid")
	transportName := r.URL.Query().Get("transport")

	switch transportName {
	case "polling":
		// session is empty in first polling request, or first and single websocket request
		if session != "" {
			s.polling.Serve(w, r)
			return
		}

		conn, err := s.polling.HandleConnection(w, r)
		if err != nil {
			return
		}

		s.SetupEventLoop(conn, r.RemoteAddr, r.Header)
		logging.Log().Debug("PollingConnection created")
		conn.(*transport.PollingConnection).PollingWriter(w, r)

	case "websocket":
		if session != "" {
			logging.Log().Debug("upgrade HandleConnection")
			conn, err := s.websocket.HandleConnection(w, r)
			if err != nil {
				logging.Log().Debug("upgrade error ", err)
				return
			}
			s.SetupUpgradeEventLoop(conn, r.RemoteAddr, r.Header, session)
			logging.Log().Debug("WebsocketConnection upgraded")
			return
		}

		conn, err := s.websocket.HandleConnection(w, r)
		if err != nil {
			return
		}

		s.SetupEventLoop(conn, r.RemoteAddr, r.Header)
		logging.Log().Debug("WebsocketConnection created")
	}
}

// Get amount of current connected sids
func (s *Server) AmountOfSids() int64 {
	s.sidsMutex.RLock()
	defer s.sidsMutex.RUnlock()

	return int64(len(s.sids))
}

// Get amount of rooms with at least one channel(or sid) joined
func (s *Server) AmountOfRooms() int64 {
	s.channelsMutex.RLock()
	defer s.channelsMutex.RUnlock()

	return int64(len(s.channels))
}

// Create new socket.io server
func NewServer() *Server {
	s := Server{
		websocket: transport.GetDefaultWebsocketTransport(),
		polling:   transport.GetDefaultPollingTransport(),
		channels:  make(map[string]map[*Channel]struct{}),
		rooms:     make(map[*Channel]map[string]struct{}),
		sids:      make(map[string]*Channel),
	}

	s.initEvents()
	s.onConnection = onConnectStore
	s.onDisconnection = onDisconnectCleanup

	return &s
}
