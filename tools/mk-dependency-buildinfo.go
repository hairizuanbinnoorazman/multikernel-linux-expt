// Inspect executable metadata without running the target executable.
package main

import (
	"debug/buildinfo"
	"debug/elf"
	"encoding/json"
	"os"
)

type Module struct {
	Path    string  `json:"path"`
	Version string  `json:"version"`
	Sum     string  `json:"sum,omitempty"`
	Replace *Module `json:"replace,omitempty"`
}
type Result struct {
	Path         string   `json:"path"`
	GoVersion    string   `json:"go_version,omitempty"`
	Main         *Module  `json:"main,omitempty"`
	Dependencies []Module `json:"dependencies,omitempty"`
	ELFMachine   string   `json:"elf_machine,omitempty"`
	Libraries    []string `json:"needed_libraries,omitempty"`
	GoStatus     string   `json:"go_status"`
}

func main() {
	var paths []string
	if json.NewDecoder(os.Stdin).Decode(&paths) != nil || len(paths) > 512 {
		os.Exit(2)
	}
	results := make([]Result, 0, len(paths))
	for _, p := range paths {
		r := Result{Path: p, GoStatus: "not_go_or_unreadable"}
		if b, e := buildinfo.ReadFile(p); e == nil {
			r.GoStatus = "available"
			r.GoVersion = b.GoVersion
			r.Main = &Module{Path: b.Main.Path, Version: b.Main.Version, Sum: b.Main.Sum}
			for _, d := range b.Deps {
				m := Module{Path: d.Path, Version: d.Version, Sum: d.Sum}
				if d.Replace != nil {
					m.Replace = &Module{Path: d.Replace.Path, Version: d.Replace.Version, Sum: d.Replace.Sum}
				}
				r.Dependencies = append(r.Dependencies, m)
			}
		}
		if f, e := elf.Open(p); e == nil {
			r.ELFMachine = f.Machine.String()
			r.Libraries, _ = f.ImportedLibraries()
			f.Close()
		}
		results = append(results, r)
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if e.Encode(results) != nil {
		os.Exit(2)
	}
}
