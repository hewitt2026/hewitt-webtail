package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestServeHome tests the home page handler
func TestServeHome(t *testing.T) {
	// Set up a request to the root path
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Call the handler
	serveHome(w, req)

	// Check the status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("Expected Content-Type 'text/html', got '%s'", contentType)
	}

	// Check that we got some HTML content (not empty)
	if len(w.Body.Bytes()) == 0 {
		t.Errorf("Expected non-empty body, got empty response")
	}

	// Check if body contains some expected HTML elements
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "gWebTail") {
		t.Errorf("Expected response to contain 'gWebTail', but it doesn't")
	}

	// Test wrong path
	req = httptest.NewRequest("GET", "/wrong-path", nil)
	w = httptest.NewRecorder()
	serveHome(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for wrong path, got %d", w.Code)
	}

	// Test wrong method
	req = httptest.NewRequest("POST", "/", nil)
	w = httptest.NewRecorder()
	serveHome(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for wrong method, got %d", w.Code)
	}
}

// TestListFiles tests the file listing API
func TestListFiles(t *testing.T) {
	// Create temporary directory for test logs
	tmpDir, err := os.MkdirTemp("", "webtail-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test log files
	testFiles := []string{"test1.log", "test2.log"}
	testContent := []byte("test content")

	for _, fname := range testFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Save old config and restore after test
	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	// Set log directory to temp directory
	config.LogDir = tmpDir

	// Make request to listFiles
	req := httptest.NewRequest("GET", "/api/files", nil)
	w := httptest.NewRecorder()

	listFiles(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Decode response
	var fileInfos []FileInfo
	if err := json.NewDecoder(w.Body).Decode(&fileInfos); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Check number of files
	if len(fileInfos) != len(testFiles) {
		t.Errorf("Expected %d files, got %d", len(testFiles), len(fileInfos))
	}

	// Check file names
	fileNames := make(map[string]bool)
	for _, fi := range fileInfos {
		fileNames[fi.Name] = true
	}

	for _, fname := range testFiles {
		if !fileNames[fname] {
			t.Errorf("Expected file %s not found in response", fname)
		}
	}
}

// TestReadLastBuffer tests the readLastBuffer function
func TestReadLastBuffer(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "webtail-buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test content
	testContent := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Sync(); err != nil {
		t.Fatalf("Failed to sync temp file: %v", err)
	}

	// Get file size
	fileInfo, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}
	fileSize := fileInfo.Size()

	// Test with bufferSize smaller than file
	oldBufferSize := config.BufferSize
	defer func() { config.BufferSize = oldBufferSize }()

	// Set buffer size to get just the last part of the file
	// "Line 5" is 6 bytes, plus we'll need to skip the first partial line
	config.BufferSize = 12

	// Rewind file before each test
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek file: %v", err)
	}

	// Read last buffer
	buffer, err := readLastBuffer(tmpFile, fileSize)
	if err != nil {
		t.Fatalf("readLastBuffer failed: %v", err)
	}

	// The current implementation should return content starting after the first newline
	// when reading from the middle of the file
	if !strings.Contains(buffer, "Line 5") {
		t.Errorf("Expected buffer to contain 'Line 5', got '%s'", buffer)
	}

	// Test with bufferSize larger than file
	config.BufferSize = fileSize * 2

	// Rewind file
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek file: %v", err)
	}

	buffer, err = readLastBuffer(tmpFile, fileSize)
	if err != nil {
		t.Fatalf("readLastBuffer failed: %v", err)
	}

	if buffer != testContent {
		t.Errorf("Expected buffer '%s', got '%s'", testContent, buffer)
	}

	// Test with empty file
	emptyFile, err := os.CreateTemp("", "webtail-empty-test")
	if err != nil {
		t.Fatalf("Failed to create empty temp file: %v", err)
	}
	defer os.Remove(emptyFile.Name())

	emptyInfo, err := emptyFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat empty file: %v", err)
	}

	buffer, err = readLastBuffer(emptyFile, emptyInfo.Size())
	if err != nil {
		t.Fatalf("readLastBuffer failed for empty file: %v", err)
	}

	if buffer != "" {
		t.Errorf("Expected empty buffer, got '%s'", buffer)
	}
}

// TestGetFileList tests the getFileList function
func TestGetFileList(t *testing.T) {
	// Create temporary directory for test logs
	tmpDir, err := os.MkdirTemp("", "webtail-filelist-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test log files
	testFiles := []string{"test1.log", "test2.log", "test3.log"}
	testContent := []byte("test content")

	for _, fname := range testFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create a subdirectory - should be skipped
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Save old config and restore after test
	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	// Set log directory to temp directory
	config.LogDir = tmpDir

	// Get file list
	files, err := getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	// Check number of files
	if len(files) != len(testFiles) {
		t.Errorf("Expected %d files, got %d", len(testFiles), len(files))
	}

	// Check file names
	fileMap := make(map[string]bool)
	for _, f := range files {
		fileMap[f] = true
	}

	for _, fname := range testFiles {
		if !fileMap[fname] {
			t.Errorf("Expected file %s not found in result", fname)
		}
	}

	// Subdirectory should not be in results
	if fileMap["subdir"] {
		t.Errorf("Subdirectory should not be included in results")
	}
}

// Mock WebSocket connection for testing
type mockConn struct {
	sentMessages []TailResponse
	mu           sync.Mutex
}

// mockAddr implements the net.Addr interface for testing
// This matches the behavior of the actual WebSocketWrapper.RemoteAddr() method
// which returns a net.Addr from the underlying websocket connection
type mockAddr struct{}

func (a mockAddr) Network() string { return "tcp" }
func (a mockAddr) String() string  { return "test-client:12345" }

func (m *mockConn) WriteJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	response, ok := v.(TailResponse)
	if ok {
		m.sentMessages = append(m.sentMessages, response)
	}
	return nil
}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) RemoteAddr() interface{} {
	return mockAddr{}
}

// TestSendError tests the sendError function
func TestSendError(t *testing.T) {
	mock := &mockConn{sentMessages: []TailResponse{}}

	// Send an error
	errorMsg := "Test error message"
	sendError(mock, errorMsg)

	// Check the sent message
	if len(mock.sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(mock.sentMessages))
	}

	msg := mock.sentMessages[0]
	if msg.Type != "error" {
		t.Errorf("Expected message type 'error', got '%s'", msg.Type)
	}

	if msg.Error != errorMsg {
		t.Errorf("Expected error message '%s', got '%s'", errorMsg, msg.Error)
	}
}

// Integration test for WebSocket handling (marked as skipped)
func TestWebSocketIntegration(t *testing.T) {
	t.Skip("Integration test skipped - requires actual WebSocket connections")
}

// Test the main application setup (limited test)
func TestMainSetup(t *testing.T) {
	// Save old config
	oldLogDir := config.LogDir
	oldPort := config.Port
	oldRefreshRate := config.FileRefreshRate
	oldBufferSize := config.BufferSize

	// Restore after test
	defer func() {
		config.LogDir = oldLogDir
		config.Port = oldPort
		config.FileRefreshRate = oldRefreshRate
		config.BufferSize = oldBufferSize
	}()

	// Set test values
	testDir := "/tmp/test-logs"
	config.LogDir = testDir
	config.Port = 8081
	config.FileRefreshRate = 100
	config.BufferSize = 1024

	// Verify values were set
	if config.LogDir != testDir {
		t.Errorf("Expected LogDir '%s', got '%s'", testDir, config.LogDir)
	}

	if config.Port != 8081 {
		t.Errorf("Expected Port 8081, got %d", config.Port)
	}

	if config.FileRefreshRate != 100 {
		t.Errorf("Expected FileRefreshRate 100, got %d", config.FileRefreshRate)
	}

	if config.BufferSize != 1024 {
		t.Errorf("Expected BufferSize 1024, got %d", config.BufferSize)
	}
}

// TestTailFile tests the tailFile function
func TestTailFile(t *testing.T) {
	// Create temporary directory for test logs
	tmpDir, err := os.MkdirTemp("", "webtail-tail-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test log file
	testFile := "test.log"
	testFilePath := filepath.Join(tmpDir, testFile)
	initialContent := "Initial line 1\nInitial line 2\n"

	if err := os.WriteFile(testFilePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Save old config and restore after test
	oldLogDir := config.LogDir
	oldRefreshRate := config.FileRefreshRate
	oldBufferSize := config.BufferSize
	defer func() {
		config.LogDir = oldLogDir
		config.FileRefreshRate = oldRefreshRate
		config.BufferSize = oldBufferSize
	}()

	// Set test configuration
	config.LogDir = tmpDir
	config.FileRefreshRate = 50 // Very fast refresh for testing
	config.BufferSize = 1024

	// Create mock connection with mutex-protected message slice
	mock := &mockConn{sentMessages: []TailResponse{}}

	// Create cancel channel
	cancel := make(chan bool)

	// Start tailing in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use the string that mockAddr.String() would return
		tailFile(mock, testFile, cancel, "test-client:12345")
	}()

	// Wait for initial content - use a timeout and polling approach
	initialMsgReceived := false
	startTime := time.Now()
	timeout := 2 * time.Second

	for time.Since(startTime) < timeout {
		mock.mu.Lock()
		if len(mock.sentMessages) > 0 {
			initialMsgReceived = true
			mock.mu.Unlock()
			break
		}
		mock.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}

	if !initialMsgReceived {
		t.Fatalf("Timed out waiting for initial message")
	}

	// Verify initial content was sent
	mock.mu.Lock()
	initialMsg := mock.sentMessages[0]
	if initialMsg.Type != "tail" {
		t.Errorf("Expected initial message type 'tail', got '%s'", initialMsg.Type)
	}

	if initialMsg.File != testFile {
		t.Errorf("Expected file name '%s', got '%s'", testFile, initialMsg.File)
	}

	if len(initialMsg.Lines) != 1 || !strings.Contains(initialMsg.Lines[0], initialContent) {
		t.Errorf("Expected initial content to contain '%s', got '%s'", initialContent, initialMsg.Lines[0])
	}
	mock.mu.Unlock()

	// Append to the file
	appendContent := "New line 1\nNew line 2\n"
	f, err := os.OpenFile(testFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file for appending: %v", err)
	}

	if _, err := f.WriteString(appendContent); err != nil {
		f.Close()
		t.Fatalf("Failed to append to file: %v", err)
	}
	f.Close()

	// Wait for update with a polling approach
	updateReceived := false
	startTime = time.Now()

	for time.Since(startTime) < timeout {
		mock.mu.Lock()
		for _, msg := range mock.sentMessages {
			if msg.Type == "update" && len(msg.Lines) > 0 && strings.Contains(msg.Lines[0], appendContent) {
				updateReceived = true
				break
			}
		}
		mock.mu.Unlock()

		if updateReceived {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Cancel tailing and wait for completion
	cancel <- true
	wg.Wait()

	// Test outcome
	if !updateReceived {
		t.Errorf("Update message with new content not found")
	}
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	// Test max function
	if max(5, 10) != 10 {
		t.Errorf("Expected max(5, 10) = 10, got %d", max(5, 10))
	}

	if max(10, 5) != 10 {
		t.Errorf("Expected max(10, 5) = 10, got %d", max(10, 5))
	}

	if max(0, -5) != 0 {
		t.Errorf("Expected max(0, -5) = 0, got %d", max(0, -5))
	}

	// Test min function
	if min(5, 10) != 5 {
		t.Errorf("Expected min(5, 10) = 5, got %d", min(5, 10))
	}

	if min(10, 5) != 5 {
		t.Errorf("Expected min(10, 5) = 5, got %d", min(10, 5))
	}

	if min(0, -5) != -5 {
		t.Errorf("Expected min(0, -5) = -5, got %d", min(0, -5))
	}
}

// TestParseLogDirs tests the parseLogDirs function
func TestParseLogDirs(t *testing.T) {
	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single directory",
			input:    "/var/log",
			expected: []string{"/var/log"},
		},
		{
			name:     "comma separated",
			input:    "/var/log,/tmp/logs,/data/logs",
			expected: []string{"/var/log", "/tmp/logs", "/data/logs"},
		},
		{
			name:     "semicolon separated",
			input:    "/var/log;/tmp/logs;/data/logs",
			expected: []string{"/var/log", "/tmp/logs", "/data/logs"},
		},
		{
			name:     "mixed separators",
			input:    "/var/log,/tmp/logs;/data/logs",
			expected: []string{"/var/log", "/tmp/logs", "/data/logs"},
		},
		{
			name:     "with spaces",
			input:    " /var/log , /tmp/logs ; /data/logs ",
			expected: []string{"/var/log", "/tmp/logs", "/data/logs"},
		},
		{
			name:     "with glob pattern",
			input:    "/var/log/*.log;/data/app*.log",
			expected: []string{"/var/log/*.log", "/data/app*.log"},
		},
		{
			name:     "trailing separator",
			input:    "/var/log,/tmp/logs,",
			expected: []string{"/var/log", "/tmp/logs"},
		},
		{
			name:     "empty string falls back to default",
			input:    "",
			expected: []string{"/logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.LogDir = tt.input
			result := parseLogDirs()
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d entries, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("Entry %d: expected '%s', got '%s'", i, exp, result[i])
				}
			}
		})
	}
}

// TestIsGlobPattern tests the isGlobPattern function
func TestIsGlobPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"/var/log", false},
		{"/var/log/", false},
		{"/var/log/*.log", true},
		{"/var/log/app?.log", true},
		{"/var/log/[a-z]*.log", true},
		{"*.log", true},
	}

	for _, tt := range tests {
		result := isGlobPattern(tt.input)
		if result != tt.expected {
			t.Errorf("isGlobPattern(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

// TestGetFileListMultiDir tests getFileList with multiple directories
func TestGetFileListMultiDir(t *testing.T) {
	// Create two temporary directories
	tmpDir1, err := os.MkdirTemp("", "webtail-multi1")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "webtail-multi2")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	testContent := []byte("test content")

	// Create files in dir1
	for _, fname := range []string{"app.log", "error.log"} {
		if err := os.WriteFile(filepath.Join(tmpDir1, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create files in dir2
	for _, fname := range []string{"access.log", "debug.log"} {
		if err := os.WriteFile(filepath.Join(tmpDir2, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	// Test with comma separator
	config.LogDir = tmpDir1 + "," + tmpDir2
	files, err := getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("Expected 4 files, got %d: %v", len(files), files)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	for _, expected := range []string{"app.log", "error.log", "access.log", "debug.log"} {
		if !fileSet[expected] {
			t.Errorf("Expected file %s not found in result", expected)
		}
	}

	// Test with semicolon separator
	config.LogDir = tmpDir1 + ";" + tmpDir2
	files, err = getFileList()
	if err != nil {
		t.Fatalf("getFileList failed with semicolon: %v", err)
	}
	if len(files) != 4 {
		t.Errorf("Expected 4 files with semicolon separator, got %d", len(files))
	}
}

// TestGetFileListGlob tests getFileList with glob patterns
func TestGetFileListGlob(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webtail-glob")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testContent := []byte("test content")

	// Create files with different extensions
	for _, fname := range []string{"app.log", "error.log", "data.csv", "config.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	// Test with glob pattern - only .log files
	config.LogDir = filepath.Join(tmpDir, "*.log")
	files, err := getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 .log files, got %d: %v", len(files), files)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	if !fileSet["app.log"] {
		t.Errorf("Expected app.log in results")
	}
	if !fileSet["error.log"] {
		t.Errorf("Expected error.log in results")
	}
	if fileSet["data.csv"] {
		t.Errorf("data.csv should not be in results for *.log pattern")
	}
}

// TestGetFileListGlobPrefix tests getFileList with prefix glob patterns like app*.log
func TestGetFileListGlobPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webtail-glob-prefix")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testContent := []byte("test content")

	for _, fname := range []string{"app-server.log", "app-client.log", "error.log", "app.data"} {
		if err := os.WriteFile(filepath.Join(tmpDir, fname), testContent, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	config.LogDir = filepath.Join(tmpDir, "app*.log")
	files, err := getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files matching app*.log, got %d: %v", len(files), files)
	}
}

// TestGetFileListDisambiguation tests that files with the same name in different dirs are disambiguated
func TestGetFileListDisambiguation(t *testing.T) {
	tmpDir1, err := os.MkdirTemp("", "webtail-disamb1")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "webtail-disamb2")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	testContent := []byte("test content")

	// Create a file with the same name in both directories
	if err := os.WriteFile(filepath.Join(tmpDir1, "app.log"), testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir2, "app.log"), testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	config.LogDir = tmpDir1 + "," + tmpDir2
	files, err := getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 disambiguated entries, got %d: %v", len(files), files)
	}

	// Both entries should contain a directory prefix
	for _, f := range files {
		if !strings.Contains(f, "/") {
			t.Errorf("Expected disambiguated name with '/', got '%s'", f)
		}
	}

	// Check that fileMap resolves correctly
	fileMapMutex.Lock()
	for _, fullPath := range fileMap {
		if _, err := os.Stat(fullPath); err != nil {
			t.Errorf("fileMap entry '%s' does not resolve to a valid file", fullPath)
		}
	}
	fileMapMutex.Unlock()
}

// TestResolveFilePath tests the resolveFilePath function
func TestResolveFilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webtail-resolve")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testContent := []byte("test content")
	if err := os.WriteFile(filepath.Join(tmpDir, "test.log"), testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	config.LogDir = tmpDir

	// Populate fileMap via getFileList
	_, err = getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	// Test resolving a known file
	resolved := resolveFilePath("test.log")
	expected := filepath.Join(tmpDir, "test.log")
	if resolved != expected {
		t.Errorf("Expected resolved path '%s', got '%s'", expected, resolved)
	}
}

// TestListFilesMultiDir tests the HTTP handler with multiple directories
func TestListFilesMultiDir(t *testing.T) {
	tmpDir1, err := os.MkdirTemp("", "webtail-http-multi1")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "webtail-http-multi2")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	testContent := []byte("test content")
	os.WriteFile(filepath.Join(tmpDir1, "a.log"), testContent, 0644)
	os.WriteFile(filepath.Join(tmpDir2, "b.log"), testContent, 0644)

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()

	config.LogDir = tmpDir1 + "," + tmpDir2

	req := httptest.NewRequest("GET", "/api/files", nil)
	w := httptest.NewRecorder()

	listFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var fileInfos []FileInfo
	if err := json.NewDecoder(w.Body).Decode(&fileInfos); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if len(fileInfos) != 2 {
		t.Errorf("Expected 2 files, got %d", len(fileInfos))
	}
}

// TestDownloadFile tests the download file endpoint
func TestDownloadFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webtail-download")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testContent := []byte("log line 1\nlog line 2\nlog line 3\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "test.log"), testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	oldLogDir := config.LogDir
	defer func() { config.LogDir = oldLogDir }()
	config.LogDir = tmpDir

	// Populate fileMap
	_, err = getFileList()
	if err != nil {
		t.Fatalf("getFileList failed: %v", err)
	}

	// Test successful download
	req := httptest.NewRequest("GET", "/api/download?file=test.log", nil)
	w := httptest.NewRecorder()
	downloadFile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentDisp := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisp, "test.log") {
		t.Errorf("Expected Content-Disposition to contain 'test.log', got '%s'", contentDisp)
	}

	if w.Body.String() != string(testContent) {
		t.Errorf("Expected body '%s', got '%s'", string(testContent), w.Body.String())
	}

	// Test missing file parameter
	req = httptest.NewRequest("GET", "/api/download", nil)
	w = httptest.NewRecorder()
	downloadFile(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing file param, got %d", w.Code)
	}

	// Test non-existent file
	req = httptest.NewRequest("GET", "/api/download?file=nonexistent.log", nil)
	w = httptest.NewRecorder()
	downloadFile(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent file, got %d", w.Code)
	}

	// Test wrong method
	req = httptest.NewRequest("POST", "/api/download?file=test.log", nil)
	w = httptest.NewRecorder()
	downloadFile(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", w.Code)
	}
}

// TestMiddleware tests the logging middleware
func TestMiddleware(t *testing.T) {
	// Create a test handler that just returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply the middleware
	handlerWithMiddleware := logRequestMiddleware(testHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test-path", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handlerWithMiddleware.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
