package resolver

import (
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func (idx *index) resolveReceiverType(scopeID, receiver string) string {
	if receiver == "" {
		return ""
	}
	parts := strings.Split(receiver, ".")
	if len(parts) == 0 {
		return ""
	}

	first := parts[0]
	var currentType string

	switch first {
	case "this", "self":
		if t, ok := idx.varTypes[lang.VarKey(scopeID, first)]; ok && t != "" {
			currentType = t
		} else {
			currentType = enclosingClassName(scopeID)
		}
	case "super":
		encClass := enclosingClassName(scopeID)
		if bases, ok := idx.typeBases[encClass]; ok && len(bases) > 0 {
			currentType = bases[0]
		}
	default:
		if t, ok := idx.varTypes[lang.VarKey(scopeID, first)]; ok && t != "" {
			currentType = t
		} else {
			filePath := scopeFilePath(scopeID)
			if t, ok := idx.varTypes[lang.VarKey(filePath, first)]; ok && t != "" {
				currentType = t
			}
		}
	}

	if currentType == "" {
		return ""
	}

	for i := 1; i < len(parts); i++ {
		field := parts[i]
		fieldType := idx.findFieldInHierarchy(currentType, field)
		if fieldType == "" {
			return ""
		}
		currentType = fieldType
	}

	return currentType
}

func (idx *index) findFieldInHierarchy(typeName, fieldName string) string {
	key := typeName + "." + fieldName
	if ft, ok := idx.typeFields[key]; ok && ft != "" {
		return ft
	}

	visited := map[string]bool{typeName: true}
	queue := []string{typeName}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, base := range idx.typeBases[curr] {
			if visited[base] {
				continue
			}
			visited[base] = true
			if ft, ok := idx.typeFields[base+"."+fieldName]; ok && ft != "" {
				return ft
			}
			queue = append(queue, base)
		}
	}

	return ""
}

func (idx *index) findMethodInHierarchy(typeName, methodName string) []store.Node {
	if methods, ok := idx.typeMethods[typeName]; ok {
		if node, ok := methods[methodName]; ok {
			return []store.Node{node}
		}
	}

	var results []store.Node
	visited := map[string]bool{typeName: true}
	queue := []string{typeName}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, base := range idx.typeBases[curr] {
			if visited[base] {
				continue
			}
			visited[base] = true
			if methods, ok := idx.typeMethods[base]; ok {
				if node, ok := methods[methodName]; ok {
					results = append(results, node)
				}
			}
			queue = append(queue, base)
		}
		if len(results) > 0 {
			break
		}
	}

	return results
}

func enclosingClassName(scopeID string) string {
	parts := strings.SplitN(scopeID, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	qname := parts[1]
	qparts := strings.Split(qname, ".")
	if len(qparts) >= 2 {
		return qparts[0]
	}
	return ""
}

func scopeFilePath(scopeID string) string {
	parts := strings.SplitN(scopeID, ":", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
