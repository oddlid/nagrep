package main

import (
	//"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/vgtmnm/nagioscfg"
	//"gopkg.in/urfave/cli.v2"
	"github.com/urfave/cli"
	"os"
	"time"
	//"strings"
)

const VERSION string = "2017-01-12"
var BUILD_DATE string

func verifyObjTypes(names []string) []nagioscfg.CfgType {
	nlen := len(names)
	if nlen == 0 {
		return nil
	}
	validTypes := make([]nagioscfg.CfgType, 0, nlen)
	for _, n := range names {
		//log.Debugf("Name string: %s", n)
		name := nagioscfg.CfgName(n)
		//log.Debugf("Name: %s", name)
		typ := name.Type()
		//log.Debugf("Type: %s", typ)
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

func toStringSlicePtr(s []string) *cli.StringSlice {
	css := make(cli.StringSlice, len(s))
	for i := range s {
		css[i] = s[i]
	}
	return &css
}

func isPipe() bool {
	fi, _ := os.Stdin.Stat()
	return (fi.Mode() & os.ModeCharDevice) == 0
}

func entryPoint(ctx *cli.Context) error {
	types := verifyObjTypes(ctx.StringSlice("type"))
	keys := verifyObjProps(ctx.StringSlice("key"))
	exprs := ctx.StringSlice("expression")
	args := ctx.Args() // files
	log.Debugf("Types: %#v", types)
	log.Debugf("Keys: %#v", keys)
	log.Debugf("Exprs: %#v", exprs)
	log.Debugf("Args: %#v", args)

	ncfg := nagioscfg.NewNagiosCfg()

	if isPipe() {
		log.Debug("Input from pipe")
		ncfg.Config = nagioscfg.NewReader(os.Stdin).ReadAllMap("")
	}
	if len(args) > 1 {
		log.Debug("We need a MultiFileReader")
		ncfg.LoadFiles(args...)
	}
	if len(types) > 0 {
		log.Debug("We need to filter out a subset of given types")
	}
	if len(keys) > 0 {
		log.Debug("We only match against specific keys")
	}
	log.Debug("Ready to get our working set")
	log.Debug("Time for modifications")
	log.Debug("Time for writing back to file(s)")

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
		cli.StringFlag{
			Name:  "log-level, l",
			Value: "error",
			Usage: "Log level (options: debug, info, warn, error, fatal, panic)",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "Run in debug mode",
		},
		cli.StringSliceFlag{
			Name: "type, t",
			Usage: "Search only these object types. May be repeated.",
			//Value: toStringSlicePtr(nagioscfg.ValidCfgNames()),
		},
		cli.StringSliceFlag{
			Name: "key, k",
			Usage: "Match only against the given keys/properties. May be repeated.\n\tNumber of invocations must match that of given expressions.",
		},
		cli.StringSliceFlag{
			Name: "expression, e",
			Usage: "The regular expression to use. May be repeated.\n\tNumber of invocations must match that of given keys.",
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
	//app.Action = func(ctx *cli.Context) error {
	//	log.Debug("Running and exiting...")
	//	fmt.Fprintf(ctx.App.Writer, "Expr: %#v\n", ctx.StringSlice("expression"))
	//	types := ctx.StringSlice("types")
	//	nctypes := verifyObjTypes(types)
	//	fmt.Fprintf(ctx.App.Writer, "Types: %#v\n", nctypes)
	//	return nil
	//}
	app.Action = entryPoint

	//app.After = func(ctx *cli.Context) error {
	//	fmt.Fprintf(ctx.App.Writer, "Compiled: %s\n", ctx.App.Compiled)
	//	//fmt.Fprintf(ctx.App.Writer, "Build date: %s\n", BUILD_DATE)
	//	return nil
	//}
	app.Run(os.Args)

	//fmt.Printf("nagrep using %s v%s\n", nagioscfg.PKGNAME, nagioscfg.VERSION)
}
