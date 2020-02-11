package mobell

import (
	"context"
	"encoding/json"
	"mobell-proxy/log"
	"mobell-proxy/mobell/mxpeg"
	"mobell-proxy/mobell/stream"
	"net"
	"time"
)

type connection struct {
	server *Server

	rb  *mxpeg.RingBuffer
	str *stream.Stream

	videoEnabled bool
	bellEvtId    int
}

func handleConnection(ctx context.Context, conn net.Conn, server *Server) {
	str := stream.NewStream(ctx, conn)
	str.ReadTimeout = time.Second * 180

	c := &connection{
		server: server,
		rb:     mxpeg.NewRingBuffer(16*1024, str),
		str:    str,
	}
	go c.run()
}

func (c *connection) send(data []byte) {
	_, _ = c.str.Write(data)
}

func (c *connection) sendEvent(evt map[string]interface{}) {
	b, err := json.Marshal(evt)
	if err != nil {
		log.WithError(err).Error("error marshalling event")
		return
	}

	log.WithField("evt", string(b)).Debug("sending client event")

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

func (c *connection) run() {
	c.server.addConnection(c)

	str := c.str

	defer func() {
		c.server.delConnection(c)
		str.Close()
	}()

	rb := mxpeg.NewRingBuffer(16*1024, str)

	// http part
	err := handleHttp(rb)
	if err != nil {
		log.WithError(err).Error("error reading http request headers")
		return
	}

	// send status
	c.send([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	for {
		data, err := readEvt(rb)
		if err != nil {
			log.WithError(err).Error("error reading event")
			break
		}

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
			log.WithField("evt", string(data)).Debug("got client event")

			var evt map[string]interface{}

			err := json.Unmarshal(data, &evt)
			if err != nil {
				log.WithError(err).Error("error unmarshalling event")
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
		c.server.client.SendCmdSilent("trigger", params.v)
	case "bell_ack":
		isAck := params.arrGet(0).asBool()
		if isAck {
			c.server.bellAck(c)
		} else {
			c.server.bellReject(c)
		}
	case "stop":
		c.server.bellAck(c)
	case "register_device":
		c.server.registerBell(c, id)
	}

	c.sendEvent(map[string]interface{}{"result": r, "error": nil, "id": id})
}

func handleHttp(rb *mxpeg.RingBuffer) (err error) {
	defer mxpeg.Recover(&err)

	for {
		line := mxpeg.ReadLine(rb)
		if len(line) == 0 {
			break
		}
	}

	return nil
}

func readEvt(rb *mxpeg.RingBuffer) (data []byte, err error) {
	defer mxpeg.Recover(&err)

	if rb.Get() == 0xff {
		rb.Move(1)
		if rb.Next() != 0xeb {
			return nil, mxpeg.ErrReadError
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
			return nil, mxpeg.ErrReadError
		}

		rb.Move(-2)
		d := rb.GetAndCut()
		rb.Move(2)
		rb.Cut()

		return d, nil
	}
}
