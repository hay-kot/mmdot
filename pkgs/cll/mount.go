// Package cll provides utilities for building CLI applications with urfave/cli/v3.
package cll

import "github.com/urfave/cli/v3"

// Registerable defines a type that can register itself with a CLI command.
// Implementations typically add subcommands, flags, or other modifications
// to the provided root command.
type Registerable interface {
	Register(*cli.Command) *cli.Command
}

// Register chains multiple Registerable implementations onto a root command.
// Each Registerable is applied in order, allowing modular composition of CLI structure.
//
// Example:
//
//	root := &cli.Command{Name: "app"}
//	root = cll.Register(root, generateCmd, brewCmd, runCmd)
func Register(root *cli.Command, subs ...Registerable) *cli.Command {
	for _, s := range subs {
		root = s.Register(root)
	}

	return root
}

// EnvWithPrefix returns a function that creates environment variable sources
// with a consistent prefix. This is useful for namespacing all environment
// variables for an application.
//
// Example:
//
//	env := cll.EnvWithPrefix("MYAPP_")
//	flag := &cli.StringFlag{
//		Name:    "config",
//		Sources: env("CONFIG", "CONFIG_PATH"), // reads MYAPP_CONFIG, MYAPP_CONFIG_PATH
//	}
func EnvWithPrefix(prefix string) func(strs ...string) cli.ValueSourceChain {
	return func(strs ...string) cli.ValueSourceChain {
		withPrefix := make([]string, len(strs))

		for i, str := range strs {
			withPrefix[i] = prefix + str
		}

		return cli.EnvVars(withPrefix...)
	}
}
