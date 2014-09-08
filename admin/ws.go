package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const EventsQueryKey = "e"

type (
	Event struct {
		Type     EventType
		Resource interface{}
	}
	EventType int
)

type rawevt struct {
	Type     EventType
	Resource json.RawMessage
}

func (e *Event) UnmarshalJSON(b []byte) error {
	var evt rawevt
	if err := json.Unmarshal(b, &evt); err != nil {
		return err
	}

	var res interface{}
	switch evt.Type {
	case EventTypeServerAdd:
		fallthrough
	case EventTypeServerRemove:
		res = new(ServerEvent)
	case EventTypeMountAdd:
		fallthrough
	case EventTypeMountUpdate:
		fallthrough
	case EventTypeMountRemove:
		res = new(MountEvent)
	case EventTypeFileServe:
		res = new(FileServeEvent)
	}
	if err := json.Unmarshal(evt.Resource, res); err != nil {
		return err
	}
	*e = Event{
		Type:     evt.Type,
		Resource: res,
	}
	return nil
}

const (
	EventTypeServerAdd EventType = 1 + iota
	EventTypeServerRemove
	EventTypeMountAdd
	EventTypeMountUpdate
	EventTypeMountRemove
	EventTypeFileServe
)

type (
	ServerEvent struct {
		Server Server `json:"server"`
	}
	MountEvent struct {
		Server Server `json:"server"`
		Mount  Mount  `json:"mount"`
	}
	FileServeEvent struct {
		Server Server `json:"server"`
		Path   string `json:"path"`
		Code   int    `json:"code"`
	}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1 << 8,
	WriteBufferSize: 1 << 10,
}
var (
	writeWait = time.Second * 10
	pongWait  = time.Second * 0xff
	pingWait  = (pongWait * 9) / 10
)

type serverPoolEventsHandler struct {
	conns   map[*conn]bool
	events  chan string
	sub     chan *conn
	unsub   chan *conn
	eventCh chan *Event
}

type conn struct {
	*websocket.Conn
	events  map[EventType]bool
	eventCh chan *Event
}

func (s *serverPoolEventsHandler) Send(event Event) {
	s.eventCh <- &event
}

func (s *serverPoolEventsHandler) Broadcast() {
	for {
		select {
		case conn := <-s.sub:
			s.conns[conn] = true
		case conn := <-s.unsub:
			delete(s.conns, conn)
			close(conn.eventCh)
		case event := <-s.eventCh:
			for c := range s.conns {
				if ok := c.events[event.Type]; !ok {
					continue
				}
				select {
				case c.eventCh <- event:
				case <-time.After(time.Second):
					go func() {
						s.unsub <- c
					}()
				}
			}
		}
	}
}

func (s *serverPoolEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	events := make(map[EventType]bool)
	eventsStr := r.URL.Query().Get(EventsQueryKey)
	if eventsStr != "" {
		for _, evt := range strings.Split(eventsStr, ",") {
			evti, err := strconv.Atoi(evt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			events[EventType(evti)] = true
		}
	} else {
		events = map[EventType]bool{
			EventTypeServerAdd:    true,
			EventTypeServerRemove: true,
			EventTypeMountAdd:     true,
			EventTypeMountRemove:  true,
			EventTypeMountUpdate:  true,
			EventTypeFileServe:    true,
		}
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	c := &conn{ws, events, make(chan *Event)}
	s.sub <- c
	defer func() {
		s.unsub <- c
	}()
	ticker := time.NewTicker(pingWait)
	defer ticker.Stop()

	var cls <-chan bool
	if wcn, ok := w.(http.CloseNotifier); ok {
		cls = wcn.CloseNotify()
	}

	quit := make(chan struct{})
	go func() {
		c.SetReadDeadline(time.Now().Add(pongWait))
		c.SetPongHandler(func(string) error {
			c.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
	L:
		for {
			select {
			case <-quit:
				return
			default:
				if _, _, err := c.ReadMessage(); err != nil {
					c.Close()
					break L
				}
			}
		}
		close(quit)
	}()
	go func() {
		for {
			select {
			case event, ok := <-c.eventCh:
				if !ok {
					c.SetWriteDeadline(time.Now().Add(writeWait))
					c.WriteMessage(websocket.CloseMessage, []byte{})
					close(quit)
					return
				}
				c.WriteJSON(event)
			case <-ticker.C:
				if err := c.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait)); err != nil {
					close(quit)
					return
				}
			case <-cls:
				close(quit)
				return
			case <-quit:
				return
			}
		}
	}()
	<-quit
}
