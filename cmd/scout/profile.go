// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ianclemence/scout/pkg/profile"
	"github.com/ianclemence/scout/pkg/runtime"
)

func profileCmd(c *runtime.Core, args []string) error {
	if len(args) == 0 || args[0] == "show" {
		p, err := c.Profile()
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if args[0] == "import" && len(args) == 2 {
		raw, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		p, _, err := profile.ImportDocument(c.DB, filepath.Base(args[1]), raw)
		if err != nil {
			return err
		}
		fmt.Printf("Imported CV for %s — %d skills detected, resume stored.\n", p.DisplayName, len(p.Skills))
		return nil
	}
	return fmt.Errorf("usage: scout profile [show|import <file>]")
}
