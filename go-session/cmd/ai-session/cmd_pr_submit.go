package main

import (
	"github.com/daniel-talonone/gemini-commands/internal/commands" // New import
)

func init() {
	prCmd.AddCommand(commands.PrSubmitCmd)
}
