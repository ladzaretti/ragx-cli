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

type ConfigError struct {
	Opt string
	Err error
}

func (e *ConfigError) Error() string {
	return "config: " + strings.Join([]string{e.Opt, e.Err.Error()}, ": ")
}

func (e *ConfigError) Unwrap() error { return e.Err }

type Config struct {
	LLM       LLMConfig        `json:"llm"                 toml:"llm"`
	Prompt    *PromptConfig    `json:"prompt,omitempty"    toml:"prompt,omitempty"`
	Embedding *EmbeddingConfig `json:"embedding,omitempty" toml:"embedding,omitempty"`
	Logging   *LoggingConfig   `json:"logging,omitempty"   toml:"logging,commented"`

	path string // path to the loaded config file. Empty if no config file was used.
}

type LLMConfig struct {
	BaseURL     string  `json:"base_url"              toml:"base_url"              comment:"Base URL for the LLM server (e.g., Ollama, OpenAI API-compatible)"`
	APIKey      string  `json:"api_key,omitempty"     toml:"api_key,commented"     comment:"Optional API key if required"`
	Model       string  `json:"model"                 toml:"model,commented"       comment:"Default model to use"`
	Temperature float64 `json:"temperature,omitempty" toml:"temperature,commented" comment:"Completion temperature"`
}

type PromptConfig struct {
	System string `json:"system_prompt,omitempty" toml:"system_prompt,commented" comment:"System prompt to override the default assistant behavior"`
}

type EmbeddingConfig struct {
	EmbeddingModel string `json:"embedding_model,omitempty" toml:"embedding_model"      comment:"Model used for embeddings"`
	Dimensions     int    `json:"dimensions,omitempty"      toml:"dimensions,commented" comment:"Embedding vector dimension (must match model output)"`
	ChunkSize      int    `json:"chunk_size,omitempty"      toml:"chunk_size,commented" comment:"Number of characters per chunk"`
	Overlap        int    `json:"overlap,omitempty"         toml:"overlap,commented"    comment:"Number of characters overlapped between chunks (must be less than chunk_size)"`
	TopK           int    `json:"top_k,omitempty"           toml:"top_k,commented"      comment:"Number of chunks to retrieve during RAG"`
}

type LoggingConfig struct {
	Dir      string `json:"log_dir,omitempty"   toml:"log_dir,commented"      comment:"Directory where log file will be stored (default: XDG_STATE_HOME or ~/.local/state/ragrat)"`
	Filename string `json:"log_file,omitempty"  toml:"log_filename,commented" comment:"Filename for the log file"`
	Level    string `json:"log_level,omitempty" toml:"log_level,commented"`
}

func newFileConfig() *Config {
	return &Config{
		LLM:       LLMConfig{},
		Prompt:    &PromptConfig{},
		Embedding: &EmbeddingConfig{},
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
	c.Logging.Level = cmp.Or(c.Logging.Level, defaultLogLevel)

	c.LLM.BaseURL = cmp.Or(c.LLM.BaseURL, string(defaultBaseURL))

	c.Embedding.ChunkSize = cmp.Or(c.Embedding.ChunkSize, defaultChunkSize)
	c.Embedding.Overlap = cmp.Or(c.Embedding.Overlap, int(defaultOverlap))
	c.Embedding.TopK = cmp.Or(c.Embedding.TopK, defaultTopK)

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

	if c.Embedding != nil {
		if c.Embedding.ChunkSize < 0 {
			return &ConfigError{Opt: "retrieval.chunk_size", Err: errors.New("must be zero or positive")}
		}

		if c.Embedding.TopK < 0 {
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

func openLogFile(dir, name string) (*os.File, error) {
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
