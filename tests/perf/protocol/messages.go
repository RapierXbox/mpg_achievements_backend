package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/gocql/gocql"
)

type Vector3 struct {
	X float32
	Y float32
	Z float32
}

type Vector2I32 struct {
	X int32
	Y int32
}

const (
	// TCP
	MsgTypeHello            uint8 = 0x00
	MsgTypeHelloAck         uint8 = 0x01
	MsgTypeNewEntity        uint8 = 0x02
	MsgTypeCustomData       uint8 = 0x03
	MsgTypeRemoveEntity     uint8 = 0x04
	MsgTypeRequestChunkData uint8 = 0x05
	MsgTypeChunkData        uint8 = 0x06
	MsgTypeChatMessage      uint8 = 0x07

	// UDP
	MsgTypeEntityMove uint8 = 0x81
	MsgTypePing       uint8 = 0x82
	MsgTypePong       uint8 = 0x83
)

const (
	MagicBytes   = "MPG"
	HeaderLength = 8 // 3 bytes magic + 1 byte type + 4 bytes payload size
)

type Message interface {
	Type() uint8
	Encode() ([]byte, error)
	Decode([]byte) error
}

type Hello struct {
	RoomId      uint32
	DeviceID    gocql.UUID
	AccessToken []byte
}

type HelloAck struct {
	Success bool
}

type NewEntity struct {
	EntityID   gocql.UUID
	EntityType uint16
	Position   Vector3
	Rotation   Vector3
	CustomData []byte
}

type CustomData struct {
	EntityID   gocql.UUID
	CustomData []byte
}

type RemoveEntity struct {
	EntityID gocql.UUID
}

type RequestChunkData struct {
	ChunkX int32
	ChunkY int32
}

type ChunkData struct {
	ChunkX    int32
	ChunkY    int32
	ChunkData []byte
}

type ChatMessage struct {
	SenderID gocql.UUID
	Message  []byte
}

type EntityMove struct {
	EntityID gocql.UUID
	Position Vector3
	Rotation Vector3
}

type Ping struct{}

type Pong struct{}

// Type methods for interface compliance
func (m *Hello) Type() uint8            { return MsgTypeHello }
func (m *HelloAck) Type() uint8         { return MsgTypeHelloAck }
func (m *NewEntity) Type() uint8        { return MsgTypeNewEntity }
func (m *CustomData) Type() uint8       { return MsgTypeCustomData }
func (m *RemoveEntity) Type() uint8     { return MsgTypeRemoveEntity }
func (m *RequestChunkData) Type() uint8 { return MsgTypeRequestChunkData }
func (m *ChunkData) Type() uint8        { return MsgTypeChunkData }
func (m *ChatMessage) Type() uint8      { return MsgTypeChatMessage }
func (m *EntityMove) Type() uint8       { return MsgTypeEntityMove }
func (m *Ping) Type() uint8             { return MsgTypePing }
func (m *Pong) Type() uint8             { return MsgTypePong }

func encodeFrame(msgType uint8, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFFFFFF {
		return nil, fmt.Errorf("payload too large: %d bytes", len(payload))
	}

	buf := bytes.NewBuffer(make([]byte, 0, HeaderLength+len(payload)))
	buf.WriteString(MagicBytes)

	if err := binary.Write(buf, binary.LittleEndian, msgType); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(payload))); err != nil {
		return nil, err
	}
	buf.Write(payload)

	return buf.Bytes(), nil
}

func (m *Hello) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.RoomId); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.DeviceID); err != nil {
		return nil, err
	}
	payload.Write(m.AccessToken)

	return encodeFrame(MsgTypeHello, payload.Bytes())
}

func (m *HelloAck) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.Success); err != nil {
		return nil, err
	}

	return encodeFrame(MsgTypeHelloAck, payload.Bytes())
}

func (m *NewEntity) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.EntityID); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.EntityType); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.Position); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.Rotation); err != nil {
		return nil, err
	}
	payload.Write(m.CustomData)

	return encodeFrame(MsgTypeNewEntity, payload.Bytes())
}

func (m *CustomData) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.EntityID); err != nil {
		return nil, err
	}
	payload.Write(m.CustomData)

	return encodeFrame(MsgTypeCustomData, payload.Bytes())
}

func (m *RemoveEntity) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.EntityID); err != nil {
		return nil, err
	}

	return encodeFrame(MsgTypeRemoveEntity, payload.Bytes())
}

func (m *RequestChunkData) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.ChunkX); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.ChunkY); err != nil {
		return nil, err
	}

	return encodeFrame(MsgTypeRequestChunkData, payload.Bytes())
}

func (m *ChunkData) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.ChunkX); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.ChunkY); err != nil {
		return nil, err
	}
	payload.Write(m.ChunkData)

	return encodeFrame(MsgTypeChunkData, payload.Bytes())
}

func (m *ChatMessage) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.SenderID); err != nil {
		return nil, err
	}
	payload.Write(m.Message)

	return encodeFrame(MsgTypeChatMessage, payload.Bytes())
}

func (m *EntityMove) Encode() ([]byte, error) {
	payload := new(bytes.Buffer)

	if err := binary.Write(payload, binary.LittleEndian, m.EntityID); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.Position); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, m.Rotation); err != nil {
		return nil, err
	}

	return encodeFrame(MsgTypeEntityMove, payload.Bytes())
}

func (m *Ping) Encode() ([]byte, error) {
	return encodeFrame(MsgTypePing, []byte{})
}

func (m *Pong) Encode() ([]byte, error) {
	return encodeFrame(MsgTypePong, []byte{})
}

func (m *Hello) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.RoomId); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.DeviceID); err != nil {
		return err
	}

	remaining := buf.Len()
	m.AccessToken = make([]byte, remaining)
	if _, err := io.ReadFull(buf, m.AccessToken); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (m *HelloAck) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.Success); err != nil {
		return err
	}

	return nil
}

func (m *NewEntity) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.EntityID); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.EntityType); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.Position); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.Rotation); err != nil {
		return err
	}

	// Read remaining bytes as custom data
	remaining := buf.Len()
	m.CustomData = make([]byte, remaining)
	if _, err := io.ReadFull(buf, m.CustomData); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (m *CustomData) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.EntityID); err != nil {
		return err
	}

	// Read remaining bytes as custom data
	remaining := buf.Len()
	m.CustomData = make([]byte, remaining)
	if _, err := io.ReadFull(buf, m.CustomData); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (m *RemoveEntity) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.EntityID); err != nil {
		return err
	}

	return nil
}

func (m *RequestChunkData) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkX); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkY); err != nil {
		return err
	}

	return nil
}

func (m *ChunkData) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkX); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkY); err != nil {
		return err
	}

	// Read remaining bytes as chunk data
	remaining := buf.Len()
	m.ChunkData = make([]byte, remaining)
	if _, err := io.ReadFull(buf, m.ChunkData); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (m *ChatMessage) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.SenderID); err != nil {
		return err
	}

	// Read remaining bytes as message
	remaining := buf.Len()
	m.Message = make([]byte, remaining)
	if _, err := io.ReadFull(buf, m.Message); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (m *EntityMove) Decode(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &m.EntityID); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.Position); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &m.Rotation); err != nil {
		return err
	}

	return nil
}

func (m *Ping) Decode(data []byte) error {
	return nil
}

func (m *Pong) Decode(data []byte) error {
	return nil
}
