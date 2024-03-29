package mobell

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/apex/log"
	"mobell-proxy/mobell/mxpeg"
	"mobell-proxy/mobell/stream"
	"net"
	"strings"
	"time"
)

type connection struct {
	server *Server

	rb  *mxpeg.RingBuffer
	str *stream.Stream

	videoEnabled bool
	bellEvtId    int

	keepAliveSec int

	log log.Interface
}

func handleConnection(ctx context.Context, conn net.Conn, server *Server) {
	l := log.WithField("ctx", conn.RemoteAddr().String())

	str := stream.NewStream(ctx, conn, l)
	str.ReadTimeout = time.Second * 180

	c := &connection{
		server:       server,
		rb:           mxpeg.NewRingBuffer(256*1024, str, l),
		str:          str,
		log:          l,
		keepAliveSec: server.keepAliveSec,
	}
	go c.run()
}

func (c *connection) send(data []byte) {
	_, _ = c.str.Write(data)
}

func (c *connection) sendEvent(evt map[string]interface{}) {
	b, err := json.Marshal(evt)
	if err != nil {
		c.log.WithError(err).Error("error marshalling event")
		return
	}

	c.log.WithField("evt", string(b)).Debug("sending client event")

	l := len(b) + 2

	data := make([]byte, l+2)

	data[0] = 0xff
	data[1] = 0xec
	data[2] = (byte)(l >> 8)
	data[3] = (byte)(l)
	copy(data[4:], b)

	c.send(data)
}

func (c *connection) sendBell(isRing bool) {
	if c.bellEvtId > 0 {
		c.sendEvent(map[string]interface{}{
			"result": []interface{}{"bell", isRing, !isRing, []interface{}{1, "Main Bell", ""}},
			"type":   "cont",
			"error":  nil,
			"id":     c.bellEvtId,
		})
	}
}

func (c *connection) sendSuppress() {
	// this is a non-standard event
	// this event is supported ONLY by mobell application
	if c.bellEvtId > 0 {
		c.sendEvent(map[string]interface{}{
			"result": []interface{}{"suppress"},
			"type":   "cont",
			"error":  nil,
			"id":     c.bellEvtId,
		})
	}
}

func (c *connection) run() {
	c.server.addConnection(c)

	doneCh := make(chan struct{})
	updCh := make(chan struct{})
	go func() {
		timeout := time.Second * time.Duration(c.keepAliveSec)

		timer := time.NewTimer(timeout)
		for {
			select {
			case _ = <-doneCh:
				timer.Stop()
				return
			case _ = <-updCh:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(timeout)
			case _ = <-timer.C:
				c.sendEvent(map[string]interface{}{
					"method": "ping",
				})
				timer.Reset(timeout)
			}
		}
	}()

	str := c.str

	defer func() {
		close(doneCh)
		c.server.delConnection(c)
		str.Close()
	}()

	rb := mxpeg.NewRingBuffer(16*1024, str, c.log)

	// http part
	err := c.handleHttp(rb)
	if err != nil {
		c.log.WithError(err).Error("error reading http request headers")
		return
	}

	// send status
	c.send([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	for {
		data, err := readEvt(rb)
		if err != nil {
			c.log.WithError(err).Error("error reading event")
			break
		}

		updCh <- struct{}{}

		if data[0] == 0xff {
			if len(data) == 22 && data[6] == 0x53 {
				if data[9] == 0x81 {
					c.server.audioStart(c, data)
				} else {
					c.server.audioStop(c, data)
				}
			} else {
				c.server.audioData(c, data)
			}
		} else {
			c.log.WithField("evt", string(data)).Debug("got client event")

			var evt map[string]interface{}

			err := json.Unmarshal(data, &evt)
			if err != nil {
				c.log.WithError(err).Error("error unmarshal event")
				break
			}

			e := jsonValue{v: evt}

			id := e.mapGet("id").asInt()
			method := e.mapGet("method").asString()
			params := e.mapGet("params")
			c.handleEvt(id, method, params)
		}
	}
}

func (c *connection) handleEvt(id int, method string, params jsonValue) {
	var r interface{} = 0

	switch method {
	case "live":
		c.server.enableVideo(c)
	case "list_addressees":
		r = [][]interface{}{{1, "MainBell", ""}}
	case "trigger":
		// TODO check if param equals to ['door']
		c.server.openDoor(c)
	case "bell_ack":
		isAck := params.arrGet(0).asBool()
		if isAck {
			c.server.bellAck(c)
		} else {
			c.server.bellReject(c)
		}
	case "suppress":
		c.server.bellSupress(c)
	case "register_device":
		c.server.registerBell(c, id)
	case "pong":
		return
	}

	c.sendEvent(map[string]interface{}{"result": r, "error": nil, "id": id})
}

func (c *connection) handleHttp(rb *mxpeg.RingBuffer) (err error) {
	defer rb.Recover(&err)

	cmdHandled := false

	for {
		line := mxpeg.ReadLine(rb)
		if len(line) == 0 {
			break
		}

		if strings.HasPrefix(line, "GET ") || strings.HasPrefix(line, "POST ") {
			verbs := strings.Fields(line)
			if len(verbs) > 1 {
				cmd := verbs[1]

				switch cmd {
				case "/bell":
					cmdHandled = true
					c.server.sendBell(true)
				case "/nobell":
					cmdHandled = true
					c.server.sendBell(false)
				}
			}
		}
	}

	if cmdHandled {
		c.send([]byte("HTTP/1.1 200 OK\r\n\r\nCommand applied\r\n"))
		return errors.New("command applied")
	}

	return nil
}

func readEvt(rb *mxpeg.RingBuffer) (data []byte, err error) {
	defer rb.Recover(&err)

	if rb.Get() == 0xff {
		rb.Move(1)
		if rb.Next() != 0xeb {
			return nil, mxpeg.ErrParseError
		}

		l := (rb.Next() << 8) | rb.Next()

		rb.Move(l - 2)

		return rb.GetAndCut(), nil
	} else {
		for {
			if rb.Next() == 0x0a {
				break
			}
		}

		if rb.Next() != 0x00 {
			return nil, mxpeg.ErrParseError
		}

		rb.Move(-2)
		d := rb.GetAndCut()
		rb.Move(2)
		rb.Cut()

		return d, nil
	}
}
