package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dominicluechinger/esqlorer/cmd/auth"
	"github.com/dominicluechinger/esqlorer/cmd/query"
	"github.com/dominicluechinger/esqlorer/cmd/tui"
	"github.com/dominicluechinger/esqlorer/internal/agentmode"
	"github.com/dominicluechinger/esqlorer/internal/clihelp"
	"github.com/dominicluechinger/esqlorer/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:           "esqlorer",
	Short:         "A TUI for querying Elasticsearch using ES|QL",
	Long:          "esqlorer is a Go-based TUI for querying Elasticsearch using ES|QL. It mimics the Kibana Discover experience, providing a lightweight, terminal-native way to explore data with support for multiple environments.",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configPath := cfgFile
		if configPath == "" {
			configPath = config.DefaultConfigPath()
		}

		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		viper.SetConfigFile(configPath)
		viper.SetConfigType("yaml")

		if _, err := os.Stat(configPath); err == nil {
			return viper.ReadInConfig()
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "Path to config file (default "+config.DefaultConfigPath()+")")
	rootCmd.PersistentFlags().Bool("agent-mode", false, "Enable machine-readable agent mode")
	_ = viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	rootCmd.AddCommand(auth.Cmd)
	rootCmd.AddCommand(query.Cmd)
	rootCmd.AddCommand(tui.Cmd)

	defaultHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if agentmode.EnabledForCommand(cmd) {
			if err := clihelp.PrintJSON(cmd.OutOrStdout(), cmd); err != nil {
				writeError(cmd.ErrOrStderr(), err, true)
			}
			return
		}
		defaultHelpFunc(cmd, args)
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [shell]",
		Short: "Generate shell completion script",
		Long: `To load completions:

Bash:
  $ source <(esqlorer completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ esqlorer completion bash > /etc/bash_completion.d/esqlorer
  # macOS:
  $ esqlorer completion bash > /usr/local/etc/bash_completion.d/esqlorer

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -Uz compinit
  $ compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ esqlorer completion zsh > "${fpath[1]}/_esqlorer"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ esqlorer completion fish | source

  # To load completions for each session, execute once:
  $ esqlorer completion fish > ~/.config/fish/completions/esqlorer.fish

PowerShell:
  $ esqlorer completion powershell | Import-Module

  # To load completions for every session, add the output of the following command to your $PROFILE:
  $ esqlorer completion powershell >> $PROFILE

`,
		Args:                  cobra.ExactValidArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var filename string
			filename = "/tmp/esqlorer_completion." + args[0]
			switch args[0] {
			case "bash":
				if err := rootCmd.GenBashCompletionFile(filename); err != nil {
					return err
				}
			case "zsh":
				if err := rootCmd.GenZshCompletionFile(filename); err != nil {
					return err
				}
			case "fish":
				if err := rootCmd.GenFishCompletionFile(filename, true); err != nil {
					return err
				}
			case "powershell":
				if err := rootCmd.GenPowerShellCompletionFile(filename); err != nil {
					return err
				}
			default:
				return cmd.Help()
			}
			data, err := os.ReadFile(filename)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			os.Remove(filename)
			return nil
		},
	})
}

func main() {
	if err := Execute(); err != nil {
		writeError(os.Stderr, err, agentmode.EnabledForArgs(os.Args[1:]))
		os.Exit(1)
	}
}

func writeError(stream io.Writer, err error, agentMode bool) {
	if agentMode {
		_ = json.NewEncoder(stream).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}
	fmt.Fprintf(stream, "Error: %v\n", err)
}
