package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	MessageOpen        = "0"
	MessageClose       = "1"
	MessagePing        = "2"
	MessagePingProbe   = "2probe"
	MessagePongProbe   = "3probe"
	MessagePong        = "3"
	messageMSG         = "4"
	MessageEmpty       = "40"
	messageCloseClient = "41"
	messageCommon      = "42"
	messageACK         = "43"
	MessageUpgrade     = "5"
	MessageBlank       = "6"
	MessageStub        = "stub"
)

var (
	ErrorWrongMessageType = errors.New("wrong message type")
	ErrorWrongPacket      = errors.New("wrong packet")
)

func typeToText(messageType int) (string, error) {
	m := map[int]string{
		MessageTypeOpen:        MessageOpen,
		MessageTypeClose:       MessageClose,
		MessageTypePing:        MessagePing,
		MessageTypePong:        MessagePong,
		MessageTypeEmpty:       MessageEmpty,
		MessageTypeEmit:        messageCommon,
		MessageTypeAckRequest:  messageCommon,
		MessageTypeAckResponse: messageACK,
	}
	msg, exists := m[messageType]
	if !exists {
		return "", ErrorWrongMessageType
	}
	return msg, nil
}

// Encode a socket.io message to the protocol format
func Encode(message *Message) (string, error) {
	result, err := typeToText(message.Type)
	if err != nil {
		return "", err
	}

	switch message.Type {
	case MessageTypeEmpty, MessageTypePing, MessageTypePong:
		return result, nil
	case MessageTypeAckRequest:
		result += strconv.Itoa(message.AckId)
	case MessageTypeAckResponse:
		result += strconv.Itoa(message.AckId)
		return result + "[" + message.Args + "]", nil
	case MessageTypeOpen, MessageTypeClose:
		return result + message.Args, nil
	}

	jsonMethod, err := json.Marshal(&message.Event)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s[%s,%s]`, result, string(jsonMethod), message.Args), nil
}

// MustEncode the message acts like Encode but panics on error
func MustEncode(message *Message) string {
	result, err := Encode(message)
	if err != nil {
		panic(err)
	}
	return result
}

func getMessageType(data string) (int, error) {
	if len(data) == 0 {
		return 0, ErrorWrongMessageType
	}
	switch data[0:1] {
	case MessageOpen:
		return MessageTypeOpen, nil
	case MessageClose:
		return MessageTypeClose, nil
	case MessagePing:
		return MessageTypePing, nil
	case MessagePong:
		return MessageTypePong, nil
	case MessageUpgrade:
		return MessageTypeUpgrade, nil
	case MessageBlank:
		return MessageTypeBlank, nil
	case messageMSG:
		if len(data) == 1 {
			return 0, ErrorWrongMessageType
		}
		switch data[0:2] {
		case MessageEmpty:
			return MessageTypeEmpty, nil
		case messageCloseClient:
			return MessageTypeClose, nil
		case messageCommon:
			return MessageTypeAckRequest, nil
		case messageACK:
			return MessageTypeAckResponse, nil
		}
	}
	return 0, ErrorWrongMessageType
}

// getAck extracts an id of the current packet if present
func getAck(text string) (ackId int, restText string, err error) {
	if len(text) < 4 {
		return 0, "", ErrorWrongPacket
	}
	text = text[2:]

	pos := strings.IndexByte(text, '[')
	if pos == -1 {
		return 0, "", ErrorWrongPacket
	}

	ack, err := strconv.Atoi(text[0:pos])
	if err != nil {
		return 0, "", err
	}

	return ack, text[pos:], nil
}

// getMethod extracts a message event name of the current packet if present
func getMethod(text string) (event, restText string, err error) {
	var start, end, rest, countQuote int

	for i, c := range text {
		if c == '"' {
			switch countQuote {
			case 0:
				start = i + 1
			case 1:
				end, rest = i, i+1
			default:
				return "", "", ErrorWrongPacket
			}
			countQuote++
		}
		if c == ',' {
			if countQuote < 2 {
				continue
			}
			rest = i + 1
			break
		}
	}

	if (end < start) || (rest >= len(text)) {
		return "", "", ErrorWrongPacket
	}

	return text[start:end], text[rest : len(text)-1], nil
}

// Decode the given data string into a Message
func Decode(data string) (*Message, error) {
	var err error
	msg := &Message{Source: data}

	msg.Type, err = getMessageType(data)
	if err != nil {
		return nil, err
	}

	switch msg.Type {
	case MessageTypeUpgrade, MessageTypeClose, MessageTypePing, MessageTypePong, MessageTypeEmpty, MessageTypeBlank:
		return msg, nil
	case MessageTypeOpen:
		msg.Args = data[1:]
		return msg, nil
	}

	ack, rest, err := getAck(data)
	msg.AckId = ack
	if msg.Type == MessageTypeAckResponse {
		if err != nil {
			return nil, err
		}
		msg.Args = rest[1 : len(rest)-1]
		return msg, nil
	}

	if err != nil {
		msg.Type = MessageTypeEmit
		rest = data[2:]
	}

	msg.Event, msg.Args, err = getMethod(rest)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
