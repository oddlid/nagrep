package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/vgtmnm/nagioscfg"
	"github.com/urfave/cli"
	"os"
	"strings"
	"time"
)

const VERSION string = "2017-07-13"

var BUILD_DATE string

func verifyObjTypes(names []string) []nagioscfg.CfgType {
	nlen := len(names)
	if nlen == 0 {
		return nil
	}
	validTypes := make([]nagioscfg.CfgType, 0, nlen)
	for _, n := range names {
		name := nagioscfg.CfgName(n)
		typ := name.Type()
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
func formatPipe(sort bool) error {
	ncfg := nagioscfg.NewNagiosCfg()
	err := ncfg.LoadStdin()
	if err != nil {
		return cli.NewExitError("Unable to load STDIN", 74) // EX_IOERR=74 # input/output error (from sysexits.h)
	}
	ncfg.Print(os.Stdout, sort)
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
	args := ctx.Args() // files
	eq := "="

	log.Debugf("Types: %#v", types)
	log.Debugf("Keys: %#v", keys)
	log.Debugf("Exprs: %#v", exprs)
	log.Debugf("Args: %#v", args)

	// take the easy way out
	if format {
		return formatPipe(sort)
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
	ncfg.Search(q) // now searches either whole content or subset depending on if FilterType was called

	// this seems like a good place for inverting matches if requested...

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
	// if delete
	if delobjs {
		removed_objs = ncfg.DeleteMatches() // save ret for a CfgMap of what was deleted, for printing later
		if save && !ncfg.InPipe() {
			err := ncfg.SaveToOrigin(sort)
			if err != nil {
				log.Error(err)
			}
			removed_objs.Print(os.Stdout, sort)
		}
		if ncfg.InPipe() {
			ncfg.Print(os.Stdout, sort)
		}
	} else {
		ncfg.PrintMatches(os.Stdout, sort)
	}

	//log.Debugf("Content (from %s):\n%s", src, ncfg.DumpString())
	log.Debugf("Content from: %s", src)
	log.Debugf("Objects in DB: %d", ncfg.Len())
	log.Debugf("Matching objects: %d", ncfg.GetMatches().Len())
	log.Debugf("Keys deleted: %d", keys_deleted)
	log.Debugf("Keys modified: %d", keys_modified)
	log.Debugf("Objects deleted: %d", len(removed_objs))

	return nil
}

func main() {
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
			Name:  "not",
			Usage: "Negate/inverse search to only list objects NOT matching given expressions.\n\tThe evaluation is done after all other keys/expressions are matched.",
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
			Name:  "no-sort",
			Usage: "Do not sort output according to Nagios specs.",
		},
		cli.BoolFlag{
			Name:  "save",
			Usage: "Save modified config back to given source files. Will not happen if input on STDIN.",
		},
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "Don't output anything.",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "Print extra info.",
		},
		cli.StringFlag{
			Name:  "log-level, l",
			Value: "error",
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

		return nil
	}

	app.Action = entryPoint
	app.Run(os.Args)
}
