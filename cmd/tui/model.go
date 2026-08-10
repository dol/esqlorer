package tui

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"gopkg.in/yaml.v3"

	"github.com/dominicluechinger/esqlorer/internal/config"
	"github.com/dominicluechinger/esqlorer/internal/paths"
	"github.com/dominicluechinger/esqlorer/pkg/elastic"
)

type Model struct {
	config          *config.Config
	server          *config.Server
	client          *elastic.Client
	input           textinput.Model
	table           Table
	detail          detailState
	history         *queryHistoryManager
	results         *elastic.QueryResult
	query           string
	executedQuery   string
	filters         []queryFilter
	timePickerOpen  bool
	timeRangeIndex  int
	exportFormatIdx int
	dropNullColumns bool
	hideNullValues  bool
	quitArmed       bool
	quitStage       int
	focus           Focus
	err             error
	status          string
	loading         bool
	spinner         Spinner
	width           int
	height          int
}

type Focus int

const (
	FocusInput Focus = iota
	FocusTable
	FocusDetail
)

type queryFilter struct {
	Key     string
	Value   string
	Include bool
}

type timeRangePreset struct {
	Label     string
	Condition string
}

type fieldEntry struct {
	Key   string
	Value string
}

type detailState struct {
	filter        textinput.Model
	filterFocused bool
	filterPinned  bool
	selected      int
	scroll        int
}

type queryDoneMsg struct {
	query   string
	results *elastic.QueryResult
	err     error
}

type spinnerTickMsg struct{}

type quitArmTimeoutMsg struct{}

const quitConfirmStatus = "Press Ctrl+C again within 3s to exit"

var timeRangePresets = []timeRangePreset{
	{Label: "all time", Condition: ""},
	{Label: "last 15 minutes", Condition: "@timestamp >= NOW() - 15m"},
	{Label: "last 1 hour", Condition: "@timestamp >= NOW() - 1h"},
	{Label: "last 2 hours", Condition: "@timestamp >= NOW() - 2h"},
	{Label: "last 24 hours", Condition: "@timestamp >= NOW() - 24h"},
}

var exportFormats = []string{"table", "csv", "json", "yaml"}

func New() (*Model, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	history, err := loadQueryHistory(paths.QueryHistoryPath())
	if err != nil {
		return nil, err
	}

	var server *config.Server
	if cfg.CurrentContext != "" {
		server = cfg.GetCurrentServer()
	}

	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "FROM logs-*"
	input.Focus()

	filter := textinput.New()
	filter.Prompt = "Filter: "
	filter.Placeholder = "fuzzy search keys or values (f/Tab)"
	filter.Blur()

	return &Model{
		config:          cfg,
		server:          server,
		input:           input,
		table:           NewTable(),
		detail:          detailState{filter: filter},
		history:         history,
		timeRangeIndex:  3,
		exportFormatIdx: 0,
		dropNullColumns: true,
		hideNullValues:  true,
		focus:           FocusInput,
		spinner:         NewSpinner(),
	}, nil
}

func (m *Model) Connect() error {
	if m.server == nil {
		return fmt.Errorf("no server configured")
	}

	client, err := elastic.NewClient(*m.server)
	if err != nil {
		return err
	}

	m.client = client
	return nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.syncMouseMode())
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	if width > 12 {
		m.input.Width = width - 12
		m.detail.filter.Width = width - 18
	} else {
		m.input.Width = 0
		m.detail.filter.Width = 0
	}
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.timePickerOpen {
		return m.renderTimePicker()
	}

	top := m.renderTopPane()
	remaining := m.height - lipgloss.Height(top) - 1
	if remaining < 8 {
		remaining = 8
	}

	var body string
	if m.focus == FocusDetail {
		body = m.renderDetailPane(remaining)
	} else {
		m.table.SetSize(maxInt(20, m.width-4), remainingForPane(remaining))
		m.table.SetFocus(m.focus == FocusTable)
		body = m.renderResultsPane(remaining)
	}

	return top + "\n" + body
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case queryDoneMsg:
		m.loading = false
		m.spinner.Stop()
		if msg.err != nil {
			m.err = msg.err
			m.status = "Query failed"
			return m, nil
		}

		m.err = nil
		m.results = msg.results
		m.executedQuery = msg.query
		m.query = strings.TrimSpace(m.input.Value())
		m.table.SetColumns(msg.results.Columns)
		m.table.SetRows(msg.results.Values)
		m.table.SetSelected(0)
		m.detail.selected = 0
		m.status = fmt.Sprintf("Loaded %d rows", len(msg.results.Values))
		return m, nil

	case spinnerTickMsg:
		if m.loading {
			return m, spinnerTickCmd()
		}
		return m, nil

	case quitArmTimeoutMsg:
		if m.quitArmed {
			m.quitArmed = false
			if m.status == quitConfirmStatus {
				m.status = ""
			}
		}
		if m.quitStage != 0 {
			m.quitStage = 0
			if strings.Contains(m.status, "Ctrl+C") {
				m.status = ""
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.MouseMsg:
		if m.timePickerOpen {
			return m.handleTimePickerMouse(msg)
		}
		if m.focus == FocusDetail {
			return m.handleDetailMouse(msg)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+q" {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			return m.handleCtrlC()
		}
		if m.focus == FocusInput && m.history != nil && m.history.searchActive() {
			if msg.String() == "esc" {
				m.input.SetValue(m.history.cancelSearch())
				m.status = "History search cancelled"
				return m, nil
			}
		}
		if msg.String() == "ctrl+c" {
			return m.handleGlobalQuit()
		}
		if m.timePickerOpen {
			return m.handleTimePickerKey(msg)
		}
		if m.focus == FocusDetail {
			return m.handleDetailKey(msg)
		}
		return m.handleListKey(msg)
	}

	return m, nil
}

func (m *Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus == FocusInput && m.history != nil && m.history.searchActive() {
		return m.handleInputHistorySearchKey(msg)
	}

	switch msg.String() {
	case "q":
		m.focus = FocusInput
		m.table.SetFocus(false)
		m.input.Focus()
		m.detail.filterFocused = false
		m.detail.filter.Blur()
		return m, m.syncMouseMode()

	case "enter":
		if m.focus == FocusInput {
			return m, m.runQuery()
		}
		if m.focus == FocusTable && m.results != nil && len(m.results.Values) > 0 {
			m.resetDetailFilterForNewPreview()
			m.focus = FocusDetail
			m.detail.filterFocused = false
			m.detail.filter.Blur()
			m.hideNullValues = true
			m.detail.selected = 0
			return m, m.syncMouseMode()
		}

	case "tab":
		switch m.focus {
		case FocusInput:
			m.focus = FocusTable
			m.input.Blur()
			m.table.SetFocus(true)
		case FocusTable:
			m.focus = FocusInput
			m.table.SetFocus(false)
			m.input.Focus()
		}
		return m, m.syncMouseMode()

	case "n":
		if m.focus == FocusInput || m.focus == FocusTable {
			m.dropNullColumns = !m.dropNullColumns
			if m.dropNullColumns {
				m.status = "Null columns: hidden"
			} else {
				m.status = "Null columns: shown"
			}
			return m, nil
		}

	case "up", "ctrl+p":
		if m.focus == FocusInput && m.history != nil {
			m.input.SetValue(m.history.previous(m.input.Value()))
			m.input.CursorEnd()
			m.status = "Query history"
			return m, nil
		}
		if m.focus == FocusTable {
			m.table.Up()
			m.detail.selected = 0
		}
		return m, nil

	case "down", "ctrl+n":
		if m.focus == FocusInput && m.history != nil {
			m.input.SetValue(m.history.next(m.input.Value()))
			m.input.CursorEnd()
			m.status = "Query history"
			return m, nil
		}
		if m.focus == FocusTable {
			m.table.Down()
			m.detail.selected = 0
		}
		return m, nil

	case "ctrl+r":
		if m.focus == FocusInput && m.history != nil {
			m.history.resetNavigation()
			m.input.SetValue(m.history.beginSearch(m.input.Value()))
			m.input.CursorEnd()
			m.status = m.history.searchStatus()
			return m, nil
		}

	case "backspace":
		if m.focus == FocusTable {
			m.focus = FocusInput
			m.table.SetFocus(false)
			m.input.Focus()
			return m, m.syncMouseMode()
		}

	case "alt+t":
		m.openTimePicker()
		m.err = nil
		m.status = "Select a time range"
		return m, m.syncMouseMode()

	case "alt+e":
		m.cycleExportFormat()
		m.err = nil
		m.status = "Export format: " + strings.ToUpper(m.currentExportFormat())
		return m, nil

	case "ctrl+e":
		if err := m.exportCurrentTable(); err != nil {
			m.err = err
			m.status = "Export failed"
		}
		return m, nil
	}

	if m.focus == FocusInput {
		var cmd tea.Cmd
		if m.history != nil && m.history.browsing() {
			m.history.setDraft(m.input.Value())
		}
		m.input, cmd = m.input.Update(msg)
		if m.history != nil && !m.history.browsing() {
			m.history.resetNavigation()
		}
		return m, cmd
	}

	return m, nil
}

func (m *Model) handleInputHistorySearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+r", "up", "ctrl+p":
		m.input.SetValue(m.history.searchOlder())
		m.input.CursorEnd()
		return m, nil
	case "down", "ctrl+n":
		m.input.SetValue(m.history.searchNewer())
		m.input.CursorEnd()
		return m, nil
	case "enter":
		m.input.SetValue(m.history.acceptSearch())
		m.input.CursorEnd()
		m.status = "History search accepted"
		return m, nil
	case "esc":
		m.input.SetValue(m.history.cancelSearch())
		m.input.CursorEnd()
		m.status = "History search cancelled"
		return m, nil
	case "backspace":
		runes := []rune(m.history.searchTerm())
		if len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		m.input.SetValue(m.history.updateSearchTerm(string(runes)))
		m.input.CursorEnd()
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		term := m.history.searchTerm() + string(msg.Runes)
		m.input.SetValue(m.history.updateSearchTerm(term))
		m.input.CursorEnd()
		return m, nil
	}

	return m, nil
}

func (m *Model) handleGlobalQuit() (tea.Model, tea.Cmd) {
	if m.quitArmed {
		return m, tea.Quit
	}

	m.quitArmed = true
	m.err = nil
	m.status = quitConfirmStatus
	return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return quitArmTimeoutMsg{}
	})
}

func (m *Model) handleCtrlC() (tea.Model, tea.Cmd) {
	if m.focus != FocusInput {
		return m.handleGlobalQuit()
	}

	if strings.TrimSpace(m.input.Value()) != "" {
		m.input.SetValue("")
		if m.history != nil {
			m.history.resetNavigation()
		}
		m.quitArmed = false
		m.quitStage = 0
		m.status = "Query cleared"
		return m, nil
	}

	m.quitArmed = false
	switch m.quitStage {
	case 0:
		m.quitStage = 1
		m.err = nil
		m.status = "Press Ctrl+C two more times to exit"
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return quitArmTimeoutMsg{}
		})
	case 1:
		m.quitStage = 2
		m.err = nil
		m.status = "Press Ctrl+C one more time to exit"
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return quitArmTimeoutMsg{}
		})
	default:
		return m, tea.Quit
	}
}

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detail.filterFocused {
		switch msg.String() {
		case "esc":
			m.detail.filterFocused = false
			m.detail.filter.Blur()
			return m, nil
		case "tab":
			m.detail.filterFocused = false
			m.detail.filter.Blur()
			return m, nil
		case "enter":
			m.detail.filterFocused = false
			m.detail.filter.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.detail.filter, cmd = m.detail.filter.Update(msg)
			m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc", "backspace":
		m.focus = FocusTable
		m.detail.filterFocused = false
		m.detail.filter.Blur()
		return m, m.syncMouseMode()

	case "q":
		m.focus = FocusInput
		m.detail.filterFocused = false
		m.detail.filter.Blur()
		m.table.SetFocus(false)
		m.input.Focus()
		return m, m.syncMouseMode()

	case "tab", "f":
		m.detail.filterFocused = true
		m.detail.filter.Focus()
		return m, nil

	case "up", "k", "ctrl+p":
		m.detail.selected--
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		return m, nil

	case "down", "j", "ctrl+n":
		m.detail.selected++
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		return m, nil

	case "pgup", "ctrl+pgup":
		m.detail.selected -= m.detailPageStep()
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		return m, nil

	case "pgdown", "ctrl+pgdown":
		m.detail.selected += m.detailPageStep()
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		return m, nil

	case "home":
		m.detail.selected = 0
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		return m, nil

	case "end":
		entries := m.filteredDetailEntries()
		if len(entries) > 0 {
			m.detail.selected = len(entries) - 1
		}
		m.clampDetailSelectionWithLen(len(entries))
		return m, nil

	case "c":
		if err := m.copySelectedField(fieldCopyPair); err != nil {
			m.err = err
			m.status = "Copy failed"
		}
		return m, nil

	case "C":
		if err := m.copySelectedField(fieldCopyFilter); err != nil {
			m.err = err
			m.status = "Copy failed"
		}
		return m, nil

	case "n", " ":
		m.hideNullValues = !m.hideNullValues
		m.clampDetailSelectionWithLen(len(m.filteredDetailEntries()))
		if m.hideNullValues {
			m.status = "Detail view: hiding null values"
		} else {
			m.status = "Detail view: showing null values"
		}
		return m, nil

	case "p":
		m.detail.filterPinned = !m.detail.filterPinned
		if m.detail.filterPinned {
			m.status = "Detail filter pinned"
		} else {
			m.status = "Detail filter unpinned"
		}
		return m, nil

	case "K":
		if err := m.copySelectedField(fieldCopyKey); err != nil {
			m.err = err
			m.status = "Copy failed"
		}
		return m, nil

	case "V":
		if err := m.copySelectedField(fieldCopyValue); err != nil {
			m.err = err
			m.status = "Copy failed"
		}
		return m, nil

	case "+":
		if err := m.addSelectedFilter(true); err != nil {
			m.err = err
			m.status = "Filter add failed"
		}
		return m, nil

	case "-":
		if err := m.addSelectedFilter(false); err != nil {
			m.err = err
			m.status = "Filter add failed"
		}
		return m, nil

	case "alt+t":
		m.openTimePicker()
		m.err = nil
		m.status = "Select a time range"
		return m, nil

	case "alt+e":
		m.cycleExportFormat()
		m.err = nil
		m.status = "Export format: " + strings.ToUpper(m.currentExportFormat())
		return m, nil

	case "ctrl+e":
		if err := m.exportCurrentTable(); err != nil {
			m.err = err
			m.status = "Export failed"
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) resetDetailFilterForNewPreview() {
	if m.detail.filterPinned {
		return
	}

	m.detail.filter.SetValue("")
	m.detail.filterFocused = false
	m.detail.scroll = 0
	m.detail.selected = 0
}

func (m *Model) handleDetailMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.detail.selected -= 3
		entries := m.filteredDetailEntries()
		m.clampDetailSelectionWithLen(len(entries))
		return m, nil
	case tea.MouseButtonWheelDown:
		m.detail.selected += 3
		entries := m.filteredDetailEntries()
		m.clampDetailSelectionWithLen(len(entries))
		return m, nil
	}

	return m, nil
}

func (m *Model) runQuery() tea.Cmd {
	if m.loading {
		return nil
	}

	query := strings.TrimSpace(m.input.Value())
	if query == "" {
		m.status = "Enter a query first"
		return nil
	}
	if m.history != nil {
		if err := m.history.remember(query); err != nil {
			m.err = err
			m.status = "History update failed"
			return nil
		}
	}

	effective := m.buildEffectiveQuery(query)
	m.query = query
	m.executedQuery = effective
	m.loading = true
	m.err = nil
	m.status = "Executing query..."
	m.spinner.Start()

	return tea.Batch(
		spinnerTickCmd(),
		m.queryCmd(effective, m.dropNullColumns),
	)
}

func (m *Model) queryCmd(query string, dropNullColumns bool) tea.Cmd {
	return func() tea.Msg {
		if err := m.ensureClient(); err != nil {
			return queryDoneMsg{query: query, err: err}
		}

		ctx := context.Background()
		results, err := m.client.ExecuteESQLWithOptions(ctx, elastic.QueryOptions{
			Query:           query,
			DropNullColumns: dropNullColumns,
		})
		return queryDoneMsg{query: query, results: results, err: err}
	}
}

func (m *Model) ensureClient() error {
	if m.client != nil {
		return nil
	}
	return m.Connect()
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m *Model) renderTopPane() string {
	header := topHeaderStyle.Render(m.renderHeader())
	queryLabel := titleLabelStyle.Render("Query")

	queryView := m.input.View()
	if m.focus == FocusInput {
		queryView = focusBorderStyle.Render(queryView)
	} else {
		queryView = blurBorderStyle.Render(queryView)
	}

	meta := []string{
		chipStyle.Render("range: " + m.currentTimeRange().Label),
		chipStyle.Render("export: " + strings.ToUpper(m.currentExportFormat())),
		chipStyle.Render(m.nullColumnChipLabel()),
	}
	if len(m.filters) > 0 {
		for _, filter := range m.filters {
			meta = append(meta, filterChipStyle.Render(filter.label()))
		}
	}

	status := m.renderStatus()
	help := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		queryLabel,
		queryView,
		lipgloss.JoinHorizontal(lipgloss.Top, meta...),
		status,
		help,
	)
}

func (m *Model) renderHeader() string {
	serverName := "No server"
	if m.server != nil {
		serverName = m.server.Name
	}

	base := fmt.Sprintf(" esqlorer | %s ", serverName)
	if m.loading {
		return base + " " + m.spinner.View() + " running"
	}
	return base
}

func (m *Model) renderStatus() string {
	if m.focus == FocusInput && m.history != nil && m.history.searchActive() {
		return statusStyle.Render(m.history.searchStatus())
	}
	if m.err != nil {
		return errorStyle.Render("Error: " + m.err.Error())
	}
	if m.status != "" {
		return statusStyle.Render(m.status)
	}
	if strings.TrimSpace(m.executedQuery) != "" {
		return statusStyle.Render("Last query: " + m.executedQuery)
	}
	if strings.TrimSpace(m.query) != "" {
		return statusStyle.Render("Ready to run: " + m.query)
	}
	return statusStyle.Render("Enter a query and press Enter")
}

func (m *Model) renderFooter() string {
	switch m.focus {
	case FocusDetail:
		return footerStyle.Render("Esc/Backspace: back  q: query  Tab/F: filter  p: pin filter  n/Space: toggle nulls  +: include filter  -: exclude filter  c/C/K/V: copy  Alt+T: time range  Alt+E: export format  Ctrl+E: export")
	case FocusTable:
		return footerStyle.Render("Enter: explore  q/Tab: query  ↑↓: navigate  n: toggle null columns  Alt+T: time range  Alt+E: export format  Ctrl+E: export  Ctrl+C: quit")
	default:
		if m.history != nil && m.history.searchActive() {
			return footerStyle.Render("Type: fuzzy search  ↑↓/Ctrl+R: cycle matches  Enter: accept  Esc/Ctrl+C: cancel")
		}
		return footerStyle.Render("Enter: search  Tab: results  n: toggle null columns  Alt+T: time range  Alt+E: export format  Ctrl+E: export  Ctrl+C: quit")
	}
}

func (m *Model) renderResultsPane(height int) string {
	title := sectionTitleStyle.Render("Results")
	tableBody := m.table.View()
	if m.results == nil {
		tableBody = mutedStyle.Render("No results. Press Enter to run the query.")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		panelStyle.Render(tableBody),
	)
}

func remainingForPane(height int) int {
	if height < 8 {
		return 8
	}
	return height
}

func (m *Model) renderPreviewBody(height int) string {
	entries := m.selectedRowEntries()
	if len(entries) == 0 {
		return mutedStyle.Render("Select a row to preview it here.")
	}

	lines := []string{
		mutedStyle.Render(fmt.Sprintf("Row %d of %d", m.table.Selected(), len(m.results.Values))),
	}

	limit := len(entries)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, previewKeyStyle.Render(entries[i].Key+":")+" "+previewValueStyle.Render(entries[i].Value))
	}
	if len(entries) > limit {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("... and %d more fields", len(entries)-limit)))
	}

	return strings.Join(lines, "\n")
}

func (m *Model) renderDetailPane(height int) string {
	title := sectionTitleStyle.Render("Preview")
	entries := m.filteredDetailEntries()

	filterView := m.detail.filter.View()
	if m.detail.filterFocused {
		filterView = focusBorderStyle.Render(filterView)
	} else {
		filterView = blurBorderStyle.Render(filterView)
	}

	body := m.renderEntryList(entries, height)
	panel := panelStyle
	if m.focus == FocusDetail && !m.detail.filterFocused {
		panel = focusBorderStyle
	}
	toggleStyle := detailToggleDisabledStyle
	if m.focus == FocusDetail {
		toggleStyle = detailToggleActiveStyle
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		toggleStyle.Render(m.nullToggleLabel()),
		detailToggleStyle.Render(m.filterPinLabel()),
		filterView,
		panel.Render(body),
	)
}

func (m *Model) renderEntryList(entries []fieldEntry, height int) string {
	if len(entries) == 0 {
		return mutedStyle.Render("No matching fields.")
	}

	visible := height - 8
	if visible < 4 {
		visible = 4
	}

	if m.detail.selected >= len(entries) {
		m.detail.selected = len(entries) - 1
	}
	if m.detail.selected < 0 {
		m.detail.selected = 0
	}

	maxStart := len(entries) - visible
	if maxStart < 0 {
		maxStart = 0
	}

	if m.detail.scroll < 0 {
		m.detail.scroll = 0
	}
	if m.detail.scroll > maxStart {
		m.detail.scroll = maxStart
	}
	if m.detail.selected < m.detail.scroll {
		m.detail.scroll = m.detail.selected
	}
	if m.detail.selected >= m.detail.scroll+visible {
		m.detail.scroll = m.detail.selected - visible + 1
	}
	if m.detail.scroll < 0 {
		m.detail.scroll = 0
	}
	if m.detail.scroll > maxStart {
		m.detail.scroll = maxStart
	}

	start := m.detail.scroll
	end := start + visible
	if end > len(entries) {
		end = len(entries)
	}

	lines := make([]string, 0, visible+2)
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(entries))))
	for i := start; i < end; i++ {
		line := entries[i].Key + " = " + entries[i].Value
		if i == m.detail.selected {
			lines = append(lines, selectedDetailStyle.Render("▸ "+line))
		} else {
			lines = append(lines, detailLineStyle.Render("  "+line))
		}
	}

	lines = append(lines, mutedStyle.Render("Mouse wheel/PgUp/PgDn: scroll  Home/End: jump  c: copy pair  C: copy key == \"value\" filter  K: copy key  V: copy value  +: include filter  -: exclude filter  p: pin filter"))
	return strings.Join(lines, "\n")
}

func (m *Model) selectedRowEntries() []fieldEntry {
	if m.results == nil || len(m.results.Values) == 0 {
		return nil
	}

	row, ok := m.table.SelectedRow()
	if !ok {
		return nil
	}

	entries := make([]fieldEntry, 0, len(m.results.Columns))
	for i, col := range m.results.Columns {
		if i >= len(row) {
			continue
		}
		if m.hideNullValues && isNullValue(row[i]) {
			continue
		}
		entries = append(entries, fieldEntry{
			Key:   col.Name,
			Value: formatValue(row[i]),
		})
	}
	return entries
}

func (m *Model) filteredDetailEntries() []fieldEntry {
	entries := m.selectedRowEntries()
	if len(entries) == 0 {
		return nil
	}

	filter := strings.TrimSpace(strings.ToLower(m.detail.filter.Value()))
	if filter == "" {
		m.clampDetailSelectionWithLen(len(entries))
		return entries
	}

	candidates := make([]string, len(entries))
	for i, entry := range entries {
		candidates[i] = strings.ToLower(entry.Key + " " + entry.Value)
	}
	matches := fuzzy.FindNoSort(filter, candidates)
	if len(matches) == 0 {
		m.clampDetailSelectionWithLen(0)
		return nil
	}

	filtered := make([]fieldEntry, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, entries[match.Index])
	}

	m.clampDetailSelectionWithLen(len(filtered))
	return filtered
}

func (m *Model) clampDetailSelectionWithLen(length int) {
	if length <= 0 {
		m.detail.selected = 0
		m.detail.scroll = 0
		return
	}
	if m.detail.selected < 0 {
		m.detail.selected = 0
	}
	if m.detail.selected >= length {
		m.detail.selected = length - 1
	}

	maxScroll := length - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.detail.scroll < 0 {
		m.detail.scroll = 0
	}
	if m.detail.scroll > maxScroll {
		m.detail.scroll = maxScroll
	}
	if m.detail.selected < m.detail.scroll {
		m.detail.scroll = m.detail.selected
	}
}

func (m *Model) detailPageStep() int {
	step := (m.height - 8) / 2
	if step < 4 {
		step = 4
	}
	return step
}

func (m *Model) currentTimeRange() timeRangePreset {
	if m.timeRangeIndex < 0 || m.timeRangeIndex >= len(timeRangePresets) {
		return timeRangePresets[0]
	}
	return timeRangePresets[m.timeRangeIndex]
}

func (m *Model) currentExportFormat() string {
	if m.exportFormatIdx < 0 || m.exportFormatIdx >= len(exportFormats) {
		return exportFormats[0]
	}
	return exportFormats[m.exportFormatIdx]
}

func (m *Model) cycleExportFormat() {
	m.exportFormatIdx = (m.exportFormatIdx + 1) % len(exportFormats)
}

func (m *Model) buildEffectiveQuery(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}

	clauses := make([]string, 0, len(m.filters)+1)
	if cond := m.currentTimeRange().Condition; cond != "" {
		clauses = append(clauses, cond)
	}
	for _, filter := range m.filters {
		clauses = append(clauses, filter.condition())
	}
	if len(clauses) == 0 {
		return base
	}

	clause := "WHERE " + strings.Join(clauses, " AND ")
	return insertClauseBeforeLimit(base, clause)
}

func (m *Model) openTimePicker() {
	m.timePickerOpen = true
	m.focus = FocusInput
	m.input.Blur()
	m.detail.filter.Blur()
	m.table.SetFocus(false)
}

func (m *Model) closeTimePicker() {
	m.timePickerOpen = false
}

func (m *Model) applyTimeRangeIndex(index int) {
	if index < 0 || index >= len(timeRangePresets) {
		return
	}
	m.timeRangeIndex = index
	m.err = nil
	m.status = "Time range: " + m.currentTimeRange().Label
}

func (m *Model) handleTimePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.closeTimePicker()
		m.status = "Time range unchanged"
		return m, nil
	case "up", "ctrl+p":
		if m.timeRangeIndex > 0 {
			m.timeRangeIndex--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.timeRangeIndex < len(timeRangePresets)-1 {
			m.timeRangeIndex++
		}
		return m, nil
	case "enter":
		m.applyTimeRangeIndex(m.timeRangeIndex)
		m.closeTimePicker()
		return m, nil
	}

	return m, nil
}

func (m *Model) handleTimePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	box := m.timePickerBox()
	if !pointInRect(msg.X, msg.Y, box.x, box.y, box.width, box.height) {
		m.closeTimePicker()
		return m, nil
	}

	contentY := msg.Y - box.y
	itemLineStart := 4
	itemLineEnd := itemLineStart + len(timeRangePresets) - 1
	if contentY >= itemLineStart && contentY <= itemLineEnd {
		index := contentY - itemLineStart
		m.applyTimeRangeIndex(index)
		m.closeTimePicker()
		return m, nil
	}

	if contentY >= 0 && contentY <= 2 {
		return m, nil
	}

	if contentY == box.height-2 {
		m.closeTimePicker()
		return m, nil
	}

	return m, nil
}

type pickerBox struct {
	x, y   int
	width  int
	height int
}

func (m *Model) timePickerBox() pickerBox {
	width := 54
	for _, preset := range timeRangePresets {
		line := preset.Label
		if preset.Condition != "" {
			line += "  " + preset.Condition
		}
		if w := len(line) + 6; w > width {
			width = w
		}
	}
	if m.width > 0 && width > m.width-8 {
		width = m.width - 8
	}
	if width < 34 {
		width = 34
	}

	height := len(timeRangePresets) + 6
	if height < 10 {
		height = 10
	}

	x := (m.width - width) / 2
	y := (m.height - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return pickerBox{x: x, y: y, width: width, height: height}
}

func (m *Model) renderTimePicker() string {
	box := m.timePickerBox()

	lines := make([]string, 0, box.height-2)
	lines = append(lines, titleLineStyle.Render("Select time range"))
	lines = append(lines, pickerHintStyle.Render("Use arrows or click, Enter to apply, Esc to cancel"))
	lines = append(lines, "")

	for i, preset := range timeRangePresets {
		line := preset.Label
		if preset.Condition != "" {
			line += "  " + preset.Condition
		}
		prefix := "  "
		style := pickerItemStyle
		if i == m.timeRangeIndex {
			prefix = "▸ "
			style = pickerItemSelectedStyle
		}
		lines = append(lines, style.Render(prefix+line))
	}

	lines = append(lines, "")
	lines = append(lines, pickerHintStyle.Render("Click a row or press Esc to close"))

	panel := pickerPanelStyle.Width(box.width).Height(box.height - 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m *Model) nullToggleLabel() string {
	label := "Show null values (n/Space)"
	checkbox := "[ ]"
	if !m.hideNullValues {
		checkbox = "[x]"
	}

	return checkbox + " " + label
}

func (m *Model) nullColumnChipLabel() string {
	if m.dropNullColumns {
		return "null cols: hidden (n)"
	}
	return "null cols: shown (n)"
}

func (m *Model) syncMouseMode() tea.Cmd {
	if m.timePickerOpen {
		return func() tea.Msg { return tea.EnableMouseAllMotion() }
	}
	if m.focus == FocusInput {
		return func() tea.Msg { return tea.DisableMouse() }
	}
	return func() tea.Msg { return tea.EnableMouseCellMotion() }
}

func (m *Model) filterPinLabel() string {
	if m.detail.filterPinned {
		return detailToggleEnabledStyle.Render("Preview filter pinned (p)")
	}
	return detailToggleDisabledStyle.Render("Preview filter unpinned (p)")
}

func pointInRect(x, y, rectX, rectY, width, height int) bool {
	return x >= rectX && x < rectX+width && y >= rectY && y < rectY+height
}

func insertClauseBeforeLimit(query, clause string) string {
	lower := strings.ToLower(query)
	idx := strings.LastIndex(lower, "| limit")
	if idx == -1 {
		return query + " | " + clause
	}

	prefix := strings.TrimSpace(query[:idx])
	suffix := strings.TrimSpace(query[idx:])
	return prefix + " | " + clause + " " + suffix
}

func (m *Model) addSelectedFilter(include bool) error {
	entries := m.filteredDetailEntries()
	if len(entries) == 0 {
		return fmt.Errorf("no field selected")
	}
	if m.detail.selected < 0 || m.detail.selected >= len(entries) {
		return fmt.Errorf("no field selected")
	}

	entry := entries[m.detail.selected]
	filter := queryFilter{
		Key:     entry.Key,
		Value:   entry.Value,
		Include: include,
	}
	if !m.hasFilter(filter) {
		m.filters = append(m.filters, filter)
	}
	m.err = nil
	if include {
		m.status = "Added include filter: " + filter.label()
	} else {
		m.status = "Added exclude filter: " + filter.label()
	}
	return nil
}

func (m *Model) hasFilter(filter queryFilter) bool {
	for _, existing := range m.filters {
		if existing.Key == filter.Key && existing.Value == filter.Value && existing.Include == filter.Include {
			return true
		}
	}
	return false
}

type copyMode int

const (
	fieldCopyPair copyMode = iota
	fieldCopyKey
	fieldCopyValue
	fieldCopyFilter
)

func (m *Model) copySelectedField(mode copyMode) error {
	entries := m.filteredDetailEntries()
	if len(entries) == 0 {
		return fmt.Errorf("no field selected")
	}
	if m.detail.selected < 0 || m.detail.selected >= len(entries) {
		return fmt.Errorf("no field selected")
	}

	entry := entries[m.detail.selected]
	var value string
	switch mode {
	case fieldCopyKey:
		value = entry.Key
	case fieldCopyValue:
		value = entry.Value
	case fieldCopyFilter:
		value = fmt.Sprintf("`%s` == %s", entry.Key, strconv.Quote(entry.Value))
	default:
		value = entry.Key + "=" + entry.Value
	}

	if err := clipboard.WriteAll(value); err != nil {
		return err
	}

	m.err = nil
	switch mode {
	case fieldCopyKey:
		m.status = "Copied key to clipboard"
	case fieldCopyValue:
		m.status = "Copied value to clipboard"
	case fieldCopyFilter:
		m.status = "Copied `key` == \"value\" filter to clipboard"
	default:
		m.status = "Copied key=value to clipboard"
	}
	return nil
}

func (m *Model) exportCurrentTable() error {
	if m.results == nil {
		return fmt.Errorf("no results to export")
	}

	data, err := m.exportAs(m.currentExportFormat())
	if err != nil {
		return err
	}

	if err := clipboard.WriteAll(data); err != nil {
		return err
	}

	m.err = nil
	m.status = fmt.Sprintf("Copied %s export to clipboard", strings.ToUpper(m.currentExportFormat()))
	return nil
}

func (m *Model) exportAs(format string) (string, error) {
	switch format {
	case "csv":
		return m.exportCSV()
	case "json":
		return m.exportJSON()
	case "yaml":
		return m.exportYAML()
	case "table":
		return m.exportTable()
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}
}

func (m *Model) exportTable() (string, error) {
	if m.results == nil {
		return "", fmt.Errorf("no results to export")
	}

	return renderPlainTable(m.results), nil
}

func (m *Model) exportJSON() (string, error) {
	rows := m.exportRows()
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Model) exportYAML() (string, error) {
	rows := m.exportRows()
	data, err := yaml.Marshal(rows)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Model) exportCSV() (string, error) {
	if m.results == nil {
		return "", fmt.Errorf("no results to export")
	}

	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	headers := make([]string, len(m.results.Columns))
	for i, col := range m.results.Columns {
		headers[i] = col.Name
	}
	if err := writer.Write(headers); err != nil {
		return "", err
	}

	for _, row := range m.results.Values {
		record := make([]string, len(m.results.Columns))
		for i := range m.results.Columns {
			if i < len(row) {
				record[i] = formatCSVValue(row[i])
			}
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func (m *Model) exportRows() []map[string]any {
	rows := make([]map[string]any, 0, len(m.results.Values))
	for _, row := range m.results.Values {
		record := make(map[string]any, len(m.results.Columns))
		for i, col := range m.results.Columns {
			if i < len(row) {
				record[col.Name] = row[i]
			} else {
				record[col.Name] = nil
			}
		}
		rows = append(rows, record)
	}
	return rows
}

func formatValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case []byte:
		return string(v)
	case map[string]any:
		return compactJSON(v)
	case []any:
		return compactJSON(v)
	default:
		raw, err := json.Marshal(v)
		if err == nil && len(raw) > 0 {
			if raw[0] == '{' || raw[0] == '[' {
				return compactJSON(json.RawMessage(raw))
			}
		}
		return fmt.Sprint(v)
	}
}

func isNullValue(value interface{}) bool {
	if value == nil {
		return true
	}

	if s, ok := value.(string); ok {
		return strings.EqualFold(strings.TrimSpace(s), "null")
	}

	return false
}

func formatCSVValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any:
		return compactJSON(v)
	case []any:
		return compactJSON(v)
	default:
		return fmt.Sprint(v)
	}
}

func compactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return strings.TrimSpace(string(data))
}

func renderPlainTable(result *elastic.QueryResult) string {
	if result == nil || len(result.Columns) == 0 {
		return ""
	}

	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = len(col.Name)
	}
	for _, row := range result.Values {
		for i, val := range row {
			if i >= len(widths) {
				break
			}
			if width := len(formatCSVValue(val)); width > widths[i] {
				widths[i] = width
			}
		}
	}

	var builder strings.Builder
	for i, col := range result.Columns {
		builder.WriteString(fmt.Sprintf("%-*s", widths[i]+2, col.Name))
	}
	builder.WriteString("\n")
	for _, width := range widths {
		builder.WriteString(strings.Repeat("-", width+2))
	}
	builder.WriteString("\n")

	for _, row := range result.Values {
		for i := range result.Columns {
			var value string
			if i < len(row) {
				value = formatCSVValue(row[i])
			}
			builder.WriteString(fmt.Sprintf("%-*s", widths[i]+2, value))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func (f queryFilter) label() string {
	prefix := "+"
	if !f.Include {
		prefix = "-"
	}
	return prefix + " " + f.Key + "=" + f.Value
}

func (f queryFilter) condition() string {
	operator := "=="
	expression := f.Key + " " + operator + " " + strconv.Quote(f.Value)
	if f.Include {
		return expression
	}
	return "NOT (" + expression + ")"
}

var (
	topHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236")).
			Bold(true).
			Padding(0, 1)

	titleLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	sectionTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("237")).
				Padding(0, 1).
				Bold(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	focusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("42")).
				Padding(0, 1)

	blurBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	chipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("237")).
			Padding(0, 1).
			MarginRight(1)

	filterChipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("60")).
			Padding(0, 1).
			MarginRight(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true)

	previewKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	previewValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	selectedDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("236")).
				Bold(true)

	detailLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	detailToggleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Italic(true)

	detailToggleActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("60")).
				Bold(true).
				Padding(0, 1)

	detailToggleEnabledStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("42")).
					Bold(true)

	detailToggleDisabledStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("244"))

	titleLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236")).
			Bold(true)

	pickerHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	pickerPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("42")).
				Background(lipgloss.Color("234"))

	pickerItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	pickerItemSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("60")).
				Bold(true)
)
