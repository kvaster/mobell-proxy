package mobell

import (
	"container/list"
	"context"
	"github.com/apex/log"
	"mobell-proxy/mobell/codec"
	"mobell-proxy/mobell/mxpeg"
	"net"
	"syscall"
	"time"
)

var audioStopEvt = []byte{
	0xff, 0xeb, 0x00, 0x14, 0x4d, 0x58, 0x53, 0x00, 0x01, 0x01, 0x00,
	0x00, 0x80, 0x3e, 0x00, 0x00, 0x20, 0x50, 0x31, 0x36, 0x01, 0x01,
}

type Server struct {
	listenAddr   string
	mac          string
	keepAliveSec int

	conns     *list.List
	audioConn *connection

	client *mxpeg.Client

	connListener net.Listener

	runCtx      context.Context
	runCancel   context.CancelFunc
	runFinished chan struct{}

	cmdCh chan func()

	codec *codec.Codec

	dht      []byte
	dqt      []byte
	patchDxt bool
}

func New(listenAddr string, mobotixAddr string, mobotixUser string, mobotixPass string, mac string, keepAliveSec int) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		listenAddr:   listenAddr,
		mac:          mac,
		keepAliveSec: keepAliveSec,
		conns:        list.New(),
		runCtx:       ctx,
		runCancel:    cancel,
		runFinished:  make(chan struct{}),
		cmdCh:        make(chan func()),
	}

	s.client = mxpeg.NewClient(mobotixAddr, mobotixUser, mobotixPass, &mxpeg.Listener{
		OnStreamStart: s.OnStreamStart,
		OnStreamStop:  s.OnStreamStop,
		OnEvent:       s.OnEvent,
		OnVideo:       s.OnVideo,
		OnAudio:       s.OnAudio,
	})

	return s
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}

	s.connListener = ln

	s.codec = codec.Create()

	s.client.Start()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Debug("finished accepting new connections")
				break
			}

			log.Debug("connection accepted")

			s.setKeepalive(conn.(*net.TCPConn))

			handleConnection(s.runCtx, conn, s)
		}
	}()

	go s.run()

	return nil
}

func (s *Server) setKeepalive(conn *net.TCPConn) {
	if conn.SetKeepAlive(true) != nil {
		log.Warn("can't enable keepalive")
	}

	if conn.SetKeepAlivePeriod(time.Second*120) != nil {
		log.Warn("can't set keepalive period")
	}

	rawConn, err := conn.SyscallConn()
	if err != nil {
		log.Warn("can't get raw connection")
		return
	}

	err = rawConn.Control(func(fdPtr uintptr) {
		fd := int(fdPtr)

		if syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 3) != nil {
			log.Warn("can't set number of probes")
		}

		if syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 5) != nil {
			log.Warn("can't set retry delay")
		}
	})

	if err != nil {
		log.Warn("can't set additional keepalive paramteres")
	}
}

func (s *Server) run() {
	for {
		select {
		case cmd := <-s.cmdCh:
			cmd()
		case _ = <-s.runCtx.Done():
			close(s.runFinished)
			_ = s.connListener.Close()
			log.Debug("server finished run")
			return
		}
	}
}

func (s *Server) Stop() {
	log.Info("stopping client")
	s.client.Stop()
	log.Info("stopping server")
	s.runCancel()
	<-s.runFinished
	s.codec.Destroy()
	log.Info("stopped")
}

func (s *Server) OnStreamStart() {
	s.codec.OnStreamStart()

	c := s.client
	c.SendCmdSilent("mode", []string{"mxpeg"})
	c.SendCmdSilent("audiooutput", []string{"pcm16"})
	c.SendCmdSilent("live", []interface{}{false})
	c.SendCmd("list_addressees", nil, func(evt map[string]interface{}) bool {
		devId := jsonValue{v: evt}.mapGet("result").arrGet(0).arrGet(0).asInt()
		c.SendCmd(
			"add_device",
			[]interface{}{s.mac, []int{devId}, "MoBell+" + s.mac},
			func(evt map[string]interface{}) bool {
				c.SendCmd("register_device", []string{s.mac}, s.onBell)
				return true
			},
		)
		return true
	})
}

func (s *Server) onBell(evt map[string]interface{}) bool {
	r := jsonValue{v: evt}.mapGet("result")
	t := r.arrGet(0).asString()

	if t == "bell" {
		isRing := r.arrGet(1).asBool()
		log.WithField("ringing", isRing).Debug("received bell")
		s.sendBell(isRing)
	}

	return false
}

func (s *Server) sendBell(isRing bool) {
	s.cmdCh <- func() {
		for e := s.conns.Front(); e != nil; e = e.Next() {
			e.Value.(*connection).sendBell(isRing)
		}
	}
}

func (s *Server) OnStreamStop() {
	s.codec.OnStreamStop()
}

func (s *Server) OnEvent(_ map[string]interface{}) bool {
	return true
}

func (s *Server) sendVideo(data []byte) {
	for e := s.conns.Front(); e != nil; e = e.Next() {
		c := e.Value.(*connection)
		if c.videoEnabled {
			c.send(data)
		}
	}
}

func (s *Server) OnVideo(data []byte, frameStart bool) {
	if !s.codec.OnVideoPacket(data) {
		log.Error("error decoding video frame")
		s.client.Reconnect()
		return
	}

	s.cmdCh <- func() {
		// we need to store dqt and dht from original stream
		// we will patch motion frames with this values right after key frame generation
		dqt, dht := mxpeg.ExtractDqtDht(data)
		if dqt != nil {
			s.dqt = dqt
		}
		if dht != nil {
			s.dht = dht
		}

		// patch
		if s.patchDxt {
			s.patchDxt = false
			data = mxpeg.PatchDqtDht(data, s.dqt, s.dht)
		}

		s.sendVideo(data)
	}
}

func (s *Server) OnAudio(data []byte) {
	s.cmdCh <- func() {
		s.sendVideo(data)
	}
}

func (s *Server) addConnection(conn *connection) {
	s.cmdCh <- func() {
		s.conns.PushBack(conn)
	}
}

func (s *Server) delConnection(conn *connection) {
	s.cmdCh <- func() {
		for e := s.conns.Front(); e != nil; e = e.Next() {
			if e.Value == conn {
				s.conns.Remove(e)
			}
		}

		if s.audioConn == conn {
			s.audioConn = nil
			// send stop command, cause it was not sent by connection itself
			s.client.Write(audioStopEvt)
		}
	}
}

func (s *Server) enableVideo(conn *connection) {
	s.cmdCh <- func() {
		if !conn.videoEnabled {
			conn.videoEnabled = true
			data := s.codec.EncodeFrame()
			if data != nil {
				conn.send(data)
			}
		}

		s.patchDxt = true
	}
}

func (s *Server) audioStart(conn *connection, data []byte) {
	s.cmdCh <- func() {
		if s.audioConn == nil {
			log.Debug("audio recording started")
			s.audioConn = conn
			s.client.Write(data)
		} else {
			log.Debug("can't start audio recording - busy with another connection")
		}
	}
}

func (s *Server) audioStop(conn *connection, data []byte) {
	s.cmdCh <- func() {
		if s.audioConn == conn {
			log.Debug("audio recording stopped")
			s.audioConn = nil
			s.client.Write(data)
		} else {
			log.Debug("can't stop audio recording - busy with another connection")
		}
	}
}

func (s *Server) audioData(conn *connection, data []byte) {
	s.cmdCh <- func() {
		if s.audioConn == conn {
			s.client.Write(data)
		}
	}
}

func (s *Server) registerBell(conn *connection, evtId int) {
	s.cmdCh <- func() {
		conn.bellEvtId = evtId
	}
}

type notifyAction func(*connection)

func (s *Server) notifyOthers(conn *connection, na notifyAction) {
	s.cmdCh <- func() {
		for e := s.conns.Front(); e != nil; e = e.Next() {
			c := e.Value.(*connection)
			if c != conn {
				na(c)
			}
		}
	}
}

func (s *Server) bellResp(conn *connection, method string, params interface{}) {
	s.client.SendCmdSilent(method, params)
	s.notifyOthers(conn, func(c *connection) {
		c.sendBell(false)
	})
}

func (s *Server) bellAck(conn *connection) {
	s.bellResp(conn, "bell_ack", []interface{}{true})
}

func (s *Server) bellReject(conn *connection) {
	s.bellResp(conn, "bell_ack", []interface{}{false})
}

func (s *Server) bellSupress(conn *connection) {
	s.notifyOthers(conn, func(c *connection) {
		c.sendSuppress()
	})
}

func (s *Server) openDoor(conn *connection) {
	s.bellResp(conn, "trigger", []interface{}{"door"})
}
