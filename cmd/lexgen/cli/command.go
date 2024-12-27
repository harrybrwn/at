package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/harrybrwn/at/lex"
	"github.com/harrybrwn/at/lexgen"
	"github.com/harrybrwn/at/queue"
)

type Flags struct {
	SchemaFiles []string
	Dirs        []string
	OutDir      string
	UseStdout   bool
	Force       bool
}

func NewLexGenCmd(template *cobra.Command, flags *Flags) *cobra.Command {
	if template == nil {
		template = new(cobra.Command)
	}
	if len(template.Use) > 0 {
		template.Use = "gen"
	}
	if len(template.Short) > 0 {
		template.Short = "Generate code from a set of lexicons"
	}
	c := *template
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if len(flags.Dirs) > 0 {
			for _, d := range flags.Dirs {
				extraFiles, err := findLexFiles(d)
				if err != nil {
					return err
				}
				flags.SchemaFiles = append(flags.SchemaFiles, extraFiles...)
			}
		}
		schemas, err := readSchemas(flags.SchemaFiles)
		if err != nil {
			return err
		}
		generator := lexgen.NewGenerator("github.com/harrybrwn/at/api")
		generator.AddSchemas(schemas)
		if !flags.Force && exists(flags.OutDir) {
			return errors.Errorf("%q already exists", flags.OutDir)
		}

		for _, sch := range schemas {
			parts := strings.Split(sch.ID, ".")
			if len(parts) < 4 {
				return errors.Errorf("unexpected lexicon ID format %q. Expected 4 parts", sch.ID)
			}
			baseDir := filepath.Join(flags.OutDir, parts[0], parts[1])
			if err = os.MkdirAll(baseDir, 0755); err != nil {
				return err
			}
			var name string
			if parts[len(parts)-1] != "defs" {
				name = parts[len(parts)-2] + "_" + parts[len(parts)-1]
				if hasRPC(sch) {
					name += "_xrpc"
				}
			} else {
				name = parts[len(parts)-2] + "_defs"
			}
			filename := filepath.Join(baseDir, name+".go")
			if !flags.Force && exists(filename) {
				return errors.Errorf("generated file %q already exists", filename)
			}
			var f io.Writer
			if flags.UseStdout {
				f = os.Stdout
			} else {
				file, err := os.OpenFile(
					filename,
					os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
					0644,
				)
				if err != nil {
					return err
				}
				defer file.Close()
				f = file
			}

			fg, err := generator.NewGenerator(sch)
			if err != nil {
				return err
			}
			fmt.Fprintf(f, "// AUTO GENERATED. DO NOT EDIT!\n")
			fmt.Fprintf(f, "// Source:      %q\n", sch.Path())
			fmt.Fprintf(f, "// Destination: %q\n", filename)
			fmt.Fprintf(f, "// id:          %q\n", sch.ID)
			fmt.Fprintf(f, "\n")
			fmt.Fprintf(f, "package %s\n\n", fg.PackageName())
			err = fg.GenImports(f)
			if err != nil {
				return err
			}
			err = fg.GenTypes(f)
			if err != nil {
				return err
			}
		}
		err = generator.GenClients(flags.OutDir)
		if err != nil {
			return err
		}
		return nil
	}
	c.Flags().StringArrayVarP(&flags.SchemaFiles, "schema", "s", flags.SchemaFiles, "schema file")
	c.Flags().StringArrayVarP(&flags.Dirs, "dir", "d", flags.Dirs, "lexicon directory")
	c.Flags().StringVarP(&flags.OutDir, "out", "o", flags.OutDir, "output directory")
	c.Flags().BoolVar(&flags.UseStdout, "stdout", flags.UseStdout, "write generated code to stdout")
	c.Flags().BoolVarP(&flags.Force, "force", "f", flags.Force, "force the generator to overwrite files")
	c.AddCommand(
		newStubCmd(),
		newGenCbor(),
	)
	return &c
}

func newGenCbor() *cobra.Command {
	c := cobra.Command{
		Use:   "cbor",
		Short: "Generate code for CBOR support",
		RunE: func(cmd *cobra.Command, args []string) error {
			filenames := make([]string, 0)
			for _, a := range args {
				stat, err := os.Stat(a)
				if err != nil {
					return err
				}
				if stat.IsDir() {
					extraFiles, err := findLexFiles(a)
					if err != nil {
						return err
					}
					filenames = append(filenames, extraFiles...)
				} else {
					filenames = append(filenames, a)
				}
			}
			schemas, err := readSchemas(filenames)
			if err != nil {
				return err
			}
			g := lexgen.NewGenerator("github.com/harrybrwn/at")
			g.AddSchemas(schemas)
			types, err := g.List()
			if err != nil {
				return err
			}
			return lexgen.GenCborCmd(os.Stdout, types)
		},
	}
	return &c
}

func newStubCmd() *cobra.Command {
	var (
		stubName     = "demoStub"
		filterPrefix string
	)
	c := cobra.Command{
		Use:   "stub",
		Short: "Generate an empty interface stub from a lexicon.",
		RunE: func(cmd *cobra.Command, args []string) error {
			filenames := make([]string, 0)
			for _, a := range args {
				stat, err := os.Stat(a)
				if err != nil {
					return err
				}
				if stat.IsDir() {
					extraFiles, err := findLexFiles(a)
					if err != nil {
						return err
					}
					filenames = append(filenames, extraFiles...)
				} else {
					filenames = append(filenames, a)
				}
			}
			schemas, err := readSchemas(filenames)
			if err != nil {
				return err
			}
			g := lexgen.Generator{Defs: make(map[string]*lex.TypeSchema)}
			g.AddSchemas(schemas)
			types, err := g.List(func(st *lexgen.StructType) bool {
				d := st.GetDef()
				return d == nil || !d.Type.IsRPC() ||
					(len(filterPrefix) > 0 && !strings.HasPrefix(st.Schema.ID, filterPrefix))
			})
			if err != nil {
				return err
			}
			for _, st := range types {
				err = g.GenStub(os.Stdout, st, stubName)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&stubName, "stub-name", "n", stubName, "default stub name")
	c.Flags().StringVarP(&filterPrefix, "filter-prefix", "f", filterPrefix, "filter out all lexicons that have this prefix in its id")
	return &c
}

func readSchemas(files []string) ([]*lex.Schema, error) {
	schemas := make([]*lex.Schema, 0)
	for _, sFile := range files {
		s, err := lex.ReadSchema(sFile)
		if err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, nil
}

func findLexFiles(base string) ([]string, error) {
	files := make([]string, 0)
	for info, err := range walk(base) {
		if err != nil {
			return nil, err
		}
		mode := info.Info.Mode()
		if mode&fs.ModeDir != 0 ||
			mode&fs.ModeSymlink != 0 ||
			!strings.HasSuffix(info.Path, ".json") {
			continue
		}
		files = append(files, info.Path)
	}
	return files, nil
}

func hasRPC(sch *lex.Schema) bool {
	for _, def := range sch.Defs {
		switch def.Type {
		case lex.TypeQuery, lex.TypeProcedure, lex.TypeSubscription:
			return true
		}
	}
	return false
}

type walkData struct {
	Path string
	Info fs.FileInfo
}

func walk(root string) func(yield func(*walkData, error) bool) {
	var q queue.ListQueue[*walkData]
	q.Init()
	return func(yield func(*walkData, error) bool) {
		info, err := os.Lstat(root)
		if err != nil {
			yield(&walkData{Path: root}, err)
			return
		}
		q.Push(&walkData{Path: root, Info: info})
		for !q.Empty() {
			w, _ := q.Pop()
			mode := w.Info.Mode()
			if mode&fs.ModeSymlink != 0 || mode&fs.ModeDir != 0 {
				names, err := readDirNames(w.Path)
				if !yield(w, err) {
					return
				}
				if err != nil {
					continue
				}
				for _, name := range names {
					filename := filepath.Join(w.Path, name)
					fileInfo, err := os.Lstat(filename)
					newW := walkData{Path: filename, Info: fileInfo}
					if err != nil {
						if !yield(&newW, err) {
							return
						}
					} else {
						q.Push(&newW)
					}
				}
			} else {
				if !yield(w, nil) {
					return
				}
			}
		}
	}
}

func readDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	return names, nil
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}
