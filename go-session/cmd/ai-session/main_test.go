package main

import (
	"bytes"
	"strings"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return strings.TrimSpace(buf.String()), err
}

func boolPtr(b bool) *bool {
	return &b
}
