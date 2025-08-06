package cli

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	appName                  = "ragrat"
	envConfigPathKeyOverride = "RAGRAT_CONFIG_PATH"
	defaultBaseURL           = "http://localhost:11434"
	defaultConfigName        = ".ragrat.toml"
	defaultLogFilename       = ".log"
	defaultTemperature       = 0.7
	defaultChunkSize         = 500
	defaultTopK              = 4
)

type ConfigError struct {
	Opt string
	Err error
}

func (e *ConfigError) Error() string {
	return "config: " + strings.Join([]string{e.Opt, e.Err.Error()}, ": ")
}

func (e *ConfigError) Unwrap() error { return e.Err }

type Config struct {
	LLM       LLMConfig        `toml:"llm" json:"llm"`
	Prompt    *PromptConfig    `toml:"prompt,omitempty" json:"prompt,omitempty"`
	Retrieval *RetrievalConfig `toml:"retrieval,omitempty" json:"retrieval,omitempty"`
	Logging   *LoggingConfig   `toml:"logging,commented" json:"logging,omitempty"`

	path string // path to the loaded config file. Empty if no config file was used.
}

type LLMConfig struct {
	BaseURL        string  `toml:"base_url" comment:"Base URL for the LLM server (e.g., Ollama, OpenAI-compatible)" json:"base_url"`
	APIKey         string  `toml:"api_key,commented" comment:"Optional API key if required" json:"api_key,omitempty"`
	Model          string  `toml:"model,commented" comment:"Default model to use" json:"model"`
	EmbeddingModel string  `toml:"embedding_model" comment:"Model used for embeddings" json:"embedding_model,omitempty"`
	Temperature    float64 `toml:"temperature,commented" comment:"Completion temperature" json:"temperature,omitempty"`
}

type PromptConfig struct {
	System string `toml:"system_prompt,commented" comment:"System prompt to override the default assistant behavior" json:"system_prompt,omitempty"`
}

type RetrievalConfig struct {
	ChunkSize int `toml:"chunk_size,commented" comment:"Number of characters per chunk" json:"chunk_size,omitempty"`
	TopK      int `toml:"top_k,commented" comment:"Number of chunks to retrieve during RAG" json:"top_k,omitempty"`
}

type LoggingConfig struct {
	Dir      string `toml:"log_dir,commented" comment:"Directory where log file will be stored (default: XDG_STATE_HOME or ~/.local/state/ragrat)" json:"log_dir,omitempty"`
	Filename string `toml:"log_filename,commented" comment:"Filename for the log file" json:"log_file,omitempty"`
}

func newFileConfig() *Config {
	return &Config{
		LLM:       LLMConfig{},
		Prompt:    &PromptConfig{},
		Retrieval: &RetrievalConfig{},
		Logging:   &LoggingConfig{},
	}
}

func (c *Config) ConfigPath() (string, bool) {
	return c.path, c.path != ""
}

// setDefaults fills zero-valued optional fields.
func (c *Config) setDefaults() error {
	if c == nil {
		return &ConfigError{Err: errors.New("cannot set defaults on nil FileConfig")}
	}

	dir, err := defaultLogDir()
	if err != nil {
		return &ConfigError{Opt: "logging.log_dir", Err: err}
	}

	c.Logging.Dir = cmp.Or(c.Logging.Dir, dir)
	c.Logging.Filename = cmp.Or(c.Logging.Filename, defaultLogFilename)

	c.LLM.BaseURL = cmp.Or(c.LLM.BaseURL, string(defaultBaseURL))
	c.LLM.Temperature = cmp.Or(c.LLM.Temperature, defaultTemperature)

	c.Retrieval.ChunkSize = cmp.Or(c.Retrieval.ChunkSize, defaultChunkSize)
	c.Retrieval.TopK = cmp.Or(c.Retrieval.TopK, defaultTopK)

	return nil
}

func (c *Config) validate() error {
	if c == nil {
		return &ConfigError{Err: errors.New("cannot validate a nil config")}
	}

	if c.LLM.BaseURL == "" {
		return &ConfigError{Opt: "llm.base_url", Err: errors.New("must be set")}
	}

	if c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		return &ConfigError{Opt: "llm.temperature", Err: errors.New("must be between 0 and 2")}
	}

	if strings.Contains(c.Logging.Filename, "/") {
		return &ConfigError{Opt: "logging.log_filename", Err: errors.New("must not contain slashes")}
	}

	if c.Retrieval != nil {
		if c.Retrieval.ChunkSize < 0 {
			return &ConfigError{Opt: "retrieval.chunk_size", Err: errors.New("must be zero or positive")}
		}

		if c.Retrieval.TopK < 0 {
			return &ConfigError{Opt: "retrieval.top_k", Err: errors.New("must be zero or positive")}
		}
	}

	return nil
}

// LoadFileConfig loads the config from the given or default path.
func LoadFileConfig(path string) (*Config, error) {
	defaultPath, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	configPath := cmp.Or(path, defaultPath)

	c, err := parseFileConfig(configPath)
	if err != nil {
		// config file not found at default location; fallback to empty config
		if path == "" && errors.Is(err, fs.ErrNotExist) { //nolint:revive // clearer with explicit fallback logic
			c = newFileConfig()
		} else {
			return nil, err
		}
	} else {
		c.path = configPath
	}

	if err := c.setDefaults(); err != nil {
		return nil, err
	}

	return c, c.validate()
}

// GenerateDefault returns a TOML string with default values and comments.
func GenerateDefault() string {
	c := newFileConfig()

	if err := c.setDefaults(); err != nil {
		panic("config: failed to set config defaults: " + err.Error())
	}

	out, err := toml.Marshal(c)
	if err != nil {
		panic("config: failed to marshal default config: " + err.Error())
	}

	return string(out)
}

func OpenLogFile(dir string, name string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	var (
		filename = filepath.Join(dir, name)
		flag     = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	return os.OpenFile(filepath.Clean(filename), flag, 0o600) //nolint:gosec // internal filename
}

func defaultLogDir() (string, error) {
	if stateDir, ok := os.LookupEnv("XDG_STATE_HOME"); ok {
		return filepath.Join(stateDir, appName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "state", appName), nil
}

func defaultConfigPath() (string, error) {
	if p, ok := os.LookupEnv(envConfigPathKeyOverride); ok {
		return p, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultConfigName), nil
}

func parseFileConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("config: stat file: %w", err)
	}

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	config := newFileConfig()
	if err := toml.Unmarshal(raw, config); err != nil {
		return nil, fmt.Errorf("config: parse file: %w", err)
	}

	return config, nil
}
