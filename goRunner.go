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
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"
)

// -------------------------------------------------------------------------------------------------
// Flags
var (
	clients     int
	debug       bool
	dump        bool
	targetTPS   float64
	baseUrl     string
	configFile  string
	inputFile   string
	inputFile1  string
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
	flag.DurationVar(&testTimeout, "t", 0, "Time in seconds for a timed load test")
	flag.StringVar(&inputFile, "f", "", "Read input from file rather than stdin")
	flag.StringVar(&inputFile1, "fh", "", "Read input from csv file which has a header row")
	flag.DurationVar(&rampUp, "rampUp", -1, "Specify ramp up delay as duration (1m2s, 300ms, 0 ..). Default will auto compute from client sessions.")
	flag.Float64Var(&targetTPS, "targetTPS", 1000000, "The default max TPS is set to 1 million. Good luck reaching this :p")
	flag.StringVar(&baseUrl, "baseUrl", "", "The host to test. Example https://test2.someserver.org")
	flag.BoolVar(&debug, "debug", false, "Show the body returned for the api call")
	flag.BoolVar(&dump, "dump", false, "Show the HTTP dump for the api call")
	flag.StringVar(&configFile, "configFile", "config.ini", "Config file location")
	flag.StringVar(&delimeter, "d", ",", "Output file delimeter")
	flag.BoolVar(&headerExit, "hx", false, "Print output header row and exit")
	flag.BoolVar(&noHeader, "nh", false, "Suppress output header row. Ignored if hx is set")
	flag.DurationVar(&readTimeout, "readtimeout", time.Duration(30)*time.Second, "Timeout in seconds for the target API to send the first response byte. Default 30 seconds")
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
}

func main() {
	// ---------------------------------------------------------------------------------------------
	// Parse flags
	flag.Parse()

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

	if len(inputFile1) > 0 {
		if len(inputFile) > 0 {
			flagError("don't use -f and -fh together")
		} else {
			inputFile = inputFile1
		}
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
	results := make(map[int]*Result)
	rampUpDelay := runner.RampUpDelay()
	trafficChannel := make(chan string)
	//	startTraffic(trafficChannel) //start reading on the channel
	httpTransport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //allow wrong ssl certs
	httpTransport.ResponseHeaderTimeout = readTimeout

	// ---------------------------------------------------------------------------------------------
	// Catch interrupt
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-signalChannel
		runner.ExitWithStatus(results)
	}()

	if !headerExit {
		runner.printSessionSummary()
	}

	fmt.Fprintf(os.Stderr, "Spacing sessions %v apart to ramp up to %d client sessions\n", rampUpDelay, clients)

	// ---------------------------------------------------------------------------------------------
	// Start clients
	var wgClients sync.WaitGroup
	var stopTime time.Time // client loops will work to the end of trafficChannel unless we explicitly init a stopTime
	if testTimeout > 0 {
		stopTime = time.Now().Add(testTimeout)
	}
	for i := 0; i < clients; i++ {
		wgClients.Add(1)
		result := &Result{}
		results[i] = result
		clientDelay := rampUpDelay * time.Duration(i)
		go client(httpTransport, runner, result, &wgClients, trafficChannel, i, stopTime, clientDelay)
	}

	// ---------------------------------------------------------------------------------------------
	// Read input from file or stdin
	scanner := bufio.NewScanner(os.Stdin)
	if len(inputFile) > 0 {
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err.Error())
			os.Exit(1)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	}

	_ = scanner.Scan()
	if !noHeader {
		runner.PrintLogHeader(scanner.Text(), len(inputFile1) > 0)
		if headerExit {
			os.Exit(0)
		}
	}
	if len(inputFile1) > 0 && HasInputColHeaders() == false {
		// name our column headers
		HeadInputColumns(scanner.Text())
	} else {
		// anonymous column headers, put work into the channel instead
		trafficChannel <- scanner.Text()
	}

	for scanner.Scan() {
		trafficChannel <- scanner.Text() //put work into the channel from Stdin
	}
	close(trafficChannel)
	wgClients.Wait()
	runner.ExitWithStatus(results)
}

//one client per -c=thread
//func noRedirect(req *http.Request, via []*http.Request) error {
//	return errors.New("Don't redirect!")
//}
func client(tr *http.Transport, runner *Runner, result *Result, done *sync.WaitGroup, trafficChannel chan string, clientId int, stopTime time.Time, initialDelay time.Duration) {
	defer done.Done()
	// strangely, this sleep cannot be moved outside the client function
	// or all client threads will wait until the last one is spawned before starting their own execution loops
	time.Sleep(initialDelay)

	msDelay := 0
	for id := range trafficChannel {
		//read in the line. either a generated id or (TODO) a url
		//		id := <-trafficChannel //urlTemplate : 100|10000|about

		//Do Heartbeat
		// cookieMap and sessionVars should start fresh every time we start a DoReq session
		var cookieMap = make(map[string]*http.Cookie)
		var sessionVars = make(map[string]string)
		runner.DoReq(0, id, result, clientId, baseUrl, msDelay, tr, cookieMap, sessionVars, stopTime, 0.0) //val,resp, err
		msDelay = runner.PostSessionDelay
	}
	if time.Now().Before(stopTime) {
		fmt.Fprintf(os.Stderr, "client %d ran out of test input %.2fs before full test time\n", clientId, stopTime.Sub(time.Now()).Seconds())
	}
}

func flagError(err string) {
	fmt.Fprintf(os.Stderr, "\n%s\n\n", err)
	flag.Usage()
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
