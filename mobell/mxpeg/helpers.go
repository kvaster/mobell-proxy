package mxpeg

func ExtractDqtDht(frame []byte) (dqt []byte, dht [] byte) {
	// skip initial header - it was checked by packet reader before
	p := 2
	l := len(frame)

	for p < (l - 4) {
		if frame[p] != 0xff {
			return
		}

		m := int(frame[p+1])

		if m == EOI || m == SOS {
			return
		}

		s := ((int(frame[p+2]) << 8) | int(frame[p+3])) + 2

		if (p + s) > l {
			return
		}

		if m == DQT || m == DHT {
			d := make([]byte, s)
			copy(d, frame[p:p+s])
			if m == DQT {
				dqt = append(dqt, d...)
			} else {
				dht = append(dht, d...)
			}
		}

		p += s
	}

	return
}

func PatchDqtDht(frame []byte, dqt []byte, dht []byte) []byte {
	p := 2
	l := len(frame)

	f := make([]byte, 0, l + len(dht) + len(dqt))
	f = append(f, frame[:2]...)

	for p < (l - 4) {
		if frame[p] != 0xff {
			break
		}

		m := int(frame[p+1])

		if m == SOS {
			if dqt != nil {
				f = append(f, dqt...)
			}
			if dht != nil {
				f = append(f, dht...)
			}
			break
		}

		if m == EOI {
			break
		}

		s := ((int(frame[p+2]) << 8) | int(frame[p+3])) + 2

		if (p + s) > l {
			break
		}

		if m == DHT {
			dht = nil
		}

		if m == DQT {
			dqt = nil
		}

		f = append(f, frame[p:p+s]...)
		p += s
	}

	if p < l {
		f = append(f, frame[p:]...)
	}

	return f
}
