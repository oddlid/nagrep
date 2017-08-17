/*
   Copyright 2017 Odd Eivind Ebbesen

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/oddlid/oddebug"
	"github.com/urfave/cli"
	"github.com/vgtmnm/nagioscfg"
	"os"
	"strings"
	"time"
)

const VERSION string = "2017-08-18"

const (
	// same as in sysexits.h from Linux
	EX_USAGE int = 64
	EX_IOERR int = 74
)

var (
	BUILD_DATE string
	DBG_NOOP   bool
)

func dbgStr() string {
	if DBG_NOOP {
		return ""
	}
	callchain_lvl := 2
	basename := true
	funcname, filename, line := oddebug.DebugForWraps(DBG_NOOP, callchain_lvl, "", basename)
	return fmt.Sprintf("(in: %s[%s:%d])", funcname, filename, line)
}

func verifyObjTypes(names []string) []nagioscfg.CfgType {
	nlen := len(names)
	if nlen == 0 {
		return nil
	}
	validTypes := make([]nagioscfg.CfgType, 0, nlen)
	for _, n := range names {
		typ := nagioscfg.CfgName(n).Type()
		if typ != nagioscfg.T_INVALID {
			validTypes = append(validTypes, typ)
		}
	}
	return validTypes
}

func verifyObjProps(keys []string) []string {
	klen := len(keys)
	if klen == 0 {
		return nil
	}
	validProps := make([]string, 0, klen)
	for _, k := range keys {
		if nagioscfg.IsValidProperty(k) {
			validProps = append(validProps, k)
		}
	}
	return validProps
}

//func toStringSlicePtr(s []string) *cli.StringSlice {
//	css := make(cli.StringSlice, len(s))
//	for i := range s {
//		css[i] = s[i]
//	}
//	return &css
//}

func isPipe() bool {
	fi, _ := os.Stdin.Stat()
	return (fi.Mode() & os.ModeCharDevice) == 0
}

func splitKV(in, sep string) (string, string, bool) {
	parts := strings.Split(in, sep)
	if len(parts) < 2 {
		return "", "", false
	}
	k := parts[0]
	if !nagioscfg.IsValidProperty(k) {
		return "", "", false
	}
	v := strings.Join(parts[1:], sep)
	if v == "" {
		return k, v, false
	}
	return k, v, true
}

// Same functionality as "nafmt", because I have time right now...
func formatPipe(sort, removedups bool) error {
	ncfg := nagioscfg.NewNagiosCfg()
	err := ncfg.LoadStdin()
	if err != nil {
		return cli.NewExitError(err.Error(), EX_IOERR)
	}
	if removedups {
		ncfg.RemoveServiceDuplicates(nil)
	}
	ncfg.Print(os.Stdout, sort)
	return nil
}

// createStub creates a new Nagios object and prints it out.
func createStub(ctx *cli.Context) error {
	types := verifyObjTypes(ctx.StringSlice("type")) // we'll only care about first type given
	if len(types) == 0 {
		return cli.NewExitError("You need to specify a type when creating a stub", EX_USAGE)
	}
	props := ctx.StringSlice("append-key")
	plen := len(props)
	const eq string = "="

	if plen == 0 {
		return cli.NewExitError("You need to specify at least one key=value (-a)", EX_USAGE)
	}

	o := nagioscfg.NewCfgObjWithUUID(types[0])

	ak := make([]string, 0, plen)
	av := make([]string, 0, plen)
	for i := range props {
		k, v, ok := splitKV(props[i], eq)
		if ok {
			ak = append(ak, k)
			av = append(av, v)
		}
	}
	o.SetKeys(ak, av)

	o.Print(os.Stdout, true)

	return nil
}

func entryPoint(ctx *cli.Context) error {
	types := verifyObjTypes(ctx.StringSlice("type"))
	keys := verifyObjProps(ctx.StringSlice("key"))
	exprs := ctx.StringSlice("expression")
	delkey := ctx.StringSlice("del-key")
	setkey := ctx.StringSlice("set-key")
	delobjs := ctx.Bool("del-objs")
	save := ctx.Bool("save")
	sort := !ctx.Bool("no-sort")
	format := ctx.Bool("format-only")
	deldups := ctx.Bool("remove-duplicates")
	invmatch := ctx.Bool("not")
	stub := ctx.Bool("stub")
	listfiles := ctx.Bool("list-files-only")
	args := ctx.Args() // files
	const eq string = "="

	log.Debugf("Types: %#v", types)
	log.Debugf("Keys: %#v", keys)
	log.Debugf("Exprs: %#v", exprs)
	log.Debugf("Args: %#v", args)
	log.Debugf("Negate: %v", invmatch)

	// take the easy way out
	if stub {
		return createStub(ctx)
	}
	if format {
		return formatPipe(sort, deldups)
	}

	ncfg := nagioscfg.NewNagiosCfg()
	src := "stdin"

	if isPipe() {
		err := ncfg.LoadStdin()
		if err != nil {
			log.Error(err)
		}
	} else if len(args) > 0 {
		ncfg.LoadFiles(args...)
		src = "files"
	}

	// should remove dups here if requested, before anything else is done,
	// so we don't include the dups in matches and edits

	tlen := len(types)
	klen := len(keys)
	elen := len(exprs)
	slen := len(setkey)
	var keys_deleted int
	var keys_modified int
	var removed_objs nagioscfg.CfgMap

	if tlen > 0 {
		ncfg.FilterType(types...) // as this is done before Search, it should speed up searching, with a smaller set to search
	}

	q := nagioscfg.NewCfgQuery()
	if elen > 0 {
		for i := range exprs {
			if !q.AddRX(exprs[i]) {
				log.Fatalf("Invalid regular expression: %q", exprs[i])
			}
		}
	}
	if klen > 0 {
		for i := range keys {
			if !q.AddKey(keys[i]) {
				log.Errorf("Invalid object property key: %q", keys[i])
			}
		}
	}
	t_start := time.Now()
	matches := ncfg.Search(q) // now searches either whole content or subset depending on if FilterType was called
	log.Debugf("Searched %d objects from %d files in %f seconds",
		ncfg.Len(), len(args), time.Duration(time.Now().Sub(t_start)).Seconds())
	log.Debugf("Matches: %q %s", matches, dbgStr())
	//log.Debugf("Matched in files: \n%s\n", strings.Join(ncfg.UniqueFileIDs(matches), "\n"))

	// this seems like a good place for inverting matches if requested...
	if invmatch {
		matches = ncfg.InverseResults()
	}

	// if requested to list-files-only, do so and exit before any modification is done
	if listfiles {
		// Only be verbose at level Info or Debug,
		// Else only print matching filesnames, in case output should be used as input in a new instance or something
		if log.StandardLogger().Level >= log.InfoLevel {
			fmt.Printf("\n# Searched files:\n\n%s\n", strings.Join(args, "\n"))
			fmt.Printf("\n# Found matches in the following files:\n\n%s\n\n", strings.Join(ncfg.UniqueFileIDs(matches), "\n"))
		} else {
			fmt.Printf("%s\n", strings.Join(ncfg.UniqueFileIDs(matches), "\n"))
		}
		return nil
	}

	// if delete-key
	if len(delkey) > 0 {
		keys_deleted = ncfg.DelKeys(delkey) // save ret for number of deleted keys
	}
	// if set-key
	if slen > 0 {
		skeys := make([]string, 0, slen)
		svals := make([]string, 0, slen)
		for i := range setkey {
			k, v, ok := splitKV(setkey[i], eq)
			if ok {
				skeys = append(skeys, k)
				svals = append(svals, v)
			}
		}
		keys_modified = ncfg.SetKeys(skeys, svals) // take ret for number of added/modified keys. Should be number of objects in current match set X number of key/value pairs
	}

	if delobjs {
		removed_objs = ncfg.DeleteMatches()
	}

	if save && !isPipe() {
		//if deldups {
		//	log.Debug("Removing duplicates...")
		//} else {
		//	hasdups, _ := ncfg.HasServiceDuplicates()
		//	if hasdups {
		//		log.Fatalf("Duplicates in config! Refusing to save! %s", dbgStr())
		//	}
		//}
		err := ncfg.SaveToOrigin(sort) // should really find a way to only save what's modified, and not every file
		if err != nil {
			log.Error(err)
		}
	}

	if isPipe() {
		// Input came from STDIN. If something was modified, we print everything back. If not, we print matches.
		if removed_objs != nil || keys_deleted > 0 || keys_modified > 0 {
			ncfg.Print(os.Stdout, sort)
		} else {
			ncfg.PrintMatches(os.Stdout, sort)
		}
	} else {
		if removed_objs != nil {
			// Input came from files, and might have been saved back above.
			// We only print what's removed, so it can be piped into a new file if desired.
			removed_objs.Print(os.Stdout, sort)
		} else {
			// Input came from files, and might have been saved back above.
			// No objects are deleted, so we print the matching, and possibly modified, contents back.
			ncfg.PrintMatches(os.Stdout, sort)
		}
	}

	//log.Debugf("Content (from %s):\n%s", src, ncfg.DumpString())
	log.Infof("Content from     : %d %s", len(args), src)
	log.Infof("Objects in DB    : %d", ncfg.Len())
	log.Infof("Matching objects : %d", ncfg.GetMatches().Len())
	log.Infof("Keys deleted     : %d", keys_deleted)
	log.Infof("Keys modified    : %d", keys_modified)
	log.Infof("Objects deleted  : %d", len(removed_objs))
	log.Infof("Time used        : %f", time.Duration(time.Now().Sub(t_start)).Seconds())

	return nil
}

// setCustomAppHelpTmpl slightly changes the help text to include BUILD_DATE
// See https://github.com/urfave/cli/blob/master/help.go
func setCustomAppHelpTmpl() {
	cli.AppHelpTemplate = `NAME:
   {{.Name}}{{if .Usage}} - {{.Usage}}{{end}}
USAGE:
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}
VERSION / BUILD_DATE:
   {{.Version}} / {{.Compiled}}{{end}}{{end}}{{if .Description}}
DESCRIPTION:
   {{.Description}}{{end}}{{if len .Authors}}
AUTHOR{{with $length := len .Authors}}{{if ne 1 $length}}S{{end}}{{end}}:
   {{range $index, $author := .Authors}}{{if $index}}
   {{end}}{{$author}}{{end}}{{end}}{{if .VisibleCommands}}
COMMANDS:{{range .VisibleCategories}}{{if .Name}}
   {{.Name}}:{{end}}{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}
GLOBAL OPTIONS:
   {{range $index, $option := .VisibleFlags}}{{if $index}}
   {{end}}{{$option}}{{end}}{{end}}{{if .Copyright}}
COPYRIGHT:
   {{.Copyright}}{{end}}
`
}

func main() {
	setCustomAppHelpTmpl()
	app := cli.NewApp()
	app.Name = "nagrep"
	app.Version = VERSION
	app.Compiled, _ = time.Parse(time.RFC3339, BUILD_DATE)
	app.Usage = "Search and manipulate Nagios config files from the command line"
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Odd E. Ebbesen",
			Email: "oddebb@gmail.com",
		},
	}

	app.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name: "type, t",
			Usage: "Search only these object types. May be repeated.\n\tAllowed values:\n\t\t" +
				strings.Join(nagioscfg.ValidCfgNames(), "\n\t\t"),
			//Value: toStringSlicePtr(nagioscfg.ValidCfgNames()),
		},
		cli.StringSliceFlag{
			Name:  "key, k",
			Usage: "Match only against the given keys/properties. May be repeated.",
		},
		cli.StringSliceFlag{
			Name:  "expression, e",
			Usage: "The regular expression(s) to use. May be repeated.",
		},
		cli.BoolFlag{
			Name:  "format-only, f",
			Usage: "Only read input from stdin, and output formatted to stdout. No matching.",
		},
		cli.BoolFlag{
			Name:  "list-files-only",
			Usage: "Only list the files that contain matching objects. Modification parameters will be ignored.",
		},
		cli.BoolFlag{
			Name:  "not",
			Usage: "Negate/inverse search to only list objects NOT matching given expressions.\n\tThe evaluation is done after all other keys/expressions are matched.",
		},
		cli.BoolFlag{
			Name:  "stub",
			Usage: "Create new Nagios object, populate and print out",
		},
		cli.StringSliceFlag{
			Name:  "append-key, a",
			Usage: "Appends the given value to the given key's existing value. `FORMAT`: \"key_name=value\". May be repeated.",
		},
		cli.StringSliceFlag{
			Name:  "set-key, s",
			Usage: "Adds or overwrites the given key(s) for the matching objects.\n\t`FORMAT`: \"key_name=value\". May be repeated.",
		},
		cli.StringSliceFlag{
			Name:  "del-key",
			Usage: "Delete the given key from the matching objects. May be repeated.",
		},
		cli.BoolFlag{
			Name:  "del-objs",
			Usage: "Deletes all matching objects.\n\tIf input was read from files, the files will be overwritten (if \"--save\" is set),\n\t\tand the deleted objects printed on STDOUT.\n\tIf input was read from STDIN, the remaining objects will be printed on STDOUT.",
		},
		cli.BoolFlag{
			Name:  "remove-duplicates, r",
			Usage: "Remove duplicate service object definitions, if found",
		},
		cli.BoolFlag{
			Name:  "no-sort",
			Usage: "Do not sort object keys according to Nagios specs.",
		},
		cli.BoolFlag{
			Name:  "save",
			Usage: "Save modified config back to given source files. Will not happen if input on STDIN.",
		},
		//cli.BoolFlag{
		//	Name:  "quiet",
		//	Usage: "Don't output anything.",
		//},
		//cli.BoolFlag{
		//	Name:  "verbose",
		//	Usage: "Print extra info.",
		//},
		cli.StringFlag{
			Name:  "log-level, l",
			Value: "warn",
			Usage: "Log level (options: debug, info, warn, error, fatal, panic).",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "Run in debug mode.",
		},
	}

	app.Before = func(ctx *cli.Context) error {
		level, err := log.ParseLevel(ctx.String("log-level"))
		if err != nil {
			log.Fatal(err.Error())
		}
		log.SetLevel(level)
		if !ctx.IsSet("log-level") && !ctx.IsSet("l") && ctx.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}
		log.SetFormatter(&log.TextFormatter{
			DisableTimestamp: false,
			FullTimestamp:    true,
		})
		//log.SetOutput(os.Stderr)

		// (possibly) speed up oddebug
		if log.StandardLogger().Level != log.DebugLevel {
			DBG_NOOP = true
		}

		return nil
	}

	app.Action = entryPoint
	app.Run(os.Args)
}
