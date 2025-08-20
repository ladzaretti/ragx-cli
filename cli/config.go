package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"

	"github.com/spf13/cobra"
)

// configOptions holds cli, file, and resolved global configuration.
type configOptions struct {
	*genericclioptions.StdioOptions

	flags *Flags

	fileConfig *Config
	resolved   *Config
}

// Flags holds cli overrides for configuration.
type Flags struct {
	configPath     string
	baseURL        string
	model          string
	embeddingModel string
	dimensions     int
	logDir         string
	logFilename    string
	logLevel       string
}

type Duration time.Duration

func (d Duration) String() string { return time.Duration(d).String() }

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

var _ genericclioptions.CmdOptions = &configOptions{}

// NewConfigOptions initializes ConfigOptions with default values.
func NewConfigOptions(stdio *genericclioptions.StdioOptions) *configOptions {
	return &configOptions{
		StdioOptions: stdio,
		fileConfig:   newFileConfig(),
		flags:        &Flags{},
	}
}

func (o *configOptions) Resolved() *Config { return o.resolved }

func (o *configOptions) Complete() error {
	c, err := LoadFileConfig(o.flags.configPath)
	if err != nil {
		return err
	}

	o.fileConfig = c

	return o.resolve()
}

func (o *configOptions) resolve() error {
	o.resolved = o.fileConfig

	o.resolved.path = cmp.Or(o.flags.configPath, o.fileConfig.path)

	o.resolved.LLM.BaseURL = cmp.Or(os.Getenv("OPENAI_API_BASE"), o.flags.baseURL, o.fileConfig.LLM.BaseURL)
	o.resolved.LLM.APIKey = cmp.Or(os.Getenv("OPENAI_API_KEY"), o.fileConfig.LLM.APIKey)
	o.resolved.LLM.Model = cmp.Or(os.Getenv("OPENAI_MODEL"), o.flags.model, o.fileConfig.LLM.Model)

	o.resolved.Prompt.System = cmp.Or(o.fileConfig.Prompt.System, prompt.DefaultSystemPrompt)
	o.resolved.Prompt.UserPromptTmpl = cmp.Or(o.fileConfig.Prompt.UserPromptTmpl, prompt.DefaultUserPromptTmpl)

	o.resolved.Embedding.EmbeddingModel = cmp.Or(o.flags.embeddingModel, o.fileConfig.Embedding.EmbeddingModel)
	o.resolved.Embedding.Dimensions = cmp.Or(o.flags.dimensions, o.fileConfig.Embedding.Dimensions)

	o.resolved.Logging.Dir = cmp.Or(o.flags.logDir, o.fileConfig.Logging.Dir)
	o.resolved.Logging.Filename = cmp.Or(o.flags.logFilename, o.fileConfig.Logging.Filename)
	o.resolved.Logging.Level = cmp.Or(os.Getenv("LOG_LEVEL"), o.flags.logLevel, o.fileConfig.Logging.Level)

	return nil
}

func (o *configOptions) Validate() error {
	if _, err := genericclioptions.ParseLevel(o.resolved.Logging.Level); err != nil {
		return err
	}

	return nil
}

func (*configOptions) Run(context.Context, ...string) error { return nil }

// NewCmdConfig creates the cobra config command tree.
func NewCmdConfig(defaults *DefaultRAGOptions) *cobra.Command {
	hiddenFlags := []string{
		"base-url",
		"dim",
		"embedding-model",
		"match",
		"model",
	}

	o := NewConfigOptions(defaults.StdioOptions)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show and inspect configuration",
		Long: fmt.Sprintf(`Show the active ragrat configuration.

If --config is not provided, the default path (~/%s) is used.`, defaultConfigName),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := clierror.Check(genericclioptions.RejectDisallowedFlags(cmd, hiddenFlags...)); err != nil {
				return err
			}
			if err := clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o)); err != nil {
				return err
			}

			if len(o.fileConfig.path) == 0 {
				o.Infof("no config file found; using default values.\n")
				return nil
			}

			c := struct {
				Path     string `json:"path"`
				Parsed   any    `json:"parsed_config"`   //nolint:tagliatelle
				Resolved any    `json:"resolved_config"` //nolint:tagliatelle
			}{
				Path:     o.fileConfig.path,
				Parsed:   o.fileConfig,
				Resolved: o.resolved,
			}

			o.Printf("%s", stringifyPretty(c))

			return nil
		},
	}

	cmd.AddCommand(newGenerateConfigCmd(defaults))
	cmd.AddCommand(newValidateConfigCmd(defaults))

	genericclioptions.MarkFlagsHidden(cmd, hiddenFlags...)

	return cmd
}

// stringifyPretty returns the pretty-printed JSON representation of v.
// If marshalling fails, it returns the error message instead.
func stringifyPretty(v any) string {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)

	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("stringify error: %v", err)
	}

	return buf.String()
}

type generateConfigOptions struct {
	*genericclioptions.StdioOptions
}

var _ genericclioptions.CmdOptions = &generateConfigOptions{}

// newGenerateConfigOptions initializes the options struct.
func newGenerateConfigOptions(stdio *genericclioptions.StdioOptions) *generateConfigOptions {
	return &generateConfigOptions{
		StdioOptions: stdio,
	}
}

func (*generateConfigOptions) Complete() error { return nil }

func (*generateConfigOptions) Validate() error { return nil }

func (o *generateConfigOptions) Run(context.Context, ...string) error {
	o.Printf("%s", GenerateDefault())

	return nil
}

// newGenerateConfigCmd creates the 'generate' subcommand for generating default config.
func newGenerateConfigCmd(defaults *DefaultRAGOptions) *cobra.Command {
	o := newGenerateConfigOptions(defaults.StdioOptions)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a default config file",
		Long: `Generate the default configuration in TOML format 
and write it to stdout.`,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	genericclioptions.MarkAllFlagsHidden(cmd, "help")

	return cmd
}

type validateConfigOptions struct {
	*genericclioptions.StdioOptions

	configPath string
}

var _ genericclioptions.CmdOptions = &validateConfigOptions{}

// newValidateConfigOptions initializes the options struct.
func newValidateConfigOptions(stdio *genericclioptions.StdioOptions) *validateConfigOptions {
	return &validateConfigOptions{
		StdioOptions: stdio,
	}
}

func (*validateConfigOptions) Complete() error { return nil }

func (*validateConfigOptions) Validate() error { return nil }

func (o *validateConfigOptions) Run(context.Context, ...string) error {
	c, err := LoadFileConfig(o.configPath)
	if err := clierror.Check(err); err != nil {
		return err
	}

	if len(c.path) == 0 {
		o.Infof("no config file found; Nothing to validate.\n")
		return nil
	}

	o.Infof("%s: OK\n", c.path)

	return nil
}

// newValidateConfigCmd creates the 'validate' subcommand for validating the config file.
func newValidateConfigCmd(defaults *DefaultRAGOptions) *cobra.Command {
	o := newValidateConfigOptions(defaults.StdioOptions)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file",
		Long: fmt.Sprintf(`Load the configuration file and check for common errors.

If --config is not provided, the default path (~/%s) is used.`, defaultConfigName),
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.configPath, _ = cmd.InheritedFlags().GetString("config")

			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	genericclioptions.MarkAllFlagsHidden(cmd, "help", "config")

	return cmd
}
