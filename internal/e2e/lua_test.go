// This file embeds readable Lua programs used by the Factorio E2E suite.
package e2e

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed testdata/*.lua
var luaFiles embed.FS

type luaArg struct {
	name  string
	value string
}

// luaCommand prefixes an embedded Lua program with its typed local arguments.
func luaCommand(name string, args ...luaArg) string {
	raw, err := luaFiles.ReadFile("testdata/" + name)
	if err != nil {
		panic(fmt.Sprintf("read embedded Lua program %q: %v", name, err))
	}

	var command strings.Builder
	command.WriteString("/silent-command ")
	for _, arg := range args {
		command.WriteString("local ")
		command.WriteString(arg.name)
		command.WriteByte('=')
		command.WriteString(arg.value)
		command.WriteByte(';')
	}
	command.WriteString(flattenLua(name, string(raw)))
	return command.String()
}

// luaString returns one string-valued Lua local argument.
func luaString(name, value string) luaArg {
	return luaArg{name: name, value: strconv.Quote(value)}
}

// luaInt returns one integer-valued Lua local argument.
func luaInt(name string, value int) luaArg {
	return luaArg{name: name, value: strconv.Itoa(value)}
}

// luaBool returns one Boolean-valued Lua local argument.
func luaBool(name string, value bool) luaArg {
	return luaArg{name: name, value: strconv.FormatBool(value)}
}

// flattenLua converts a readable program into the single line RCON expects.
func flattenLua(name, source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	lines := strings.Split(source, "\n")
	flattened := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "--") {
			panic(fmt.Sprintf(
				"embedded Lua program %q contains a line comment",
				name,
			))
		}
		flattened = append(flattened, line)
	}
	return strings.Join(flattened, " ")
}
