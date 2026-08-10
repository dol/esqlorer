package auth

import (
	"encoding/json"
	"fmt"
	"os"

	authTUI "github.com/dominicluechinger/esqlorer/cmd/auth/tui"
	"github.com/dominicluechinger/esqlorer/internal/agentmode"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dominicluechinger/esqlorer/internal/config"
)

var (
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
)

var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Elasticsearch server configurations",
	Long:  "Manage multiple Elasticsearch cluster connections.",
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured servers",
	RunE:  runList,
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new server configuration",
	Long: `Add a new server configuration.

Examples:
  esqlorer auth add --name local --url http://localhost:9200
  esqlorer auth add --name prod --url https://elastic:9200 --auth-method basic --username admin --password secret

Use --interactive for interactive mode.`,
	RunE: runAdd,
}

var removeCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a server configuration",
	Args:  cobra.ExactValidArgs(1),
	RunE:  runRemove,
}

var switchCmd = &cobra.Command{
	Use:   "switch [name]",
	Short: "Switch to a different server context",
	Args:  cobra.RangeArgs(0, 1),
	RunE:  runSwitch,
}

var (
	addName        string
	addURL         string
	addAuthMethod  string
	addAPIKey      string
	addUsername    string
	addPassword    string
	addInteractive bool
)

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(addCmd)
	Cmd.AddCommand(removeCmd)
	Cmd.AddCommand(switchCmd)

	addCmd.Flags().StringVarP(&addName, "name", "n", "", "Server name")
	addCmd.Flags().StringVarP(&addURL, "url", "u", "", "Server URL")
	addCmd.Flags().StringVarP(&addAuthMethod, "auth-method", "a", "", "Auth method: api-key or basic")
	addCmd.Flags().StringVarP(&addAPIKey, "api-key", "k", "", "API key")
	addCmd.Flags().StringVarP(&addUsername, "username", "", "", "Username (for basic auth)")
	addCmd.Flags().StringVarP(&addPassword, "password", "p", "", "Password (for basic auth)")
	addCmd.Flags().BoolVarP(&addInteractive, "interactive", "i", false, "Run in interactive mode")
}

func isInteractive() bool {
	if addInteractive {
		return true
	}
	if os.Getenv("CI") != "" {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func loadConfig() (*config.Config, error) {
	configPath := viper.GetString("config")
	return config.Load(configPath)
}

func saveConfig(cfg *config.Config) error {
	configPath := viper.GetString("config")
	return cfg.Save(configPath)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(cfg.Servers) == 0 {
		if agentmode.EnabledForCommand(cmd) {
			fmt.Println("[]")
			return nil
		}
		fmt.Println("No servers configured.")
		return nil
	}

	if agentmode.EnabledForCommand(cmd) {
		type serverItem struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Current bool   `json:"current"`
		}

		items := make([]serverItem, 0, len(cfg.Servers))
		for _, server := range cfg.Servers {
			items = append(items, serverItem{
				Name:    server.Name,
				URL:     server.URL,
				Current: server.Name == cfg.CurrentContext,
			})
		}

		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	for _, server := range cfg.Servers {
		prefix := "  "
		if server.Name == cfg.CurrentContext {
			prefix = colorGreen + "* " + colorReset
		}
		fmt.Printf("%s%s %s\n", prefix, server.Name, server.URL)
	}

	return nil
}

func runAdd(cmd *cobra.Command, args []string) error {
	if addName == "" || addURL == "" {
		if !isInteractive() {
			return fmt.Errorf("missing required flags: --name and --url\nRun 'esqlorer auth add --help' for usage")
		}

		fmt.Println("Adding new server configuration")
		fmt.Println()

		model := authTUI.NewAddServer()
		if err := model.Run(); err != nil {
			return err
		}

		server := model.Server()
		if server == nil {
			return fmt.Errorf("interactive add was cancelled")
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if cfg.GetServer(server.Name) != nil {
			return fmt.Errorf("server %q already exists", server.Name)
		}

		if err := cfg.AddServer(*server); err != nil {
			return err
		}

		if cfg.CurrentContext == "" {
			cfg.CurrentContext = server.Name
		}

		if err := saveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Added server %s\n", server.Name)
		return nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if cfg.GetServer(addName) != nil {
		return fmt.Errorf("server %q already exists", addName)
	}

	server := config.Server{
		Name: addName,
		URL:  addURL,
	}

	if addAuthMethod == "api-key" {
		server.APIKey = addAPIKey
	} else if addAuthMethod == "basic" {
		server.Username = addUsername
		server.Password = addPassword
	}

	if err := cfg.AddServer(server); err != nil {
		return err
	}

	if cfg.CurrentContext == "" {
		cfg.CurrentContext = server.Name
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Added server %s\n", server.Name)
	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	name := args[0]

	if err := cfg.RemoveServer(name); err != nil {
		return err
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Removed server %s\n", name)
	return nil
}

func runSwitch(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	} else {
		for _, s := range cfg.Servers {
			fmt.Println(s.Name)
		}
		return nil
	}

	if err := cfg.SetCurrentContext(name); err != nil {
		return err
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Switched to %s\n", name)
	return nil
}
