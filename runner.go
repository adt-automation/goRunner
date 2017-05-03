package main

//author: Doug Watson

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// -------------------------------------------------------------------------------------------------
// Models
type Result struct {
	Requests        int32
	success         int32
	networkFailed   int32
	badFailed       int32
	readThroughput  int32
	writeThroughput int32
}

type Runner struct {
	config                 *Config
	stdoutMutex            sync.Mutex
	CommandQueue           []string
	SessionCookieName      string
	alwaysFoundSessionVars bool
	PostSessionDelay       int
	GrepCommand            *regexp.Regexp
}

func NewRunner(config *Config) *Runner {
	toReturn := &Runner{config: config, alwaysFoundSessionVars: true}
	toReturn.GrepCommand = regexp.MustCompile(config.Search.CommandGrep)
	toReturn.SessionCookieName = config.Search.SessionCookieName

	// ---------------------------------------------------------------------------------------------
	// Init command queue
	if len(config.CommandSequence.Sequence) > 0 {
		for _, cmd := range strings.Split(config.CommandSequence.Sequence, ",") {
			toReturn.CommandQueue = append(toReturn.CommandQueue, strings.TrimSpace(cmd))
		}
	} else {
		toReturn.CommandQueue = append(toReturn.CommandQueue, "_start")
		cmd := config.Command["_start"].DoCall
		for len(cmd) > 0 {
			if cmd == "none" {
				break
			} else {
				toReturn.CommandQueue = append(toReturn.CommandQueue, cmd)
				cmd = config.Command[cmd].DoCall
			}
		}
	}
	li := len(toReturn.CommandQueue) - 1
	if li >= 0 {
		toReturn.PostSessionDelay = config.FieldInteger("MsecDelay", toReturn.CommandQueue[li])
	}

	// ---------------------------------------------------------------------------------------------
	// Init runner macros
	for _, cmd := range toReturn.CommandQueue {
		InitMacros(cmd, config.FieldString("ReqBody", cmd))
		InitMacros(cmd, config.FieldString("ReqUrl", cmd))
		InitMacros(cmd, config.FieldString("EncryptIv", cmd))
		InitMacros(cmd, config.FieldString("EncryptKey", cmd))
		for _, session_var := range config.Command[cmd].SessionVar {
			s := strings.SplitN(session_var, " ", 2) // s = ['CUSTNO', '<extId>{%VAL}</extId>']
			InitMacros(cmd, s[1])
		}
		InitMd5Macro(cmd, config.FieldString("Md5Input", cmd))
		InitBase64Macro(cmd, config.FieldString("Base64Input", cmd))
	}
	InitSessionLogMacros(config.CommandSequence.SessionLog)
	InitUnixtimeMacros()

	return toReturn
}

func (runner *Runner) PrintLogHeader(inputLine1 string, isInputHeader bool) {
	d := delimeter[0]
	if strings.Index(inputLine1, ",") == -1 {
		inputLine1 = ""
	} else {
		inputLine1 = inputLine1[strings.Index(inputLine1, ",")+1:] // skip past first comma
		if isInputHeader {
			inputLine1 = "," + inputLine1 // put the initial comma back onto the string
		} else {
			vals := make([]string, 0)
			for i, _ := range strings.Split(inputLine1, ",") { // substitute "value0", etc, since this is not a column header
				vals = append(vals, fmt.Sprintf(",value%d", i))
			}
			inputLine1 = strings.Join(vals, ",")
		}
		if d != ',' {
			inputLine1 = strings.Replace(inputLine1, ",", delimeter[0:1], -1)
		}
	}
	runner.stdoutMutex.Lock()
	fmt.Printf("startTime%ccommand%cstep%crequestType%csessionKey%csession%cid%cshortUrl%cstatusCode%csessionVarsOk%cclientId%cbyteSize%cserver%cduration%cserverDuration%cbuildId%s\n", d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, inputLine1)
	runner.stdoutMutex.Unlock()
	if len(runner.config.CommandSequence.SessionLog) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Replace(strings.Replace(strings.Replace(runner.config.CommandSequence.SessionLog, "{%", "", -1), "{$", "", -1), "}", "", -1))
	}
}

func (runner *Runner) EstimateSessionTime() time.Duration {
	ncq := len(runner.CommandQueue)
	dur := time.Duration(ncq*100) * time.Millisecond // estimate 100ms / call
	for i := 0; i < ncq; i++ {
		repeat := runner.config.FieldInteger("MsecRepeat", runner.CommandQueue[i])
		if repeat > 0 {
			dur += time.Millisecond * time.Duration(repeat)
		} else if i < ncq-1 { // no post-call delay for final command in sesssion sequence
			dur += time.Millisecond * time.Duration(runner.config.FieldInteger("MsecDelay", runner.CommandQueue[i]))
		}
	}
	return dur
}

func (runner *Runner) RampUpDelay() time.Duration {
	if clients == 0 {
		return time.Duration(0)
	} else {
		dur := runner.EstimateSessionTime()
		return dur / time.Duration(clients)
	}
}

func (runner *Runner) httpReq(inputData string, config *Config, command string, baseUrl string, tr *http.Transport, cookieMap map[string]*http.Cookie, sessionVars map[string]string, reqTime time.Time) (*http.Request, *http.Response, error) {

	var reqErr error

	//this is where all the good stuff happens
	//"DEVICE_INFORMATION", "RING", "SET_ADMIN", "MESSAGE", "INSTALL_MDM", "InstallProfile", "TENANT_INFO", ...
	arr := strings.Split(inputData, delimeter) // for 2 value inputs to stdin
	var key, val, body, urlx string
	var r *strings.Replacer
	if len(arr) > 1 {
		key = arr[0]
		val = arr[1] //need to check if this exists, it will only be in the input line for APIs that req. 2 inputs
		//add here if you need to add new config substitutions
		r = strings.NewReplacer(
			"{%KEY}", key,
			"{%VAL}", val,
		)
	} else {
		key = inputData //no delimeter in the input, so we take the whole line as the key
		//and here for new config substitutions
		r = strings.NewReplacer(
			"{%KEY}", key,
		)
	}

	body = r.Replace(config.FieldString("ReqBody", command))
	urlx = config.FieldString("ReqUrl", command)
	if strings.HasPrefix(urlx, "http://") || strings.HasPrefix(urlx, "https://") {
		urlx = r.Replace(urlx)
	} else {
		urlx = r.Replace(baseUrl + urlx)
	}

	requestContentType := config.FieldString("ReqContentType", command)
	requestType := config.FieldString("ReqType", command)

	body = RunnerMacros(command, inputData, sessionVars, reqTime, body)
	urlx = RunnerMacros(command, inputData, sessionVars, reqTime, urlx)

	reqReader := io.Reader(bytes.NewReader([]byte(body)))
	requestContentSize := int64(len(body))

	reqUpload := config.FieldString("ReqUpload", command)
	if len(reqUpload) > 0 {
		file, err := os.Open(reqUpload)
		defer file.Close()
		if err != nil {
			return nil, nil, err
		}
		fi, err := file.Stat()
		if err != nil {
			return nil, nil, err
		}
		reqReader = file // io.File implements io.Reader
		requestContentSize = fi.Size()
	}

	req, reqErr := http.NewRequest(requestType, urlx, reqReader)

	if reqErr != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "\nERROR=%v URL==%v requestType=%v body=%v\n", reqErr, urlx, requestType, body)
		}
		fmt.Fprintf(os.Stderr, "ERROR: command %s input %s TODO- Need a log entry here because we returned without logging due to an error generating the request!\n", command, inputData)
		var empty *http.Response
		return req, empty, reqErr
	}

	// default headers here
	for _, hdr := range config.Command["default"].ReqHeaders {
		str := strings.Split(hdr, ":")
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}
	// command-specific headers here
	for _, hdr := range config.Command[command].ReqHeaders {
		str := strings.Split(hdr, ":")
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}

	// any duplicate headers here will replace defaults
	if len(requestContentType) > 0 {
		req.Header.Set("Content-Type", requestContentType)
	}

	for hdr, vals := range req.Header {
		req.Header.Set(hdr, strings.Replace(vals[0], "{%KEY}", string(inputData), -1))
	}
	if requestContentSize > 0 {
		req.ContentLength = requestContentSize
	}

	// hack for https://github.com/golang/go/issues/7682
	if len(req.Header.Get("Host")) > 0 {
		req.Host = req.Header.Get("Host")
	}

	for _, cookie := range cookieMap {
		// verify the cookie should be sent to requests of this host and path
		if strings.HasSuffix(req.Host, cookie.Domain) && strings.HasPrefix(req.URL.Path, cookie.Path) {
			// check for a prior cookie with the same name
			priorCookie, _ := req.Cookie(cookie.Name)
			// replace it if this cookie's path is more specific, or if the path is the same and this cookie's domain is more specific
			if priorCookie == nil || len(priorCookie.Path) < len(cookie.Path) || (len(priorCookie.Path) == len(cookie.Path) && len(priorCookie.Domain) < len(cookie.Domain)) {
				req.AddCookie(cookie)
			}
		}
	}

	//session := ""
	/*
		if session != "" {
			//getting the previous session from the function argument, saves us a hashmap lookup
			//also allows us the option to make virtual device interactions without using a session hashmap
			//this will normally be used during registration loops before a session has been saved to the session hashmap
			expiration := time.Now().Add(365 * 24 * time.Hour)
			//JSESSIONID=17BAC3B4C633DCE99E6494BA8FF622A1.aurlt3621; Path=/admin; Secure; HttpOnly
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session, Expires: expiration})
			//, Path: /admin, Secure: true, HttpOnly: true
			//}else if sessionMap[mdi] != "" {
			//	expiration := time.Now().Add(365 * 24 * time.Hour)
			//	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionMap[mdi], Expires: expiration})
		} else {
			if debug {
				fmt.Fprintf(os.Stderr, "Session missing: session=%s\n", mdi)
			}
		}
	*/
	if verbose {
		dumpBody := requestContentSize <= 512
		dump, err := httputil.DumpRequestOut(req, dumpBody)
		if err == nil {
			fmt.Fprintf(os.Stderr, "REQUEST DUMP==============\n\n%v\n", string(dump))
			if !dumpBody {
				fmt.Fprintf(os.Stderr, "============== UPLOADING %db REQUEST BODY ======\n", requestContentSize)
			}
		} else {
			fmt.Fprintf(os.Stderr, "REQUEST DUMP ERROR========\n\n%v\n", err)
		}
	}

	resp, err := httpRoundTrip(tr, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s ERROR %s: %v\n", command, inputData, time.Now(), err.Error())
	} else if verbose {
		dump2, err2 := httputil.DumpResponse(resp, true)
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "%s RESPONSE DUMP ERROR========\n\n%v\n", command, err2)
		} else {
			fmt.Fprintf(os.Stderr, "RESPONSE DUMP==============\n%v\n\n", string(dump2))
		}
	}

	//start tracking the sessions now. Before we kept the cookies are 1-to-1-to-1 between the device, mdi and account ids
	//this was possible due to the following simplification prior to loading the devices (it skips the 2 admin users in the setup)
	//alter table adam2db.devices  AUTO_INCREMENT=3;
	//alter table adam2db.mdis  AUTO_INCREMENT=3;

	if resp != nil {
		for _, cookie := range resp.Cookies() {
			if len(cookie.Domain) == 0 {
				// explicitly set the domain if the cookie didn't include one,
				// so that it can't jump over to a different api host
				cookie.Domain = req.Host
			}
			cookieKey := cookie.Domain + "\n" + cookie.Path + "\n" + cookie.Name
			cookieMap[cookieKey] = cookie
		}
	}

	return req, resp, err
}

func httpRoundTrip(tr *http.Transport, req *http.Request) (*http.Response, error) {
	if keepAlive {
		// tr.RoundTrip avoids automatic redirects
		return tr.RoundTrip(req)
	} else {
		// this "set object to dereferenced object" will execute a deep copy
		var tr1 http.Transport = *tr
		return tr1.RoundTrip(req)
	}
}

// e.g. servAddr := "gsess-dr.adtpulse.com:11083"
func (runner *Runner) tcpReq(inputData string, config *Config, command string, servAddr string, sessionVars map[string]string) []byte {

	var reqTime time.Time = time.Now()

	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ResolveTCPAddr failed: %s\n", err.Error())
		os.Exit(1)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Dial failed: %s\n", err.Error())
		os.Exit(1)
	}

	input := config.FieldString("ReqBody", command)
	input = RunnerMacros(command, inputData, sessionVars, reqTime, input)

	send, err := hex.DecodeString(strings.Replace(input, " ", "", -1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hex decode failed: %s\n", err.Error())
		os.Exit(1)
	}

	encryptStart := config.FieldInteger("EncryptStartByte", command) - 1
	encryptCt := config.FieldInteger("EncryptNumBytes", command)
	if encryptCt > 0 && encryptStart > -1 {
		if encryptStart+encryptCt > len(send) {
			fmt.Fprintf(os.Stderr, "command %s: encrypt range past end of input text\n", command)
			os.Exit(1)
		}
		ebytes := send[encryptStart : encryptStart+encryptCt]

		ivStr := config.FieldString("EncryptIv", command)
		ivStr = RunnerMacros(command, inputData, sessionVars, reqTime, ivStr)
		iv, err := hex.DecodeString(strings.Replace(ivStr, " ", "", -1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "command %s hex decode failed: %s\n", command, err.Error())
			os.Exit(1)
		}
		iv = buildIv(reqTime)
		keyStr := config.FieldString("EncryptKey", command)
		keyStr = RunnerMacros(command, inputData, sessionVars, reqTime, keyStr)

		if len(keyStr) == 0 {
			log.Println("encryption key has empty value")

		}
		key := buildKey(keyStr)
		encrypted, err := encrypt(key, iv, ebytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "command %s encrypt error: %v\n", command, err.Error())
			os.Exit(1)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "%s PRE-ENCRYPT: % x\n", command, send)
		}
		send = bytes.Replace(send, ebytes, encrypted, 1)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "%s TCP SEND: % x\n", command, send)
	}

	_, err = conn.Write(send)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Write to server failed: %s\n", err.Error())
		os.Exit(1)
	}

	// 1024 handles up to 0x40 in first 2 bytes
	// 65,535 is largest response the first 2 bytes could indicate
	// but we won't allocate that much yet because the LWG is currently sending 10 byte responses
	reply := make([]byte, 1024)

	_, err = conn.Read(reply)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Write to server failed: %s\n", err.Error())
		os.Exit(1)
	}
	conn.Close()

	// first 2 bytes are 000a, indicating 10 byte response
	// example reply := []byte{0x00, 0x0a, 0x01, 0x57, 0x1e, 0xb0, 0x3c, 0xf7, 0x01, 0x00}
	responseLen := int(reply[0])*256 + int(reply[1])
	if verbose {
		fmt.Fprintf(os.Stderr, "%s TCP REPLY: % x\n", command, reply[0:responseLen])
	}
	return reply[0:responseLen]
}

func (runner *Runner) DoReq(stepCounter int, mdi string, result *Result, clientId int, baseUrl string, baseUrlFilter *regexp.Regexp, delay int, tr *http.Transport, cookieMap map[string]*http.Cookie, sessionVars map[string]string, stopTime time.Time, commandTime float64) string {

	if !stopTime.IsZero() && time.Now().Add(time.Duration(delay)*time.Millisecond).After(stopTime) {
		return ""
	}
	time.Sleep(time.Duration(delay) * time.Millisecond) // default value is 0 milliseconds

	var (
		tcpReply    []byte
		httpResp    *http.Response
		httpError   error
		httpRequest *http.Request
		shortUrl    string
	)

	command := runner.CommandQueue[stepCounter]
	stepCounter += 1
	session := ""
	continueSession := true

	_, ok := runner.config.Command[command]
	if !ok {
		fmt.Fprintf(os.Stderr, "ERROR: command %q is not defined in the .ini file\n", command)
	} else {
		// -----------------------------------------------------------------------------------------
		// Perform TCP or HTTP request
		startTime := time.Now()
		requestType := runner.config.FieldString("ReqType", command)
		if requestType == "TCP" {
			tcpReply = runner.tcpReq(mdi, runner.config, command, baseUrl, sessionVars)
			shortUrl = (baseUrlFilter).ReplaceAllString(baseUrl, "")
		} else {
			httpRequest, httpResp, httpError = runner.httpReq(mdi, runner.config, command, baseUrl, tr, cookieMap, sessionVars, startTime)
			shortUrl = (baseUrlFilter).ReplaceAllString(httpRequest.URL.String(), "")
			requestType = httpRequest.Method

		}

		// -----------------------------------------------------------------------------------------
		// Process request response & log
		session, continueSession = runner.doLog(command, runner.config, requestType, tcpReply, httpResp, result, httpError, startTime, shortUrl, mdi, clientId, stepCounter, "", sessionVars)

		delay = runner.config.FieldInteger("MsecDelay", command)

		repeatTime := float64(runner.config.FieldInteger("MsecRepeat", command))
		requestTime := float64(delay) + (time.Since(startTime)).Seconds()*1000.0
		// add delay again here, to find out whether the delay will put us past the repeat time
		if commandTime+requestTime+float64(delay) < repeatTime {
			stepCounter -= 1
			commandTime += requestTime
		} else {
			commandTime = 0.0
		}
	}

	if verbose && stepCounter < len(runner.CommandQueue) {
		fmt.Fprintf(os.Stderr, "mdi %s stepCounter %d nextCommand=%v\n", mdi, stepCounter, runner.CommandQueue[stepCounter])
	}

	if continueSession && stepCounter < len(runner.CommandQueue) && runner.CommandQueue[stepCounter] != "none" {
		session = runner.DoReq(stepCounter, mdi, result, clientId, baseUrl, baseUrlFilter, delay, tr, cookieMap, sessionVars, stopTime, commandTime)
	} else if len(runner.config.CommandSequence.SessionLog) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", SessionLogMacros(mdi, sessionVars, time.Now(), runner.config.CommandSequence.SessionLog))
	}

	return session
}

func (runner *Runner) printSessionSummary() {
	fmt.Fprintf(os.Stderr, "GORUNNER\n")
	if len(runner.config.Version.ConfigVersion) > 5 {
		fmt.Fprintf(os.Stderr, "Configuration:                  %s version %s\n", configFile, runner.config.Version.ConfigVersion[5:len(runner.config.Version.ConfigVersion)-1])
	} else {
		fmt.Fprintf(os.Stderr, "Configuration:                  %s\n", configFile)
	}
	fmt.Fprintf(os.Stderr, "Session profile:                %d requests: %s\n", len(runner.CommandQueue), strings.Join(runner.CommandQueue, ", "))
	fmt.Fprintf(os.Stderr, "Estimated session time:         %v\n", runner.EstimateSessionTime())
	fmt.Fprintf(os.Stderr, "Simultaneous sessions:          %d\n", clients)
	fmt.Fprintf(os.Stderr, "API host:                       %s\n", baseUrl)
	const layout = "2006-01-02 15:04:05"
	fmt.Fprintf(os.Stderr, "Test start time:                %v\n", time.Now().Format(layout))
}

func GetResults(results map[int]*Result, overallStartTime time.Time) map[string]int32 {
	summary := make(map[string]int32)
	for _, result := range results {
		summary["requests"] += result.Requests
		summary["success"] += result.success
		summary["networkFailed"] += result.networkFailed
		summary["badFailed"] += result.badFailed
		summary["readThroughput"] += result.readThroughput
		summary["writeThroughput"] += result.writeThroughput
		//need to get final time here (last log entry time) in case the user hits contrl-c late after a run is done.
	}
	return summary
}
func (runner *Runner) ExitWithStatus(results map[int]*Result, overallStartTime time.Time) {
	myMap := GetResults(results, overallStartTime)
	PrintResults(myMap, overallStartTime)
	os.Exit(runner.exitStatus(myMap))
}

func PrintResults(myMap map[string]int32, overallStartTime time.Time) {
	elapsed := int64(time.Since(overallStartTime).Seconds())
	if elapsed == 0 {
		elapsed = 1
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Requests:                       %10d hits\n", myMap["requests"])
	fmt.Fprintf(os.Stderr, "Successful requests:            %10d hits\n", myMap["success"])
	fmt.Fprintf(os.Stderr, "Network failed:                 %10d hits\n", myMap["networkFailed"])
	fmt.Fprintf(os.Stderr, "Bad requests failed (!2xx):     %10d hits\n", myMap["badFailed"])
	fmt.Fprintf(os.Stderr, "Successfull requests rate:      %10f hits/sec\n", float32(myMap["success"])/(float32(elapsed)+.01))
	fmt.Fprintf(os.Stderr, "Read throughput:                %f bytes/sec\n", float32(myMap["readThroughput"])/(float32(myMap["elapsed"])+.01))
	fmt.Fprintf(os.Stderr, "Write throughput:               %f bytes/sec\n", float32(myMap["writeThroughput"])/(float32(myMap["elapsed"])+.01))
	fmt.Fprintf(os.Stderr, "Test time:                      %d sec\n", elapsed)
	const layout = "2006-01-02 15:04:05"
	fmt.Fprintf(os.Stderr, "Test end time:                  %v\n", time.Now().Format(layout))
}

func (runner *Runner) exitStatus(myMap map[string]int32) int {
	if myMap["success"] != myMap["requests"] {
		return 32
	} else if !runner.alwaysFoundSessionVars {
		return 33
	} else {
		return 0
	}
}

func (runner *Runner) findSessionVars(command string, config *Config, input string, inputData string, startTime time.Time, sessionVars map[string]string, hex bool) (bool, bool) {

	if len(input) <= 2 {
		// automatically false due to no chance to capture the session var
		return false, false
	}

	foundSessionVars := true
	foundMustCaptures := true

	// set any session vars listed for current command, e.g. SessionVar = XTOKEN detail="(.+)"
	for _, session_var := range config.Command[command].SessionVar {
		s := strings.SplitN(session_var, " ", 2) // s = ['XTOKEN', 'detail="(.+)"']
		svar := s[0]
		sgrep := RunnerMacrosRegexp(command, inputData, sessionVars, startTime, s[1])
		regex := regexp.MustCompile(sgrep) // /detail="(.+)"/
		if len(regex.String()) <= 0 {
			continue
		}

		svals := regex.FindStringSubmatch(input)
		limit := 32 // hex only
		if len(svals) > 1 {
			if hex {
				if len(svals[1]) < 32 {
					limit = len(svals[1])
				}
				sessionVars[svar] = svals[1][0:limit]
			} else {
				sessionVars[svar] = svals[1] // detail="abcdefg" --> svals[1] = "abcdefg"
			}
		} else if len(svals) == 1 && strings.Index(regex.String(), "(") == -1 && strings.Index(regex.String(), ")") == -1 {
			if hex {
				if len(svals[0]) < 32 {
					limit = len(svals[0])
				}
				sessionVars[svar] = svals[0][0:limit]
			} else {
				sessionVars[svar] = svals[0]
			}

		} else {
			fmt.Fprintf(os.Stderr, "ERROR: SessionVar %s from command \"%s\" was not set \n", svar, command)
			foundSessionVars = false

			if config.MustCaptureElement(svar, command) {
				foundMustCaptures = false
			}
		}
	}
	return foundSessionVars, foundMustCaptures

}

func (runner *Runner) doLog(command string, config *Config, requestMethod string, tcpResponse []byte, httpResponse *http.Response, result *Result, err error, startTime time.Time, shortUrl string, mdi string, clientId int, stepCounter int, lastSession string, sessionVars map[string]string) (string, bool) {

	var (
		sessionKey        string
		session           string
		statusCode        int
		byteSize          int
		server            string
		serverTime        float64
		duration          float64
		inputVals         string
		inputData         string
		foundSessionVars  bool
		foundMustCaptures bool
		continueSession   bool
		tcp               bool
	)

	// ---------------------------------------------------------------------------------------------
	// Init common vars between HTTP and TCP
	duration = (time.Since(startTime)).Seconds()
	inputData = mdi // capture mdi before it is split
	if strings.Index(mdi, ",") > -1 {
		inputSplit := strings.SplitN(mdi, ",", 2)
		mdi = inputSplit[0]
		inputVals = inputSplit[1]
		if len(inputVals) > 0 {
			inputVals = "," + inputVals
			if delimeter[0] != ',' {
				inputVals = strings.Replace(inputVals, ",", delimeter[0:1], -1)
			}
		}
	}
	continueSession = true
	sessionKey = "0"
	tcp = requestMethod == "TCP"

	atomic.AddInt32(&result.Requests, 1) //atomic++

	// ---------------------------------------------------------------------------------------------
	// Process TCP
	if tcp {
		session = lastSession
		statusCode = 200
		byteSize = len(tcpResponse)
		atomic.AddInt32(&result.success, 1)
		// no failure cases yet, use these when we add that logic
		//atomic.AddInt32(&result.networkFailed, 1)
		//atomic.AddInt32(&result.badFailed, 1)

		foundSessionVars, foundMustCaptures := runner.findSessionVars(command, config, fmt.Sprintf("%x", tcpResponse), inputData, startTime, sessionVars, tcp)
		runner.alwaysFoundSessionVars = runner.alwaysFoundSessionVars && foundSessionVars
		continueSession = continueSession && foundMustCaptures
		goto OUTPUTLOG
	}

	// ---------------------------------------------------------------------------------------------
	// Process HTTP
	if requestMethod == "" {
		requestMethod = "REQ"
	}
	statusCode = 499
	server = "-1"     //default unknown
	serverTime = 10.0 //default to a big number so it will be noticed in the output data
	if httpResponse != nil {
		//The reason we check for session here is so that registration does not have to use the sessionMap
		//The registration process can be defined to use the account_key, while regular device interaction might use mdi (or device) key
		if httpResponse.Header.Get("Set-Cookie") != "" {
			for _, cookie := range httpResponse.Cookies() {
				if verbose {
					fmt.Fprintf(os.Stderr, "cookie nameX=%v\n\n", cookie)
				}
				if cookie.Name == runner.SessionCookieName {
					session = cookie.Value
					if verbose {
						fmt.Fprintf(os.Stderr, "session=%v\n", cookie)
					}
				}
			}
		}
		if session == "" {
			session = lastSession
		}

		dump, err := httputil.DumpResponse(httpResponse, true)
		byteSize = len(dump)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR \"%s\" dumping http response to local (cient %d command %s input %s)\n", err.Error(), clientId, command, mdi)
		}

		sessionVarsInput := strings.Replace(string(dump), "\r", "", -1)
		foundSessionVars, foundMustCaptures = runner.findSessionVars(command, config, sessionVarsInput, inputData, startTime, sessionVars, false)
		runner.alwaysFoundSessionVars = runner.alwaysFoundSessionVars && foundSessionVars
		continueSession = continueSession && foundMustCaptures

		if httpResponse.StatusCode >= 200 && httpResponse.StatusCode < 400 { //was300
			atomic.AddInt32(&result.success, 1) //atomic++
		} else {
			atomic.AddInt32(&result.badFailed, 1) //atomic++
		}
		statusCode = httpResponse.StatusCode

		server = httpResponse.Header.Get("X-someserver")

		if server == "" {
			server = "-1" //web01/02/03 would be 1,2,3. -1 means unknown
		}
		serverTimeStr := httpResponse.Header.Get("X-someserver-Load-Time")
		serverTime, err = strconv.ParseFloat(serverTimeStr, 10)
		if err != nil {
			serverTime = 10 //just a big number (in seconds) so we notice if it was missing
		} else {
			serverTime = serverTime / 1000
		}
		if httpResponse.Body != nil {
			httpResponse.Body.Close()
		}
	} else {
		atomic.AddInt32(&result.networkFailed, 1) //atomic++
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s on command \"%s\" response (client %d, input \"%s\")\n", err.Error(), command, clientId, mdi)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: no response on command \"%s\" response (client %d, input \"%s\")\n", command, clientId, mdi)
		}
		sessionVars := make([]string, 0)
		for _, session_var := range config.Command[command].SessionVar {
			s := strings.SplitN(session_var, " ", 2) // s = ['XTOKEN', 'detail="(.+)"']
			sessionVars = append(sessionVars, s[0])
		}
		if len(sessionVars) > 0 {
			fmt.Fprintf(os.Stderr, "ERROR: SessionVars \"%s\" from command \"%s\" were not matched in bad/empty/undelivered response (client %d, input \"%s\")\n",
				strings.Join(sessionVars, ","), command, clientId, mdi)
			foundSessionVars = false
		}
		var mustCapture = config.FieldString("MustCapture", command)
		if len(mustCapture) > 0 {
			fmt.Fprintf(os.Stderr, "ERROR: MustCapture \"%s\" from command \"%s\" was not matched in bad/empty/undelivered response (client %d, input \"%s\")\n",
				mustCapture, command, clientId, mdi)
			continueSession = false
		}
	}

OUTPUTLOG:
	const layout = "2006-01-02 15:04:05.000"

	d := delimeter[0]

	runner.stdoutMutex.Lock()
	fmt.Printf("%v%c%s%c%d%c%s%c%s%c%s%c%s%c%s%c%d%c%v%c%d%c%d%c%v%c%.3f%c%.3f%c%s%s\n", startTime.Format(layout), d, command, d, stepCounter, d, requestMethod, d, sessionKey, d, session, d, mdi, d, shortUrl, d, statusCode, d, foundSessionVars, d, clientId, d, byteSize, d, server, d, duration, d, serverTime, d, Build, inputVals)
	runner.stdoutMutex.Unlock()

	return session, continueSession
}
