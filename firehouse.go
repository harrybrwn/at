package main

import "github.com/spf13/cobra"

func newFireHoseCmd() *cobra.Command {
	c := cobra.Command{
		Use: "firehose",
	}
	return &c
}
