package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	msg "github.com/PatrickELee/sockets/internal/messages"
	"github.com/PatrickELee/sockets/internal/utils"
)

const (
	SERVER_TYPE = "udp"
)

type (
	errMsg error
)

type model struct {
	viewport    viewport.Model
	messages    []msg.Message
	client      *Client
	textarea    textarea.Model
	senderStyle lipgloss.Style
	login       textinput.Model
	err         error
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(0, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and Enter to send it.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	client, err := CreateClient()
	if err != nil {
		panic(err)
	}

	ti := textinput.New()
	ti.Placeholder = "Please enter a nickname"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	return model{
		textarea:    ta,
		messages:    []msg.Message{},
		client:      client,
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
		login:       ti,
	}
}

func (c *Client) listenForMessage() tea.Cmd {
	return func() tea.Msg {
		for {
			n, _, err := c.Connection.ReadFromUDP(c.buffer[0:])
			if err != nil {
				fmt.Println(err)
			}
			c.IncomingQueue <- c.buffer[0:n]
		}

	}
}

func (c *Client) ReadMessage() tea.Cmd {
	return func() tea.Msg {
		message := msg.ParseStringToMessage(string(<-c.IncomingQueue))
		return msg.Message(message)
	}
}

func (c *Client) SendMessageTea() tea.Cmd {
	return func() tea.Msg {
		for {
			content := <-c.OutgoingQueue
			message := msg.NewContentMessage(c.ClientID.String(), c.Author, content)
			c.sendMessage(message)
		}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.client.listenForMessage(),
		m.client.ReadMessage(),
		m.client.SendMessageTea(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.String()
		if k == "esc" || k == "ctrl+c" {
			m.client.leaveServer()
			return m, tea.Quit
		}
	}

	if m.client.Author == "" {
		return updateLogin(msg, m)
	}
	return updateChat(msg, m)
}

func updateLogin(message tea.Msg, m model) (tea.Model, tea.Cmd) {
	m.login, _ = m.login.Update(message)

	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.Type {
		case tea.KeyEnter:
			m.client.Author = m.login.Value()
			message := msg.NewFunctionalMessage(m.client.ClientID.String(), m.client.Author, "connect_me")
			m.client.sendMessage(message)
			return m, nil
		}

	case errMsg:
		m.err = message
		return m, nil
	}
	return m, nil
}

func updateChat(message tea.Msg, m model) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(message)
	m.viewport, vpCmd = m.viewport.Update(message)

	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.textarea.Value() == "" {
				return m, nil
			}
			m.client.OutgoingQueue <- m.textarea.Value()
			message := msg.NewContentMessage(m.client.ClientID.String(), m.client.Author, m.textarea.Value())

			m.messages = append(m.messages, message)
			to_string := ""
			for _, loop := range m.messages {
				cur_line := m.senderStyle.Render(loop.Author+": ") + loop.Content
				if len(cur_line) >= 80 {
					spaced_lines := utils.Chunks(cur_line, 79)
					to_string += strings.Join(spaced_lines, "\n") + "\n"
				} else {
					to_string += m.senderStyle.Render(loop.Author+": ") + loop.Content + "\n"
				}
			}
			m.viewport.SetContent(to_string)
			m.textarea.Reset()
			m.viewport.GotoBottom()
		}
	case msg.Message:
		m.messages = append(m.messages, message)
		to_string := ""
		for _, message := range m.messages {
			to_string += m.senderStyle.Render(message.Author+": ") + message.Content + "\n"
		}
		m.viewport.SetContent(to_string)
		m.textarea.Reset()
		m.viewport.GotoBottom()
		return m, m.client.ReadMessage()

	case errMsg:
		m.err = message
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m model) View() string {
	var s string
	if m.client.Author != "" {
		s = chatView(m)
	} else {
		s = loginView(m)
	}
	return s
}

func chatView(m model) string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n"
}

func loginView(m model) string {
	return fmt.Sprintf(
		"What's your name?\n\n%s\n\n%s",
		m.login.View(),
		"(esc to quit)",
	) + "\n"
}

type Client struct {
	Connection    *net.UDPConn
	OutgoingQueue chan string
	IncomingQueue chan []byte
	IsOpen        bool
	buffer        []byte
	ClientID      uuid.UUID
	Author        string
}

func (c *Client) leaveServer() {
	message := msg.NewFunctionalMessage(c.ClientID.String(), c.Author, "quit")
	c.sendMessage(message)
}

func (c *Client) sendMessage(message msg.Message) {
	bytes := []byte(msg.ParseMessageToString(message))

	_, err := c.Connection.Write(bytes)
	if err != nil {
		panic(err)
	}

}

func CreateClient() (*Client, error) {
	godotenv.Load()
	serverHost, ok := os.LookupEnv("SERVER_HOST")
	if !ok {
		fmt.Println("No server host has been set, please include SERVER_HOST in your .env file.")
		os.Exit(1)
	}
	serverPort, ok := os.LookupEnv("SERVER_PORT")
	if !ok {
		fmt.Println("No server port has been set, please include SERVER_PORT in your .env file.")
	}

	udpAddr, err := net.ResolveUDPAddr(SERVER_TYPE, serverHost+":"+serverPort)
	if err != nil {
		fmt.Println("Error has occured", err.Error())
		return nil, err
	}
	connection, err := net.DialUDP(SERVER_TYPE, nil, udpAddr)
	if err != nil {
		panic(err)
	}
	client := Client{
		Connection:    connection,
		OutgoingQueue: make(chan string),
		IncomingQueue: make(chan []byte),
		IsOpen:        true,
		buffer:        make([]byte, 1024),
		ClientID:      uuid.New(),
		Author:        "",
	}

	return &client, nil
}

func main() {
	p := tea.NewProgram(initialModel())

	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}
