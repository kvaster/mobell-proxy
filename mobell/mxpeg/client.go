package mxpeg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/apex/log"
	"mobell-proxy/mobell/stream"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// 4 mb should be enough for any frame
const ringBufferSize = 4 * 1024 * 1024

type StreamStartFunc func()
type StreamStopFunc func()

type Listener struct {
	OnStreamStart StreamStartFunc
	OnStreamStop  StreamStopFunc
	OnEvent       EventFunc
	OnVideo       VideoFunc
	OnAudio       AudioFunc
}

type Client struct {
	mobotixAddr string
	mobotixUser string
	mobotixPass string

	runCtx      context.Context
	runCancel   context.CancelFunc
	runFinished chan struct{}

	listener *Listener
	stream   unsafe.Pointer

	packetId uint32
	events   sync.Map

	log log.Interface
}

func NewClient(mobotixAddr string, mobotixUser string, mobotixPass string, listener *Listener) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		mobotixAddr: mobotixAddr,
		mobotixUser: mobotixUser,
		mobotixPass: mobotixPass,
		runCtx:      ctx,
		runCancel:   cancel,
		runFinished: make(chan struct{}),
		listener:    listener,

		log: log.WithField("ctx", mobotixAddr),
	}
}

func (c *Client) Start() {
	c.log.Debug("starting")
	go c.run()
}

func (c *Client) Stop() {
	c.log.Debug("stopping")

	c.runCancel()
	<-c.runFinished

	c.log.Debug("stopped")
}

func (c *Client) Reconnect() {
	c.log.Debug("requesting reconnect")

	s := (*stream.Stream)(atomic.LoadPointer(&c.stream))
	if s != nil {
		s.Close()
	}
}

func (c *Client) run() {
	for {
		select {
		case <-c.runCtx.Done():
			close(c.runFinished)
			return
		default:
			// do nothing
		}

		c.log.Debug("connecting to mobotix")
		c.runOnce()
		c.log.Debug("connection terminated")

		waitCtx, waitCancel := context.WithTimeout(c.runCtx, time.Second*5)
		_ = <-waitCtx.Done()
		waitCancel()
	}
}

func (c *Client) runOnce() {
	s, err := stream.Connect(c.runCtx, c.mobotixAddr, time.Second*5, c.log)
	if err != nil {
		return
	}

	defer s.Close()

	rb := NewRingBuffer(ringBufferSize, s, c.log)

	host := strings.FieldsFunc(c.mobotixAddr, func(r rune) bool { return r == ':' })[0]
	auth := base64.StdEncoding.EncodeToString([]byte(c.mobotixUser + ":" + c.mobotixPass))

	msg := fmt.Sprintf(
		"POST /control/eventstream.jpg HTTP/1.1\r\nHost: %s\r\nAuthorization: Basic %s\r\n\r\n",
		host,
		auth,
	)

	_, _ = s.Write([]byte(msg))

	status, err := handleHttp(rb)
	if err != nil {
		c.log.WithError(err).Warn("error connecting")
		return
	}
	if status != http.StatusOK {
		c.log.WithField("status", status).Warn("error connecting")
		return
	}

	// reset packet id
	atomic.StoreUint32(&c.packetId, 10)
	// clear old packets
	c.events.Range(func(key, _ interface{}) bool {
		c.events.Delete(key)
		return true
	})

	atomic.StorePointer(&c.stream, unsafe.Pointer(s))
	if c.listener.OnStreamStart != nil {
		c.listener.OnStreamStart()
	}

	pr := NewReader(c.OnEvent, c.OnVideo, c.OnAudio, rb, c.log)

	for {
		err := pr.ReadPacket()
		if err != nil {
			c.log.WithField("error", err.Error()).Warn("error reading packet")
			break
		}
	}

	c.listener.OnStreamStop()
	atomic.StorePointer(&c.stream, nil)
}

func handleHttp(rb *RingBuffer) (status int, err error) {
	defer rb.Recover(&err)

	// get/post request
	f := strings.Fields(ReadLine(rb))
	if len(f) < 2 {
		return 0, ErrReadError
	}

	s, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, err
	}

	for {
		line := ReadLine(rb)
		if len(line) == 0 {
			break
		}
	}

	return s, nil
}

func ReadLine(rb *RingBuffer) string {
	for {
		c := rb.Next()
		if c == 0x0d || c == 0x0a {
			break
		}
	}

	rb.Move(-1)
	line := string(rb.GetAndCut())

	rb.Move(1)
	if rb.Get() == 0x0a {
		rb.Move(1)
	}

	rb.Cut()

	return line
}

func (c *Client) Write(data []byte) {
	s := (*stream.Stream)(atomic.LoadPointer(&c.stream))
	if s != nil {
		// ignore result - we know it will consume the whole slice
		_, _ = s.Write(data)
	}
}

func (c *Client) writeCmd(cmd []byte) {
	l := len(cmd)
	b := make([]byte, l+2)
	copy(b, cmd)
	b[l] = 0x0a
	b[l+1] = 0
	c.Write(b)
}

func (c *Client) SendCmdSilent(method string, params interface{}) {
	c.SendCmd(method, params, nil)
}

func (c *Client) SendCmd(method string, params interface{}, listener EventFunc) {
	id := atomic.AddUint32(&c.packetId, 1)

	if listener != nil {
		c.events.Store(id, listener)
	}

	evt := map[string]interface{}{
		"id":     id,
		"method": method,
	}

	if params != nil {
		evt["params"] = params
	}

	cmd, err := json.Marshal(evt)

	if err == nil {
		log.WithField("cmd", string(cmd)).Debug("sending")
		c.writeCmd(cmd)
	} else {
		log.Error("fatal error on marshalling event")
	}
}

func (c *Client) OnEvent(evt map[string]interface{}) bool {
	idi, ok := evt["id"]
	if ok {
		idf, ok := idi.(float64)
		if ok {
			id := uint32(idf)
			l, ok := c.events.Load(id)
			if ok && l != nil {
				r := l.(EventFunc)(evt)
				if r {
					c.events.Delete(id)
				}
				return r
			}
		}
	}

	return true
}

func (c *Client) OnVideo(data []byte, frameStart bool) {
	if c.listener.OnVideo != nil {
		c.listener.OnVideo(data, frameStart)
	}
}

func (c *Client) OnAudio(data []byte) {
	if c.listener.OnAudio != nil {
		c.listener.OnAudio(data)
	}
}
