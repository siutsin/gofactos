// This file protects root command discovery and shell completion.
package app

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandSubcommands verifies that NewCommand registers the expected
// subcommands.
func TestNewCommandSubcommands(t *testing.T) {
	command := NewCommand()

	require.Len(t, command.Commands, 2)
	assert.Equal(t, "analyse", command.Commands[0].Name)
	assert.Equal(t, "blueprint", command.Commands[1].Name)
}

// TestNewCommandVersion verifies that --version reports build metadata, the Go
// toolchain version, and the target platform.
func TestNewCommandVersion(t *testing.T) {
	var buf bytes.Buffer
	command := NewCommand()
	command.Writer = &buf

	err := command.Run(context.Background(), []string{"gofactos", "--version"})

	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "gofactos version")
	assert.Contains(t, out, "built")
	assert.Contains(t, out, runtime.Version())
	assert.Contains(t, out, runtime.GOOS+"/"+runtime.GOARCH)
}

// TestNewCommandShellCompletion verifies that NewCommand enables shell
// completion.
func TestNewCommandShellCompletion(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
	})
	os.Args = []string{
		"gofactos",
		"blu",
		"--generate-shell-completion",
	}

	var buf bytes.Buffer
	command := NewCommand()
	command.Writer = &buf

	err := command.Run(context.Background(), os.Args)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "blueprint:")
}
