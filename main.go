// Gom generates Active Record–style MongoDB models and migrations.
//
//	go install github.com/waasi/gom@latest
//
//	gom init
//	gom generate model Invoice title:string! total:int company:ref notes:text?
//	gom generate migration backfill_invoice_totals
//	gom migrate [up | down [n] | status]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

const version = "0.1.0"

const usage = `gom — Active Record–style models and migrations for Go + MongoDB

Usage:
  gom init [--models DIR] [--migrations DIR] [--runner DIR]
  gom generate model <Name> [field[:type][?|!] ...] [--collection NAME] [--force] [--pretend] [--skip-register]
  gom generate migration <name> [--pretend]
  gom migrate [up | down [n] | status]
  gom version

"g" is short for "generate".

Field types:
  string text int int32 int64 float decimal bool time id map any
  ref        reference to another model: company:ref → CompanyID (bson "companyId"), indexed
  []type     slice, e.g. tags:[]string, members:[]ref
  suffix ?   optional: pointer + omitempty      notes:text?
  suffix !   required: validate:"required"      title:string!
  A field with no type is a string.

Directories come from gom.json in the module root (written by gom init):
  {"models": "models", "migrations": "db/migrations", "runner": "cmd/migrate"}
`

var errUsage = errors.New("usage")

func main() {
	err := run(os.Args[1:])
	if errors.Is(err, errUsage) {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(int(exit))
		}
		fmt.Fprintln(os.Stderr, "gom:", err)
		os.Exit(1)
	}
}

// exitError carries a child process's exit status through without printing.
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func run(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return cmdInit(rest)
	case "generate", "g":
		if len(rest) == 0 {
			return errUsage
		}
		switch rest[0] {
		case "model":
			return cmdModel(rest[1:])
		case "migration":
			return cmdMigration(rest[1:])
		}
		return fmt.Errorf("unknown generator %q (model, migration)", rest[0])
	case "migrate":
		return cmdMigrate(rest)
	case "version", "--version", "-v":
		fmt.Println("gom", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q (run `gom help`)", cmd)
}

// parseInterspersed parses flags that may appear anywhere among positional
// arguments, as in `gom g model Invoice title --force`.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if f := fs.Lookup(name); f != nil {
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
				continue
			}
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return pos, nil
}
