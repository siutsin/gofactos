// This file starts and isolates the headless Factorio process used by E2E.
package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	factorioE2EEnv          = "GOFACTOS_FACTORIO_E2E"
	factorioStartAttempts   = 3
	factorioShutdownTimeout = 10 * time.Second
	factorioReapTimeout     = 2 * time.Second
)

type e2ePaths struct {
	factorio string
	data     string
	mods     string
	save     string
	gofactos string
	root     string
}

type factorioServer struct {
	t        *testing.T
	client   *rconClient
	command  *exec.Cmd
	context  context.Context
	cancel   context.CancelFunc
	done     chan error
	log      *os.File
	logPath  string
	password string
	address  string
	waited   bool
}

type processExit struct {
	err    error
	exited bool
}

type factorioModList struct {
	Mods []factorioMod `json:"mods"`
}

type factorioMod struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// resolveE2EPaths honours explicit overrides while validating every E2E input.
func resolveE2EPaths(t *testing.T) e2ePaths {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	userData := filepath.Join(home, ".factorio")
	factorio := ""
	if runtime.GOOS == "darwin" {
		userData = filepath.Join(
			home,
			"Library/Application Support/factorio",
		)
		factorio = filepath.Join(
			home,
			"Library/Application Support/Steam/steamapps/common/Factorio",
			"factorio.app/Contents/MacOS/factorio",
		)
	}
	factorio = envOr("GOFACTOS_FACTORIO_BIN", factorio)
	if factorio == "" {
		factorio, err = exec.LookPath("factorio")
		require.NoError(t, err)
	}

	paths := e2ePaths{
		factorio: factorio,
		mods: envOr(
			"GOFACTOS_FACTORIO_MOD_DIR",
			filepath.Join(userData, "mods"),
		),
		save: envOr(
			"GOFACTOS_FACTORIO_SAVE",
			filepath.Join(userData, "saves", "gofactos.zip"),
		),
		gofactos: envOr(
			"GOFACTOS_BIN",
			filepath.Join(root, "build", "gofactos"),
		),
		root: root,
	}
	paths.data = os.Getenv("GOFACTOS_FACTORIO_DATA")
	if paths.data == "" {
		paths.data = findFactorioData(t, paths.factorio)
	}
	requireRegularFile(t, paths.factorio)
	requireRegularFile(t, paths.save)
	requireRegularFile(t, paths.gofactos)
	require.DirExists(t, paths.data)
	require.DirExists(t, paths.mods)
	return paths
}

// envOr lets local installations override portable path defaults.
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// findFactorioData supports common installations where data sits by the binary.
func findFactorioData(t *testing.T, binary string) string {
	t.Helper()
	directory := filepath.Dir(binary)
	for _, candidate := range []string{
		filepath.Join(directory, "..", "data"),
		filepath.Join(directory, "..", "..", "data"),
	} {
		candidate = filepath.Clean(candidate)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	t.Fatalf("cannot find Factorio data directory beside %s", binary)
	return ""
}

// requireRegularFile rejects missing or non-regular E2E dependencies.
func requireRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular(), "%s is not a regular file", path)
}

// startFactorio launches a test-owned server from the prepared gofactos save.
func startFactorio(t *testing.T, paths e2ePaths) *factorioServer {
	t.Helper()
	work := t.TempDir()
	for _, directory := range []string{
		filepath.Join(work, "saves"),
		filepath.Join(work, "script-output"),
		filepath.Join(work, "temp"),
	} {
		require.NoError(t, os.MkdirAll(directory, 0o750))
	}

	save := filepath.Join(work, "saves", "gofactos-e2e.zip")
	require.NoError(t, copyFile(paths.save, save))
	mods := filepath.Join(work, "mods")
	require.NoError(t, stageMods(paths.mods, mods))
	config := filepath.Join(work, "config.ini")
	settings := filepath.Join(work, "server-settings.json")
	require.NoError(t, writeFactorioConfig(config, paths.data, work))
	require.NoError(t, writeServerSettings(settings))

	for attempt := 1; attempt <= factorioStartAttempts; attempt++ {
		server, launchErr := launchFactorio(t, paths, work, attempt)
		if launchErr != nil {
			t.Fatalf("launch Factorio: %v", launchErr)
		}
		startupErr := server.waitForRCON(time.Minute)
		if startupErr == nil {
			t.Cleanup(server.close)
			return server
		}

		cleanupErr := server.abortStart()
		logOutput := server.readLog()
		if cleanupErr != nil {
			t.Fatalf(
				"clean up failed Factorio start: %v\n%s",
				errors.Join(startupErr, cleanupErr),
				logOutput,
			)
		}
		if !shouldRetryFactorioStart(attempt, logOutput) {
			t.Fatalf("start Factorio: %v\n%s", startupErr, logOutput)
		}
		t.Logf(
			"retry Factorio after port collision (%d/%d)",
			attempt,
			factorioStartAttempts,
		)
	}
	t.Fatal("Factorio startup attempts exhausted")
	return nil
}

// launchFactorio starts one server attempt with newly allocated ports.
func launchFactorio(
	t *testing.T,
	paths e2ePaths,
	work string,
	attempt int,
) (*factorioServer, error) {
	t.Helper()
	gamePort := freePort(t, "udp4")
	rconPort := freePort(t, "tcp4")
	password := randomPassword(t)
	logPath := filepath.Join(work, fmt.Sprintf("server-%d.log", attempt))
	// The path is under the test-owned temporary directory.
	logFile, err := os.Create(logPath) //nolint:gosec // Controlled test path.
	if err != nil {
		return nil, fmt.Errorf("create Factorio log: %w", err)
	}

	args := []string{
		"--config", filepath.Join(work, "config.ini"),
		"--mod-directory", filepath.Join(work, "mods"),
		"--start-server", filepath.Join(work, "saves", "gofactos-e2e.zip"),
		"--server-settings", filepath.Join(work, "server-settings.json"),
		"--bind", "127.0.0.1",
		"--port", fmt.Sprint(gamePort),
		"--rcon-bind", fmt.Sprintf("127.0.0.1:%d", rconPort),
		"--rcon-password", password,
		"--server-id", filepath.Join(
			work,
			fmt.Sprintf("server-id-%d.json", attempt),
		),
		"--no-log-rotation",
	}
	serverContext, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext( //nolint:gosec // Explicit local E2E binary.
		serverContext,
		paths.factorio,
		args...,
	)
	command.Stdout = logFile
	command.Stderr = logFile
	if startErr := command.Start(); startErr != nil {
		cancel()
		closeErr := logFile.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("close Factorio log: %w", closeErr)
		}
		return nil, errors.Join(
			fmt.Errorf("start Factorio process: %w", startErr),
			closeErr,
		)
	}

	server := &factorioServer{
		t:        t,
		command:  command,
		context:  serverContext,
		cancel:   cancel,
		done:     make(chan error, 1),
		log:      logFile,
		logPath:  logPath,
		password: password,
		address:  fmt.Sprintf("127.0.0.1:%d", rconPort),
	}
	go func() {
		server.done <- command.Wait()
	}()
	return server, nil
}

// copyFile stages test inputs while retaining read, write, and close failures.
func copyFile(source, destination string) (returnErr error) {
	// Both paths are either validated inputs or test-owned temporary paths.
	input, err := os.Open(source) //nolint:gosec // Intentional local file copy.
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer func() {
		if closeErr := input.Close(); closeErr != nil {
			returnErr = errors.Join(
				returnErr,
				fmt.Errorf("close %s: %w", source, closeErr),
			)
		}
	}()
	output, err := os.OpenFile( //nolint:gosec // Test-owned destination.
		destination,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	defer func() {
		if err := output.Close(); err != nil {
			returnErr = errors.Join(
				returnErr,
				fmt.Errorf("close %s: %w", destination, err),
			)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy %s: %w", source, err)
	}
	return nil
}

// stageMods gives the server an isolated copy of only its enabled dependencies.
func stageMods(source, destination string) error {
	if makeErr := os.MkdirAll(destination, 0o750); makeErr != nil {
		return fmt.Errorf("create staged mod directory: %w", makeErr)
	}
	modListPath := filepath.Join(source, "mod-list.json")
	enabled, readErr := readEnabledMods(modListPath)
	if readErr != nil {
		return readErr
	}
	copyErr := copyFile(
		modListPath,
		filepath.Join(destination, "mod-list.json"),
	)
	if copyErr != nil {
		return copyErr
	}
	if settingsErr := stageModSettings(source, destination); settingsErr != nil {
		return settingsErr
	}
	return stageEnabledMods(source, destination, enabled)
}

// stageModSettings preserves optional settings required by enabled mods.
func stageModSettings(source, destination string) error {
	settingsPath := filepath.Join(source, "mod-settings.dat")
	if _, statErr := os.Stat(settingsPath); statErr == nil {
		if copyErr := copyFile(
			settingsPath,
			filepath.Join(destination, "mod-settings.dat"),
		); copyErr != nil {
			return copyErr
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat mod settings: %w", statErr)
	}
	return nil
}

// stageEnabledMods avoids loading unrelated archives from the user's profile.
func stageEnabledMods(
	source, destination string,
	enabled map[string]struct{},
) error {
	entries, readErr := os.ReadDir(source)
	if readErr != nil {
		return fmt.Errorf("read mod directory: %w", readErr)
	}
	for _, entry := range entries {
		name := entry.Name()
		from := filepath.Join(source, name)
		to := filepath.Join(destination, name)
		isDirectory := entry.IsDir()
		if !isDirectory && !entry.Type().IsRegular() {
			continue
		}
		if !enabledModEntry(name, isDirectory, enabled) {
			continue
		}
		if isDirectory {
			if copyErr := copyDirectory(from, to); copyErr != nil {
				return copyErr
			}
			continue
		}
		if copyErr := copyFile(from, to); copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// readEnabledMods makes mod-list.json authoritative for E2E staging.
func readEnabledMods(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Explicit local mod list.
	if err != nil {
		return nil, fmt.Errorf("read mod list: %w", err)
	}
	var list factorioModList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse mod list: %w", err)
	}
	enabled := make(map[string]struct{})
	for _, mod := range list.Mods {
		if mod.Enabled {
			enabled[mod.Name] = struct{}{}
		}
	}
	return enabled, nil
}

// enabledModEntry matches exact unpacked or regular ZIP mod identities.
func enabledModEntry(
	name string,
	isDirectory bool,
	enabled map[string]struct{},
) bool {
	base := name
	if !isDirectory {
		if filepath.Ext(base) != ".zip" {
			return false
		}
		base = strings.TrimSuffix(base, ".zip")
	}
	if _, ok := enabled[base]; ok {
		return true
	}
	modName, ok := stripDottedModVersion(base)
	if !ok {
		return false
	}
	_, ok = enabled[modName]
	return ok
}

// stripDottedModVersion removes only a terminal dotted numeric version.
func stripDottedModVersion(name string) (string, bool) {
	separator := strings.LastIndexByte(name, '_')
	if separator <= 0 || separator == len(name)-1 {
		return "", false
	}
	parts := strings.Split(name[separator+1:], ".")
	if len(parts) < 2 {
		return "", false
	}
	for _, part := range parts {
		if part == "" {
			return "", false
		}
		for _, digit := range []byte(part) {
			if digit < '0' || digit > '9' {
				return "", false
			}
		}
	}
	return name[:separator], true
}

// copyDirectory stages unpacked mods without following unsafe symlinks.
func copyDirectory(source, destination string) error {
	return filepath.WalkDir(
		source,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk mod directory: %w", walkErr)
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return fmt.Errorf("find mod path relative to source: %w", err)
			}
			target := filepath.Join(destination, relative)
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("enabled mod contains symlink: %s", path)
			}
			if entry.IsDir() {
				if err := os.MkdirAll(target, 0o750); err != nil {
					return fmt.Errorf("create mod directory: %w", err)
				}
				return nil
			}
			return copyFile(path, target)
		},
	)
}

// writeFactorioConfig confines mutable Factorio data to the test directory.
func writeFactorioConfig(path, readData, writeData string) error {
	config := fmt.Sprintf(
		"; Generated isolated E2E configuration.\n"+
			"[path]\nread-data=%s\nwrite-data=%s\n\n"+
			"[general]\nlocale=en\n\n"+
			"[other]\nverbose-logging=false\n",
		readData,
		writeData,
	)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		return fmt.Errorf("write Factorio config: %w", err)
	}
	return nil
}

// writeServerSettings keeps the local E2E server private and deterministic.
func writeServerSettings(path string) error {
	settings := map[string]any{
		"name":        "gofactos E2E",
		"description": "Local circuit verification only",
		"visibility": map[string]bool{
			"public": false,
			"lan":    false,
		},
		"require_user_verification":       false,
		"auto_pause":                      false,
		"auto_pause_when_players_connect": false,
		"autosave_interval":               0,
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal server settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write server settings: %w", err)
	}
	return nil
}

// freePort reduces collisions between concurrent local E2E runs.
func freePort(t *testing.T, network string) int {
	t.Helper()
	address := "127.0.0.1:0"
	if network == "udp4" {
		config := net.ListenConfig{}
		connection, err := config.ListenPacket(t.Context(), network, address)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, connection.Close())
		}()
		udpAddress, ok := connection.LocalAddr().(*net.UDPAddr)
		require.True(t, ok)
		return udpAddress.Port
	}
	config := net.ListenConfig{}
	listener, err := config.Listen(t.Context(), network, address)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, listener.Close())
	}()
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return tcpAddress.Port
}

// randomPassword isolates this server's RCON control channel.
func randomPassword(t *testing.T) string {
	t.Helper()
	buffer := make([]byte, 16)
	_, err := rand.Read(buffer)
	require.NoError(t, err)
	return hex.EncodeToString(buffer)
}

// waitForRCON distinguishes slow startup from an early server exit.
func (s *factorioServer) waitForRCON(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = s.connectRCON()
		if lastErr == nil {
			return nil
		}
		select {
		case exitErr := <-s.done:
			s.waited = true
			if exitErr == nil {
				return errors.New("Factorio exited before RCON was ready")
			}
			return fmt.Errorf(
				"Factorio exited before RCON was ready: %w",
				exitErr,
			)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if lastErr == nil {
		return errors.New("RCON was not ready before its deadline")
	}
	return fmt.Errorf("RCON was not ready: %w", lastErr)
}

// connectRCON performs one readiness probe and retains a successful client.
func (s *factorioServer) connectRCON() error {
	client, err := dialRCON(s.context, s.address, s.password)
	if err != nil {
		return fmt.Errorf("dial RCON: %w", err)
	}
	output, commandErr := client.command(
		`/silent-command rcon.print("ready")`,
	)
	if commandErr == nil && output == "ready" {
		s.client = client
		return nil
	}
	if commandErr == nil {
		commandErr = fmt.Errorf(
			"unexpected RCON readiness response %q",
			output,
		)
	}
	if closeErr := client.conn.Close(); closeErr != nil {
		commandErr = errors.Join(
			commandErr,
			fmt.Errorf("close failed RCON connection: %w", closeErr),
		)
	}
	return commandErr
}

// shouldRetryFactorioStart permits bounded retries only for port collisions.
func shouldRetryFactorioStart(attempt int, logOutput string) bool {
	return attempt < factorioStartAttempts && isBindCollision(logOutput)
}

// isBindCollision recognises operating-system address-in-use diagnostics.
func isBindCollision(logOutput string) bool {
	lower := strings.ToLower(logOutput)
	return strings.Contains(lower, "address already in use") ||
		strings.Contains(lower, "only one usage of each socket address") ||
		strings.Contains(lower, "eaddrinuse")
}

// abortStart stops and reaps an unsuccessful launch without test callbacks.
func (s *factorioServer) abortStart() error {
	s.cancel()
	var resultErr error
	if s.client != nil {
		if closeErr := s.client.conn.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("close failed RCON connection: %w", closeErr),
			)
		}
		s.client = nil
	}
	if !s.waited {
		exit := s.waitForProcessExit(
			factorioShutdownTimeout,
		)
		if !exit.exited {
			killErr := s.command.Process.Kill()
			if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("kill failed Factorio start: %w", killErr),
				)
			}
			reap := s.waitForProcessExit(
				factorioReapTimeout,
			)
			if !reap.exited {
				resultErr = errors.Join(
					resultErr,
					errors.New("Factorio start was not reaped after kill"),
				)
			}
		}
	}
	if closeErr := s.log.Close(); closeErr != nil {
		resultErr = errors.Join(
			resultErr,
			fmt.Errorf("close Factorio log: %w", closeErr),
		)
	}
	return resultErr
}

// run makes every E2E RCON failure immediately test-fatal.
func (s *factorioServer) run(t *testing.T, command string) string {
	t.Helper()
	output, err := s.client.command(command)
	require.NoError(t, err)
	return output
}

// close guarantees server, connection, and log cleanup after each test.
func (s *factorioServer) close() {
	s.closeRCON()
	s.waitForExit()
	s.cancel()
	if err := s.log.Close(); err != nil {
		s.t.Logf("close Factorio log: %v", err)
	}
}

// closeRCON requests graceful shutdown before releasing the control channel.
func (s *factorioServer) closeRCON() {
	if s.client != nil {
		if _, err := s.client.command("/quit"); err != nil {
			s.t.Logf("quit Factorio over RCON: %v", err)
		}
		if err := s.client.conn.Close(); err != nil {
			s.t.Logf("close RCON: %v", err)
		}
	}
}

// waitForExit prevents a failed E2E test from leaking a Factorio process.
func (s *factorioServer) waitForExit() {
	if !s.waited {
		exit := s.waitForProcessExit(factorioShutdownTimeout)
		if exit.exited {
			if exit.err != nil && !errors.Is(exit.err, context.Canceled) {
				s.t.Errorf("Factorio exit: %v\n%s", exit.err, s.readLog())
			}
			return
		}

		s.cancel()
		reap := s.waitForProcessExit(
			factorioReapTimeout,
		)
		if !reap.exited {
			s.t.Errorf(
				"Factorio did not stop within %s and was not reaped within %s",
				factorioShutdownTimeout,
				factorioReapTimeout,
			)
			return
		}
		s.t.Errorf("Factorio did not stop within %s", factorioShutdownTimeout)
	}
}

// waitForProcessExit bounds every attempt to reap the Factorio child.
func (s *factorioServer) waitForProcessExit(
	timeout time.Duration,
) processExit {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-s.done:
		s.waited = true
		return processExit{err: err, exited: true}
	case <-timer.C:
		return processExit{}
	}
}

// readLog attaches server diagnostics to startup and shutdown failures.
func (s *factorioServer) readLog() string {
	data, err := os.ReadFile(s.logPath)
	if err != nil {
		return fmt.Sprintf("read log: %v", err)
	}
	return string(data)
}

// TestEnabledModEntry verifies exact identities and dotted numeric versions.
func TestEnabledModEntry(t *testing.T) {
	enabled := map[string]struct{}{"space_mod": {}}
	for _, tc := range []struct {
		name        string
		entry       string
		directory   bool
		wantEnabled bool
	}{
		{
			name: "exact directory", entry: "space_mod",
			directory: true, wantEnabled: true,
		},
		{
			name: "exact archive", entry: "space_mod.zip",
			wantEnabled: true,
		},
		{
			name: "versioned directory", entry: "space_mod_1.2.3",
			directory: true, wantEnabled: true,
		},
		{
			name: "versioned archive", entry: "space_mod_1.2.3.zip",
			wantEnabled: true,
		},
		{name: "regular non-ZIP", entry: "space_mod.dat"},
		{name: "regular bare name", entry: "space_mod"},
		{name: "undotted suffix", entry: "space_mod_1.zip"},
		{name: "non-numeric version", entry: "space_mod_1.2.rc1.zip"},
		{
			name:  "prefixed different mod",
			entry: "space_mod_extra_1.2.3.zip",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(
				t,
				tc.wantEnabled,
				enabledModEntry(tc.entry, tc.directory, enabled),
			)
		})
	}
}

// TestShouldRetryFactorioStart verifies collision-only bounded retries.
func TestShouldRetryFactorioStart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attempt int
		log     string
		want    bool
	}{
		{
			name: "Unix collision", attempt: 1,
			log: "bind: Address already in use", want: true,
		},
		{
			name: "Windows collision", attempt: 2,
			log: "Only one usage of each socket address", want: true,
		},
		{
			name: "last attempt", attempt: factorioStartAttempts,
			log: "EADDRINUSE",
		},
		{
			name: "permission failure", attempt: 1,
			log: "failed to bind socket: permission denied",
		},
		{name: "unrelated exit", attempt: 1, log: "invalid mod archive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(
				t,
				tc.want,
				shouldRetryFactorioStart(tc.attempt, tc.log),
			)
		})
	}
}

// TestWaitForProcessExitIsBounded proves cleanup can report an unreaped child
// instead of blocking the entire test process indefinitely.
func TestWaitForProcessExitIsBounded(t *testing.T) {
	server := &factorioServer{done: make(chan error, 1)}

	exit := server.waitForProcessExit(time.Millisecond)

	require.NoError(t, exit.err)
	assert.False(t, exit.exited)
	assert.False(t, server.waited)

	wantErr := errors.New("process exited")
	server.done <- wantErr
	exit = server.waitForProcessExit(time.Second)

	require.ErrorIs(t, exit.err, wantErr)
	assert.True(t, exit.exited)
	assert.True(t, server.waited)
}
