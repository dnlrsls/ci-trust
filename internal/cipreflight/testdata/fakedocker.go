// This owned test executable never contacts Docker or executes target code.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "hold-output" {
		time.Sleep(900 * time.Millisecond)
		if err := os.WriteFile(os.Args[2], []byte("done"), 0600); err != nil {
			panic(err)
		}
		return
	}
	f, err := os.OpenFile("calls", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	if err = json.NewEncoder(f).Encode(map[string]any{"args": os.Args[1:], "env": os.Environ()}); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	b, err := os.ReadFile("scenario")
	if err != nil {
		panic(err)
	}
	switch string(b) {
	case "unavailable":
		fmt.Fprintln(os.Stderr, "SECRET-MARKER infrastructure details")
		os.Exit(9)
	case "malformed":
		fmt.Print("SECRET-MARKER not json")
		return
	case "oversize":
		fmt.Print(strings.Repeat("x", 128<<10))
		return
	case "timeout":
		time.Sleep(5 * time.Second)
		return
	case "stderr-budget":
		fmt.Fprint(os.Stderr, strings.Repeat("x", 128<<10))
	case "pipes":
		raw, err := os.ReadFile("child-spec")
		if err != nil {
			panic(err)
		}
		var spec struct{ Executable, Done string }
		if err = json.Unmarshal(raw, &spec); err != nil {
			panic(err)
		}
		c := exec.Command(spec.Executable, "hold-output", spec.Done)
		c.Dir = filepath.Dir(spec.Executable)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err = c.Start(); err != nil {
			panic(err)
		}
		if err = c.Process.Release(); err != nil {
			panic(err)
		}
	}
	for _, a := range os.Args[1:] {
		if a == "version" {
			fmt.Print(`{"Version":"28.3.3","Os":"linux","Arch":"amd64","Secret":"SECRET-MARKER"}`)
			return
		}
	}
	fmt.Print(`{"OSType":"linux","Architecture":"x86_64","CgroupVersion":"2","NCPU":4,"MemTotal":8589934592,"MemoryLimit":true,"SwapLimit":true,"PidsLimit":true,"CpuCfsQuota":true,"Name":"SECRET-MARKER","Labels":["SECRET-MARKER"]}`)
}
