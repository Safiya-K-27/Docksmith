package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type envFlags map[string]string

func (e *envFlags) String() string {
	return ""
}

func (e *envFlags) Set(v string) error {
	parts := strings.SplitN(v, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("invalid env override %q, expected KEY=VALUE", v)
	}
	(*e)[parts[0]] = parts[1]
	return nil
}

func usage() {
	fmt.Println("Docksmith CLI")
	fmt.Println("commands:")
	fmt.Println("  build -t <name:tag> [--no-cache] <context>")
	fmt.Println("  images")
	fmt.Println("  rmi <name:tag>")
	fmt.Println("  run [-e KEY=VALUE ...] <name:tag> [cmd arg ...]")
	fmt.Println("  import-rootfs -t <name:tag> <rootfsDir>")
	fmt.Println("  diff <name:tag> <name:tag>")
}

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func realMain() error {
	if len(os.Args) < 2 {
		usage()
		return nil
	}
	root, err := mustStoreRoot()
	if err != nil {
		return err
	}

	switch os.Args[1] {
	case "build":
		fs := flag.NewFlagSet("build", flag.ContinueOnError)
		tag := fs.String("t", "", "image tag name:tag")
		noCache := fs.Bool("no-cache", false, "disable cache read/write")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *tag == "" {
			return errors.New("build requires -t name:tag")
		}
		if fs.NArg() != 1 {
			return errors.New("build requires exactly one context path")
		}
		ref, err := parseImageRef(*tag)
		if err != nil {
			return err
		}
		return buildImage(root, BuildOptions{TagRef: ref, Context: fs.Arg(0), NoCache: *noCache})

	case "images":
		return cmdImages(root)

	case "rmi":
		if len(os.Args) != 3 {
			return errors.New("rmi requires one image reference name:tag")
		}
		ref, err := parseImageRef(os.Args[2])
		if err != nil {
			return err
		}
		return cmdRmi(root, ref)

	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		env := envFlags{}
		fs.Var(&env, "e", "override env KEY=VALUE")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return errors.New("run requires image reference name:tag")
		}
		ref, err := parseImageRef(fs.Arg(0))
		if err != nil {
			return err
		}
		cmd := []string{}
		if fs.NArg() > 1 {
			cmd = fs.Args()[1:]
		}
		return cmdRun(root, RuntimeOptions{ImageRef: ref, OverrideEnv: env, OverrideCmd: cmd})

	case "import-rootfs":
		fs := flag.NewFlagSet("import-rootfs", flag.ContinueOnError)
		tag := fs.String("t", "", "image tag name:tag")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *tag == "" {
			return errors.New("import-rootfs requires -t name:tag")
		}
		if fs.NArg() != 1 {
			return errors.New("import-rootfs requires one rootfs directory")
		}
		ref, err := parseImageRef(*tag)
		if err != nil {
			return err
		}
		return cmdImportRootfs(root, ref, fs.Arg(0))

	case "diff":
		if len(os.Args) != 4 {
			return errors.New("diff requires two image references name:tag")
		}
		ref1, err := parseImageRef(os.Args[2])
		if err != nil {
			return err
		}
		ref2, err := parseImageRef(os.Args[3])
		if err != nil {
			return err
		}
		return cmdDiff(root, ref1, ref2)

	default:
		usage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
