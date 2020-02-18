package mxpeg

import (
	"encoding/json"
	"errors"
	"mobell-proxy/log"
)

var ErrParseError = errors.New("parse error")

// markers
const SOI = 0xD8
const APP0 = 0xE0
const COM = 0xFE
const DQT = 0xDB
const DHT = 0xC4

//const DRI = 0xDD
const SOF0 = 0xC0
const SOS = 0xDA
const EOI = 0xD9
const APP11 = 0xEB
const APP12 = 0xEC
const APP13 = 0xED

type EventFunc func(map[string]interface{}) bool
type VideoFunc func([]byte, bool)
type AudioFunc func([]byte)

type PacketReader struct {
	onEvent EventFunc
	onVideo VideoFunc
	onAudio AudioFunc
	reader  *RingBuffer

	log log.Interface
}

func NewReader(onEvent EventFunc, onVideo VideoFunc, onAudio AudioFunc, reader *RingBuffer, log log.Interface) *PacketReader {
	return &PacketReader{
		onEvent: onEvent,
		onVideo: onVideo,
		onAudio: onAudio,
		reader:  reader,

		log: log,
	}
}

func (p *PacketReader) ReadPacket() (err error) {
	r := p.reader

	defer r.Recover(&err)

	// skip garbage
	for r.Next() != 0xff {
	}

	// include last 0xff marker
	r.CutWithStep(-1)

	switch r.Next() {
	case SOI:
		return p.readVideo()
	case APP13:
		return p.readAudioAlaw()
	case APP11:
		return p.readAudioPcm()
	case APP12:
		return p.readEvents()
	default:
		return ErrParseError
	}
}

func (p *PacketReader) readVideo() error {
	r := p.reader

	frameStart := false

	for {
		for r.Next() != 0xff {
		}

		marker := r.Next()

		if marker == EOI {
			break
		}

		if marker != SOF0 && marker != SOS && marker != APP0 && marker != COM && marker != DQT && marker != DHT {
			return ErrParseError
		}

		if marker == SOF0 {
			frameStart = true
		}

		l := (r.Next() << 8) | r.Next()
		r.Move(l - 2)

		if marker == SOS {
			for {
				for r.Next() != 0xff {
				}

				marker = r.Next()
				if marker != 0 {
					r.Move(-2)
					break
				}
			}
		}
	}

	p.onVideo(r.GetAndCut(), frameStart)

	return nil
}

func (p *PacketReader) readAudioAlaw() error {
	/*
		r := p.reader

		l := (r.Next() << 8) | r.Next()

		// we don't need duration and timestamp right now
		duration := r.Next() | (r.Next() << 8) | (r.Next() << 16) | (r.Next() << 24)
		var timestamp uint64
		for i := 0; i < 8; i++ {
			timestamp |= uint64(r.Next()) << (i * 8)
		}

		r.Move(l - 2 - 12)

		p.listener.OnAudio(r.GetAndCut())

		return nil
	*/

	// telling the truth, alaw packets are not really supported by clients
	return ErrParseError
}

func (p *PacketReader) readAudioPcm() error {
	r := p.reader

	l := (r.Next() << 8) | r.Next()

	if r.Next() != int('M') {
		return ErrParseError
	}

	if r.Next() != int('X') {
		return ErrParseError
	}

	t := r.Next()

	r.Move(l - 2 - 3)

	if t == int('A') {
		p.onAudio(r.GetAndCut())
	} else {
		r.Cut()
	}

	return nil
}

func (p *PacketReader) readEvents() error {
	r := p.reader

	l := (r.Next() << 8) | r.Next()

	r.Cut()
	r.Move(l - 2)

	var evt map[string]interface{}

	v := r.GetAndCut()
	if v[len(v)-1] == 0 {
		v = v[:len(v)-1]
	}

	p.log.WithField("event", string(v)).Debug("received event")

	err := json.Unmarshal(v, &evt)
	if err != nil {
		p.log.Warn("error unmarshal event")
		return err
	}

	p.onEvent(evt)

	return nil
}
