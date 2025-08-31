package types

type LLMConfig struct {
	DefaultModel string           `json:"default_model,omitempty" toml:"default_model"       comment:"Default model to use"`
	Providers    []ProviderConfig `json:"providers,omitempty"     toml:"providers,commented" comment:"LLM providers (uncomment and duplicate as needed)\n[[llm.providers]]\nbase_url = 'http://localhost:11434'\napi_key = '<KEY>'\t\t# optional\ntemperature = 0.7\t\t# optional (provider default)"`
	Models       []ModelConfig    `json:"models,omitempty"        toml:"models,commented"    comment:"Optional model definitions for context length control (uncomment and duplicate as needed)\n[[llm.models]]\nid = 'qwen:8b-fast'\t\t# Model identifier\ncontext = 4096\t\t# Maximum context length in tokens\ntemperature = 0.7\t\t# optional (model override)"`
}

type ModelConfig struct {
	ID          string   `json:"id,omitempty"          toml:"id,commented"          comment:"Model identifier"`
	Context     int      `json:"context,omitempty"     toml:"context,commented"     comment:"Maximum context length in tokens"`
	Temperature *float64 `json:"temperature,omitempty" toml:"temperature,commented" comment:"Optional model-level temperature override"`
}
type ProviderConfig struct {
	BaseURL     string   `json:"base_url"              toml:"base_url"              comment:"Base URL for the LLM server (e.g., Ollama, OpenAI API-compatible)"`
	APIKey      string   `json:"api_key,omitempty"     toml:"api_key,commented"     comment:"Optional API key if required"`
	Temperature *float64 `json:"temperature,omitempty" toml:"temperature,commented" comment:"Default temperature for this provider (optional)"`
}

type PromptConfig struct {
	System         string `json:"system_prompt,omitempty"    toml:"system_prompt,commented"    comment:"System prompt to override the default assistant behavior"`
	UserPromptTmpl string `json:"user_prompt_tmpl,omitempty" toml:"user_prompt_tmpl,commented" comment:"Go text/template for building the USER QUERY + CONTEXT block"`
}

type EmbeddingConfig struct {
	Model     string `json:"embedding_model,omitempty" toml:"embedding_model"      comment:"Model used for embeddings"`
	ChunkSize int    `json:"chunk_size,omitempty"      toml:"chunk_size,commented" comment:"Number of characters per chunk"`
	Overlap   int    `json:"overlap,omitempty"         toml:"overlap,commented"    comment:"Number of characters overlapped between chunks (must be less than chunk_size)"`
	TopK      int    `json:"top_k,omitempty"           toml:"top_k,commented"      comment:"Number of chunks to retrieve during RAG"`
}

type LoggingConfig struct {
	Dir      string `json:"log_dir,omitempty"   toml:"log_dir,commented"      comment:"Directory where log file will be stored (default: XDG_STATE_HOME or ~/.local/state/ragrep)"`
	Filename string `json:"log_file,omitempty"  toml:"log_filename,commented" comment:"Filename for the log file"`
	Level    string `json:"log_level,omitempty" toml:"log_level,commented"`
}
