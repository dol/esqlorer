package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/dominicluechinger/esqlorer/pkg/elastic"
)

type Table struct {
	columns  []elastic.Column
	rows     [][]interface{}
	selected int
	focus    bool
	width    int
	height   int
}

func NewTable() Table {
	return Table{
		selected: 0,
		focus:    false,
	}
}

func (t *Table) SetColumns(columns []elastic.Column) {
	t.columns = columns
}

func (t *Table) SetRows(rows [][]interface{}) {
	t.rows = rows
	if t.selected >= len(t.rows) {
		t.selected = 0
	}
}

func (t *Table) Selected() int {
	return t.selected
}

func (t *Table) SetSelected(i int) {
	if i < 0 {
		t.selected = 0
		return
	}
	if i >= len(t.rows) && len(t.rows) > 0 {
		t.selected = len(t.rows) - 1
		return
	}
	t.selected = i
}

func (t *Table) SelectedRow() ([]interface{}, bool) {
	if t.selected < 0 || t.selected >= len(t.rows) {
		return nil, false
	}
	return t.rows[t.selected], true
}

func (t *Table) View() string {
	if len(t.columns) == 0 {
		return emptyTableStyle.Render("No results. Press Enter to search.")
	}

	colWidths := t.calculateColumnWidths()
	header := t.renderHeader(colWidths)
	separator := t.renderSeparator(colWidths)
	lines := []string{header, separator}

	start := 0
	visibleRows := t.height - 4
	if visibleRows < 1 {
		visibleRows = 1
	}
	if len(t.rows) > visibleRows {
		start = t.selected
		if start > len(t.rows)-1 {
			start = len(t.rows) - 1
		}
	}

	for i := start; i < start+visibleRows && i < len(t.rows); i++ {
		lines = append(lines, t.renderRow(t.rows[i], colWidths, i == t.selected))
	}

	return lipgloss.NewStyle().
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (t *Table) calculateColumnWidths() []int {
	widths := make([]int, len(t.columns))
	for i, col := range t.columns {
		widths[i] = runewidth.StringWidth(col.Name)
	}

	for _, row := range t.rows {
		for i, val := range row {
			w := runewidth.StringWidth(fmt.Sprint(val))
			if w > widths[i] {
				widths[i] = w
			}
		}
	}

	maxTotal := t.width - 4
	if maxTotal > 0 {
		total := 0
		for _, width := range widths {
			total += width + 2
		}
		if total > maxTotal {
			maxCol := maxInt(8, (maxTotal-(len(widths)*2))/len(widths))
			for i := range widths {
				if widths[i] > maxCol {
					widths[i] = maxCol
				}
			}
		}
	}

	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	return widths
}

func (t *Table) renderHeader(widths []int) string {
	parts := make([]string, 0, len(t.columns))
	for i, col := range t.columns {
		parts = append(parts, headerCellStyle.Render(runewidth.FillRight(truncateString(col.Name, widths[i]), widths[i]+2)))
	}
	return strings.Join(parts, "")
}

func (t *Table) renderSeparator(widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		parts = append(parts, separatorCellStyle.Render(strings.Repeat("─", w+2)))
	}
	return strings.Join(parts, "")
}

func (t *Table) renderRow(row []interface{}, widths []int, selected bool) string {
	parts := make([]string, 0, len(row)+1)
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	parts = append(parts, prefixStyle.Render(prefix))

	for i, val := range row {
		if i >= len(widths) {
			break
		}
		cell := runewidth.FillRight(truncateString(fmt.Sprint(val), widths[i]), widths[i]+2)
		if selected && t.focus {
			cell = selectedCellStyle.Render(cell)
		} else {
			cell = cellStyle.Render(cell)
		}
		parts = append(parts, cell)
	}
	return strings.Join(parts, "")
}

func (t *Table) Up() {
	if t.selected > 0 {
		t.selected--
	}
}

func (t *Table) Down() {
	if t.selected < len(t.rows)-1 {
		t.selected++
	}
}

func (t *Table) SetFocus(focus bool) {
	t.focus = focus
}

func (t *Table) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func truncateString(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	return runewidth.Truncate(value, width, "…")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	emptyTableStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true).
			Padding(0, 1)

	headerCellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	separatorCellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	prefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	cellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selectedCellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("236"))
)
