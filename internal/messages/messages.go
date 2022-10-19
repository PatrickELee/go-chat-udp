package messages

import (
	"strconv"
	"strings"
)

type Message struct {
	UserID  string
	Author  string
	Content string
	Type    MessageType
}

type MessageType int

const (
	Functional = iota
	Content
)

func ParseMessageToString(message Message) string {
	id, author, content := message.UserID, message.Author, message.Content
	messageType := strconv.Itoa(int(message.Type))
	return strings.Join([]string{id, author, messageType, content}, "\x01")
}

func ParseStringToMessage(content string) Message {
	stringArray := strings.Split(content, "\x01")

	userID := stringArray[0]
	author := stringArray[1]
	messageContent := stringArray[3]

	messageType, _ := strconv.Atoi(stringArray[2])

	message := newMessage(userID, author, messageContent, MessageType(messageType))

	return message
}

func NewFunctionalMessage(id, author, content string) Message {
	return newMessage(id, author, content, Functional)
}

func NewContentMessage(id, author, content string) Message {
	return newMessage(id, author, content, Content)
}

func newMessage(id, author, content string, messageType MessageType) Message {
	message := Message{
		UserID:  id,
		Author:  author,
		Content: content,
		Type:    messageType,
	}
	return message
}
