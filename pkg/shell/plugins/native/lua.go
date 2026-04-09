// Package native contains the built-in native Go plugins shipped with memsh.
package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	lua "github.com/yuin/gopher-lua"
	"mvdan.cc/sh/v3/interp"
)

// LuaPlugin executes Lua code using gopher-lua.
//
//	lua [-e 'code']        execute inline Lua code
//	lua [file.lua]         execute Lua file from virtual filesystem
//	                    (reads Lua code from stdin when no args)
type LuaPlugin struct{}

func (LuaPlugin) Name() string        { return "lua" }
func (LuaPlugin) Description() string { return "execute Lua code using gopher-lua" }
func (LuaPlugin) Usage() string       { return "lua [-e 'code' | file.lua]" }

func (LuaPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	var code string

	// Parse arguments
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-e":
			if i+1 < len(args) {
				code = args[i+1]
				i++
			} else {
				return fmt.Errorf("lua: -e requires an argument")
			}
		default:
			// Treat as a file path
			filePath := sc.ResolvePath(args[i])
			f, err := sc.FS.Open(filePath)
			if err != nil {
				return fmt.Errorf("lua: %s: %w", args[i], err)
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("lua: %s: read error: %w", args[i], err)
			}
			code = string(data)
		}
	}

	// If no code provided, read from stdin
	if code == "" {
		input, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return fmt.Errorf("lua: read stdin: %w", err)
		}
		code = string(input)
	}

	// Create new Lua state
	L := lua.NewState()
	defer L.Close()

	// Capture print output
	printBuf := &strings.Builder{}
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		for i := 1; i <= top; i++ {
			if i > 1 {
				printBuf.WriteString("\t")
			}
			val := L.CheckAny(i)
			printBuf.WriteString(val.String())
		}
		printBuf.WriteString("\n")
		return 0
	}))

	// Provide filesystem access via fs module
	L.SetGlobal("fs_readfile", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		resolvedPath := sc.ResolvePath(path)
		f, err := sc.FS.Open(resolvedPath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(data)))
		return 1
	}))

	// Execute the code
	if err := L.DoString(code); err != nil {
		return fmt.Errorf("lua: %w", err)
	}

	// Write captured output
	if printBuf.Len() > 0 {
		if _, err := hc.Stdout.Write([]byte(printBuf.String())); err != nil {
			return err
		}
	}

	return nil
}

// ensure LuaPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = LuaPlugin{}
