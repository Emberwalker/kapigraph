package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/exp/maps"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var rootCmd = &cobra.Command{
	Use:   "kapigraph",
	Short: "Graph Kapitan class hierarchies",
	Long:  `Generates a .dot file for a Kapitan inventory, for use with Graphviz.`,
	RunE:  Run,
}

type ClassHeader struct {
	Classes []string `yaml:"classes,flow"`
}

var target string = ""
var invPath string = ""
var outPath string = ""
var fontName string = ""

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&target, "target", "t", "", "Render for a specific target (defaults to all targets)")
	rootCmd.Flags().StringVarP(&invPath, "inventory", "i", "inventory", "Path to inventory root (default: inventory)")
	rootCmd.Flags().StringVarP(&outPath, "output", "o", "kapitan.dot", "Path to inventory root (default: kapitan.dot)")
	rootCmd.Flags().StringVarP(&fontName, "font", "f", "", "Set graphviz font")
}

func dbg(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func Run(cmd *cobra.Command, args []string) error {
	invPath, err := filepath.Abs(invPath)
	if err != nil {
		return err
	}
	classes, err := crawlYaml(filepath.Join(invPath, "classes"), true)
	if err != nil {
		return err
	}
	targets, err := crawlYaml(filepath.Join(invPath, "targets"), false)
	if err != nil {
		return err
	}

	if target != "" {
		contents, exists := targets[target]
		if !exists {
			return errors.New("target not found: " + target)
		}
		targets = map[string][]string{target: contents}
		dbg("Only using classes referenced by target '%s'.", target)
	} else {
		dbg("Using all targets. If the resulting graph is too large, filter down with '-t TARGET_NAME'.")
	}

	allClasses := make(map[string][]string)
	maps.Copy(allClasses, classes)
	maps.Copy(allClasses, targets)

	relationships := make(map[string]map[string]interface{})

	for target := range targets {
		mergeable, err := descendTree(allClasses, relationships, target)
		if err != nil {
			return err
		}
		maps.Copy(relationships, mergeable)
	}

	// dbg("RELATIONS:")
	// for k, v := range relationships {
	// 	dbg("\t%s -> %v", k, maps.Keys(v))
	// }

	dbg("Generating dot for %v nodes...", len(relationships))

	graphProps := "fontsize=9,splines=ortho,overlap=false"
	nodeProps := "shape=rect"
	edgeProps := "shape=normal"

	if fontName != "" {
		nodeProps += ",fontname=\"" + fontName + "\""
	}

	dot := []string{
		"digraph kapitan {",
		"graph[" + graphProps + "];",
		"node[" + nodeProps + "];",
		"edge[" + edgeProps + "];",
	}

	for node, descendants := range relationships {
		for descendant := range descendants {
			dot = append(dot, "\""+node+"\" -> \""+descendant+"\";")
		}
	}

	dot = append(dot, "}")
	dotCompiled := strings.Join(dot, "\n")

	dbg("Writing out to %s", outPath)
	if err := ioutil.WriteFile(outPath, []byte(dotCompiled), 0666); err != nil {
		return err
	}

	return nil
}

func descendTree(classes map[string][]string, relationships map[string]map[string]interface{}, class string) (map[string]map[string]interface{}, error) {
	descendantClasses := classes[class]
	descendants := make(map[string]interface{})
	for _, cls := range descendantClasses {
		descendants[cls] = struct{}{}
		if _, exists := relationships[cls]; !exists {
			mergeable, err := descendTree(classes, relationships, cls)
			if err != nil {
				return nil, err
			}
			maps.Copy(relationships, mergeable)
		}
	}
	relationships[class] = descendants
	return relationships, nil
}

func crawlYaml(rootPath string, includePath bool) (map[string][]string, error) {
	mappings := make(map[string][]string)
	err := filepath.WalkDir(rootPath, func(itemPath string, d fs.DirEntry, _walkErr error) error {
		path := strings.TrimPrefix(itemPath, rootPath+"/")
		if !includePath {
			path = filepath.Base(path)
		}
		path = strings.ReplaceAll(path, string(os.PathSeparator), ".")
		ext := filepath.Ext(path)
		if d.IsDir() || !(ext == ".yml" || ext == ".yaml") {
			return nil
		}
		path = strings.TrimSuffix(path, ext)
		rawYaml, err := os.ReadFile(itemPath)
		if err != nil {
			return err
		}
		classes := ClassHeader{}
		err = yaml.Unmarshal(rawYaml, &classes)
		if err != nil {
			dbg("YAML parse error in %s: %v\n", path, err)
			return err
		}
		mappings[path] = classes.Classes
		return nil
	})
	return mappings, err
}
