package cmdutils

import (
	"flag"
	"fmt"
	"os"
)

var (
	VFlag bool
)

func Set(use, description string) {
	// 定義命令行參數
	flag.Usage = func() {
		fmt.Println(description)
		fmt.Println()
		fmt.Println(use)
		fmt.Println(`
  -h, --help               Display this help and exit.
  -v, --version            Output version information and exit.`)
	}

	flag.BoolVar(&VFlag, "v", false, "version")
	flag.BoolVar(&VFlag, "version", false, "version")
}

func SetArgs(argsCount int, appVersion string) []string {
	flag.Parse()

	args := flag.Args()

	if VFlag {
		appVersion += `
Written by icps.`
		fmt.Println(appVersion)
		os.Exit(0)
	}

	if len(args) < argsCount {
		flag.Usage()
		os.Exit(0)
	}
	return args
}

func ShowHelp() {
	flag.Usage()
	os.Exit(1)
}
