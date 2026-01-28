package network

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// MessageType identifies the type of network message
type MessageType uint8

const (
	// Handshake messages
	MsgHello MessageType = iota + 1
	MsgHelloAck
	MsgPairRequest
	MsgPairResponse

	// Sync messages
	MsgFileList
	MsgFileRequest
	MsgFileData
	MsgFileDelete
	MsgSyncComplete

	// Control messages
	MsgPing
	MsgPong
	MsgError
)

// Message is the base network message
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   []byte      `json:"payload"`
}

// HelloMessage is sent when connecting to a peer
type HelloMessage struct {
	DeviceName string `json:"device_name"`
	DeviceID   string `json:"device_id"`
	Version    string `json:"version"`
}

// HelloAckMessage acknowledges a hello
type HelloAckMessage struct {
	DeviceName string `json:"device_name"`
	DeviceID   string `json:"device_id"`
	Accepted   bool   `json:"accepted"`
	Reason     string `json:"reason,omitempty"`
}

// PairRequestMessage requests pairing with a peer
type PairRequestMessage struct {
	DeviceName string `json:"device_name"`
	DeviceID   string `json:"device_id"`
	PublicKey  []byte `json:"public_key,omitempty"`
}

// PairResponseMessage responds to a pairing request
type PairResponseMessage struct {
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason,omitempty"`
	PublicKey []byte `json:"public_key,omitempty"`
}

// FileInfo describes a file for sync
type FileInfo struct {
	RelPath    string      `json:"rel_path"`
	Size       int64       `json:"size"`
	ModTime    time.Time   `json:"mod_time"`
	Hash       string      `json:"hash"`
	IsDir      bool        `json:"is_dir"`
	Permission uint32      `json:"permission"`
	FolderPath string      `json:"folder_path"` // Base folder being synced
}

// FileListMessage contains a list of files
type FileListMessage struct {
	FolderPath string     `json:"folder_path"`
	FolderName string     `json:"folder_name"` // Base folder name (e.g., "Desktop", "Documents")
	Files      []FileInfo `json:"files"`
}

// FileRequestMessage requests a specific file
type FileRequestMessage struct {
	FolderPath string `json:"folder_path"`
	FolderName string `json:"folder_name"`
	RelPath    string `json:"rel_path"`
}

// FileDataMessage contains file content
type FileDataMessage struct {
	FolderPath string    `json:"folder_path"`
	FolderName string    `json:"folder_name"` // Base folder name for mapping on receiver
	RelPath    string    `json:"rel_path"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	Permission uint32    `json:"permission"`
	Hash       string    `json:"hash"`
	Data       []byte    `json:"data"`
	IsChunked  bool      `json:"is_chunked"`
	ChunkIndex int       `json:"chunk_index"`
	TotalChunks int      `json:"total_chunks"`
}

// FileDeleteMessage notifies about a deleted file
type FileDeleteMessage struct {
	FolderPath string `json:"folder_path"`
	FolderName string `json:"folder_name"`
	RelPath    string `json:"rel_path"`
}

// ErrorMessage contains an error
type ErrorMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Protocol constants
const (
	ProtocolVersion = "1.0"
	MaxMessageSize  = 64 * 1024 * 1024 // 64MB max message size
	ChunkSize       = 1 * 1024 * 1024  // 1MB chunks for large files
)

// WriteMessage writes a message to a writer
func WriteMessage(w io.Writer, msg *Message) error {
	// Serialize the message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write length prefix (4 bytes, big endian)
	length := uint32(len(data))
	if length > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes", length)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, length)
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write message data
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// ReadMessage reads a message from a reader
func ReadMessage(r io.Reader) (*Message, error) {
	// Read length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	// Read message data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	// Deserialize
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// NewMessage creates a new message with the given type and payload
func NewMessage(msgType MessageType, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Payload:   data,
	}, nil
}

// DecodePayload decodes the message payload into the given struct
func (m *Message) DecodePayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// String returns a string representation of the message type
func (t MessageType) String() string {
	switch t {
	case MsgHello:
		return "Hello"
	case MsgHelloAck:
		return "HelloAck"
	case MsgPairRequest:
		return "PairRequest"
	case MsgPairResponse:
		return "PairResponse"
	case MsgFileList:
		return "FileList"
	case MsgFileRequest:
		return "FileRequest"
	case MsgFileData:
		return "FileData"
	case MsgFileDelete:
		return "FileDelete"
	case MsgSyncComplete:
		return "SyncComplete"
	case MsgPing:
		return "Ping"
	case MsgPong:
		return "Pong"
	case MsgError:
		return "Error"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}
