// Command dzm builds the official doze plugin modules and assembles their
// release layout. It is a thin loop over modules.yaml around the SDK's modtool
// package — the same library any third-party module repo releases with — plus
// the compiled-in describers map (every module's driver, for Describe()).
//
//	dzm --repo . --out dist               # build every module, all triples
//	dzm --repo . --out dist --module valkey
//	dzm --repo . --out dist --triples aarch64-apple-darwin
//	dzm meta --out dist                   # (re)generate every meta.yaml
//	dzm versions                          # modules.yaml name->version map as JSON
//	dzm readme                            # regenerate README.md's module table
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/doze-dev/doze-sdk/modtool"
)

type modulesFile struct {
	DozeRef string                 `yaml:"doze_ref"`
	Modules map[string]moduleEntry `yaml:"modules"`
}

type moduleEntry struct {
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
}

func main() {
	// `dzm meta` generates each module's registry meta.yaml from its driver's
	// Describe() (single source of truth); `dzm versions` prints the manifest's
	// name->version map as JSON (CI ships it in the registry dispatch payload so
	// the signer can wait out release-asset CDN staleness); the default (no
	// subcommand) builds the plugin archives.
	if len(os.Args) > 1 && os.Args[1] == "meta" {
		check(runMeta(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "readme" {
		check(runReadme(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "versions" {
		mf, err := loadModules("modules.yaml")
		check(err)
		versions := make(map[string]string, len(mf.Modules))
		for name, m := range mf.Modules {
			versions[name] = m.Version
		}
		b, err := json.Marshal(versions)
		check(err)
		fmt.Println(string(b))
		return
	}

	repo := flag.String("repo", ".", "module source root (this doze-modules repo)")
	out := flag.String("out", "dist", "output directory (release layout)")
	only := flag.String("module", "all", "module name to build, or \"all\"")
	triplesCSV := flag.String("triples", "", "comma-separated triples (default: all)")
	namespace := flag.String("namespace", "doze", "publisher namespace stamped into each index")
	flag.Parse()

	mf, err := loadModules("modules.yaml")
	check(err)
	buildTriples := modtool.AllTriples()
	if *triplesCSV != "" {
		buildTriples = strings.Split(*triplesCSV, ",")
	}

	names := make([]string, 0, len(mf.Modules))
	for n := range mf.Modules {
		if *only == "all" || *only == n {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		check(fmt.Errorf("no module matched %q", *only))
	}

	for _, name := range names {
		m, err := toModule(*repo, *namespace, name, mf.Modules[name])
		check(err)
		fmt.Printf("== %s %s ==\n", name, m.Version)
		for _, triple := range buildTriples {
			check(modtool.Build(m, *out, triple))
		}
	}
	fmt.Println("done.")
}

// toModule assembles a modtool.Module from a modules.yaml entry. The driver's
// Describe() is mandatory: it is where the index's engine-support list comes
// from, so an undescribed module cannot be published.
func toModule(repo, namespace, name string, entry moduleEntry) (modtool.Module, error) {
	drv, ok := describers[name]
	if !ok {
		return modtool.Module{}, fmt.Errorf("module %q has no describer (add Describe() to its driver and register it in cmd/dzm/meta.go)", name)
	}
	return modtool.Module{
		Name:       name,
		Version:    entry.Version,
		Namespace:  namespace,
		PluginPath: entry.Path,
		RepoDir:    repo,
		Driver:     drv,
	}, nil
}

func loadModules(path string) (*modulesFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf modulesFile
	if err := yaml.Unmarshal(b, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "dzm:", err)
		os.Exit(1)
	}
}
