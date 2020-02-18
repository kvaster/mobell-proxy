package stream

import (
	"container/list"
	"context"
	"mobell-proxy/log"
	"mobell-proxy/mobell/syncchan"
	"net"
	"time"
)

type Stream struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	cancel context.CancelFunc
	conn   net.Conn

	asyncCh *syncchan.Chan
	syncCh  chan []byte
	queue   *list.List

	log log.Interface
}

func Connect(ctx context.Context, addr string, timeout time.Duration, log log.Interface) (*Stream, error) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		log.WithError(err).Error("error connecting to host")
		return nil, err
	}

	return NewStream(ctx, conn, log), nil
}

func NewStream(ctx context.Context, conn net.Conn, log log.Interface) *Stream {
	c, cancel := context.WithCancel(ctx)

	s := &Stream{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,

		cancel: cancel,
		conn:   conn,

		asyncCh: syncchan.MakeChan(1),
		syncCh:  make(chan []byte),
		queue:   list.New(),

		log: log,
	}

	go s.queueData()
	go s.writeData()

	go func() {
		defer cancel()
		_ = <-c.Done()
		s.close()
	}()

	return s
}

func (s *Stream) Close() {
	s.cancel()
}

func (s *Stream) close() {
	_ = s.conn.Close()
	s.asyncCh.Close()
}

func (s *Stream) Read(buf []byte) (int, error) {
	_ = s.conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
	return s.conn.Read(buf)
}

func (s *Stream) Write(data []byte) (int, error) {
	// we may ignore send errors
	_ = s.asyncCh.Push(data)
	return len(data), nil
}

func (s *Stream) queueData() {
	var data []byte

	for {
		var syncCh chan []byte

		if data == nil {
			e := s.queue.Front()
			if e != nil {
				data = s.queue.Remove(e).([]byte)
			}
		}

		if data != nil {
			syncCh = s.syncCh
		}

		select {
		case d, ok := <-s.asyncCh.Chan():
			if !ok {
				close(s.syncCh)
				s.log.Debug("finished data queue")
				return
			}
			s.queue.PushBack(d)
		case syncCh <- data:
			data = nil
		}
	}
}

func (s *Stream) writeData() {
	for {
		data, ok := <-s.syncCh
		if !ok {
			return
		}

		for len(data) > 0 {
			_ = s.conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
			nr, err := s.conn.Write(data)
			if err != nil {
				s.log.WithError(err).Warn("error writing data")
				s.Close()
				return
			}

			data = data[nr:]
		}
	}
}
