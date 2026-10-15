// owned-image only prepares data; it does not load or execute an image.
package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

const usage = "usage: owned-image pack --binary FILE --marker FILE | inspect --archive FILE --binary FILE --marker FILE\n"

func run(args []string, out, errout io.Writer) int {
	report := func(code int, s string) int {
		if _, e := io.WriteString(errout, s); e != nil {
			return 1
		}
		return code
	}
	if len(args) == 1 && args[0] == "--help" || len(args) == 2 && (args[0] == "pack" || args[0] == "inspect") && args[1] == "--help" {
		n, e := io.WriteString(out, usage)
		if e != nil || n != len(usage) {
			return report(1, "owned-image: output failed\n")
		}
		return 0
	}
	if len(args) == 0 || (args[0] != "pack" && args[0] != "inspect") {
		return report(2, usage)
	}
	flags := map[string]string{}
	for i := 1; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return report(2, usage)
		}
		k := args[i]
		if k != "--binary" && k != "--marker" && (k != "--archive" || args[0] != "inspect") {
			return report(2, usage)
		}
		if flags[k] != "" || args[i+1] == "" {
			return report(2, usage)
		}
		flags[k] = args[i+1]
	}
	if flags["--binary"] == "" || flags["--marker"] == "" || args[0] == "inspect" && flags["--archive"] == "" {
		return report(2, usage)
	}
	var data []byte
	var e error
	if args[0] == "pack" {
		data, e = ownedimage.Pack(flags["--binary"], flags["--marker"])
	} else {
		var a []byte
		a, e = ownedimage.ReadFile(flags["--archive"], ownedimage.MaxArchive)
		if e == nil {
			var m ownedimage.Metadata
			m, e = ownedimage.Inspect(a, flags["--binary"], flags["--marker"])
			if e == nil {
				data, e = json.Marshal(m)
				data = append(data, '\n')
			}
		}
	}
	if e != nil {
		return report(1, "owned-image: invalid input\n")
	}
	n, e := out.Write(data)
	if e != nil || n != len(data) {
		return report(1, "owned-image: output failed\n")
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
