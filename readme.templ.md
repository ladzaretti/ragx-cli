<!-- omit in toc -->
# ragx — a terminal first local RAG assistant

![status: experimental](https://img.shields.io/badge/status-experimental-yellow)
[![Go Report Card](https://goreportcard.com/badge/github.com/ladzaretti/ragx-cli)](https://goreportcard.com/report/github.com/ladzaretti/ragx-cli)
![license](https://img.shields.io/github/license/ladzaretti/ragx-cli)

> [!NOTE]
> **Experimental project**: This is an experimental exploration of LLM programming. Expect some rough edges and ongoing changes as it evolves.

`ragx` is a minimal and hackable **Retrieval-Augmented Generation (RAG)** CLI tool designed for the terminal. It embeds your local files (or stdin), retrieves relevant chunks with KNN search, and queries OpenAI-compatible LLMs (local or remote) via a CLI/TUI workflow.

- [Installation](#installation)
  - [Option 1: Install via Go](#option-1-install-via-go)
  - [Option 2: Install via curl](#option-2-install-via-curl)
  - [Option 3: Download a release](#option-3-download-a-release)
- [Overview](#overview)
  - [Key features](#key-features)
  - [Use cases](#use-cases)
- [Pipeline Overview](#pipeline-overview)
- [Usage](#usage)
- [Configuration file](#configuration-file)
  - [Default prompts](#default-prompts)
  - [Config precedence (highest -\> lowest)](#config-precedence-highest---lowest)
- [Examples](#examples)
  - [Example: Listing available models](#example-listing-available-models)
  - [Example: TUI session](#example-tui-session)
  - [Example: CLI one-shot query](#example-cli-one-shot-query)
  - [Common command patterns](#common-command-patterns)
- [Notes \& Limitation](#notes--limitation)

## Installation

### Option 1: Install via Go
```bash
 go install github.com/ladzaretti/ragx-cli/cmd/ragx@latest
```

### Option 2: Install via curl

```bash
curl -sSL https://raw.githubusercontent.com/ladzaretti/ragx-cli/main/install.sh | bash
```
This script:
- Detects your OS and architecture
- Downloads the latest release from GitHub
- Copies the `ragx` binary to `/usr/local/bin`

### Option 3: Download a release

Visit the [Releases](https://github.com/ladzaretti/ragx-cli/releases) page for a list of available downloads.

## Overview

`ragx` focuses on the essentials of RAG:
- **Embed**: split content into chunks and generate embeddings with your chosen embedding model.
- **Retrieve**: run KNN over embeddings to select the most relevant chunks.
- **Generate**: send a prompt (system + user template + retrieved context) to an OpenAI-API compatible chat model.

### Key features

- **OpenAI API–compatible (v1)**: point `ragx` at any base URL (local Ollama or remote).
- **Per-provider/per-model overrides**: control temperature and context length.
- **TUI chat**: a lightweight Bubble Tea interface for iterative querying.
- **Terminal first**: pipe text in, embed directories/files, and print results.

### Use cases

- Local knowledge bases: notes, READMEs, docs.
- Quick “ask my files” workflows.

## Pipeline Overview
```mermaid
flowchart LR
  subgraph Ingest
    A["Files / stdin"] --> B["Chunker"]
    B --> C["Embedder"]
    C --> D["Vector Index / KNN"]
  end

  subgraph Query
    Q["User Query"] --> QE["Embed Query"]
    QE --> D
    D --> K["Top-K Chunks"]
    K --> P["Prompt Builder (system + template + context)"]
    P --> M["LLM (OpenAI-compatible)"]
    M --> R["Answer"]
  end
```

## Usage
```console
$ ragx --help
{{USAGE}}
```

## Configuration file

The optional configuration file can be generated using `ragx config generate` command:

```toml
{{CONFIG}}
```

### Default prompts
[System Prompt](https://github.com/ladzaretti/ragx-cli/blob/92ff0957b34b5a55a21601ed95a41ef2f9558d57/cli/prompt/prompt.go#L15)

[User Query Template](https://github.com/ladzaretti/ragx-cli/blob/92ff0957b34b5a55a21601ed95a41ef2f9558d57/cli/prompt/prompt.go#L96)


### Config precedence (highest -> lowest)

- CLI flags
- Environment variables (if supported)
  - OpenAI environment variables are auto-detected: `OPENAI_API_BASE`, `OPENAI_API_KEY`
- Config file
- Defaults

## Examples

### Example: Listing available models
```bash
$ ragx list
http://localhost:11434/v1
      jina/jina-embeddings-v2-base-en:latest
      gpt-oss:20b
      qwen3:8b-fast
      nomic-embed-text:latest
      mxbai-embed-large:latest
      llama3.1:8b
      qwen2.5-coder:14b
      deepseek-r1:8b
      qwen3:8b
      nomic-embed-text:v1.5
      hf.co/unsloth/DeepSeek-R1-0528-Qwen3-8B-GGUF:Q4_K_XL
```

### Example: TUI session
<img src="./assets/screenshot_tui.png" alt="ragx tui screenshot" width="768">

### Example: CLI one-shot query
```bash
$ ragx query readme.md \
            --model qwen3:8b \
            --embedding-model jina/jina-embeddings-v2-base-en:latest \
            "how do i tune chunk_size and overlap for large docs?"
- Tune `chunk_size` (chars per chunk) and `overlap` (chars overlapped between chunks) via config or CLI flags. For large documents, increase `chunk_size` (e.g., 2000+ chars) but keep `overlap` < `chunk_size` (e.g., 200). Adjust based on your content type and retrieval needs. [1]

Sources:
[1] (chunk 2) /home/gbi/GitHub/Gabriel-Ladzaretti/ragx-cli/readme.md
```

These are minimal examples to get you started.  
For detailed usage and more examples, run each subcommand with `--help`.

### Common command patterns

> [!NOTE] 
> These examples assume you already have a valid config file with at least one provider, a default chat model, and an embedding model set.  
> Generate a starter config with: `ragx config generate > ~/.ragx.toml`.

```shell
  # embed all .go files in current dir and query via --query/-q
  ragx query . -M '\.go$' -q "<query>"

  # embed a single file and provide query after flag terminator --
  ragx query readme.md -- "<query>"

  # embed stdin and provide query as the last positional argument
  cat readme.md | ragx query "<query>"

  # embed multiple paths with filter
  ragx query docs src -M '(?i)\.(md|txt)$' -q "<query>"

  # embed all .go files in current dir and start the TUI
  ragx chat . -M '\.go$'

  # embed multiple paths (markdown and txt) and start the TUI
  ragx chat ./docs ./src -M '(?i)\.(md|txt)$'

  # embed stdin and start the TUI
  cat readme.md | ragx chat
```

## Notes & Limitation

- Chunking is character-based by default; adjust `chunk_size`/`overlap` for your content and use case.
- The vector database is ephemeral: created fresh per session and not saved to disk.