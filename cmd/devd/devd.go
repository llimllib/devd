package main

import (
	"fmt"
	"net/http"
	"os"
	"path"

	flag "github.com/spf13/pflag"

	"github.com/cortesi/termlog"
	"github.com/llimllib/devd"
	"github.com/toqueteos/webbrowser"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "devd: error: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	var (
		address      string
		allIfaces    bool
		certFile     string
		forceColor   bool
		downKbps     uint
		logHeaders   bool
		lrNaked      bool
		lrRoutes     bool
		moddMode     bool
		latency      int
		openBrowser  bool
		port         int
		credspec     string
		quiet        bool
		tlsFlag      bool
		noTimestamps bool
		logTime      bool
		upKbps       uint
		corsFlag     bool
		debug        bool
		version      bool
		notfound     []string
		ignoreLogs   []string
		watch        []string
		excludes     []string
	)

	flag.StringVarP(&address, "address", "A", "127.0.0.1", "Address to listen on")
	flag.BoolVarP(&allIfaces, "all", "a", false, "Listen on all addresses")
	flag.StringVarP(&certFile, "cert", "c", "", "Certificate bundle file - enables TLS")
	flag.BoolVarP(&forceColor, "color", "C", false, "Enable colour output, even if devd is not connected to a terminal")
	flag.UintVarP(&downKbps, "down", "d", 0, "Throttle downstream from the client to N kilobytes per second")
	flag.StringArrayVarP(&notfound, "notfound", "f", nil, "Default when a static file is not found")
	flag.BoolVarP(&logHeaders, "logheaders", "H", false, "Log headers")
	flag.StringArrayVarP(&ignoreLogs, "ignore", "I", nil, "Disable logging matching requests. Regexes are matched over 'host/path'")
	flag.BoolVarP(&lrNaked, "livereload", "L", false, "Enable livereload")
	flag.BoolVarP(&lrRoutes, "livewatch", "l", false, "Enable livereload and watch for static file changes")
	flag.BoolVarP(&moddMode, "modd", "m", false, "Modd is our parent - synonym for -LCt")
	flag.IntVarP(&latency, "latency", "n", 0, "Add N milliseconds of round-trip latency")
	flag.BoolVarP(&openBrowser, "open", "o", false, "Open browser window on startup")
	flag.IntVarP(&port, "port", "p", 0, "Port to listen on - if not specified, devd will auto-pick a sensible port")
	flag.StringVarP(&credspec, "password", "P", "", "HTTP basic password protection (USER:PASS)")
	flag.BoolVarP(&quiet, "quiet", "q", false, "Silence all logs")
	flag.BoolVarP(&tlsFlag, "tls", "s", false, "Serve TLS with auto-generated self-signed certificate (~/.devd.cert)")
	flag.BoolVarP(&noTimestamps, "notimestamps", "t", false, "Disable timestamps in output")
	flag.BoolVarP(&logTime, "logtime", "T", false, "Log timing")
	flag.UintVarP(&upKbps, "up", "u", 0, "Throttle upstream from the client to N kilobytes per second")
	flag.StringArrayVarP(&watch, "watch", "w", nil, "Watch path to trigger livereload")
	flag.BoolVarP(&corsFlag, "crossdomain", "X", false, "Set the CORS headers to allow everything (origin, credentials, headers, methods)")
	flag.StringArrayVarP(&excludes, "exclude", "x", nil, "Glob pattern for files to exclude from livereload")
	flag.BoolVar(&debug, "debug", false, "Debugging for devd development")
	flag.BoolVarP(&version, "version", "v", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: devd [flags] route [route ...]\n\n")
		fmt.Fprintf(os.Stderr, "Routes have the following forms:\n")
		fmt.Fprintf(os.Stderr, "    [SUBDOMAIN]/<PATH>=<DIR>\n")
		fmt.Fprintf(os.Stderr, "    [SUBDOMAIN]/<PATH>=<URL>\n")
		fmt.Fprintf(os.Stderr, "    <DIR>\n")
		fmt.Fprintf(os.Stderr, "    <URL>\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if version {
		fmt.Printf("devd %s\n", devd.Version)
		os.Exit(0)
	}

	routes := flag.Args()
	if len(routes) == 0 {
		fmt.Fprintf(os.Stderr, "devd: error: required argument 'route' not provided\n")
		flag.Usage()
		os.Exit(1)
	}

	if moddMode {
		forceColor = true
		noTimestamps = true
		lrNaked = true
	}

	realAddr := address
	if allIfaces {
		realAddr = "0.0.0.0"
	}

	var creds *devd.Credentials
	if credspec != "" {
		var err error
		creds, err = devd.CredentialsFromSpec(credspec)
		if err != nil {
			fatalf("%s", err)
		}
	}

	hdrs := make(http.Header)
	if corsFlag {
		hdrs.Set("Access-Control-Allow-Credentials", "true")
	}

	var servingScheme string
	if tlsFlag {
		servingScheme = "https"
	} else {
		servingScheme = "http"
	}

	dd := devd.Devd{
		// Shaping
		Latency:       latency,
		DownKbps:      downKbps,
		UpKbps:        upKbps,
		ServingScheme: servingScheme,

		AddHeaders: &hdrs,

		// Livereload
		LivereloadRoutes: lrRoutes,
		Livereload:       lrNaked,
		WatchPaths:       watch,
		Excludes:         excludes,

		Cors: corsFlag,

		Credentials: creds,
	}

	if err := dd.AddRoutes(routes, notfound); err != nil {
		fatalf("%s", err)
	}

	if err := dd.AddIgnores(ignoreLogs); err != nil {
		fatalf("%s", err)
	}

	logger := termlog.NewLog()
	if quiet {
		logger.Quiet()
	}
	if debug {
		logger.Enable("debug")
	}
	if logTime {
		logger.Enable("timer")
	}
	if logHeaders {
		logger.Enable("headers")
	}
	if forceColor {
		logger.Color(true)
	}
	if noTimestamps {
		logger.TimeFmt = ""
	}

	for _, i := range dd.Routes {
		logger.Say("Route %s -> %s", i.MuxMatch(), i.Endpoint.String())
	}

	if tlsFlag {
		home, err := os.UserHomeDir()
		if err != nil {
			fatalf("Could not get user's homedir: %s", err)
		}
		dst := path.Join(home, ".devd.cert")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			err := devd.GenerateCert(dst)
			if err != nil {
				fatalf("Could not generate cert: %s", err)
			}
		}
		certFile = dst
	}

	err := dd.Serve(
		realAddr,
		port,
		certFile,
		logger,
		func(url string) {
			if openBrowser {
				err := webbrowser.Open(url)
				if err != nil {
					fatalf("Failed to open browser: %s", err)
				}
			}
		},
	)
	if err != nil {
		fatalf("%s", err)
	}
}
