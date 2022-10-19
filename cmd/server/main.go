// socket-server project main.go
package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	msg "github.com/PatrickELee/sockets/internal/messages"
	"github.com/PatrickELee/sockets/internal/utils"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

const (
	SERVER_TYPE = "udp"
)

type Server struct {
	Server           *net.UDPConn
	IncomingMessages chan msg.Message
	Clients          map[string]*Client
	buffer           []byte
}

type Client struct {
	Address *net.UDPAddr
	Author  string
}

func newClient(addr *net.UDPAddr, author string) *Client {
	client := Client{
		Address: addr,
		Author:  author,
	}
	return &client
}

func createServer(serverHost, serverPort string) *Server {
	udpAddr, err := net.ResolveUDPAddr(SERVER_TYPE, serverHost+":"+serverPort)
	if err != nil {
		fmt.Println("Error has occured", err.Error())
		return nil
	}

	server, err := net.ListenUDP(SERVER_TYPE, udpAddr)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	chatServer := Server{
		Server:           server,
		IncomingMessages: make(chan msg.Message),
		Clients:          make(map[string]*Client),
		buffer:           make([]byte, 1024),
	}

	return &chatServer
}

func (server *Server) broadcast(message msg.Message) {
	for clientID, client := range server.Clients {
		if clientID == message.UserID {
			continue
		} else {
			outgoingMessage := msg.ParseMessageToString(message)
			_, err := server.Server.WriteToUDP([]byte(outgoingMessage), client.Address)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
			}
			if err != nil {
				panic(err)
			}
		}
	}
}

type model struct {
	width         int
	height        int
	userbox       viewport.Model
	terminal      viewport.Model
	focus         int
	senderStyle   lipgloss.Style
	server        *Server
	terminalLines []string
}

func newModel() model {
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

	userbox := viewport.New(30, 20)
	userbox.SetContent(`No users connected`)

	terminal := viewport.New(80, 20)
	terminal.SetContent(fmt.Sprintf("Server Running...\nListening on %s:%s\nWaiting for client...", serverHost, serverPort))

	server := createServer(serverHost, serverPort)

	m := model{
		server:        server,
		terminal:      terminal,
		userbox:       userbox,
		terminalLines: []string{},
		senderStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
	}

	return m
}

func (s *Server) listenForMessage() tea.Cmd {
	return func() tea.Msg {
		for {
			n, addr, err := s.Server.ReadFromUDP(s.buffer)
			if err != nil {
				fmt.Printf("Error reading package")
			}
			message := msg.ParseStringToMessage(string(s.buffer[0:n]))
			if _, ok := s.Clients[message.UserID]; !ok {
				s.Clients[message.UserID] = newClient(addr, message.Author)
			}
			s.IncomingMessages <- message
		}
	}
}

func (s *Server) ReadMessage() tea.Cmd {
	return func() tea.Msg {
		message := <-s.IncomingMessages
		return msg.Message(message)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.server.listenForMessage(),
		m.server.ReadMessage(),
	)
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var (
		userboxCmd  tea.Cmd
		terminalCmd tea.Cmd
	)
	m.userbox, userboxCmd = m.userbox.Update(message)
	m.terminal, terminalCmd = m.terminal.Update(message)

	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.height = message.Height
		m.width = message.Width
	case msg.Message:
		if message.Type == msg.Functional {
			switch message.Content {
			case "connect_me":
				notif := fmt.Sprintf("A new user has connected: %s", message.Author)
				m.terminalLines = append(m.terminalLines, notif)
				to_string := strings.Join(m.terminalLines, "\n")
				m.terminal.SetContent(to_string)
				m.terminal.GotoBottom()

				to_string = ""
				for _, v := range m.server.Clients {
					to_string += fmt.Sprintf("%s\n", v.Author)
				}
				m.userbox.SetContent(to_string)
				m.userbox.GotoBottom()

			case "quit":
				delete(m.server.Clients, message.UserID)
				notif := fmt.Sprintf("User %s has left the server.", message.Author)
				m.terminalLines = append(m.terminalLines, notif)
				to_string := strings.Join(m.terminalLines, "\n")
				m.terminal.SetContent(to_string)
				m.terminal.GotoBottom()

				to_string = ""

				if len(m.server.Clients) != 0 {
					for _, v := range m.server.Clients {
						to_string += fmt.Sprintf("%s\n", v.Author)
					}
				} else {
					to_string = "No users connected"
				}
				m.userbox.SetContent(to_string)
				m.userbox.GotoBottom()

			default:
				notif := fmt.Sprintf("Random Functional Command Found: %s", message.Content)
				m.terminalLines = append(m.terminalLines, notif)
				to_string := strings.Join(m.terminalLines, "\n")
				m.terminal.SetContent(to_string)
				m.terminal.GotoBottom()
			}

		} else {
			go m.server.broadcast(message)
			notif := fmt.Sprintf("%s: %s", m.senderStyle.Render(message.Author), message.Content)

			if len(notif) >= 84 {
				spaced_lines := utils.Chunks(notif, 83)
				m.terminalLines = append(m.terminalLines, spaced_lines...)
			} else {
				m.terminalLines = append(m.terminalLines, notif)
			}

			to_string := strings.Join(m.terminalLines, "\n")
			m.terminal.SetContent(to_string)
			m.terminal.GotoBottom()
		}

		return m, m.server.ReadMessage()
	}
	return m, tea.Batch(userboxCmd, terminalCmd)
}

var (
	terminalStyle = lipgloss.NewStyle().
			Width(80).
			Height(20).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("69")).
			Align(lipgloss.Top, lipgloss.Top).
			PaddingLeft(2)

	userboxStyle = lipgloss.NewStyle().
			Width(30).
			Height(20).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("69")).
			Align(lipgloss.Center, lipgloss.Top)
)

func (m model) View() string {
	views := []string{userboxStyle.Render(m.userbox.View()), terminalStyle.Render(m.terminal.View())}

	return lipgloss.JoinHorizontal(lipgloss.Top, views...) + "\n\n"
}

func main() {
	p := tea.NewProgram(newModel())
	if err := p.Start(); err != nil {
		fmt.Println("Error while running program:", err)
		os.Exit(1)
	}
}
