package query

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dominicluechinger/esqlorer/internal/agentmode"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dominicluechinger/esqlorer/internal/config"
	"github.com/dominicluechinger/esqlorer/pkg/elastic"
)

var (
	Cmd = &cobra.Command{
		Use:   "query <query>",
		Short: "Execute an ES|QL query",
		Long: "Execute an ES|QL query against the configured Elasticsearch cluster.\n\n" +
			"Examples:\n" +
			"  # Query all documents from an index\n" +
			"  esqlorer query 'FROM logs-*'\n\n" +
			"  # Query with stats\n" +
			"  esqlorer query 'FROM logs-* | STATS avg(latency) BY service'\n\n" +
			"  # Use specific server context\n" +
			"  esqlorer query -c prod 'FROM logs-* | LIMIT 100'\n\n" +
			"  # Query with a time range on @timestamp\n" +
			"  esqlorer query --from now-2h --to now 'FROM logs-* | LIMIT 100'\n\n" +
			"  # Output as JSON\n" +
			"  esqlorer query 'FROM logs-* | LIMIT 10' -o json\n\n" +
			"  # Drop result columns that are null for all returned rows\n" +
			"  esqlorer query --drop-null-columns 'FROM logs-* | LIMIT 100'\n\n" +
			"Hint:\n" +
			"  Use single quotes around the query string.\n" +
			"  This avoids shell interpretation of ES|QL field names wrapped in backticks, for example:\n" +
			"  esqlorer query 'FROM logs-* | KEEP `host.name`, message | LIMIT 10'",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
			}
			return nil
		},
		RunE: runQuery,
	}

	context   string
	output    string
	fromValue string
	toValue   string
	dropNull  bool
)

func init() {
	Cmd.Flags().StringVar(&context, "context", "", "Server context name")
	Cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, json, csv")
	Cmd.Flags().StringVar(&fromValue, "from", "", "Lower bound for @timestamp, for example now-2h or 2026-05-01T10:00:00Z")
	Cmd.Flags().StringVar(&toValue, "to", "", "Upper bound for @timestamp, for example now or 2026-05-01T12:00:00Z")
	Cmd.Flags().BoolVar(&dropNull, "drop-null-columns", false, "Drop result columns whose values are null for every returned row")
}

func runQuery(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		if err := cmd.Help(); err != nil {
			return err
		}
		if !agentmode.EnabledForCommand(cmd) {
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Hint: wrap the ES|QL query in single quotes to avoid shell interpretation of backticks in field names.")
		}
		return nil
	}

	query := strings.TrimSpace(args[0])
	if query == "" {
		return fmt.Errorf("query must not be empty")
	}
	if err := validateTimeRange(fromValue, toValue, time.Now()); err != nil {
		return err
	}

	configPath := viper.GetString("config")
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	serverName := context
	if serverName == "" {
		serverName = cfg.CurrentContext
	}

	if serverName == "" {
		return fmt.Errorf("no server context specified. Use -c flag or set a default with 'esqlorer auth switch'")
	}

	server := cfg.GetServer(serverName)
	if server == nil {
		return fmt.Errorf("server %q not found", serverName)
	}

	client, err := elastic.NewClient(*server)
	if err != nil {
		return err
	}

	result, err := client.ExecuteESQLWithOptions(cmd.Context(), elastic.QueryOptions{
		Query:           query,
		From:            fromValue,
		To:              toValue,
		DropNullColumns: dropNull,
	})
	if err != nil {
		return err
	}

	return printResult(result, resolveOutputFormat(cmd))
}

func resolveOutputFormat(cmd *cobra.Command) string {
	if cmd.Flags().Changed("output") {
		return output
	}
	if agentmode.EnabledForCommand(cmd) {
		return "json"
	}
	return output
}

var relativeTimePattern = regexp.MustCompile(`^now(?:([+-])(\d+)([smhdw]))?$`)

func validateTimeRange(from, to string, now time.Time) error {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return nil
	}

	fromTime, fromOK := resolveComparableTime(from, now)
	toTime, toOK := resolveComparableTime(to, now)
	if !fromOK || !toOK {
		return nil
	}

	if fromTime.After(toTime) {
		return fmt.Errorf("invalid time range: --from %q must be earlier than or equal to --to %q", from, to)
	}

	return nil
}

func resolveComparableTime(value string, now time.Time) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}

	matches := relativeTimePattern.FindStringSubmatch(strings.ToLower(value))
	if matches == nil {
		return time.Time{}, false
	}
	if matches[1] == "" {
		return now, true
	}

	amount, err := strconv.Atoi(matches[2])
	if err != nil {
		return time.Time{}, false
	}

	duration, ok := relativeDuration(amount, matches[3])
	if !ok {
		return time.Time{}, false
	}

	if matches[1] == "+" {
		return now.Add(duration), true
	}
	return now.Add(-duration), true
}

func relativeDuration(amount int, unit string) (time.Duration, bool) {
	switch unit {
	case "s":
		return time.Duration(amount) * time.Second, true
	case "m":
		return time.Duration(amount) * time.Minute, true
	case "h":
		return time.Duration(amount) * time.Hour, true
	case "d":
		return time.Duration(amount) * 24 * time.Hour, true
	case "w":
		return time.Duration(amount) * 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func printResult(result *elastic.QueryResult, format string) error {
	switch format {
	case "json":
		return printJSON(result)
	case "csv":
		return printCSV(result)
	case "table":
		return printTable(result)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func printJSON(result *elastic.QueryResult) error {
	rows := queryRows(result)
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printCSV(result *elastic.QueryResult) error {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	headers := make([]string, len(result.Columns))
	for i, col := range result.Columns {
		headers[i] = col.Name
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	for _, row := range result.Values {
		record := make([]string, len(result.Columns))
		for i := range result.Columns {
			if i < len(row) {
				record[i] = formatCSVValue(row[i])
			}
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	fmt.Print(builder.String())
	return nil
}

func printTable(result *elastic.QueryResult) error {
	colWidths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		colWidths[i] = len(col.Name)
	}

	for _, row := range result.Values {
		for i, val := range row {
			if w := len(fmt.Sprint(val)); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	printHeader(result.Columns, colWidths)
	printSeparator(colWidths)
	for _, row := range result.Values {
		printRow(row, colWidths)
	}

	return nil
}

func queryRows(result *elastic.QueryResult) []map[string]any {
	rows := make([]map[string]any, 0, len(result.Values))
	for _, row := range result.Values {
		record := make(map[string]any, len(result.Columns))
		for i, col := range result.Columns {
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

func printHeader(cols []elastic.Column, widths []int) {
	for i, col := range cols {
		fmt.Printf("%-*s", widths[i]+2, col.Name)
	}
	fmt.Println()
}

func printSeparator(widths []int) {
	for _, w := range widths {
		fmt.Printf("%s", strings.Repeat("-", w+2))
	}
	fmt.Println()
}

func printRow(row []interface{}, widths []int) {
	for i, val := range row {
		fmt.Printf("%-*s", widths[i]+2, fmt.Sprint(val))
	}
	fmt.Println()
}
