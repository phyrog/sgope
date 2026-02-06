package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"strings"
)

type Node struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Group string `json:"group"`
}

type Link struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphData struct {
	Nodes []Node `json:"nodes"`
	Links []Link `json:"links"`
}

// Internal representation for the analyzer
type DepNode struct {
	name     string
	kind     string // "type", "func", "method"
	callsTo  map[string]bool
	readsTo  map[string]bool
	embedsTo map[string]bool
}

type DepGraph struct {
	nodes map[string]*DepNode
}

func analyze(pkgPath string) *DepGraph {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgPath, nil, 0)
	if err != nil {
		log.Fatalf("Analysis error: %v", err)
	}

	graph := &DepGraph{nodes: make(map[string]*DepNode)}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			collectDeclarations(file, graph)
		}
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			findDependencies(file, graph)
		}
	}
	return graph
}

const (
	kindType     = "type"
	kindFunction = "func"
	kindMethod   = "method"
	kindVar      = "var"
	kindConst    = "const"
	kindUnknown  = "unknown"
)

func collectDeclarations(file *ast.File, graph *DepGraph) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			name := getFuncName(node)
			kind := kindFunction
			if node.Recv != nil {
				kind = kindMethod
			}
			graph.getOrCreate(name, kind)
			return false
		case *ast.GenDecl:
			switch node.Tok {
			case token.TYPE:
				for _, spec := range node.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						graph.getOrCreate(ts.Name.Name, kindType)
					}
				}
			case token.VAR, token.CONST:
				kind := kindVar
				if node.Tok == token.CONST {
					kind = kindConst
				}
				for _, spec := range node.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if name.Name == "_" {
								continue
							}
							graph.getOrCreate(name.Name, kind)
						}
					}
				}
			}
		}
		return true
	})
}

func findDependencies(file *ast.File, graph *DepGraph) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			fromName := getFuncName(n)
			fromNode := graph.nodes[fromName]
			if fromNode == nil {
				return true
			}

			findTypesInFunc(n.Type, fromNode, graph)

			ast.Inspect(n.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					callee := getCalleeName(call.Fun, graph)
					if callee != "" && callee != fromName {
						fromNode.callsTo[callee] = true
					}
				}

				if ident, ok := n.(*ast.Ident); ok {
					if target, exists := graph.nodes[ident.Name]; exists {
						if target.kind == kindVar || target.kind == kindConst {
							fromNode.readsTo[ident.Name] = true
						}
					}
				}
				return true
			})
			ast.Inspect(n.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SelectorExpr:
					if ident, ok := node.X.(*ast.Ident); ok {
						if graph.nodes[ident.Name] != nil && graph.nodes[ident.Name].kind == kindType {
							fromNode.readsTo[ident.Name] = true
						}
					}
				case *ast.CompositeLit:
					typeName := getTypeName(node.Type)
					if typeName != "" && graph.nodes[typeName] != nil {
						fromNode.readsTo[typeName] = true
					}
				}
				return true
			})
		case *ast.GenDecl:
			switch n.Tok {
			case token.TYPE:
				for _, spec := range n.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					structType, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}

					fromNode := graph.nodes[ts.Name.Name]
					if fromNode == nil {
						continue
					}

					for _, field := range structType.Fields.List {
						typeName := getTypeName(field.Type)
						if typeName != "" && graph.nodes[typeName] != nil {
							if len(field.Names) == 0 {
								// Embedded field
								fromNode.embedsTo[typeName] = true
							} else {
								// Regular field
								fromNode.readsTo[typeName] = true
							}
						}
					}
				}
			case token.VAR, token.CONST:
				for _, spec := range n.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							vNode, ok := graph.nodes[name.Name]
							if !ok {
								continue
							}
							// Var -> Type dependency
							if vs.Type != nil {
								typeName := getTypeName(vs.Type)
								if _, ok := graph.nodes[typeName]; ok {
									vNode.readsTo[typeName] = true
								}
							}
							// Var -> Func dependency (Initializers)
							for _, val := range vs.Values {
								ast.Inspect(val, func(vn ast.Node) bool {
									if call, ok := vn.(*ast.CallExpr); ok {
										callee := getCalleeName(call.Fun, graph)
										if callee != "" {
											vNode.callsTo[callee] = true
										}
									}
									return true
								})
							}
						}
					}
				}
			}
		}
		return true
	})
}

var bar = getFuncName(&ast.FuncDecl{Name: &ast.Ident{Name: "bar"}, Recv: &ast.FieldList{}})

func getFuncName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		recvType := getTypeName(funcDecl.Recv.List[0].Type)
		return recvType + "." + funcDecl.Name.Name
	}
	return funcDecl.Name.Name
}

func getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return getTypeName(t.X)
	case *ast.ArrayType:
		return getTypeName(t.Elt)
	case *ast.MapType:
		return getTypeName(t.Value)
	}
	return ""
}

func getCalleeName(expr ast.Expr, graph *DepGraph) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if graph.nodes[e.Name] != nil {
			return e.Name
		}
	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			methodName := ident.Name + "." + e.Sel.Name
			if graph.nodes[methodName] != nil {
				return methodName
			}
		}
	}
	return ""
}

func (g *DepGraph) getOrCreate(name, kind string) *DepNode {
	if g.nodes[name] == nil {
		g.nodes[name] = &DepNode{name: name, kind: kind, callsTo: make(map[string]bool), readsTo: make(map[string]bool), embedsTo: make(map[string]bool)}
	}
	return g.nodes[name]
}

func (g *DepGraph) toString() string {
	var sb strings.Builder
	for _, node := range g.nodes {
		fmt.Fprintf(&sb, "\n%s (%s):\n", node.name, node.kind)
		for dep := range node.callsTo {
			fmt.Fprintf(&sb, "  - %s\n", dep)
		}
		for dep := range node.readsTo {
			fmt.Fprintf(&sb, "  - %s\n", dep)
		}
		for dep := range node.embedsTo {
			fmt.Fprintf(&sb, "  - %s\n", dep)
		}
	}
	return sb.String()
}

func findTypesInFunc(fType *ast.FuncType, fromNode *DepNode, graph *DepGraph) {
	if fType == nil {
		return
	}
	// Check parameters
	if fType.Params != nil {
		for _, field := range fType.Params.List {
			recordType(field.Type, fromNode, graph)
		}
	}
	// Check return values
	if fType.Results != nil {
		for _, field := range fType.Results.List {
			recordType(field.Type, fromNode, graph)
		}
	}
}

func recordType(expr ast.Expr, fromNode *DepNode, graph *DepGraph) {
	switch t := expr.(type) {
	case *ast.Ident:
		// Base case: check if this identifier is a known type in our graph
		if graph.nodes[t.Name] != nil && graph.nodes[t.Name].kind == kindType {
			fromNode.readsTo[t.Name] = true
		}
	case *ast.StarExpr:
		recordType(t.X, fromNode, graph)
	case *ast.ArrayType:
		recordType(t.Elt, fromNode, graph)
	case *ast.MapType:
		recordType(t.Key, fromNode, graph)
		recordType(t.Value, fromNode, graph)
	case *ast.ChanType:
		recordType(t.Value, fromNode, graph)
	case *ast.FuncType:
		// Recursive case: Function signature as a type (e.g., func(MyType) int)
		findTypesInFunc(t, fromNode, graph)
	case *ast.SelectorExpr:
		// Handle internal package types referenced via selector if applicable
		if ident, ok := t.X.(*ast.Ident); ok {
			if graph.nodes[ident.Name] != nil && graph.nodes[ident.Name].kind == kindType {
				fromNode.readsTo[ident.Name] = true
			}
		}
	}
}
