package resolver

import (
	"path/filepath"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type index struct {
	files           []*lang.FileIR
	nodes           []store.Node
	moduleToFiles   map[string][]string
	inFile          map[string]map[string]store.Node
	inFileQName     map[string]map[string]store.Node
	byName          map[string][]store.Node
	byQName         map[string][]store.Node
	bindings        map[string]map[string]string
	importAliases   map[string]map[string]string
	wildcardImports map[string][]string
	typeFacts       map[string]lang.TypeFacts
	typeBases       map[string][]string
	typeMethods     map[string]map[string]store.Node
	typeFields      map[string]string
	varTypes        map[string]string
}

func newIndex() *index {
	return &index{
		moduleToFiles:   make(map[string][]string),
		inFile:          make(map[string]map[string]store.Node),
		inFileQName:     make(map[string]map[string]store.Node),
		byName:          make(map[string][]store.Node),
		byQName:         make(map[string][]store.Node),
		bindings:        make(map[string]map[string]string),
		importAliases:   make(map[string]map[string]string),
		wildcardImports: make(map[string][]string),
		typeFacts:       make(map[string]lang.TypeFacts),
		typeBases:       make(map[string][]string),
		typeMethods:     make(map[string]map[string]store.Node),
		typeFields:      make(map[string]string),
		varTypes:        make(map[string]string),
	}
}

func buildIndex(repoRoot string, files []*lang.FileIR) *index {
	idx := newIndex()
	idx.files = files

	for _, f := range files {
		idx.typeFacts[f.Path] = f.Types
		for k, v := range f.Types.Vars {
			idx.varTypes[k] = v
		}
		for k, v := range f.Types.Fields {
			idx.typeFields[k] = v
		}
		for k, v := range f.Types.Bases {
			idx.typeBases[k] = append(idx.typeBases[k], v...)
		}

		if p, ok := lang.For(f.Path); ok {
			mod := p.ModulePath(repoRoot, f.Path)
			idx.moduleToFiles[mod] = append(idx.moduleToFiles[mod], f.Path)
		}

		if idx.inFile[f.Path] == nil {
			idx.inFile[f.Path] = make(map[string]store.Node)
		}
		if idx.inFileQName[f.Path] == nil {
			idx.inFileQName[f.Path] = make(map[string]store.Node)
		}

		for _, n := range f.Nodes {
			idx.nodes = append(idx.nodes, n)
			idx.inFile[f.Path][n.Name] = n
			idx.inFileQName[f.Path][n.QualifiedName] = n
			idx.byName[n.Name] = append(idx.byName[n.Name], n)
			idx.byQName[n.QualifiedName] = append(idx.byQName[n.QualifiedName], n)

			if n.Kind == store.KindMethod {
				parts := strings.SplitN(n.QualifiedName, ".", 2)
				if len(parts) == 2 {
					typeName, methodName := parts[0], parts[1]
					if idx.typeMethods[typeName] == nil {
						idx.typeMethods[typeName] = make(map[string]store.Node)
					}
					idx.typeMethods[typeName][methodName] = n
				}
			}
		}
	}

	for _, f := range files {
		if idx.bindings[f.Path] == nil {
			idx.bindings[f.Path] = make(map[string]string)
		}
		if idx.importAliases[f.Path] == nil {
			idx.importAliases[f.Path] = make(map[string]string)
		}

		for _, imp := range f.Imports {
			targetFiles := idx.resolveImportTarget(repoRoot, f.Path, imp.Path)
			if len(targetFiles) == 0 {
				continue
			}

			if imp.Wildcard {
				idx.wildcardImports[f.Path] = append(idx.wildcardImports[f.Path], targetFiles...)
				for _, tf := range targetFiles {
					if nodes, ok := idx.inFile[tf]; ok {
						for name, node := range nodes {
							if node.Visibility == store.VisExported {
								idx.bindings[f.Path][name] = node.ID
							}
						}
					}
				}
				continue
			}

			if imp.Alias != "" {
				idx.importAliases[f.Path][imp.Alias] = targetFiles[0]
			}

			if len(imp.Names) > 0 {
				for _, name := range imp.Names {
					for _, tf := range targetFiles {
						if node, ok := idx.inFile[tf][name]; ok {
							idx.bindings[f.Path][name] = node.ID
							break
						}
					}
				}
			} else if imp.Alias == "" {
				base := filepath.Base(imp.Path)
				idx.importAliases[f.Path][base] = targetFiles[0]
			}
		}
	}

	return idx
}

func (idx *index) resolveImportTarget(repoRoot, currentFilePath, importPath string) []string {
	if files, ok := idx.moduleToFiles[importPath]; ok && len(files) > 0 {
		return files
	}

	cleanImp := strings.TrimPrefix(importPath, "./")
	cleanImp = strings.TrimPrefix(cleanImp, "../")
	if files, ok := idx.moduleToFiles[cleanImp]; ok && len(files) > 0 {
		return files
	}

	dir := filepath.Dir(currentFilePath)
	relCandidate := filepath.ToSlash(filepath.Clean(filepath.Join(dir, importPath)))
	if files, ok := idx.moduleToFiles[relCandidate]; ok && len(files) > 0 {
		return files
	}

	for mod, files := range idx.moduleToFiles {
		if strings.HasSuffix(mod, importPath) || strings.HasSuffix(importPath, mod) {
			return files
		}
	}

	return nil
}
