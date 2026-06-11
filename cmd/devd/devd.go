package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/cortesi/termlog"
	"github.com/llimllib/devd"
	"github.com/mitchellh/go-homedir"
	"github.com/toqueteos/webbrowser"
)

// stringSlice implements flag.Value for flags that can be specified multiple times
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(val string) error {
	*s = append(*s, val)
	return nil
}

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
		notfound     stringSlice
		ignoreLogs   stringSlice
		watch        stringSlice
		excludes     stringSlice
	)

	flag.StringVar(&address, "address", "127.0.0.1", "Address to listen on")
	flag.StringVar(&address, "A", "127.0.0.1", "Address to listen on (shorthand)")

	flag.BoolVar(&allIfaces, "all", false, "Listen on all addresses")
	flag.BoolVar(&allIfaces, "a", false, "Listen on all addresses (shorthand)")

	flag.StringVar(&certFile, "cert", "", "Certificate bundle file - enables TLS")
	flag.StringVar(&certFile, "c", "", "Certificate bundle file - enables TLS (shorthand)")

	flag.BoolVar(&forceColor, "color", false, "Enable colour output, even if devd is not connected to a terminal")
	flag.BoolVar(&forceColor, "C", false, "Enable colour output (shorthand)")

	flag.UintVar(&downKbps, "down", 0, "Throttle downstream from the client to N kilobytes per second")
	flag.UintVar(&downKbps, "d", 0, "Throttle downstream (shorthand)")

	flag.Var(&notfound, "notfound", "Default when a static file is not found (can be repeated)")
	flag.Var(&notfound, "f", "Default when a static file is not found (shorthand, can be repeated)")

	flag.BoolVar(&logHeaders, "logheaders", false, "Log headers")
	flag.BoolVar(&logHeaders, "H", false, "Log headers (shorthand)")

	flag.Var(&ignoreLogs, "ignore", "Disable logging matching requests. Regexes are matched over 'host/path' (can be repeated)")
	flag.Var(&ignoreLogs, "I", "Disable logging matching requests (shorthand, can be repeated)")

	flag.BoolVar(&lrNaked, "livereload", false, "Enable livereload")
	flag.BoolVar(&lrNaked, "L", false, "Enable livereload (shorthand)")

	flag.BoolVar(&lrRoutes, "livewatch", false, "Enable livereload and watch for static file changes")
	flag.BoolVar(&lrRoutes, "l", false, "Enable livereload and watch (shorthand)")

	flag.BoolVar(&moddMode, "modd", false, "Modd is our parent - synonym for -LCt")
	flag.BoolVar(&moddMode, "m", false, "Modd is our parent (shorthand)")

	flag.IntVar(&latency, "latency", 0, "Add N milliseconds of round-trip latency")
	flag.IntVar(&latency, "n", 0, "Add N milliseconds of round-trip latency (shorthand)")

	flag.BoolVar(&openBrowser, "open", false, "Open browser window on startup")
	flag.BoolVar(&openBrowser, "o", false, "Open browser window on startup (shorthand)")

	flag.IntVar(&port, "port", 0, "Port to listen on - if not specified, devd will auto-pick a sensible port")
	flag.IntVar(&port, "p", 0, "Port to listen on (shorthand)")

	flag.StringVar(&credspec, "password", "", "HTTP basic password protection (USER:PASS)")
	flag.StringVar(&credspec, "P", "", "HTTP basic password protection (shorthand)")

	flag.BoolVar(&quiet, "quiet", false, "Silence all logs")
	flag.BoolVar(&quiet, "q", false, "Silence all logs (shorthand)")

	flag.BoolVar(&tlsFlag, "tls", false, "Serve TLS with auto-generated self-signed certificate (~/.devd.cert)")
	flag.BoolVar(&tlsFlag, "s", false, "Serve TLS (shorthand)")

	flag.BoolVar(&noTimestamps, "notimestamps", false, "Disable timestamps in output")
	flag.BoolVar(&noTimestamps, "t", false, "Disable timestamps in output (shorthand)")

	flag.BoolVar(&logTime, "logtime", false, "Log timing")
	flag.BoolVar(&logTime, "T", false, "Log timing (shorthand)")

	flag.UintVar(&upKbps, "up", 0, "Throttle upstream from the client to N kilobytes per second")
	flag.UintVar(&upKbps, "u", 0, "Throttle upstream (shorthand)")

	flag.Var(&watch, "watch", "Watch path to trigger livereload (can be repeated)")
	flag.Var(&watch, "w", "Watch path to trigger livereload (shorthand, can be repeated)")

	flag.BoolVar(&corsFlag, "crossdomain", false, "Set the CORS headers to allow everything (origin, credentials, headers, methods)")
	flag.BoolVar(&corsFlag, "X", false, "Set the CORS headers (shorthand)")

	flag.Var(&excludes, "exclude", "Glob pattern for files to exclude from livereload (can be repeated)")
	flag.Var(&excludes, "x", "Glob pattern for files to exclude (shorthand, can be repeated)")

	flag.BoolVar(&debug, "debug", false, "Debugging for devd development")

	flag.BoolVar(&version, "version", false, "Print version and exit")
	flag.BoolVar(&version, "v", false, "Print version and exit (shorthand)")

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
		home, err := homedir.Dir()
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
