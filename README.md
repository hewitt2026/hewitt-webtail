# gWebTail [![build](https://github.com/bsv9/gwebtail/actions/workflows/ci.yml/badge.svg)](https://github.com/bsv9/gwebtail/actions/workflows/ci.yml)&nbsp;[![Coverage Status](https://coveralls.io/repos/github/bsv9/gwebtail/badge.svg?branch=main)](https://coveralls.io/github/bsv9/gwebtail?branch=main)


A simple, lightweight web-based log file viewer with real-time log tailing via WebSockets.

![WebTail Screenshot](assets/images/gwebtail.png)

## Features

- **Web-based log viewer** - Access your logs from any browser
- **Real-time updates** - See new log entries immediately as they're written
- **No external dependencies** - Can work in on-prem environment
- **Syntax highlighting** - Highlight specific text patterns for easier reading
- **Adjustable display** - Configure how many lines to show
- **Full-width viewing** - Maximize screen space for log content
- **Start/Stop controls** - Pause updates when needed
- **Auto-scroll** - Automatically follow new log entries
- **Status indicators** - See connection status and view metrics

## Quick Start

### Using Docker

```bash
# Pull the image
docker pull docker.io/bsv9/gwebtail

# Run with default settings (logs in /logs directory)
docker run -p 8080:8080 -d docker.io/bsv9/gwebtail

# Run with custom log directory
docker run -p 8080:8080 -v /path/to/logs:/logs -d docker.io/bsv9/gwebtail

# Run with custom port
docker run -p 9000:8080 -d docker.io/bsv9/gwebtail

# Access WebTail
open http://localhost:8080
```

> **Note**: The Docker image supports multiple architectures including amd64, arm64, and armv7, allowing it to run on a variety of platforms including Raspberry Pi, AWS Graviton instances, and Apple Silicon devices.

### Manual Installation

```bash
# Clone the repository
git clone https://github.com/bsv9/gwebtail.git
cd gwebtail

# Build the application
go build -o gwebtail

# Run with default settings
./gwebtail

# Run with custom log directory
LOG_DIR=/path/to/logs ./gwebtail

# Run with custom port
./gwebtail -port 9000
```

## Configuration

WebTail can be configured through environment variables or command-line flags:

| Environment Variable | Flag            | Description                                | Default       |
|----------------------|-----------------|--------------------------------------------|--------------:|
| `LOG_DIR`            | `-logdir`       | Log directories and file patterns (see below) | `/logs`    |
| (none)               | `-port`         | HTTP server port                           | `8080`        |
| (none)               | `-refreshrate`  | File check interval in milliseconds        | `500`         |
| (none)               | `-buffersize`   | Buffer size for reading file updates (bytes) | `4096`      |
| `WEBTAIL_AUTH`       | `-auth`         | Basic auth credentials (see below)         | (none)      |

### Multiple Directories and Glob Patterns

The `-logdir` option supports multiple directories and file patterns, separated by `,` or `;`:

```bash
# Multiple directories
./gwebtail -logdir "/var/log,/tmp/logs,/data/logs"

# Glob patterns to filter files
./gwebtail -logdir "/var/log/*.log"

# Prefix patterns
./gwebtail -logdir "/var/log/app*.log;/data/server*.log"

# Mix directories and patterns
./gwebtail -logdir "/var/log,/data/logs/*.log,/tmp/debug*.log"
```

When the same filename exists in multiple directories, the file list automatically disambiguates them with a directory prefix (e.g., `log1/app.log`, `log2/app.log`).

### Authentication

Use `-auth` to enable HTTP Basic Authentication. When not set, the server is open (no auth).

```bash
# Single user
./gwebtail -auth "admin:123456"

# Multiple users (comma or semicolon separated)
./gwebtail -auth "admin:123456,viewer:abc123"

# Via environment variable
export WEBTAIL_AUTH="admin:123456"
./gwebtail
```

Passwords may contain colons (only the first colon separates user from password). When auth is enabled, the browser will show a login dialog. WebSocket connections use the same credentials automatically.

## Usage

1. **Select a log file** - Choose from the dropdown menu
2. **Start tailing** - Click the "Start" button to begin monitoring
3. **Highlight text** - Enter text to highlight across all log entries
4. **Adjust settings** - Change max lines to display or toggle auto-scroll
5. **Stop tailing** - Pause updates with the "Stop" button

## Building from Source

### Prerequisites

- Go 1.16+
- [Gorilla WebSocket](https://github.com/gorilla/websocket) package

### Build Instructions

```bash
# Install dependencies
go get github.com/gorilla/websocket

# Build the application
go build -o gwebtail
```

## Docker Build

```bash
docker build -t gwebtail .
```

## License

MIT


## Thanks

Anthropic Claude
