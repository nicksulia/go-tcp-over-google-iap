package iap

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/coder/websocket"
)

type IncomingFrame struct {
	data []byte
}

func NewIncomingFrame(msg []byte) *IncomingFrame {
	return &IncomingFrame{
		data: msg,
	}
}

func (f *IncomingFrame) Type() uint16 {
	tag := binary.BigEndian.Uint16(f.data[:MessageTagLen])
	return tag
}

func (f *IncomingFrame) SID() uint64 {
	return binary.BigEndian.Uint64(f.data[MessageTagLen:SIDHeaderLen])
}

func (f *IncomingFrame) ACK() uint64 {
	return binary.BigEndian.Uint64(f.data[MessageTagLen:ACKHeaderLen])
}

// Data returns message and rest.
func (f *IncomingFrame) Data() ([]byte, []byte) {
	msgLen := binary.BigEndian.Uint32(f.data[MessageTagLen:DataMessageHeaderLen])
	fullPayload := f.data[DataMessageHeaderLen:]
	return fullPayload[:msgLen], fullPayload[msgLen:]
}

type ACKFrame struct {
	frame []byte
}

func (f *ACKFrame) Send(conn *websocket.Conn) (int, error) {
	writer, err := conn.Writer(context.Background(), websocket.MessageBinary)
	if err != nil {
		fmt.Println("Websocket writer init failure", "err", err)
		return 0, fmt.Errorf("websocket writer init failure: %w", err)
	}
	defer writer.Close()
	written, err := writer.Write(f.frame)
	dataWritten := written - ACKHeaderLen
	fmt.Println("Sent ACK frame", "bytes", dataWritten, "payload_len", len(f.frame))

	return dataWritten, err
}

func NewACKFrame(inboundDataLen uint64) *ACKFrame {
	ackFrame := &ACKFrame{}
	ackFrame.frame = make([]byte, ACKHeaderLen)
	binary.BigEndian.PutUint16(ackFrame.frame[0:], RelayACK)
	binary.BigEndian.PutUint64(ackFrame.frame[MessageTagLen:], inboundDataLen)
	return ackFrame
}

type DataFrame struct {
	frame []byte
}

func (f *DataFrame) Send(conn *websocket.Conn) (int, error) {
	writer, err := conn.Writer(context.Background(), websocket.MessageBinary)
	if err != nil {
		return 0, fmt.Errorf("websocket writer init failure: %w", err)
	}
	defer writer.Close()
	written, err := writer.Write(f.frame)

	dataWritten := written - DataMessageHeaderLen
	return dataWritten, err
}

func NewDataFrame(sendData []byte) *DataFrame {
	dataFrame := &DataFrame{}
	dataFrame.frame = make([]byte, DataMessageHeaderLen+len(sendData))
	binary.BigEndian.PutUint16(dataFrame.frame[0:], RelayData)
	binary.BigEndian.PutUint32(dataFrame.frame[MessageTagLen:], uint32(len(sendData)))
	copy(dataFrame.frame[DataMessageHeaderLen:], sendData)
	return dataFrame
}
