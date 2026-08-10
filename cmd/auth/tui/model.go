package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dominicluechinger/esqlorer/internal/config"
)

type AddServerModel struct {
	step       int
	name       textinput.Model
	url        textinput.Model
	authMethod textinput.Model
	username   textinput.Model
	password   textinput.Model
	apiKey     textinput.Model
	err        error
	width      int
	height     int
	finished   bool
	server     *config.Server
}

const (
	stepName = iota
	stepURL
	stepAuthMethod
	stepUsername
	stepPassword
	stepAPIKey
	stepConfirm
)

func NewAddServer() *AddServerModel {
	nameInput := textinput.New()
	nameInput.Placeholder = "my-server"
	nameInput.Focus()
	nameInput.Prompt = "Server name: "

	urlInput := textinput.New()
	urlInput.Placeholder = "https://localhost:9200"
	urlInput.Prompt = "Server URL: "

	authMethodInput := textinput.New()
	authMethodInput.Placeholder = "basic (press Enter)"
	authMethodInput.Prompt = "Auth method (basic/api-key): "

	usernameInput := textinput.New()
	usernameInput.Placeholder = "elastic"
	usernameInput.Prompt = "Username: "

	passwordInput := textinput.New()
	passwordInput.Placeholder = "password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.Prompt = "Password: "

	apiKeyInput := textinput.New()
	apiKeyInput.Placeholder = "api-key"
	apiKeyInput.EchoMode = textinput.EchoPassword
	apiKeyInput.Prompt = "API Key: "

	return &AddServerModel{
		step:       stepName,
		name:       nameInput,
		url:        urlInput,
		authMethod: authMethodInput,
		username:   usernameInput,
		password:   passwordInput,
		apiKey:     apiKeyInput,
	}
}

func (m *AddServerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *AddServerModel) GetAuthMethod() string {
	val := m.authMethod.Value()
	if val == "" {
		return "basic"
	}
	return val
}

func (m *AddServerModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("green")).
		Bold(true).
		Render("Add Server Configuration")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("blue")).
		Render("Enter: next  Tab: switch  Ctrl+C: quit")

	s := title + "\n\n"

	switch m.step {
	case stepName:
		s += m.name.View()
	case stepURL:
		s += m.url.View()
	case stepAuthMethod:
		authMethod := m.authMethod.Value()
		if authMethod == "" {
			authMethod = "basic"
		}
		s += lipgloss.NewStyle().Render("Auth method: ") +
			lipgloss.NewStyle().Bold(true).Render(authMethod) + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("cyan")).
			Render("[Enter] basic  [Tab] api-key\n")
	case stepUsername:
		s += m.username.View()
	case stepPassword:
		s += m.password.View()
	case stepAPIKey:
		s += m.apiKey.View()
	case stepConfirm:
		authMethod := m.GetAuthMethod()
		summary := fmt.Sprintf("Name: %s\nURL: %s\nAuth: %s",
			m.name.Value(), m.url.Value(), authMethod)
		if authMethod == "basic" {
			summary += fmt.Sprintf("\nUsername: %s", m.username.Value())
		} else {
			summary += "\nAPI Key: ••••••••"
		}
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("cyan")).Render(summary) + "\n\n"
		s += lipgloss.NewStyle().Bold(true).Render("[Enter] Save  [Tab] Back  [q] Cancel")
	}

	if m.err != nil {
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("red")).Render("Error: "+m.err.Error())
	}

	s += "\n\n" + help

	return s
}

func (m *AddServerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *AddServerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}
	return m, nil
}

func (m *AddServerModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "enter":
		switch m.step {
		case stepName:
			if m.name.Value() == "" {
				return m, nil
			}
			m.step = stepURL
			m.name.Blur()
			m.url.Focus()
		case stepURL:
			if m.url.Value() == "" {
				return m, nil
			}
			m.step = stepAuthMethod
			m.url.Blur()
		case stepAuthMethod:
			m.step = stepUsername
			if m.GetAuthMethod() == "basic" {
				m.username.Focus()
			} else {
				m.step = stepAPIKey
				m.apiKey.Focus()
			}
		case stepUsername:
			m.step = stepPassword
			m.username.Blur()
			m.password.Focus()
		case stepPassword:
			m.step = stepConfirm
			m.password.Blur()
		case stepAPIKey:
			m.step = stepConfirm
			m.apiKey.Blur()
		case stepConfirm:
			m.saveServer()
			if m.err != nil {
				return m, nil
			}
			m.finished = true
			return m, tea.Quit
		}

	case "tab":
		switch m.step {
		case stepName:
			m.step = stepURL
			m.name.Blur()
			m.url.Focus()
		case stepURL:
			m.step = stepName
			m.url.Blur()
			m.name.Focus()
		case stepAuthMethod:
			m.authMethod.SetValue("api-key")
			m.step = stepAPIKey
			m.apiKey.Focus()
		case stepUsername:
			m.step = stepPassword
			m.username.Blur()
			m.password.Focus()
		case stepPassword:
			m.step = stepUsername
			m.password.Blur()
			m.username.Focus()
		case stepAPIKey:
			m.step = stepConfirm
		case stepConfirm:
			m.step = stepAuthMethod
		}

	case "shift+tab":
		switch m.step {
		case stepName:
			m.step = stepURL
			m.name.Blur()
			m.url.Focus()
		case stepURL:
			m.step = stepName
			m.url.Blur()
			m.name.Focus()
		case stepAuthMethod:
			m.authMethod.SetValue("basic")
			m.step = stepUsername
			m.username.Focus()
		case stepUsername:
			m.step = stepPassword
			m.username.Blur()
			m.password.Focus()
		case stepPassword:
			m.step = stepUsername
			m.password.Blur()
			m.username.Focus()
		case stepAPIKey:
			m.step = stepName
			m.apiKey.Blur()
		case stepConfirm:
			authMethod := m.GetAuthMethod()
			if authMethod == "basic" {
				m.step = stepPassword
				m.password.Focus()
			} else {
				m.step = stepAPIKey
				m.apiKey.Focus()
			}
		}

	default:
		var cmd tea.Cmd
		switch m.step {
		case stepName:
			m.name, cmd = m.name.Update(msg)
		case stepURL:
			m.url, cmd = m.url.Update(msg)
		case stepAuthMethod:
			m.authMethod, cmd = m.authMethod.Update(msg)
		case stepUsername:
			m.username, cmd = m.username.Update(msg)
		case stepPassword:
			m.password, cmd = m.password.Update(msg)
		case stepAPIKey:
			m.apiKey, cmd = m.apiKey.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m *AddServerModel) saveServer() {
	server := config.Server{
		Name: m.name.Value(),
		URL:  m.url.Value(),
	}

	if m.GetAuthMethod() == "basic" {
		server.Username = m.username.Value()
		server.Password = m.password.Value()
	} else {
		server.APIKey = m.apiKey.Value()
	}

	m.server = &server
}

func (m *AddServerModel) Server() *config.Server {
	return m.server
}

func (m *AddServerModel) Run() error {
	if m.finished {
		return nil
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		if m.server != nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return err
	}
	return nil
}
