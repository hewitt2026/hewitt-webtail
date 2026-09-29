// DISCLAIMER: This code was generated mostly by Claude, an AI assistant created by Anthropic.
// While efforts have been made to ensure the code is accurate and functional,
// it may contain errors or not perform as intended in all environments.
// This code is provided "as is" without warranty of any kind.
// Users should review, test, and validate the code before using it in production environments.
// The user assumes all responsibility for the implementation and use of this code.

package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed assets/html/index.html
var indexHTML []byte

//go:embed assets/css/bootstrap.min.css
var bootstrapCSS []byte

// Configuration struct
type Config struct {
	LogDir          string
	Port            int
	FileRefreshRate int // Milliseconds between file checks
	BufferSize      int64
	Auth            string // Auth credentials in user:password format, multiple separated by , or ;
}

// FileInfo struct for listing files
type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

// TailCommand struct for WebSocket commands
type TailCommand struct {
	Command     string `json:"command"`
	File        string `json:"file,omitempty"`
	Lines       int    `json:"lines,omitempty"`       // Max number of lines to display in the UI
	SearchStr   string `json:"searchStr,omitempty"`
	RefreshRate int    `json:"refreshRate,omitempty"` // File check interval in milliseconds
}

// TailResponse struct for WebSocket responses
type TailResponse struct {
	Type      string   `json:"type"`
	File      string   `json:"file,omitempty"`
	Lines     []string `json:"lines,omitempty"`
	Error     string   `json:"error,omitempty"`
	SearchStr string   `json:"searchStr,omitempty"`
}

// WebSocketConnection defines an interface for websocket connections
// This allows us to mock the connection in tests
type WebSocketConnection interface {
	WriteJSON(v interface{}) error
	Close() error
	RemoteAddr() interface{} // Works with both net.Addr and string
}

// WebSocketWrapper wraps a *websocket.Conn to implement our WebSocketConnection interface
type WebSocketWrapper struct {
	*websocket.Conn
}

// RemoteAddr implements the WebSocketConnection interface
func (w *WebSocketWrapper) RemoteAddr() interface{} {
	return w.Conn.RemoteAddr()
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all connections
		},
	}
	activeConnections = make(map[*websocket.Conn]bool)
	connectionsMutex  = sync.Mutex{}
	config            Config

	// fileMap maps display names to full file paths, rebuilt on each getFileList call
	fileMap      = make(map[string]string)
	fileMapMutex sync.Mutex
)

func main() {
	// Configure logger with timestamp
	log.SetFlags(log.LstdFlags)

	// Parse command line flags
	flag.StringVar(&config.LogDir, "logdir", "", "Directory containing log files")
	flag.IntVar(&config.Port, "port", 8080, "HTTP server port")
	flag.IntVar(&config.FileRefreshRate, "refreshrate", 1000, "File check interval in milliseconds")
	flag.Int64Var(&config.BufferSize, "buffersize", 32*1024, "Buffer size for reading file updates (bytes)")
	flag.StringVar(&config.Auth, "auth", "", "Basic auth credentials (user:password), multiple separated by , or ;")
	flag.Parse()

	// Check if logdir is set from environment variable
	if config.LogDir == "" {
		config.LogDir = os.Getenv("LOG_DIR")
		if config.LogDir == "" {
			config.LogDir = "/logs" // Default value
		}
	}

	// Check if auth is set from environment variable
	if config.Auth == "" {
		config.Auth = os.Getenv("WEBTAIL_AUTH")
	}

	// Create log directories if they don't exist (skip glob patterns)
	for _, dir := range parseLogDirs() {
		if strings.ContainsAny(dir, "*?[") {
			continue // Glob patterns are resolved at query time
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Fatalf("Failed to create log directory %s: %v", dir, err)
			}
		}
	}

	// Set up HTTP handlers with logging middleware
	loggedMux := http.NewServeMux()
	loggedMux.HandleFunc("/", serveHome)
	loggedMux.HandleFunc("/api/files", listFiles)
	loggedMux.HandleFunc("/api/download", downloadFile)
	loggedMux.HandleFunc("/ws", handleWebSocket)

	// Add route to serve embedded bootstrap.min.css
	loggedMux.HandleFunc("/assets/css/bootstrap.min.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write(bootstrapCSS)
	})

	// Create middleware to log HTTP requests
	var handler http.Handler = logRequestMiddleware(loggedMux)

	// Wrap with basic auth middleware if credentials are configured
	if config.Auth != "" {
		creds := parseAuthCredentials()
		if len(creds) > 0 {
			handler = basicAuthMiddleware(handler, creds)
			log.Printf("Basic auth enabled with %d credential(s)", len(creds))
		}
	}

	// Start HTTP server
	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Starting WebTail server on %s with log directory: %s", addr, config.LogDir)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// logRequestMiddleware logs all HTTP requests
func logRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("HTTP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("HTTP %s %s completed in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// parseAuthCredentials splits Auth by ',' or ';' and returns a user→password map
func parseAuthCredentials() map[string]string {
	result := make(map[string]string)
	parts := strings.FieldsFunc(config.Auth, func(r rune) bool {
		return r == ',' || r == ';'
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx <= 0 || idx >= len(part)-1 {
			log.Printf("Invalid auth credential format (expected user:password): '%s'", part)
			continue
		}
		user := strings.TrimSpace(part[:idx])
		pass := strings.TrimSpace(part[idx+1:])
		if user != "" && pass != "" {
			result[user] = pass
		}
	}
	return result
}

// basicAuthMiddleware enforces HTTP Basic Authentication
func basicAuthMiddleware(next http.Handler, creds map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract credentials from the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="gWebTail"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Expect "Basic <base64>"
		if !strings.HasPrefix(authHeader, "Basic ") {
			http.Error(w, "Unsupported auth scheme", http.StatusUnauthorized)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(authHeader[6:])
		if err != nil {
			http.Error(w, "Invalid credentials encoding", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			http.Error(w, "Invalid credentials format", http.StatusUnauthorized)
			return
		}

		user, pass := parts[0], parts[1]
		expectedPass, ok := creds[user]
		if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="gWebTail"`)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// serveHome handles the home page
func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write(indexHTML); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// parseLogDirs splits LogDir by ',' or ';' and returns trimmed non-empty entries
func parseLogDirs() []string {
	parts := strings.FieldsFunc(config.LogDir, func(r rune) bool {
		return r == ',' || r == ';'
	})
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		result = append(result, "/logs")
	}
	return result
}

// isGlobPattern checks if a path contains wildcard characters
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// resolveAllFiles collects files from all configured directories/globs,
// rebuilds the fileMap, and returns display names sorted by modification time (newest first).
// When the same filename appears in multiple directories, it is disambiguated
// with a "parentDir/filename" prefix.
func resolveAllFiles() ([]string, map[string]string, error) {
	dirs := parseLogDirs()

	type fileEntry struct {
		name     string    // base filename
		fullPath string
		modTime  time.Time // modification time for sorting
	}
	var allFiles []fileEntry

	for _, dir := range dirs {
		if isGlobPattern(dir) {
			matches, err := filepath.Glob(dir)
			if err != nil {
				log.Printf("Invalid glob pattern '%s': %v", dir, err)
				continue
			}
			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil || info.IsDir() {
					continue
				}
				allFiles = append(allFiles, fileEntry{name: filepath.Base(m), fullPath: m, modTime: info.ModTime()})
			}
		} else {
			entries, err := os.ReadDir(dir)
			if err != nil {
				log.Printf("Error reading directory '%s': %v", dir, err)
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				allFiles = append(allFiles, fileEntry{
					name:     e.Name(),
					fullPath: filepath.Join(dir, e.Name()),
					modTime:  info.ModTime(),
				})
			}
		}
	}

	// Count name occurrences for disambiguation
	nameCount := make(map[string]int)
	for _, f := range allFiles {
		nameCount[f.name]++
	}

	// Build display-name → full-path map and collect sortable entries
	type namedEntry struct {
		displayName string
		fullPath    string
		modTime     time.Time
	}
	var sortable []namedEntry

	for _, f := range allFiles {
		displayName := f.name
		if nameCount[f.name] > 1 {
			parentDir := filepath.Base(filepath.Dir(f.fullPath))
			displayName = parentDir + "/" + f.name
		}
		sortable = append(sortable, namedEntry{
			displayName: displayName,
			fullPath:    f.fullPath,
			modTime:     f.modTime,
		})
	}

	// Sort by modification time descending (newest first)
	sort.Slice(sortable, func(i, j int) bool {
		return sortable[i].modTime.After(sortable[j].modTime)
	})

	// Build result map and ordered names
	resultMap := make(map[string]string)
	var names []string
	for _, e := range sortable {
		resultMap[e.displayName] = e.fullPath
		names = append(names, e.displayName)
	}

	return names, resultMap, nil
}

// listFiles handles listing files in the log directory
func listFiles(w http.ResponseWriter, r *http.Request) {
	names, resolvedMap, err := resolveAllFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error resolving files: %v", err), http.StatusInternalServerError)
		return
	}

	// Update global fileMap
	fileMapMutex.Lock()
	fileMap = resolvedMap
	fileMapMutex.Unlock()

	var fileInfos []FileInfo
	for _, name := range names {
		fullPath := resolvedMap[name]
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, FileInfo{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fileInfos); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding JSON: %v", err), http.StatusInternalServerError)
	}
}

// downloadFile handles downloading a log file
func downloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "Missing 'file' parameter", http.StatusBadRequest)
		return
	}

	filePath := resolveFilePath(fileName)

	// Verify the file exists and is accessible
	info, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "File not found: "+fileName, http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "Cannot download a directory", http.StatusBadRequest)
		return
	}

	// Use the base name for the downloaded file
	downloadName := filepath.Base(filePath)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(w, file, buf); err != nil {
		log.Printf("Error streaming file %s: %v", fileName, err)
	}
}

// handleWebSocket handles WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}

	// Wrap the connection to implement our interface
	wsConn := &WebSocketWrapper{conn}

	defer func() {
		if err := wsConn.Close(); err != nil {
			log.Printf("Error closing WebSocket connection: %v", err)
		}
	}()

	// Register connection
	connectionsMutex.Lock()
	connID := conn.RemoteAddr().String()
	activeConnections[conn] = true
	activeCount := len(activeConnections)
	connectionsMutex.Unlock()

	log.Printf("WebSocket connected from %s (active connections: %d)", connID, activeCount)

	defer func() {
		connectionsMutex.Lock()
		delete(activeConnections, conn)
		remainingCount := len(activeConnections)
		connectionsMutex.Unlock()
		log.Printf("WebSocket disconnected from %s (active connections: %d)", connID, remainingCount)
	}()

	// Send initial file list
	files, _ := getFileList()
	response := TailResponse{
		Type:  "files",
		Lines: files,
	}
	if err := wsConn.WriteJSON(response); err != nil {
		log.Println("Error sending file list:", err)
		return
	}

	// Handle WebSocket messages
	var tailCancel chan bool
	var wg sync.WaitGroup

	for {
		var cmd TailCommand
		if err := wsConn.ReadJSON(&cmd); err != nil {
			log.Println("Error reading JSON:", err)
			break
		}

		// Log the received command
		cmdDetails := cmd.Command
		if cmd.File != "" {
			cmdDetails += fmt.Sprintf(" (file: %s)", cmd.File)
		}
		if cmd.SearchStr != "" {
			cmdDetails += fmt.Sprintf(" (search: %s)", cmd.SearchStr)
		}
		if cmd.Lines > 0 {
			cmdDetails += fmt.Sprintf(" (lines: %d)", cmd.Lines)
		}
		log.Printf("WebSocket command from %s: %s", connID, cmdDetails)

		switch cmd.Command {
		case "list":
			files, err := getFileList()
			if err != nil {
				sendError(wsConn, "Failed to list files: "+err.Error())
				continue
			}
			response := TailResponse{
				Type:  "files",
				Lines: files,
			}
			if err := wsConn.WriteJSON(response); err != nil {
				log.Println("Error sending file list:", err)
			}

		case "tail":
			// Cancel previous tail if any
			if tailCancel != nil {
				tailCancel <- true
				wg.Wait()
				log.Printf("WebSocket previous tail canceled for %s", connID)
			}

			// Start new tail
			tailCancel = make(chan bool)
			wg.Add(1)
			log.Printf("WebSocket starting tail for %s (file: %s)", connID, cmd.File)
			go func(file string, cancel chan bool) {
				defer wg.Done()
				tailFile(wsConn, file, cancel, connID)
			}(cmd.File, tailCancel)

		case "stop":
			// Stop current tail if any
			if tailCancel != nil {
				tailCancel <- true
				wg.Wait()
				tailCancel = nil
				log.Printf("WebSocket tail stopped for %s", connID)

				// Send acknowledgment
				response := TailResponse{
					Type: "stopped",
				}
				if err := wsConn.WriteJSON(response); err != nil {
					log.Println("Error sending stop acknowledgment:", err)
				}
			}

		case "config":
			// Update server configuration at runtime
			if cmd.RefreshRate > 0 {
				config.FileRefreshRate = cmd.RefreshRate
				log.Printf("WebSocket refresh rate updated to %dms by %s", cmd.RefreshRate, connID)
				response := TailResponse{
					Type: "config",
				}
				if err := wsConn.WriteJSON(response); err != nil {
					log.Println("Error sending config acknowledgment:", err)
				}
			}

		case "search":
			// Update search in current tail
			if tailCancel != nil {
				// This doesn't interrupt the current tail, just updates the search
				response := TailResponse{
					Type:      "search",
					SearchStr: cmd.SearchStr,
				}
				if err := wsConn.WriteJSON(response); err != nil {
					log.Println("Error sending search update:", err)
				}
				log.Printf("WebSocket search updated for %s (search: %s)", connID, cmd.SearchStr)
			}
		}
	}

	// Cancel tail on connection close
	if tailCancel != nil {
		tailCancel <- true
		wg.Wait()
	}
}

// getFileList returns a list of files across all configured log directories/globs
func getFileList() ([]string, error) {
	names, resolvedMap, err := resolveAllFiles()
	if err != nil {
		return nil, err
	}

	// Update global fileMap
	fileMapMutex.Lock()
	fileMap = resolvedMap
	fileMapMutex.Unlock()

	return names, nil
}

// resolveFilePath looks up a display name in the fileMap.
// If not found, it tries joining the name with each configured directory as a fallback.
func resolveFilePath(fileName string) string {
	fileMapMutex.Lock()
	if fullPath, ok := fileMap[fileName]; ok {
		fileMapMutex.Unlock()
		return fullPath
	}
	fileMapMutex.Unlock()

	// Fallback: search across configured directories
	for _, dir := range parseLogDirs() {
		if isGlobPattern(dir) {
			continue
		}
		candidate := filepath.Join(dir, fileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Last resort: return as-is (for backward compatibility)
	return filepath.Join(parseLogDirs()[0], fileName)
}

func tailFile(conn WebSocketConnection, fileName string, cancel chan bool, connID string) {
	filePath := resolveFilePath(fileName)
	file, err := os.Open(filePath)
	if err != nil {
		sendError(conn, "Failed to open file: "+err.Error())
		log.Printf("WebSocket error for %s: Failed to open file %s: %v", connID, fileName, err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()

	// Get initial file size
	fileInfo, err := file.Stat()
	if err != nil {
		sendError(conn, "Failed to stat file: "+err.Error())
		log.Printf("WebSocket error for %s: Failed to stat file %s: %v", connID, fileName, err)
		return
	}
	fileSize := fileInfo.Size()

	// Send initial buffer content
	initialBuffer, err := readLastBuffer(file, fileSize)
	if err != nil {
		sendError(conn, "Failed to read file: "+err.Error())
		log.Printf("WebSocket error for %s: Failed to read file %s: %v", connID, fileName, err)
		return
	}

	response := TailResponse{
		Type:  "tail",
		File:  fileName,
		Lines: []string{initialBuffer},
	}
	if err := conn.WriteJSON(response); err != nil {
		log.Println("Error sending initial buffer:", err)
		return
	}

	// Set current position to end of file for watching new updates
	currentPos := fileSize

	// Watch for file changes
	ticker := time.NewTicker(time.Duration(config.FileRefreshRate) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if file has been updated
			fileInfo, err := file.Stat()
			if err != nil {
				sendError(conn, "Failed to stat file: "+err.Error())
				log.Printf("WebSocket error for %s: Failed to stat file during tail %s: %v", connID, fileName, err)
				return
			}
			newSize := fileInfo.Size()

			if newSize > currentPos {
				// File has grown, read only the new content
				_, err := file.Seek(currentPos, io.SeekStart)
				if err != nil {
					sendError(conn, "Failed to seek in file: "+err.Error())
					log.Printf("WebSocket error for %s: Failed to seek in file %s: %v", connID, fileName, err)
					return
				}

				// Read only the new bytes, but limit to config.BufferSize
				bytesToRead := min(newSize-currentPos, config.BufferSize)
				buffer := make([]byte, bytesToRead)
				bytesRead, err := file.Read(buffer)
				if err != nil && err != io.EOF {
					sendError(conn, "Failed to read file: "+err.Error())
					log.Printf("WebSocket error for %s: Failed to read file %s: %v", connID, fileName, err)
					return
				}

				response := TailResponse{
					Type:  "update",
					File:  fileName,
					Lines: []string{string(buffer[:bytesRead])},
				}
				if err := conn.WriteJSON(response); err != nil {
					log.Println("Error sending updates:", err)
					log.Printf("WebSocket error for %s: Failed to send updates for %s: %v", connID, fileName, err)
					return
				}

				currentPos = newSize
			} else if newSize < currentPos {
				// File has been truncated, reset and read from beginning
				currentPos = 0
				_, err := file.Seek(0, io.SeekStart)
				if err != nil {
					sendError(conn, "Failed to seek to beginning of file: "+err.Error())
					log.Printf("WebSocket error for %s: Failed to seek to beginning of %s after truncation: %v", connID, fileName, err)
					return
				}

				initialBuffer, err := readLastBuffer(file, newSize)
				if err != nil {
					sendError(conn, "Failed to read file after truncation: "+err.Error())
					log.Printf("WebSocket error for %s: Failed to read %s after truncation: %v", connID, fileName, err)
					return
				}

				response := TailResponse{
					Type:  "tail",
					File:  fileName,
					Lines: []string{initialBuffer},
				}
				if err := conn.WriteJSON(response); err != nil {
					log.Println("Error sending reset after truncation:", err)
					log.Printf("WebSocket error for %s: Failed to send reset after truncation of %s: %v", connID, fileName, err)
					return
				}
				log.Printf("WebSocket file %s was truncated, reset and sent buffer to %s", fileName, connID)

				currentPos = newSize
			}

		case <-cancel:
			// Cancel the tail
			log.Printf("WebSocket tail of %s canceled for %s", fileName, connID)
			return
		}
	}
}

// readLastBuffer reads the last buffer from the file
func readLastBuffer(file *os.File, fileSize int64) (string, error) {
	if fileSize == 0 {
		return "", nil
	}

	bufferSize := config.BufferSize
	if bufferSize > fileSize {
		bufferSize = fileSize
	}

	buffer := make([]byte, bufferSize)

	// Calculate the correct offset to include complete lines
	// In this case, we need to ensure we start at a position where we get full lines
	offset := max(0, fileSize-bufferSize)

	_, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to seek to offset %d: %w", offset, err)
	}

	bytesRead, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read buffer: %w", err)
	}

	content := string(buffer[:bytesRead])

	// If we're not starting from the beginning of the file, we might have a partial line
	// Find the position of the first newline and skip the partial line if needed
	if offset > 0 {
		if idx := strings.Index(content, "\n"); idx >= 0 {
			// Skip the first partial line if we didn't start at the beginning of the file
			content = content[idx+1:]
		}
	}

	return content, nil
}

// sendError sends an error message over WebSocket
func sendError(conn WebSocketConnection, message string) {
	response := TailResponse{
		Type:  "error",
		Error: message,
	}
	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Error sending error message: %v", err)
	}
}

// Helper function to get the maximum of two int64 values
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// Helper function to get the minimum of two int64 values
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
