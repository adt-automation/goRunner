package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

var port string

//var baseUrl string

func TestXxx(*testing.T) {
	verbose = true
	done := make(chan bool)
	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	go func() {
		<-sigs
		done <- true
	}()

	go http.ListenAndServe(":9009", nil)
	http.HandleFunc("/main", Landing)
	http.HandleFunc("/GetSession", ReturnSession)
	http.HandleFunc("/reqapi/check/token", HandleToken)
	http.HandleFunc("/reqapi/test/account/pass", HandleTicket)
	http.HandleFunc("/reqapi/test/account/pass/", HandleTicket2)

	clients = 1
	baseUrl = "http://localhost:9009"
	// ---------------------------------------------------------------------------------------------
	// Init runner
	runner := NewRunner("test_public.ini")
	// ---------------------------------------------------------------------------------------------
	// Start clients
	trafficChannel := make(chan string)
	//	startTraffic(trafficChannel) //start reading on the channel

	runner.StartClients(trafficChannel)

	// ---------------------------------------------------------------------------------------------
	// Output

	runner.printSessionSummary()

	PrintLogHeader(",", 1)
	runner.PrintSessionLog() // ???

	trafficChannel <- "1,2" //put work from the line we read to get nbDelimeters
	close(trafficChannel)
	// ---------------------------------------------------------------------------------------------
	// Wait for clients to be done and exit
	runner.Wait()
	runner.Exit()

	/*
			baseUrlFilter := regexp.MustCompile(baseUrl)


			transportConfig := &tls.Config{InsecureSkipVerify: true} //allow wrong ssl certs
			tr := &http.Transport{TLSClientConfig: transportConfig}
			tr.ResponseHeaderTimeout = time.Second * time.Duration(30) //timeout in seconds
			var stopTime time.Time                                     // client loops will work to the end of trafficChannel unless we explicitly init a stopTime
			result := &runner.Result{}
			results[0] = result
			msDelay := 0
			grep1, grep2 := "", ""
			id := "123"
			clientId := 0
			var cookieMap = make(map[string]*http.Cookie)
			var sessionVars = make(map[string]string)
			overallStartTime := time.Now()
			d := runner.Delimeter
			fmt.Printf("startTime%vcommand%vnextCommand%vstep%vrequestType%vsessionKey%vsession%vgrep1%vgrep2%vid%vshortUrl%vstatusCode%vsessionVarsOk%vclientId%vbyteSize%vserver%vduration%vserverDuration\n", d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, d)
		//	runner.DoReq(0, id, configuration, result, clientId, baseUrl, baseUrlFilter, msDelay, tr, cookieMap, sessionVars, grep1, grep2, stopTime, 0.0) //val,resp, err


	*/
	//	results := make(map[int]*runner.Result)
	//				result := &runner.Result{}
	//			results[0] = result
	//	message, exitCode := runner.GetResults(results, time.Now())

	//	runner.PrintResults(message, overallStartTime)
	exitCode := 0
	os.Exit(int(exitCode))

}

func ReturnSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("SESSIONVAR", "C1NC0d3M6Y0")
	w.WriteHeader(200)
	w.Write([]byte("TokenValue:=abcdefgh.localserver.com"))
	io.WriteString(w, "Return SessionVariable, TokenValue=TOKEN, UserID in header")
}

func Landing(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "This is the Landing Page Body")
}

func HandleToken(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	access_type := r.FormValue("access_type")

	response := ""
	switch access_type {
	case "password":
		response = "{\"access_token\":\"babd1aed-2e5b-40bb-a53f-d31d69d8571a\",\"refresh_token\":\"a77a8bfe-e9e6-4009-90f8-3e817685fed9\",\"token_type\":\"Carier\",\"expires_in\":3599,\"scope\":\"DEFAULT_SCOPE\"}"
	case "refresh_token":
		response = "{\"access_token\":\"babd1aed-2e5b-40bb-a53f-d31d69d8571a\",\"refresh_token\":\"a77a8bfe-e9e6-4009-90f8-3e817685fed9\",\"token_type\":\"Carier\",\"expires_in\":3599,\"scope\":\"DEFAULT_SCOPE\"}"
	}
	fmt.Fprintf(w, "%v\n", response)
}
func HandleTicket(w http.ResponseWriter, r *http.Request) {
	call_id := r.URL.Query().Get("call_id")
	response := ""
	if call_id == "" {
		response = "{\"call_id\":\"testing_owner@xyz.com\"}"
	} else {
		response = "{\"pass\":\"451dbd05-67bd-4397-9557-ccfef789d8f0\"}"
	}
	fmt.Fprintf(w, "%v\n", response)
}

func HandleTicket2(w http.ResponseWriter, r *http.Request) {
	pathArr := strings.SplitN(r.URL.RequestURI()[1:] /*trim the first slash*/, "/", -1)
	fmt.Fprintf(w, "PATH=%v\n", pathArr[4])
}
