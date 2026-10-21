// protoc-gen-gleam is a protoc/buf plugin that generates idiomatic Gleam
// types and encode/decode functions from proto3 definitions.
//
// Install:
//
//	go install github.com/qwexvf/protoc-gen-gleam/cmd/protoc-gen-gleam@latest
//
// Usage with buf (buf.gen.yaml):
//
//	plugins:
//	  - name: gleam
//	    out: src/my_app/proto
//	    opt: package_prefix=my_app/proto
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/qwexvf/protoc-gen-gleam/internal/generator"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("protoc-gen-gleam %s\n", version)
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-gleam: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(in, req); err != nil {
		return err
	}

	// protogen refuses files without a Go import path, which Gleam output
	// never uses. Fill in a placeholder so .proto files don't need go_package.
	for _, f := range req.ProtoFile {
		if f.GetOptions().GetGoPackage() == "" {
			if f.Options == nil {
				f.Options = &descriptorpb.FileOptions{}
			}
			f.Options.GoPackage = proto.String(path.Dir(f.GetName()))
		}
	}

	var flags flag.FlagSet
	packagePrefix := flags.String("package_prefix", "", "Gleam module path prefix (e.g. my_app/proto)")
	gen, err := protogen.Options{ParamFunc: flags.Set}.New(req)
	if err != nil {
		return err
	}
	gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		if err := generator.GenerateFile(gen, f, *packagePrefix); err != nil {
			gen.Error(fmt.Errorf("generating %s: %w", f.Desc.Path(), err))
			break
		}
	}

	out, err := proto.Marshal(gen.Response())
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}
