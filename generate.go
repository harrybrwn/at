package main

import (
	"github.com/spf13/cobra"

	lexgencli "github.com/harrybrwn/at/cmd/lexgen/cli"
)

func newLexCmd() *cobra.Command {
	c := cobra.Command{
		Use:   "lex",
		Short: "Lexicon tools",
	}
	gencmd := lexgencli.NewLexGenCmd(
		&cobra.Command{
			Use: "gen",
		},
		&lexgencli.Flags{},
	)
	c.AddCommand(gencmd)
	return &c
}
