package codec

// #cgo pkg-config: libavutil libavcodec
// #include "codec.h"
import "C"
import "unsafe"

type Codec struct {
	codec unsafe.Pointer
}

func Create() *Codec {
	return &Codec{codec: C.create()}
}

func (c *Codec) Destroy() {
	C.destroy(c.codec)
}

func (c *Codec) OnStreamStart() {
	C.onStreamStart(c.codec)
}

func (c *Codec) OnStreamStop() {
	C.onStreamStop(c.codec)
}

func (c *Codec) OnVideoPacket(data []byte) bool {
	return C.onVideoPacket(c.codec, (*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data))) == 0
}

func (c *Codec) EncodeFrame() []byte {
	var data []byte

	pkt := C.encodeFrame(c.codec)
	if pkt.size > 0 {
		data = C.GoBytes(unsafe.Pointer(pkt.data), C.int(pkt.size))
	}

	C.resetEncoder(c.codec, pkt)

	return data
}
