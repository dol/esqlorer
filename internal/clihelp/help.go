package clihelp

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CommandHelp struct {
	Name        string        `json:"name"`
	CommandPath string        `json:"commandPath"`
	Description string        `json:"description"`
	Long        string        `json:"long,omitempty"`
	Usage       string        `json:"usage"`
	Example     string        `json:"example,omitempty"`
	Aliases     []string      `json:"aliases,omitempty"`
	Flags       []FlagHelp    `json:"flags,omitempty"`
	Subcommands []CommandHelp `json:"subcommands,omitempty"`
}

type FlagHelp struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Type       string `json:"type"`
	Default    string `json:"default,omitempty"`
	Usage      string `json:"usage"`
	Required   bool   `json:"required,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`
}

func PrintJSON(w io.Writer, cmd *cobra.Command) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(build(cmd)); err != nil {
		return fmt.Errorf("encode help json: %w", err)
	}
	return nil
}

func build(cmd *cobra.Command) CommandHelp {
	out := CommandHelp{
		Name:        cmd.Name(),
		CommandPath: cmd.CommandPath(),
		Description: cmd.Short,
		Long:        cmd.Long,
		Usage:       cmd.UseLine(),
		Example:     cmd.Example,
		Aliases:     append([]string(nil), cmd.Aliases...),
	}

	requiredFlags := requiredFlagSet(cmd)
	seen := map[string]bool{}

	addFlags := func(set *pflag.FlagSet, persistent bool) {
		if set == nil {
			return
		}
		set.VisitAll(func(f *pflag.Flag) {
			if f.Hidden || f.Name == "help" || seen[f.Name] {
				return
			}
			seen[f.Name] = true
			out.Flags = append(out.Flags, FlagHelp{
				Name:       f.Name,
				Shorthand:  f.Shorthand,
				Type:       f.Value.Type(),
				Default:    f.DefValue,
				Usage:      f.Usage,
				Required:   requiredFlags[f.Name],
				Persistent: persistent,
			})
		})
	}

	addFlags(cmd.LocalFlags(), false)
	addFlags(cmd.InheritedFlags(), true)

	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() || sub.Name() == "help" {
			continue
		}
		out.Subcommands = append(out.Subcommands, build(sub))
	}

	return out
}

func requiredFlagSet(cmd *cobra.Command) map[string]bool {
	required := map[string]bool{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Annotations != nil && len(f.Annotations[cobra.BashCompOneRequiredFlag]) > 0 {
			required[f.Name] = true
		}
	})
	return required
}
