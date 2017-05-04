package main

//author: Doug Watson

//macOS will need more open files than the default 256 if you want to run over a couple hundred goroutines
//launchctl limit maxfiles 10200

//For a really big test, if you need a million open files on a mac:
//nvram boot-args="serverperfmode=1"
//shutdown -r now
//launchctl limit maxfiles 999999
//ulimit -n 999999

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

// -------------------------------------------------------------------------------------------------
// Flags
var (
	clients     int
	targetTPS   float64
	baseUrl     string
	configFile  string
	inputFile   string
	delimeter   string
	headerExit  bool
	noHeader    bool
	cpuProfile  string
	verbose     bool
	keepAlive   bool
	testTimeout time.Duration
	readTimeout time.Duration
	rampUp      time.Duration
)

func init() {
	flag.IntVar(&clients, "c", 100, "Number of concurrent clients to launch.")
	flag.DurationVar(&testTimeout, "t", 0, "Timed load test duration (1m23s, 240s, ..). Defaults no timeout.")
	flag.StringVar(&inputFile, "f", "", "Read input from file rather than stdin")
	flag.DurationVar(&rampUp, "rampUp", -1, "Specify ramp up delay as duration (1m2s, 300ms, 0 ..). Default will auto compute from client sessions.")
	flag.Float64Var(&targetTPS, "targetTPS", 1000000, "The default max TPS is set to 1 million. Good luck reaching this :p")
	flag.StringVar(&baseUrl, "baseUrl", "", "The host to test. Example https://test2.someserver.org")
	flag.StringVar(&configFile, "configFile", "config.ini", "Config file location")
	flag.StringVar(&delimeter, "d", ",", "Output file delimeter")
	flag.BoolVar(&headerExit, "hx", false, "Print output header row and exit")
	flag.BoolVar(&noHeader, "nh", false, "Don't output header row. Default to false.")
	flag.DurationVar(&readTimeout, "readtimeout", time.Duration(30)*time.Second, "Timeout duration for the target API to send the first response byte. Default 30s")
	flag.StringVar(&cpuProfile, "cpuprofile", "", "write cpu profile to file")
	flag.BoolVar(&verbose, "verbose", false, "verbose debugging output flag")
	flag.BoolVar(&keepAlive, "keepalive", true, "enable/disable keepalive")
}

// -------------------------------------------------------------------------------------------------
// Build commit id from git
var Build string

func init() {
	if Build == "" {
		Build = "unset"
	}

	defaultUsage := flag.Usage

	flag.Usage = func() {
		defaultUsage()
		// fmt.Fprintf(os.Stderr, "\n\n")
		// PrintLogHeader(delimeter)
	}
}

func main() {
	// ---------------------------------------------------------------------------------------------
	// Parse flags
	flag.Parse()

	if headerExit {
		PrintLogHeader(delimeter)
		os.Exit(0)
	}

	// ---------------------------------------------------------------------------------------------
	// Validate input & flags
	if clients < 1 {
		flagError("Number of concurrent client should be at least 1")
	}

	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			flagError(err.Error())
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if !headerExit && baseUrl == "" {
		flagError("Please provide the baseUrl")
	}

	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	if verbose {
		println("verbose")
	}

	// ---------------------------------------------------------------------------------------------
	// Init runner
	runner := NewRunner(configFile)

	// ---------------------------------------------------------------------------------------------
	// Catch interrupt
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-signalChannel
		runner.Exit()
	}()

	// ---------------------------------------------------------------------------------------------
	// Output log headers
	if !noHeader {
		PrintLogHeader(delimeter)
		runner.PrintSessionLog() // ???
	}

	// ---------------------------------------------------------------------------------------------
	// Start clients
	if !headerExit {
		runner.printSessionSummary()
	}
	trafficChannel := make(chan string)
	//	startTraffic(trafficChannel) //start reading on the channel
	runner.StartClients(trafficChannel)

	// ---------------------------------------------------------------------------------------------
	// Read input from file or stdin
	scanner := bufio.NewScanner(os.Stdin)
	if len(inputFile) > 0 {
		file, err := os.Open(inputFile)
		if err != nil {
			flagError(err.Error())
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	}

	for scanner.Scan() {
		trafficChannel <- scanner.Text() //put work into the channel from Stdin
	}
	close(trafficChannel)

	// ---------------------------------------------------------------------------------------------
	// Wait for clients to be done and exit
	runner.Wait()
	runner.Exit()
}

func flagError(err string) {
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n%s\n\n", err)
	os.Exit(1)
}

/*
func generateInput() chan string {
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		//scanner.Err()
		fmt.Printf("scanner done:\n\n")
	}()
	return lines
}
*/
//func noRedirect(req *http.Request, via []*http.Request) error {
//	return errors.New("Don't redirect!")
//}
