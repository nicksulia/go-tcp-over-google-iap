package iap

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/coder/websocket"
	"github.com/nicksulia/go-tcp-over-google-iap/logger"
)

// IncomingFrame represents an incoming frame used in the IAP tunnel protocol.
// It contains the raw data of the frame and provides methods to extract information from it.
// The frame structure is defined by the protocol specification and includes a message TAG, SID, ACK, and data.
// The frame is expected to be in binary format with specific lengths for each field.
type IncomingFrame struct {
	data []byte
}

// NewIncomingFrame creates a new IncomingFrame with the provided message data.
func NewIncomingFrame(msg []byte) *IncomingFrame {
	return &IncomingFrame{
		data: msg,
	}
}

// Type returns the TAG of the incoming frame.
// The TAG is a 2-byte field that indicates the type of message being sent.
func (f *IncomingFrame) Type() uint16 {
	tag := binary.BigEndian.Uint16(f.data[:MessageTagLen])
	return tag
}

// SID returns the SID (Session ID) of the incoming frame.
// The SID is a variable-length field that starts after the message TAG and its length.
// The length of the SID is specified in the frame header, and it is encoded as a 4-byte unsigned integer.
// The SID is used to identify the session associated with the frame.
func (f *IncomingFrame) SID() string {
	sidLen := binary.BigEndian.Uint32(f.data[MessageTagLen:SIDHeaderLen])
	return string(f.data[SIDHeaderLen : SIDHeaderLen+sidLen])
}

// ACK returns the ACK (Acknowledgment) value of the incoming frame.
// The ACK is a 64-bit unsigned integer that starts after the message TAG and is used
// to acknowledge the receipt of data. It is encoded in big-endian format.
// The ACK value is used to confirm the number of bytes received in the session.
func (f *IncomingFrame) ACK() uint64 {
	return binary.BigEndian.Uint64(f.data[MessageTagLen:ACKHeaderLen])
}

// Data returns the data payload of the incoming frame.
// The data payload is a variable-length field that starts after the message TAG and the message length.
// The length of the data payload is specified in the frame header as a 4-byte unsigned integer
func (f *IncomingFrame) Data() ([]byte, []byte) {
	msgLen := binary.BigEndian.Uint32(f.data[MessageTagLen:DataMessageHeaderLen])
	fullPayload := f.data[DataMessageHeaderLen:]
	return fullPayload[:msgLen], fullPayload[msgLen:]
}

// ACKFrame represents an ACK frame used in the IAP tunnel protocol.
type ACKFrame struct {
	frame  []byte
	ackVal uint64
	logger logger.Logger
}

// Send sends the ACK frame over the provided WebSocket connection.
func (f *ACKFrame) Send(conn *websocket.Conn) (int, error) {
	writer, err := conn.Writer(context.Background(), websocket.MessageBinary)
	if err != nil {
		return 0, fmt.Errorf("websocket writer init failure: %w", err)
	}
	defer writer.Close()
	written, err := writer.Write(f.frame)
	dataWritten := written - ACKHeaderLen
	f.logger.Debug("Send ACK frame", "frame size", len(f.frame), "binary_data", f.frame)

	return dataWritten, err
}

// NewACKFrame creates a new ACK frame with the specified inbound data length.
func NewACKFrame(inboundDataLen uint64, logger logger.Logger) *ACKFrame {
	ackFrame := &ACKFrame{
		ackVal: inboundDataLen,
		logger: logger,
	}
	ackFrame.frame = make([]byte, ACKHeaderLen)
	binary.BigEndian.PutUint16(ackFrame.frame[0:], RelayACK)
	binary.BigEndian.PutUint64(ackFrame.frame[MessageTagLen:], inboundDataLen)
	return ackFrame
}

// DataFrame represents a data frame used in the IAP tunnel protocol.
type DataFrame struct {
	frame  []byte
	logger logger.Logger
}

// Send sends the data frame over the provided WebSocket connection.
func (f *DataFrame) Send(conn *websocket.Conn) (int, error) {
	writer, err := conn.Writer(context.Background(), websocket.MessageBinary)
	if err != nil {
		return 0, fmt.Errorf("websocket writer init failure: %w", err)
	}
	defer writer.Close()
	written, err := writer.Write(f.frame)
	f.logger.Debug("Send Data frame", "frame size", len(f.frame), "bytes_to_send[:20]", f.frame[:20])
	dataWritten := written - DataMessageHeaderLen
	return dataWritten, err
}

// NewDataFrame creates a new DataFrame with the provided data.
func NewDataFrame(sendData []byte, logger logger.Logger) *DataFrame {
	dataFrame := &DataFrame{
		logger: logger,
	}
	dataFrame.frame = make([]byte, DataMessageHeaderLen+len(sendData))
	binary.BigEndian.PutUint16(dataFrame.frame[0:], RelayData)
	binary.BigEndian.PutUint32(dataFrame.frame[MessageTagLen:], uint32(len(sendData)))
	copy(dataFrame.frame[DataMessageHeaderLen:], sendData)
	return dataFrame
}
