// +build tools

// Openapigen is a script to take the OpenAPI YAML file, turn it into a JSON
// document, and embed it into a source file for easy deployment.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	inFile := flag.String("in", "../openapi.yaml", "input YAML file")
	outFile := flag.String("out", "discoveryhandler_gen.go", "output go file")
	pkgName := flag.String("name", "httptransport", "package name for generated file")
	flag.Parse()

	inF, err := os.Open(*inFile)
	if inF != nil {
		defer inF.Close()
	}
	if err != nil {
		log.Fatal(err)
	}

	tmp := map[interface{}]interface{}{}
	if err := yaml.NewDecoder(inF).Decode(&tmp); err != nil {
		log.Fatal(err)
	}
	embed, err := json.Marshal(convert(tmp))
	if err != nil {
		log.Fatal(err)
	}
	ck := sha256.Sum256(embed)

	fs := token.NewFileSet()
	// Make up a some file offsets to get things to not mash together so
	// badly...
	gf := fs.AddFile(*outFile, -1, 1024)
	gf.AddLine(48)
	gf.AddLine(49)
	gf.AddLine(98)
	gf.AddLine(99)
	gend := &ast.File{
		Package: gf.Pos(50),
		Name:    ast.NewIdent(*pkgName),
		Comments: []*ast.CommentGroup{
			{List: []*ast.Comment{&ast.Comment{
				Slash: gf.Pos(0),
				Text:  `// Code generated by openapigen.go DO NOT EDIT.`,
			}}}},
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok:    token.CONST,
				TokPos: gf.Pos(100),
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{ast.NewIdent("_openapiJSON")},
						Values: []ast.Expr{&ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf("%#q", string(embed)),
						}},
					},
					&ast.ValueSpec{
						Names: []*ast.Ident{ast.NewIdent("_openapiJSONEtag")},
						Values: []ast.Expr{&ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf("`\"%x\"`", ck),
						}},
					},
				},
			},
		},
	}

	outF, err := os.OpenFile(*outFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer outF.Close()
	if err := format.Node(outF, fs, gend); err != nil {
		log.Fatal(err)
	}
}

// Convert yoinked from:
// https://stackoverflow.com/questions/40737122/convert-yaml-to-json-without-struct/40737676#40737676
func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[fmt.Sprint(k)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	case map[string]interface{}:
		for k, v := range x {
			x[k] = convert(v)
		}
	}
	return i
}