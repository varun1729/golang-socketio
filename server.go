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
	return c.address
}

// RequestHeader returns a connection request connectionHeader
func (c *Channel) RequestHeader() http.Header { return c.header }

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

// Amount returns an amount of channels joined to the given room, using channel
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

// List returns a list of channels joined to the given room, using channel
func (c *Channel) List(room string) []*Channel {
	if c.server == nil {
		return []*Channel{}
	}
	return c.server.List(room)
}

// List returns a list of channels joined to the given room, using server
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

// BroadcastTo the the given room an event with payload, using channel
func (c *Channel) BroadcastTo(room, event string, payload interface{}) {
	if c.server == nil {
		return
	}
	c.server.BroadcastTo(room, event, payload)
}

// BroadcastTo the the given room an event with payload, using server
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

// generateNewId for the socket.io connection
func generateNewId(custom string) string {
	hash := fmt.Sprintf("%s %s %n %n", custom, time.Now(), rand.Uint32(), rand.Uint32())
	buf, sum := bytes.NewBuffer(nil), md5.Sum([]byte(hash))
	encoder := base64.NewEncoder(base64.URLEncoding, buf)
	encoder.Write(sum[:])
	encoder.Close()
	return buf.String()[:20]
}

// onConnection fires on connection and on connection upgrade
func onConnection(c *Channel) {
	c.server.sidsMutex.Lock()
	c.server.sids[c.Id()] = c
	c.server.sidsMutex.Unlock()
}

// onDisconnection fires on disconnection
func onDisconnection(c *Channel) {
	c.server.channelsMutex.Lock()
	defer c.server.channelsMutex.Unlock()

	defer func() {
		c.server.sidsMutex.Lock()
		defer c.server.sidsMutex.Unlock()
		delete(c.server.sids, c.Id())
	}()

	_, ok := c.server.rooms[c]
	if !ok {
		return
	}

	for room := range c.server.rooms[c] {
		if curRoom, ok := c.server.channels[room]; ok {
			delete(curRoom, c)
			if len(curRoom) == 0 {
				delete(c.server.channels, room)
			}
		}
	}
	delete(c.server.rooms, c)
}

func (s *Server) sendOpenSequence(c *Channel) {
	jsonHdr, err := json.Marshal(&c.connHeader)
	if err != nil {
		panic(err)
	}
	c.out <- protocol.MustEncode(&protocol.Message{Type: protocol.MessageTypeOpen, Args: string(jsonHdr)})
	c.out <- protocol.MustEncode(&protocol.Message{Type: protocol.MessageTypeEmpty})
}

// setupEventLoop for the given connection
func (s *Server) setupEventLoop(conn transport.Connection, address string, header http.Header) {
	interval, timeout := conn.PingParams()
	connHeader := connectionHeader{
		Sid:          generateNewId(address),
		Upgrades:     []string{"websocket"},
		PingInterval: int(interval / time.Millisecond),
		PingTimeout:  int(timeout / time.Millisecond),
	}

	c := &Channel{conn: conn, address: address, header: header, server: s, connHeader: connHeader}
	c.initChannel()

	switch conn.(type) {
	case *transport.PollingConnection:
		conn.(*transport.PollingConnection).Transport.SetSid(connHeader.Sid, conn)
	}

	s.sendOpenSequence(c)

	go c.inLoop(&s.methods)
	go c.outLoop(&s.methods)

	s.callLoopEvent(c, OnConnection)
}

// setupUpgradeEventLoop for connection upgrading
func (s *Server) setupUpgradeEventLoop(conn transport.Connection, remoteAddr string, header http.Header, sid string) {
	logging.Log().Debug("setupUpgradeEventLoop(): entered")

	cp, err := s.GetChannel(sid)
	if err != nil {
		logging.Log().Warnf("setupUpgradeEventLoop() can't find channel for session %s", sid)
		return
	}

	logging.Log().Debug("setupUpgradeEventLoop() obtained channel")
	interval, timeout := conn.PingParams()
	connHeader := connectionHeader{
		Sid:          sid,
		Upgrades:     []string{},
		PingInterval: int(interval / time.Millisecond),
		PingTimeout:  int(timeout / time.Millisecond),
	}

	c := &Channel{conn: conn, address: remoteAddr, header: header, server: s, connHeader: connHeader}
	c.initChannel()
	logging.Log().Debug("setupUpgradeEventLoop init channel")

	go c.inLoop(&s.methods)
	go c.outLoop(&s.methods)

	logging.Log().Debug("setupUpgradeEventLoop go loops")
	onConnection(c)

	// synchronize stubbing polling channel with receiving "2probe" message
	<-c.upgraded
	cp.Stub()
}

// ServeHTTP makes Server to implement http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session, transportName := r.URL.Query().Get("sid"), r.URL.Query().Get("transport")

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

		s.setupEventLoop(conn, r.RemoteAddr, r.Header)
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
			s.setupUpgradeEventLoop(conn, r.RemoteAddr, r.Header, session)
			logging.Log().Debug("WebsocketConnection upgraded")
			return
		}

		conn, err := s.websocket.HandleConnection(w, r)
		if err != nil {
			return
		}

		s.setupEventLoop(conn, r.RemoteAddr, r.Header)
		logging.Log().Debug("WebsocketConnection created")
	}
}

// CountChannels returns an amount of connected channels
func (s *Server) CountChannels() int {
	s.sidsMutex.RLock()
	defer s.sidsMutex.RUnlock()
	return len(s.sids)
}

// CountRooms returns an amount of rooms with at least one joined channel
func (s *Server) CountRooms() int {
	s.channelsMutex.RLock()
	defer s.channelsMutex.RUnlock()
	return len(s.channels)
}

// NewServer creates new socket.io server
func NewServer() *Server {
	s := &Server{
		websocket: transport.GetDefaultWebsocketTransport(),
		polling:   transport.GetDefaultPollingTransport(),
		channels:  make(map[string]map[*Channel]struct{}),
		rooms:     make(map[*Channel]map[string]struct{}),
		sids:      make(map[string]*Channel),
		methods: methods{
			onConnection:    onConnection,
			onDisconnection: onDisconnection,
		},
	}
	s.methods.initEvents()
	return s
}
