package mxpeg

import (
	"errors"
	"io"
	"mobell-proxy/log"
	"strings"
)

var ErrReadError = errors.New("read error")
var ErrClosed = errors.New("connection closed")
var ErrOverflow = errors.New("buffer overflow")

type RingBuffer struct {
	buf    []byte
	size   int
	mask   int
	start  int
	end    int
	pos    int
	reader io.Reader

	log log.Interface
}

func NewRingBuffer(size int, reader io.Reader, log log.Interface) *RingBuffer {
	s := 1
	for s < size {
		s <<= 1
	}

	return &RingBuffer{
		buf:    make([]byte, s),
		size:   s,
		mask:   s - 1,
		reader: reader,
		log:    log,
	}
}

func (r *RingBuffer) Recover(err *error) {
	if e := recover(); e != nil {
		if e == ErrClosed {
			*err = ErrClosed
		} else if e == ErrReadError || e == ErrOverflow {
			e := e.(error)
			r.log.WithField("error", e.Error()).Error("ring buffer error")
			*err = e
		} else {
			r.log.WithField("error", e).Error("ring buffer fatal error")
			panic(e)
		}
	}
}

func (r *RingBuffer) Reset() {
	r.pos = 0
	r.start = 0
	r.end = 0
}

func (r *RingBuffer) norm(pos int) int {
	return pos & r.mask
}

func (r *RingBuffer) dist(pos int) int {
	return r.norm(pos - r.start)
}

func (r *RingBuffer) read() {
	if r.dist(r.end) >= (r.size - 1) {
		panic(ErrOverflow)
	}

	var b []byte
	if r.end >= r.start {
		b = r.buf[r.end:]
	} else {
		b = r.buf[r.end:r.start]
	}

	nr, err := r.reader.Read(b)

	if err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			panic(ErrClosed)
		} else {
			panic(ErrReadError)
		}
	}

	r.end = r.norm(r.end + nr)
}

func (r *RingBuffer) readToPos(pos int) {
	for r.dist(pos) > r.dist(r.end) {
		r.read()
	}
}

func (r *RingBuffer) Move(step int) {
	if (r.dist(r.pos) + step) >= r.size {
		panic(ErrOverflow)
	}

	r.pos = r.norm(r.pos + step)
}

func (r *RingBuffer) Get() int {
	r.readToPos(r.pos + 1)
	return int(r.buf[r.pos])
}

func (r *RingBuffer) Next() int {
	v := r.Get()
	r.Move(1)
	return v
}

func (r *RingBuffer) cut(pos int) {
	pos = r.norm(pos)
	r.readToPos(pos)
	r.start = pos
}

func (r *RingBuffer) Cut() {
	r.cut(r.pos)
}

func (r *RingBuffer) CutWithStep(step int) {
	r.cut(r.pos + step)
}

func (r *RingBuffer) GetAndCut() []byte {
	b := r.get(r.start, r.pos)
	r.Cut()
	return b
}

func (r *RingBuffer) get(from int, to int) []byte {
	r.readToPos(to)

	from = r.norm(from)
	to = r.norm(to-1) + 1

	s := r.norm(to - from)
	b := make([]byte, s)

	if from < to {
		copy(b, r.buf[from:to])
	} else if from > to {
		copy(b, r.buf[from:])
		copy(b[r.size-from:], r.buf[:to])
	}

	return b
}
