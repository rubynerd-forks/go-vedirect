package vedirect

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/tarm/serial"
)

const (
	InChecksum = 1
	InFrame    = 2
	InLabel    = 3
	InValue    = 4
	WaitHeader = 5
)

// Block :
type Block struct {
	checksum int
	fields   map[string]string
}

// Stream :
type Stream struct {
	Device string
	Port   io.Reader
	State  int
}

// Streamer :
type Streamer interface {
	Read() int
}

// OpenSerial as the name suggests is for opening serial devices
// to be fed into NewStream
func OpenSerial(dev string) io.Reader {
	c := &serial.Config{Name: dev, Baud: 19200}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	return s
}

// OpenFile for opening files to be fed into NewStream
func OpenFile(dev string) io.Reader {
	s, err := os.Open(dev)
	if err != nil {
		log.Fatal(err)
	}

	return s
}

// NewStream is for initalising a stream for reading blocks
func NewStream(stream io.Reader) Stream {
	s := Stream{}
	s.Port = stream
	s.State = 0

	log.Println("Stream initialized:", s)
	return s
}

// ReadBlock where the magic happens!
// Field format: <Newline><Field-Label><Tab><Field-Value>
// Last field in block will always be "Checksum".
// The value is a single byte, and the modulo 256 sum
// of all bytes in a block will equal 0 if there were
// no transmission errors.
func (s *Stream) ReadBlock() (Block, int) {
	var b = Block{}
	b.fields = make(map[string]string)
	// frameBuf captures the bytes of an in-flight HEX frame for diagnostic
	// logging. Capped at frameBufCap; further bytes are counted but not stored.
	const frameBufCap = 256
	const postFrameDebugBytes = 32
	var frameBuf = make([]byte, 0, frameBufCap)
	var frameTotalLen int = 0
	var postFrameDebug int = 0
	var prevState int
	//var label string
	//var value string
	var label = make([]byte, 0, 9)  // VE recommended buffer size.
	var value = make([]byte, 0, 33) // VE recommended buffer size.

	buf := make([]byte, 1)

	for {
		n, err := s.Port.Read(buf)
		if err != nil {
			log.Fatal(err)
		}

		str := string(buf[:n])
		var char byte = buf[0]

		// HEX mode is documented in BlueSolar-HEX-protocol-MPPT.pdf.
		// catch and ignore VE.Direct HEX frames from stream, otherwise
		// they mess up our checksum and we lose the current block.
		if char == ':' && s.State != InChecksum { // ":": beginning of frame
			//if str == ":" { // ":": beginning of frame
			prevState = s.State // save state
			s.State = InFrame
			frameBuf = frameBuf[:0]
			frameBuf = append(frameBuf, char)
			frameTotalLen = 1
			continue
		}
		if s.State == InFrame {
			frameTotalLen++
			if len(frameBuf) < frameBufCap {
				frameBuf = append(frameBuf, char)
			}
			if str == "\n" { // end of frame
				s.State = prevState // restore state
				truncNote := ""
				if frameTotalLen > frameBufCap {
					truncNote = fmt.Sprintf(" [truncated, captured %d of %d]", frameBufCap, frameTotalLen)
				}
				fmt.Printf("HEX frame ignored: prevState=%d len=%d bytes=% x%s\n", prevState, frameTotalLen, frameBuf, truncNote)
				postFrameDebug = postFrameDebugBytes
			}
			continue // ignore frame contents
		}

		if postFrameDebug > 0 {
			fmt.Printf("post-HEX byte: state=%d 0x%02x %q\n", s.State, char, string(char))
			postFrameDebug--
		}

		// convert byte to integer and add to sum.
		b.checksum += int(buf[0])

		// end of block. must process before byte evaluation.
		// checksum byte could have any value.
		if s.State == InChecksum {
			s.State = WaitHeader
			//if b.checksum % 256 == 0 {
			return b, b.checksum % 256 // 0 on valid checksum
			//} else {
			//  fmt.Println("Bad block!", b.fields)
			//}
		}

		switch char {
		case 13: // "\r": beginning of field
			if s.State != WaitHeader { // avoid zero-valued entry on first run
				//  b.fields[label] = value
				b.fields[string(label)] = string(value)
			}
			//label = ""
			//value = ""
			label = label[:0] // clear slice
			value = value[:0] // clear slice
			s.State = InLabel
			//continue

		case 10: // "\n": avoid appending \n to label
			//continue

		case 9: // "\t": label/value seperator
			if string(label) == "Checksum" {
				s.State = InChecksum
			} else {
				s.State = InValue
			}
			//continue

		default:
			if s.State == InLabel {
				//label += str
				label = append(label, buf[0])
			} else if s.State == InValue {
				//value += str
				value = append(value, buf[0])
			}
		}
	}
}

// Fields : Getter for fields
func (b Block) Fields() map[string]string {
	return b.fields
}
