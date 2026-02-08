// SPDX-License-Identitfier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

const (
	kindType  = "type"
	kindFunc  = "func"
	kindConst = "const"
	kindVar   = "var"

	typeStruct    = "struct"
	typeInterface = "interface"
	typeBasic     = "basic"
	typeFunc      = "func"

	funcMethod = "method"
	funcBasic  = "func"
)

type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Links []Link           `json:"links"`
}

func (g *Graph) findContainingNode(pkg *packages.Package, file *ast.File, n ast.Node) *Node {
	if n == nil {
		return nil
	}
	path, _ := astutil.PathEnclosingInterval(file, n.Pos(), n.End())

	for _, node := range path {
		var obj types.Object

		switch decl := node.(type) {
		case *ast.FuncDecl:
			obj = pkg.TypesInfo.Defs[decl.Name]
		case *ast.TypeSpec:
			obj = pkg.TypesInfo.Defs[decl.Name]
		case *ast.ValueSpec:
			if len(decl.Names) > 0 {
				obj = pkg.TypesInfo.Defs[decl.Names[0]]
			}
		}

		if obj != nil {
			return g.Nodes[id(obj)]
		}
	}

	return nil
}

func (g *Graph) MarshalJSON() ([]byte, error) {
	var out struct {
		Graph
		Nodes []*Node `json:"nodes"`
	}

	out.Links = g.Links

	for _, node := range g.Nodes {
		out.Nodes = append(out.Nodes, node)
	}

	return json.Marshal(out)
}

type Node struct {
	Kind      string `json:"kind"`
	Type      string `json:"type,omitempty"`
	Pkg       string `json:"pkg"`
	Id        string `json:"id"`
	LocalName string `json:"name"`
	obj       types.Object
	pkg       *packages.Package
}

type Link struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func analyzePackages(paths ...string) (*Graph, error) {
	cfg := &packages.Config{
		// Tests: true,
		Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, err
	}

	var graph Graph
	graph.Nodes = make(map[string]*Node)

	// Collect nodes
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)

			for _, node := range objNodes(pkg, obj) {
				graph.Nodes[node.Id] = &node
			}
		}
	}

	links := make(map[string]map[string]bool)

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				parentNode := graph.findContainingNode(pkg, file, n)
				if parentNode == nil {
					return true
				}

				if ident, ok := n.(*ast.Ident); ok {
					if refObj := pkg.TypesInfo.Uses[ident]; refObj != nil {
						if refEntity := graph.Nodes[id(refObj)]; refEntity != nil {
							m := links[parentNode.Id]
							if m == nil {
								m = make(map[string]bool)
								links[parentNode.Id] = m
							}
							m[refEntity.Id] = true
						}
					}
				}
				return true
			})
		}
	}

	for _, node := range graph.Nodes {
		if named, ok := node.obj.Type().(*types.Named); ok {
			if iface, ok := named.Underlying().(*types.Interface); ok {
				for method := range iface.ExplicitMethods() {
					graph.Links = append(graph.Links, Link{From: id(method), To: node.Id})
				}
				for embedded := range iface.EmbeddedTypes() {
					graph.Links = append(graph.Links, Link{From: node.Id, To: embedded.String()})
				}
			}
			for method := range named.Methods() {
				graph.Links = append(graph.Links, Link{From: id(method), To: node.Id})
			}
		}
	}

	for from, v := range links {
		for to := range v {
			graph.Links = append(graph.Links, Link{From: from, To: to})
		}
	}

	return &graph, nil
}

func objNodes(pkg *packages.Package, obj types.Object) []Node {
	switch t := obj.(type) {
	case *types.Func:
		return []Node{{obj: obj, pkg: pkg, Kind: kindFunc, Type: funcBasic, Id: id(t), LocalName: t.Name(), Pkg: obj.Pkg().Path()}}
	case *types.TypeName:
		var nodes []Node

		switch u := t.Type().Underlying().(type) {
		case *types.Struct:
			nodes = append(nodes, Node{obj: obj, pkg: pkg, Kind: kindType, Type: typeStruct, Id: id(t), LocalName: t.Name(), Pkg: obj.Pkg().Path()})
		case *types.Interface:
			nodes = append(nodes, Node{obj: obj, pkg: pkg, Kind: kindType, Type: typeInterface, Id: id(t), LocalName: t.Name(), Pkg: obj.Pkg().Path()})

			for method := range u.ExplicitMethods() {
				nodes = append(nodes, Node{obj: method, pkg: pkg, Kind: kindFunc, Type: funcMethod, Id: id(method), LocalName: t.Name() + "." + method.Name(), Pkg: obj.Pkg().Path()})
			}
		case *types.Basic:
			nodes = append(nodes, Node{obj: obj, pkg: pkg, Kind: kindType, Type: typeBasic, Id: id(t), LocalName: t.Name(), Pkg: obj.Pkg().Path()})
		case *types.Signature:
			nodes = append(nodes, Node{obj: obj, pkg: pkg, Kind: kindType, Type: typeFunc, Id: id(t), LocalName: t.Name(), Pkg: obj.Pkg().Path()})
		}

		if named, ok := t.Type().(*types.Named); ok {
			for method := range named.Methods() {
				nodes = append(nodes, Node{obj: method, pkg: pkg, Kind: kindFunc, Type: funcMethod, Id: id(method), LocalName: t.Name() + "." + method.Name(), Pkg: obj.Pkg().Path()})
			}
		}
		return nodes
	case *types.Const:
		return []Node{{obj: obj, pkg: pkg, Kind: kindConst, Id: t.Id(), LocalName: t.Name(), Pkg: obj.Pkg().Path()}}
	case *types.Var:
		return []Node{{obj: obj, pkg: pkg, Kind: kindVar, Id: t.Id(), LocalName: t.Name(), Pkg: obj.Pkg().Path()}}
	}

	return nil
}

func ifaceMethodId(obj types.Object, iface types.Object) string {
	pkgPath := ""
	if obj.Pkg() != nil {
		pkgPath = obj.Pkg().Path()
	}
	typeName := iface.Name()
	return fmt.Sprintf("(%s.%s).%s", pkgPath, typeName, obj.Name())
}

func id(obj types.Object) string {
	pkgPath := ""
	if obj.Pkg() != nil {
		pkgPath = obj.Pkg().Path()
	}

	// Check if the object is a function/method
	if fn, ok := obj.(*types.Func); ok {
		sig := fn.Type().(*types.Signature)
		if recv := sig.Recv(); recv != nil {
			typeName := recv.Type().String()
			return fmt.Sprintf("(%s).%s", typeName, obj.Name())
		}
	}

	// Default for package-level variables, constants, and types
	if pkgPath == "" {
		return obj.Name()
	}
	return pkgPath + "." + obj.Name()
}

func elem(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.Pointer:
		return elem(tt.Elem())
	case *types.Chan:
		return elem(tt.Elem())
	case *types.Array:
		return elem(tt.Elem())
	default:
		return t
	}
}

func nodeContains(parent, child ast.Node) bool {
	if parent == nil || child == nil {
		return false
	}
	return parent.Pos() <= child.Pos() && child.End() <= parent.End()
}
