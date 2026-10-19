package discovery

import (
	"errors"
	"io"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
)

// ReadSnapshot reads known static markers and root workflow YAML without evaluation.
// Zero limits use defaults. Shared hard caps are depth/script depth 64,
// entries 65536, reference edges 4096, individual bytes 16 MiB and aggregate 64 MiB. Directory
// enumeration is incremental; an overflowing directory is omitted in full rather
// than selecting an unstable filesystem-order subset. Counts include every entry.
// Read attempts, including rejected files, consume the aggregate byte budget;
// at most one extra byte is read to detect overflow. Excluded directories are
// .git, node_modules, vendor, .venv, dist, build and .next. All symlinks and
// non-regular files are omitted. os.Root does not prohibit mounted filesystems.
// This is not an atomic snapshot: concurrent filesystem changes may cause omissions.
func ReadSnapshot(rootPath string, options Options) Snapshot {
	s := Snapshot{Documents: []Document{}, Diagnostics: []Diagnostic{}, Limitations: []string{}}
	add := func(code, p, message string) {
		s.Diagnostics = append(s.Diagnostics, Diagnostic{Code: code, Path: p, Message: message, State: "UNKNOWN"})
	}
	var valid bool
	options, valid = normalizeOptions(options)
	if !valid {
		add("INVALID_OPTIONS", ".", "Options exceed supported bounds.")
		return s
	}
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		add("UNSUPPORTED_HOST", ".", "Host cannot provide required confinement.")
		s.Limitations = append(s.Limitations, "Discovery unavailable: host lacks required confinement.")
		return s
	}
	info, err := os.Lstat(rootPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		add("ROOT_UNAVAILABLE", ".", "Root cannot be opened.")
		return s
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		add("ROOT_UNAVAILABLE", ".", "Root cannot be opened.")
		return s
	}
	defer root.Close()
	markers := map[string]bool{}
	for _, n := range []string{"go.mod", "package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", ".eslintrc", ".eslintrc.json", "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".golangci.yml", ".golangci.yaml", "pyproject.toml", "requirements.txt", "Cargo.toml", "pom.xml"} {
		markers[n] = true
	}
	excluded := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true, "dist": true, "build": true, ".next": true}
	visited := 0
	var total int64
	exhausted := false
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if exhausted {
			add("ENTRY_LIMIT", dir, "Directory omitted: entry limit reached.")
			return
		}
		if dir != "." {
			// Inspect only the entry: gitdir payloads and symlink targets are not inputs.
			_, err := root.Lstat(path.Join(dir, ".git"))
			if err == nil {
				add("EXCLUDED_REPOSITORY", dir, "Nested repository excluded from selected root.")
				return
			}
			if !errors.Is(err, os.ErrNotExist) {
				add("REPOSITORY_MARKER_READ", dir, "Directory omitted: repository marker cannot be inspected.")
				return
			}
		}
		f, err := root.Open(dir)
		if err != nil {
			add("DIRECTORY_READ", dir, "Directory cannot be read.")
			return
		}
		// Recheck after open because directory entries may change during traversal.
		st, err := f.Stat()
		if err != nil || !st.IsDir() {
			f.Close()
			add("DIRECTORY_READ", dir, "Directory cannot be read.")
			return
		}
		entries := []os.DirEntry{}
		for {
			remaining := options.MaxEntries - visited
			batch := remaining + 1
			if batch > 128 {
				batch = 128
			}
			if batch < 1 {
				batch = 1
			}
			chunk, e := f.ReadDir(batch)
			visited += len(chunk)
			if visited > options.MaxEntries {
				exhausted = true
				f.Close()
				add("ENTRY_LIMIT", dir, "Directory omitted: entry limit reached.")
				return
			}
			entries = append(entries, chunk...)
			if e == io.EOF {
				break
			}
			if e != nil {
				f.Close()
				add("DIRECTORY_READ", dir, "Directory cannot be read.")
				return
			}
		}
		f.Close()
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		dirs := []string{}
		for _, entry := range entries {
			p := path.Join(dir, entry.Name())
			// Lstat is confined; do not trust cached directory entry types.
			st, e := root.Lstat(p)
			if e != nil {
				add("ENTRY_READ", p, "Entry cannot be inspected.")
				continue
			}
			if st.Mode()&os.ModeSymlink != 0 {
				add("SYMLINK_SKIPPED", p, "Symbolic link omitted.")
				continue
			}
			if st.IsDir() {
				if excluded[entry.Name()] {
					add("EXCLUDED_DIRECTORY", p, "Directory excluded from discovery.")
					continue
				}
				if depth >= options.MaxDepth {
					add("DEPTH_LIMIT", p, "Directory exceeds depth limit.")
					continue
				}
				dirs = append(dirs, p)
				continue
			}
			if !st.Mode().IsRegular() {
				add("SPECIAL_FILE_SKIPPED", p, "Non-regular entry omitted.")
				continue
			}
			if !markers[entry.Name()] && !workflowDocument(p) && p != ".ci-trust.json" {
				continue
			}
			if total >= options.MaxTotalBytes {
				add("TOTAL_LIMIT", p, "Aggregate byte limit reached.")
				continue
			}
			file, e := root.Open(p)
			if e != nil {
				add("FILE_READ", p, "File cannot be read.")
				continue
			}
			st, e = file.Stat()
			if e != nil || !st.Mode().IsRegular() {
				file.Close()
				add("FILE_READ", p, "File cannot be read.")
				continue
			}
			limit := options.MaxFileBytes
			remaining := options.MaxTotalBytes - total
			if remaining < limit {
				limit = remaining
			}
			content, e := io.ReadAll(io.LimitReader(file, limit+1))
			closeErr := file.Close()
			// Charge attempted reads too, including rejected or partially read files.
			total += int64(len(content))
			if total > options.MaxTotalBytes {
				total = options.MaxTotalBytes
			}
			if e != nil || closeErr != nil {
				add("FILE_READ", p, "File cannot be read.")
				continue
			}
			if int64(len(content)) > limit {
				if remaining < options.MaxFileBytes {
					add("TOTAL_LIMIT", p, "Aggregate byte limit reached.")
				} else {
					add("FILE_LIMIT", p, "File exceeds byte limit.")
				}
				continue
			}
			s.Documents = append(s.Documents, Document{Path: p, Content: content})
		}
		for _, p := range dirs {
			walk(p, depth+1)
		}
	}
	walk(".", 0)
	sort.Slice(s.Documents, func(i, j int) bool { return s.Documents[i].Path < s.Documents[j].Path })
	sort.Slice(s.Diagnostics, func(i, j int) bool {
		a, b := s.Diagnostics[i], s.Diagnostics[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Code < b.Code
	})
	return s
}

// workflowDocument keeps workflow YAML separate from manifest/tool marker analysis.
func workflowDocument(p string) bool {
	return path.Dir(p) == ".github/workflows" && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml"))
}
