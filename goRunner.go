package main

//author: Doug Watson

//macOS will need more open files than the default 256 if you want to run over a couple hundred goroutines
//launchctl limit maxfiles 10200

//For a really big test, if you need a million open files on a mac:
//nvram boot-args="serverperfmode=1"
//shutdown -r now
//launchctl limit maxfiles 999990
//ulimit -n 999998

//to build"
//BUILD=`git rev-parse HEAD`
//GOOS=linux go build -o goRunner.linux -ldflags "-s -w -X main.Build=${BUILD}"
import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// -------------------------------------------------------------------------------------------------
// Flags
var (
	clients        int
	targetTPS      float64
	baseUrl        string
	configFile     string
	inputFile      string
	delimeter      string
	headerExit     bool
	noHeader       bool
	cpuProfile     string
	verbose        bool
	keepAlive      bool
	testTimeout    time.Duration
	readTimeout    time.Duration
	rampUp         time.Duration
	listenPort     int
	trafficChannel chan string
)

func init() {
	flag.IntVar(&clients, "c", 100, "Number of concurrent clients to launch.")
	flag.DurationVar(&testTimeout, "t", 0, "Timed load test duration (1m23s, 240s, ..). Defaults no timeout.")
	flag.StringVar(&inputFile, "f", "", "Read input from file rather than stdin")
	flag.DurationVar(&rampUp, "rampUp", -1, "Specify ramp up delay as duration (1m2s, 300ms, 0 ..). Default will auto compute from client sessions.")
	flag.Float64Var(&targetTPS, "targetTPS", 1000000, "The default max TPS is set to 1 million. Good luck reaching this :p")
	flag.StringVar(&baseUrl, "baseUrl", "", "The host to test. Example https://test2.someserver.org")
	flag.StringVar(&configFile, "configFile", "config.ini", "Config file location")
	flag.StringVar(&delimeter, "delimeter", ",", "Delimeter for output csv and input file")
	flag.BoolVar(&headerExit, "hx", false, "Print output header row and exit")
	flag.BoolVar(&noHeader, "nh", false, "Don't output header row. Default to false.")
	flag.DurationVar(&readTimeout, "readtimeout", time.Duration(30)*time.Second, "Timeout duration for the target API to send the first response byte. Default 30s")
	flag.StringVar(&cpuProfile, "cpuprofile", "", "write cpu profile to file")
	flag.BoolVar(&verbose, "verbose", false, "verbose debugging output flag")
	flag.BoolVar(&keepAlive, "keepalive", true, "enable/disable keepalive")
	flag.IntVar(&listenPort, "p", 0, "Default off. Port to listen on for input (as opposed to STDIN). HTTP GET or POST calls accepted i.e http://localhost/john,pass1\nor\ncurl POST http://localhost -d 'john,pass1\ndoug,pass2\n'")
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
	}
}

func main() {
	// ---------------------------------------------------------------------------------------------
	// Parse flags
	flag.Parse()

	if headerExit {
		PrintLogHeader(delimeter, 0)
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
		println("verbose: build=", Build)
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
	// Start clients
	trafficChannel = make(chan string)
	//	startTraffic(trafficChannel) //start reading on the channel
	runner.StartClients(trafficChannel)

	// ---------------------------------------------------------------------------------------------
	// Read input from file or stdin

	if listenPort > 0 {
		listenPortString := strconv.Itoa(listenPort)

		http.HandleFunc("/", HandleInputArgs)
		http.ListenAndServe(":"+listenPortString, nil)
	} else {

		scanner := bufio.NewScanner(os.Stdin)
		if len(inputFile) > 0 {
			file, err := os.Open(inputFile)
			if err != nil {
				flagError(err.Error())
			}
			defer file.Close()
			scanner = bufio.NewScanner(file)
		}
		// ---------------------------------------------------------------------------------------------
		// Output
		nbDelimeters := 0
		firstTime := true
		for scanner.Scan() {
			fmt.Printf("scanner.Scan\n")
			inputLine := scanner.Text()
			if firstTime {
				fmt.Printf("START FIRST_TIME\n")
				firstTime = false
				nbDelimeters = strings.Count(inputLine, delimeter)
				runner.printSessionSummary()
				if !noHeader {
					PrintLogHeader(delimeter, nbDelimeters+1)
					runner.PrintSessionLog() // ???
				}
				fmt.Printf("END FIRST_TIME\n")
			}
			if strings.Count(inputLine, delimeter) != nbDelimeters {
				fmt.Fprintf(os.Stderr, "\n/!\\ input lines must have same number of fields /!\\\n")
				runner.Exit()
			}
			if len(inputLine) == 0 {
				break //quit when we get an empty input line
			}
			fmt.Printf("inputLine=%v\n\n", inputLine)
			trafficChannel <- inputLine
		}
	}
	close(trafficChannel)

	// ---------------------------------------------------------------------------------------------
	// Wait for clients to be done and exit
	runner.Wait()
	runner.Exit()
}
func HandleInputArgs(w http.ResponseWriter, r *http.Request) {
	scanner := bufio.NewScanner(r.Body)
	for scanner.Scan() {
		inputLine := scanner.Text()
		trafficChannel <- inputLine
	}
	w.WriteHeader(200)
}

func flagError(err string) {
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n%s\n\n", err)
	os.Exit(1)
}

//func noRedirect(req *http.Request, via []*http.Request) error {
//	return errors.New("Don't redirect!")
//}
