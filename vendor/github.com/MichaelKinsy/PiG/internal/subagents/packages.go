package subagents

// Ports src/package-resources.ts. Only host-reported installed packages are read;
// arbitrary node_modules trees are never searched for executable resources.
import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type PackageRoot struct{ Path, Source string }
type packageResource struct{ path, owner string }

func packageAllowed(gate any, name, source string) bool {
	if gate == nil {
		return true
	}
	if b, ok := gate.(bool); ok {
		return b
	}
	entries, _ := stringList(gate)
	short := strings.TrimPrefix(name, "@")
	if i := strings.LastIndex(short, "/"); i >= 0 {
		short = short[i+1:]
	}
	return slices.ContainsFunc(entries, func(s string) bool {
		return strings.EqualFold(s, name) || strings.EqualFold(s, short) || strings.EqualFold(s, source)
	})
}
func packageResources(roots []PackageRoot, kind string, gate any) ([]packageResource, []string) {
	var out []packageResource
	var warnings []string
	seen := map[string]bool{}
	for _, pkg := range roots {
		root, err := filepath.EvalSymlinks(pkg.Path)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err != nil {
			continue
		}
		var manifest struct {
			Name string `json:"name"`
			PI   struct {
				Subagents map[string][]string `json:"subagents"`
			} `json:"pi"`
			Legacy map[string][]string `json:"pi-subagents"`
		}
		if json.Unmarshal(b, &manifest) != nil {
			continue
		}
		if !packageAllowed(gate, manifest.Name, pkg.Source) {
			continue
		}
		declaration := manifest.PI.Subagents
		if declaration == nil {
			declaration = manifest.Legacy
		}
		entries := declaration[kind]
		var excluded []string
		for _, entry := range entries {
			if strings.HasPrefix(entry, "!") {
				excluded = append(excluded, filepath.Clean(filepath.Join(root, entry[1:])))
			}
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry, "!") {
				continue
			}
			path := filepath.Clean(filepath.Join(root, entry))
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				warnings = append(warnings, fmt.Sprintf("package %s: resource escapes package root", pkg.Source))
				continue
			}
			matches, err := filepath.Glob(path)
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			for _, match := range matches {
				_ = filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					for _, exclude := range excluded {
						if path == exclude || strings.HasPrefix(path, exclude+string(filepath.Separator)) {
							if d.IsDir() {
								return filepath.SkipDir
							}
							return nil
						}
					}
					if d.IsDir() {
						return nil
					}
					ext := ".md"
					if kind == "workflows" {
						ext = ".js"
					}
					if filepath.Ext(path) != ext {
						return nil
					}
					real, err := filepath.EvalSymlinks(path)
					if err != nil {
						return nil
					}
					relative, err := filepath.Rel(root, real)
					if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
						warnings = append(warnings, "Skipping package symlink outside root: "+path)
						return nil
					}
					st, err := os.Stat(real)
					if err != nil || !st.Mode().IsRegular() || st.Size() > 1024*1024 || seen[real] {
						return nil
					}
					seen[real] = true
					out = append(out, packageResource{real, manifest.Name})
					return nil
				})
			}
		}
	}
	return out, warnings
}
func loadPackageConfigs(roots []PackageRoot, settings Settings) ([]Config, []string) {
	files, warnings := packageResources(roots, "agents", settings.PackageAgents)
	var out []Config
	for _, file := range files {
		b, err := os.ReadFile(file.path)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		c, err := ParseConfig(file.path, b)
		if err != nil {
			warnings = append(warnings, file.path+": "+err.Error())
			continue
		}
		c.Source = "package"
		out = append(out, c)
	}
	return out, warnings
}
func (m *Manager) reloadConfigs(s Settings) ([]Config, []string) {
	var roots []PackageRoot
	if m.host.InstalledPackages != nil {
		roots = m.host.InstalledPackages()
	}
	packages, warnings := loadPackageConfigs(roots, s)
	configs, localWarnings := LoadConfigs(m.host.CWD, m.host.AgentDir, s)
	for _, c := range configs {
		if c.Source == "default" {
			if !slices.ContainsFunc(packages, func(p Config) bool { return p.Name == c.Name }) {
				packages = append(packages, c)
			}
		} else {
			if i := slices.IndexFunc(packages, func(p Config) bool { return p.Name == c.Name }); i >= 0 {
				packages[i] = c
			} else {
				packages = append(packages, c)
			}
		}
	}
	return packages, append(warnings, localWarnings...)
}
