package main

import (
	"log"

	"github.com/spf13/cobra"

	lexgencli "github.com/harrybrwn/at/cmd/lexgen/cli"
)

func main() {
	root := lexgencli.NewLexGenCmd(
		&cobra.Command{
			Use:          "lexgen",
			SilenceUsage: true,
		},
		&lexgencli.Flags{
			Dirs:   []string{"./lexicons/"},
			OutDir: "./api/",
		},
	)
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
