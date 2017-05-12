package runner

//author: Doug Watson

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/gcfg.v1"

	rmac "github.com/adt-automation/goRunner/golib/macro"
)

type Result struct {
	Requests        int32
	success         int32
	networkFailed   int32
	badFailed       int32
	readThroughput  int32
	writeThroughput int32
}

var stdoutMutex sync.Mutex
var Verbose = flag.Bool("verbose", false, "verbose debugging output flag")
var Keepalive = flag.Bool("keepalive", true, "enable/disable keepalive")

type CfgStruct struct {
	Search struct {
		CommandGrep       string
		SessionKeyGrep    string
		SessionCookieName string
	}
	Version struct {
		ConfigVersion string
	}
	Command map[string]*struct {
		ReqUrl           string
		ReqContentType   string
		ReqType          string
		ReqBody          string
		ReqUpload        string
		EncryptStartByte string
		EncryptNumBytes  string
		EncryptKey       string
		EncryptIv        string
		DoCall           string
		MsecDelay        string
		MsecRepeat       string
		ReqHeader1       string
		ReqHeader2       string
		ReqHeader3       string
		DoGrep1          string
		DoGrep2          string
		Md5Input         string
		Base64Input      string
		MustCapture      string
		ReqHeaders       []string
		SessionVar       []string
	}
	CommandSequence struct {
		Sequence   string
		SessionLog string
	}
}

var cfg = CfgStruct{}
var GrepCommand *regexp.Regexp
var DoGrep1 *regexp.Regexp
var DoGrep2 *regexp.Regexp
var SessionCookieName string
var Delimeter string
var CommandQueue []string
var PostSessionDelay int
var initialGetField map[string]bool
var alwaysFoundSessionVars bool = true

func HeadInputColumns(csvText string) {
	rmac.HeadInputColumns(csvText)
}

func HasInputColHeaders() bool {
	return rmac.HasInputColHeaders()
}

func getFieldString(config *CfgStruct, field string, command string) string {
	data := ""

	r := reflect.ValueOf(config.Command[command])
	if !reflect.Indirect(r).IsValid() {
		r = reflect.ValueOf(config.Command["default"])
	}
	if reflect.Indirect(r).IsValid() {
		f := reflect.Indirect(r).FieldByName(field)
		if f.String() != "" {
			data = f.String()
		} else {
			r := reflect.ValueOf(config.Command["default"])
			if reflect.Indirect(r).IsValid() {
				f := reflect.Indirect(r).FieldByName(field)
				data = f.String()
			}
		}
	}
	return data
}

func getFieldInteger(config *CfgStruct, field string, command string) int {
	var str = getFieldString(config, field, command)
	data, err := strconv.Atoi(str)
	if err == nil {
		return data
	} else {
		return 0
	}
	return data
}

func responseMustCapture(config *CfgStruct, element string, command string) bool {
	var mustCapture bool = false
	var str = getFieldString(config, "MustCapture", command)
	//fmt.Printf("MustCapture for %s: %q\n", command, strings.Fields(strings.Replace(str, ",", " ", -1)))
	for _, elem := range strings.Fields(strings.Replace(str, ",", " ", -1)) {
		if elem == element {
			mustCapture = true
			break
		}
	}
	return mustCapture
}

func PrintLogHeader(inputLine1 string, isInputHeader bool) {
	d := Delimeter[0]
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
			inputLine1 = strings.Replace(inputLine1, ",", Delimeter[0:1], -1)
		}
	}
	stdoutMutex.Lock()
	fmt.Printf("startTime%ccommand%cnextCommand%cstep%crequestType%csessionKey%csession%cgrep1%cgrep2%cid%cshortUrl%cstatusCode%csessionVarsOk%cclientId%cbyteSize%cserver%cduration%cserverDuration%s\n", d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, d, inputLine1)
	stdoutMutex.Unlock()
	if len(cfg.CommandSequence.SessionLog) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Replace(strings.Replace(strings.Replace(cfg.CommandSequence.SessionLog, "{%", "", -1), "{$", "", -1), "}", "", -1))
	}
}

func NewConfiguration2(configFile string) *CfgStruct {
	bytes, err1 := ioutil.ReadFile(configFile)

	equalLine := regexp.MustCompile("=")
	openQuote := regexp.MustCompile("=[\t ]+")
	closeQuote := regexp.MustCompile("$") // (?m) is a flag that says to treat as multi-line strings (so that $ will match EOL instead of just EOF)
	output := ""
	str := strings.Split(string(bytes), "\n")
	for _, line := range str {
		if equalLine.MatchString(line) {
			line = strings.Replace(line, "\"", "\\\"", -1)
			line = openQuote.ReplaceAllString(line, "= \"")
			line = closeQuote.ReplaceAllString(line, "\"")
		}
		output += line + "\n"
	}
	if err1 != nil {
		log.Fatalf("Failed to open config.ini file: %s", err1)
	}
	err2 := gcfg.ReadStringInto(&cfg, output)
	if err2 != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err2)
	}
	GrepCommand = regexp.MustCompile(cfg.Search.CommandGrep)
	SessionCookieName = cfg.Search.SessionCookieName
	initCommandQueue(&cfg)
	initRunnerMacros(&cfg)
	return &cfg
}

func initCommandQueue(cfg *CfgStruct) {
	if len(cfg.CommandSequence.Sequence) > 0 {
		for _, cmd := range strings.Split(cfg.CommandSequence.Sequence, ",") {
			CommandQueue = append(CommandQueue, strings.TrimSpace(cmd))
		}
	} else {
		CommandQueue = append(CommandQueue, "_start")
		cmd := cfg.Command["_start"].DoCall
		for len(cmd) > 0 {
			if cmd == "none" {
				break
			} else {
				CommandQueue = append(CommandQueue, cmd)
				cmd = cfg.Command[cmd].DoCall
			}
		}
	}
	li := len(CommandQueue) - 1
	if li >= 0 {
		PostSessionDelay = getFieldInteger(cfg, "MsecDelay", CommandQueue[li])
	}
}

func EstimateSessionTime(cfg *CfgStruct) time.Duration {
	ncq := len(CommandQueue)
	dur := time.Duration(ncq*100) * time.Millisecond // estimate 100ms / call
	for i := 0; i < ncq; i++ {
		repeat := getFieldInteger(cfg, "MsecRepeat", CommandQueue[i])
		if repeat > 0 {
			dur += time.Millisecond * time.Duration(repeat)
		} else if i < ncq-1 { // no post-call delay for final command in sesssion sequence
			dur += time.Millisecond * time.Duration(getFieldInteger(cfg, "MsecDelay", CommandQueue[i]))
		}
	}
	return dur
}

func initRunnerMacros(cfg *CfgStruct) {
	rmac.KvDelimeter = Delimeter
	for _, cmd := range CommandQueue {
		rmac.InitMacros(cmd, getFieldString(cfg, "ReqBody", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "ReqUrl", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "DoGrep1", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "DoGrep2", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "ReqHeader1", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "ReqHeader2", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "ReqHeader3", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "EncryptIv", cmd))
		rmac.InitMacros(cmd, getFieldString(cfg, "EncryptKey", cmd))
		for _, session_var := range cfg.Command[cmd].SessionVar {
			s := strings.SplitN(session_var, " ", 2) // s = ['CUSTNO', '<extId>{%VAL}</extId>']
			rmac.InitMacros(cmd, s[1])
		}
		rmac.InitMd5Macro(cmd, getFieldString(cfg, "Md5Input", cmd))
		rmac.InitBase64Macro(cmd, getFieldString(cfg, "Base64Input", cmd))
	}
	rmac.InitSessionLogMacros(cfg.CommandSequence.SessionLog)
	rmac.InitUnixtimeMacros()
}

func httpReq(inputData string, config *CfgStruct, command string, baseUrl string, tr *http.Transport, cookieMap map[string]*http.Cookie, sessionVars map[string]string, grep1 string, grep2 string, reqTime time.Time) (*http.Request, *http.Response, error) {

	var reqErr error

	//this is where all the good stuff happens
	//"DEVICE_INFORMATION", "RING", "SET_ADMIN", "MESSAGE", "INSTALL_MDM", "InstallProfile", "TENANT_INFO", ...
	arr := strings.Split(inputData, Delimeter) // for 2 value inputs to stdin
	var key, val, body, urlx string
	var r *strings.Replacer
	if len(arr) > 1 {
		key = arr[0]
		val = arr[1] //need to check if this exists, it will only be in the input line for APIs that req. 2 inputs
		//add here if you need to add new config substitutions
		r = strings.NewReplacer(
			"{%KEY}", key,
			"{%VAL}", val,
			"{%1}", grep1,
			"{%2}", grep2,
		)
	} else {
		key = inputData //no delimeter in the input, so we take the whole line as the key
		//and here for new config substitutions
		r = strings.NewReplacer(
			"{%KEY}", key,
			"{%1}", grep1,
			"{%2}", grep2,
		)
	}

	body = r.Replace(getFieldString(config, "ReqBody", command))
	urlx = getFieldString(config, "ReqUrl", command)
	if strings.HasPrefix(urlx, "http://") || strings.HasPrefix(urlx, "https://") {
		urlx = r.Replace(urlx)
	} else {
		urlx = r.Replace(baseUrl + urlx)
	}
	header1String := r.Replace(getFieldString(config, "ReqHeader1", command))
	header2String := r.Replace(getFieldString(config, "ReqHeader2", command))
	header3String := r.Replace(getFieldString(config, "ReqHeader3", command))

	requestContentType := getFieldString(config, "ReqContentType", command)
	requestType := getFieldString(config, "ReqType", command)

	body = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, body)
	urlx = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, urlx)

	reqReader := io.Reader(bytes.NewReader([]byte(body)))
	requestContentSize := int64(len(body))

	reqUpload := getFieldString(config, "ReqUpload", command)
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
		if *Verbose {
			fmt.Fprintf(os.Stderr, "\nERROR=%v URL==%v requestType=%v body=%v\n", reqErr, urlx, requestType, body)
		}
		fmt.Fprintf(os.Stderr, "ERROR: command %s input %s TODO- Need a log entry here because we returned without logging due to an error generating the request!\n", command, inputData)
		var empty *http.Response
		return req, empty, reqErr
	}

	// default headers here
	for _, hdr := range cfg.Command["default"].ReqHeaders {
		str := strings.Split(hdr, ":")
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}
	// command-specific headers here
	for _, hdr := range cfg.Command[command].ReqHeaders {
		str := strings.Split(hdr, ":")
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}

	// any duplicate headers here will replace defaults
	if len(requestContentType) > 0 {
		req.Header.Set("Content-Type", requestContentType)
	}
	if len(header1String) > 0 {
		header1String = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, header1String)
		str := strings.Split(header1String, ":") /* authorization: Basic asdfasdf */
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}
	if len(header2String) > 0 {
		header2String = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, header2String)
		str := strings.Split(header2String, ":") /* fubar-request-extra: asdfasdf */
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
	}
	if len(header3String) > 0 {
		header3String = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, header3String)
		str := strings.Split(header3String, ":") /* fubar-request-extra: asdfasdf */
		req.Header.Set(str[0], strings.TrimSpace(str[1]))
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
	if *Verbose {
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
	} else if *Verbose {
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
	if *Keepalive {
		// tr.RoundTrip avoids automatic redirects
		return tr.RoundTrip(req)
	} else {
		// this "set object to dereferenced object" will execute a deep copy
		var tr1 http.Transport = *tr
		return tr1.RoundTrip(req)
	}
}

func tsByteBuffer(timestamp int64) *bytes.Buffer {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, timestamp)
	return buf
}

func buildIv(reqTime time.Time) []byte {
	timestamp := reqTime.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	buf := tsByteBuffer(timestamp)
	iv := make([]byte, 0, 16)
	iv = append(iv, buf.Bytes()[2:]...)
	iv = append(iv, buf.Bytes()[2:]...)
	iv = append(iv, buf.Bytes()[2:6]...)
	return iv
}

func buildKey(keyStr string) []byte {
	key := make([]byte, 0, 32)

	if strings.Count(keyStr, ",") != 31 {
		log.Fatal(fmt.Sprintf("32-byte key required, current key will be %d bytes", 1+strings.Count(keyStr, ",")))
	}

	for _, ds := range strings.Split(keyStr, ",") {
		ds = strings.TrimSpace(ds)
		di, err := strconv.Atoi(ds)
		if err != nil {
			log.Fatal(err.Error() + " during encryption key construction")
		} else {
			key = append(key, byte(di))
		}
	}
	return key
}

// e.g. servAddr := "gsess-dr.adtpulse.com:11083"
func tcpReq(inputData string, config *CfgStruct, command string, servAddr string, sessionVars map[string]string) []byte {

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

	input := getFieldString(config, "ReqBody", command)
	input = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, input)

	send, err := hex.DecodeString(strings.Replace(input, " ", "", -1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hex decode failed: %s\n", err.Error())
		os.Exit(1)
	}

	encryptStart := getFieldInteger(config, "EncryptStartByte", command) - 1
	encryptCt := getFieldInteger(config, "EncryptNumBytes", command)
	if encryptCt > 0 && encryptStart > -1 {
		if encryptStart+encryptCt > len(send) {
			fmt.Fprintf(os.Stderr, "command %s: encrypt range past end of input text\n", command)
			os.Exit(1)
		}
		ebytes := send[encryptStart : encryptStart+encryptCt]

		ivStr := getFieldString(config, "EncryptIv", command)
		ivStr = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, ivStr)
		iv, err := hex.DecodeString(strings.Replace(ivStr, " ", "", -1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "command %s hex decode failed: %s\n", command, err.Error())
			os.Exit(1)
		}
		iv = buildIv(reqTime)
		keyStr := getFieldString(config, "EncryptKey", command)
		keyStr = rmac.RunnerMacros(command, inputData, sessionVars, reqTime, keyStr)

		if len(keyStr) == 0 {
			log.Println("encryption key has empty value")

		}
		key := buildKey(keyStr)
		encrypted, err := encrypt(key, iv, ebytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "command %s encrypt error: %v\n", command, err.Error())
			os.Exit(1)
		}
		if *Verbose {
			fmt.Fprintf(os.Stderr, "%s PRE-ENCRYPT: % x\n", command, send)
		}
		send = bytes.Replace(send, ebytes, encrypted, 1)
	}
	if *Verbose {
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
	if *Verbose {
		fmt.Fprintf(os.Stderr, "%s TCP REPLY: % x\n", command, reply[0:responseLen])
	}
	return reply[0:responseLen]
}

func DoReq(stepCounter int, mdi string, config *CfgStruct, result *Result, clientId int, baseUrl string, baseUrlFilter *regexp.Regexp, delay int, tr *http.Transport, cookieMap map[string]*http.Cookie, sessionVars map[string]string, grep1 string, grep2 string, stopTime time.Time, commandTime float64) (lastSession string) {

	if !stopTime.IsZero() && time.Now().Add(time.Duration(delay)*time.Millisecond).After(stopTime) {
		lastSession = ""
		return
	}
	time.Sleep(time.Duration(delay) * time.Millisecond) // default value is 0 milliseconds

	command := CommandQueue[stepCounter]
	stepCounter += 1
	session := ""
	continueSession := true

	_, ok := config.Command[command]
	if !ok {
		fmt.Fprintf(os.Stderr, "ERROR: command %q is not defined in the .ini file\n", command)
	} else {
		startTime := time.Now()
		requestType := getFieldString(config, "ReqType", command)
		grep1String := rmac.RunnerMacrosRegexp(command, mdi, sessionVars, startTime, getFieldString(config, "DoGrep1", command))
		grep2String := rmac.RunnerMacrosRegexp(command, mdi, sessionVars, startTime, getFieldString(config, "DoGrep2", command))

		if requestType == "TCP" {
			tcpReply := tcpReq(mdi, config, command, baseUrl, sessionVars)
			shortUrl := (baseUrlFilter).ReplaceAllString(baseUrl, "")
			grep1, grep2, continueSession = doLogTcp(command, config, tcpReply, result, startTime, shortUrl, mdi, clientId, stepCounter, "", grep1String, grep2String)
		} else {
			req, resp, err := httpReq(mdi, config, command, baseUrl, tr, cookieMap, sessionVars, grep1, grep2, startTime)
			shortUrl := (baseUrlFilter).ReplaceAllString(req.URL.String(), "")
			session, _, grep1, grep2, continueSession = doLog(command, config, req.Method, resp, result, err, startTime, shortUrl, mdi, clientId, stepCounter, "", grep1String, grep2String, sessionVars)
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}

		delay = getFieldInteger(config, "MsecDelay", command)

		repeatTime := float64(getFieldInteger(config, "MsecRepeat", command))
		requestTime := float64(delay) + (time.Since(startTime)).Seconds()*1000.0
		// add delay again here, to find out whether the delay will put us past the repeat time
		if commandTime+requestTime+float64(delay) < repeatTime {
			stepCounter -= 1
			commandTime += requestTime
		} else {
			commandTime = 0.0
		}
	}

	if *Verbose && stepCounter < len(CommandQueue) {
		fmt.Fprintf(os.Stderr, "mdi %s stepCounter %d nextCommand=%v\n", mdi, stepCounter, CommandQueue[stepCounter])
	}

	if continueSession && stepCounter < len(CommandQueue) && CommandQueue[stepCounter] != "none" {
		session = DoReq(stepCounter, mdi, config, result, clientId, baseUrl, baseUrlFilter, delay, tr, cookieMap, sessionVars, grep1, grep2, stopTime, commandTime)
	} else if len(cfg.CommandSequence.SessionLog) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", rmac.SessionLogMacros(mdi, sessionVars, time.Now(), cfg.CommandSequence.SessionLog))
	}
	lastSession = session
	return
}

func GetResults(results map[int]*Result, overallStartTime time.Time) (map[string]int32, int32) {
//func GetResults(results map[int]*Result, overallStartTime time.Time) (map[string]int32) {
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
	status := summary["requests"]-summary["success"]
	return summary, status
	//return summary
}
func ExitWithStatus(results map[int]*Result, overallStartTime time.Time) {
	//myMap := GetResults(results, overallStartTime)
	myMap, _ := GetResults(results, overallStartTime)
	PrintResults(myMap, overallStartTime)
	//PrintResults(myMap, overallStartTime)
	os.Exit(exitStatus(myMap))
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
	fmt.Fprintf(os.Stderr, "Overall Success Rate:		%.2f%%\n", (float32(myMap["success"])/(float32(myMap["requests"])))*100)
}

func exitStatus(myMap map[string]int32) int {
	if myMap["success"] != myMap["requests"] {
		return 32
	} else if !alwaysFoundSessionVars {
		return 33
	} else {
		return 0
	}
}

func doLogTcp(command string, config *CfgStruct, dump []byte, result *Result, startTime time.Time, shortUrl string, mdi string, clientId int, stepCounter int, lastSession string, grep1String string, grep2String string) (string, string, bool) {
	var nextCommand string = ""
	var requestType string = "TCP"
	var sessionKey string = "0"
	var session string = lastSession
	var statusCode int = 200
	var byteSize int = len(dump)
	var server string = ""
	var serverTime float64 = 0.0
	var grep1 string = ""
	var grep2 string = ""
	var duration float64 = (time.Since(startTime)).Seconds()
	var hexDump string = fmt.Sprintf("%x", dump)
	var inputVals = ""
	var continueSession bool = true

	if strings.Index(mdi, ",") > -1 {
		inputSplit := strings.SplitN(mdi, ",", 2)
		mdi = inputSplit[0]
		inputVals = inputSplit[1]
		if len(inputVals) > 0 {
			inputVals = "," + inputVals
			if Delimeter[0] != ',' {
				inputVals = strings.Replace(inputVals, ",", Delimeter[0:1], -1)
			}
		}
	}

	atomic.AddInt32(&result.Requests, 1)
	atomic.AddInt32(&result.success, 1)
	// no failure cases yet, use these when we add that logic
	//atomic.AddInt32(&result.networkFailed, 1)
	//atomic.AddInt32(&result.badFailed, 1)

	grep1regex := regexp.MustCompile(grep1String)
	if len(grep1regex.String()) > 0 {
		if len(hexDump) > 2 {
			cmdArr := grep1regex.FindStringSubmatch(hexDump)
			if *Verbose {
				fmt.Fprintf(os.Stderr, "Grep1cmdArr=%v resp dump=[%v]\n\n", cmdArr, hexDump)
			}
			if len(cmdArr) > 0 {
				limit := 32
				if len(cmdArr) > 1 { //must match in the parenthesis () of the regex
					if len(cmdArr[1]) < 32 {
						limit = len(cmdArr[1])
					}
					grep1 = cmdArr[1][0:limit]
				} else if len(cmdArr) == 1 && strings.Index(grep1regex.String(), "(") == -1 && strings.Index(grep1regex.String(), ")") == -1 {
					if len(cmdArr[0]) < 32 {
						limit = len(cmdArr[0])
					}
					grep1 = cmdArr[0][0:limit]
				}
			}
		}
		if continueSession && responseMustCapture(config, "DoGrep1", command) {
			continueSession = len(grep1) > 0
		}
	}

	grep2regex := regexp.MustCompile(grep2String)
	if len(grep2regex.String()) > 0 {
		if len(hexDump) > 2 {
			cmdArr := grep2regex.FindStringSubmatch(hexDump)
			if *Verbose {
				fmt.Fprintf(os.Stderr, "Grep2cmdArr=%v resp dump=[%v]\n\n", cmdArr, hexDump)
			}
			if len(cmdArr) > 0 {
				limit := 32
				if len(cmdArr) > 1 { //must match in the parenthesis () of the regex
					if len(cmdArr[1]) < 32 {
						limit = len(cmdArr[1])
					}
					grep2 = cmdArr[1][0:limit]
				} else if len(cmdArr) == 1 && strings.Index(grep2regex.String(), "(") == -1 && strings.Index(grep2regex.String(), ")") == -1 {
					if len(cmdArr[0]) < 32 {
						limit = len(cmdArr[0])
					}
					grep2 = cmdArr[0][0:limit]
				}
			}
		}
		if continueSession && responseMustCapture(config, "DoGrep2", command) {
			continueSession = len(grep2) > 0
		}
	}

	d := Delimeter[0]
	const layout = "2006-01-02 15:04:05.000"
	stdoutMutex.Lock()
	fmt.Printf("%v%c%s%c%s%c%d%c%s%c%s%c%s%c%s%c%s%c%s%c%s%c%d%c%v%c%d%c%d%c%v%c%.3f%c%.3f%s\n", startTime.Format(layout), d, command, d, nextCommand, d, stepCounter, d, requestType, d, sessionKey, d, session, d, string(grep1), d, string(grep2), d, mdi, d, shortUrl, d, statusCode, d, true, d, clientId, d, byteSize, d, server, d, duration, d, serverTime, inputVals)
	stdoutMutex.Unlock()
	return grep1, grep2, continueSession
}

func doLog(command string, config *CfgStruct, requestType string, resp *http.Response, result *Result, err error, startTime time.Time, shortUrl string, mdi string, clientId int, stepCounter int, lastSession string, grep1String string, grep2String string, sessionVars map[string]string) (session string, nextCommand string, grep1 string, grep2 string, continueSession bool) {

	atomic.AddInt32(&result.Requests, 1) //atomic++
	byteSize := 0
	//	body := ""
	statusCode := 499
	nextCommand = "none"
	sessionKey := ""
	server := "-1"     //default unknown
	serverTime := 10.0 //default to a big number so it will be noticed in the output data
	foundSessionVars := true
	inputVals := ""
	continueSession = true
	inputData := mdi // capture mdi before it is split

	if strings.Index(mdi, ",") > -1 {
		inputSplit := strings.SplitN(mdi, ",", 2)
		mdi = inputSplit[0]
		inputVals = inputSplit[1]
		if len(inputVals) > 0 {
			inputVals = "," + inputVals
			if Delimeter[0] != ',' {
				inputVals = strings.Replace(inputVals, ",", Delimeter[0:1], -1)
			}
		}
	}

	if resp != nil {
		//bodyBytes, _ := ioutil.ReadAll(resp.Body)
		//body = string(bodyBytes[:])
		//byteSize = len(body)

		//The reason we check for session here is so that registration does not have to use the sessionMap
		//The registration process can be defined to use the account_key, while regular device interaction might use mdi (or device) key
		if resp.Header.Get("Set-Cookie") != "" {
			for _, cookie := range resp.Cookies() {
				if *Verbose {
					fmt.Fprintf(os.Stderr, "cookie nameX=%v\n\n", cookie)
				}
				if cookie.Name == SessionCookieName {
					session = cookie.Value
					if *Verbose {
						fmt.Fprintf(os.Stderr, "session=%v\n", cookie)
					}
				}
			}
		}
		if session == "" {
			session = lastSession
		}

		dump, err := httputil.DumpResponse(resp, true)
		byteSize = len(dump)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR \"%s\" dumping http response to local (cient %d command %s input %s)\n", err.Error(), clientId, command, mdi)
		}

		grep1regex := regexp.MustCompile(grep1String)
		if len(grep1regex.String()) > 0 {
			if len(dump) > 2 {
				cmdArr := grep1regex.FindStringSubmatch(strings.Replace(string(dump), "\r", "", -1))
				if *Verbose {
					fmt.Fprintf(os.Stderr, "Grep1cmdArr=%v resp dump=[%v]\n\n", cmdArr, string(dump))
				}
				if len(cmdArr) > 0 {
					if len(cmdArr) > 1 {
						grep1 = cmdArr[1] //must match in the parenthesis () of the regex
					} else if len(cmdArr) == 1 && strings.Index(grep1regex.String(), "(") == -1 && strings.Index(grep1regex.String(), ")") == -1 {
						grep1 = cmdArr[0]
					}
				}
			}
			if continueSession && responseMustCapture(config, "DoGrep1", command) {
				continueSession = len(grep1) > 0
			}
		}

		grep2regex := regexp.MustCompile(grep2String)
		if len(grep2regex.String()) > 0 {
			if len(dump) > 2 {
				cmdArr := grep2regex.FindStringSubmatch(strings.Replace(string(dump), "\r", "", -1))
				if *Verbose {
					fmt.Fprintf(os.Stderr, "Grep2cmdArr=%v resp dump=[%v]\n\n", cmdArr, string(dump))
				}
				if len(cmdArr) > 0 {
					if len(cmdArr) > 1 {
						grep2 = cmdArr[1] //must match in the parenthesis () of the regex
					} else if len(cmdArr) == 1 && strings.Index(grep2regex.String(), "(") == -1 && strings.Index(grep2regex.String(), ")") == -1 {
						grep2 = cmdArr[0]
					}
				}
			}
			if continueSession && responseMustCapture(config, "DoGrep2", command) {
				continueSession = len(grep2) > 0
			}
		}

		// set any session vars listed for current command, e.g. SessionVar = XTOKEN detail="(.+)"
		for _, session_var := range cfg.Command[command].SessionVar {
			s := strings.SplitN(session_var, " ", 2) // s = ['XTOKEN', 'detail="(.+)"']
			svar := s[0]
			sgrep := rmac.RunnerMacrosRegexp(command, inputData, sessionVars, startTime, s[1])
			regex := regexp.MustCompile(sgrep) // /detail="(.+)"/
			if len(regex.String()) > 0 {
				if len(dump) > 2 {
					svals := regex.FindStringSubmatch(strings.Replace(string(dump), "\r", "", -1))
					if len(svals) > 1 {
						sessionVars[svar] = svals[1] // detail="abcdefg" --> svals[1] = "abcdefg"
					} else if len(svals) == 1 && strings.Index(regex.String(), "(") == -1 && strings.Index(regex.String(), ")") == -1 {
						sessionVars[svar] = svals[0]
					} else {
						fmt.Fprintf(os.Stderr, "ERROR: SessionVar %s from command \"%s\" was not set (client %d, input \"%s\")\n", svar, command, clientId, mdi)
						foundSessionVars = false
						if continueSession && responseMustCapture(config, svar, command) {
							continueSession = false
						}
					}
				} else if continueSession && responseMustCapture(config, svar, command) {
					// automatically false due to no chance to capture the session var
					continueSession = false
					foundSessionVars = false
				}
			}
		}
		alwaysFoundSessionVars = alwaysFoundSessionVars && foundSessionVars
		if *Verbose {
			println("grep1=", grep1, " grep2=", grep2)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 400 { //was300
			atomic.AddInt32(&result.success, 1) //atomic++
			/*
				if sessionMap[mdi] == "" && session != "" {
					if GrepSessionKey.String() != "" {
						if len(body) > 2 {
							//						fmt.Printf("bodydddddddddddddddddddddddddddddddddddddddd=%s \n\n", string(body))
							cmdArr2 := GrepSessionKey.FindStringSubmatch(string(body))
							if len(cmdArr2) > 1 {
								sessionKey = cmdArr2[1]
								mutex.Lock()
								sessionMap[sessionKey] = session //save the session, this is the only time I lock the map
								mutex.Unlock()
							}
						}
					} else {
						mutex.Lock()
						sessionMap[mdi] = session //save the session, this is the only time I lock the map
						mutex.Unlock()
					}
				}
			*/
			nextCommand = "301"
			/*
				if byteSize > 2 && GrepCommand.String() != "" {

					cmdArr := GrepCommand.FindStringSubmatch(string(body))
					fmt.Printf("cmdArr=%v body=[%v]\n\n", cmdArr, body)
					//since the regex has three possible (match) places, we check all three and take 1
					if len(cmdArr) > 0 {
						if len(cmdArr[0]) >= 1 {
							nextCommand = cmdArr[0]
						}
						if len(cmdArr) > 1 && len(cmdArr[1]) >= 1 {
							nextCommand = cmdArr[1]
						}
						if len(cmdArr) > 2 && len(cmdArr[2]) >= 1 {
							nextCommand = cmdArr[2]
						}
					}
				}
			*/
		} else {
			atomic.AddInt32(&result.badFailed, 1) //atomic++
		}
		statusCode = resp.StatusCode

		server = resp.Header.Get("X-someserver")

		if server == "" {
			server = "-1" //web01/02/03 would be 1,2,3. -1 means unknown
		}
		serverTimeStr := resp.Header.Get("X-someserver-Load-Time")
		serverTime, err = strconv.ParseFloat(serverTimeStr, 10)
		if err != nil {
			serverTime = 10 //just a big number (in seconds) so we notice if it was missing
		} else {
			serverTime = serverTime / 1000
		}
	} else {
		atomic.AddInt32(&result.networkFailed, 1) //atomic++
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s on command \"%s\" response (client %d, input \"%s\")\n", err.Error(), command, clientId, mdi)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: no response on command \"%s\" response (client %d, input \"%s\")\n", command, clientId, mdi)
		}
		sessionVars := make([]string, 0)
		for _, session_var := range cfg.Command[command].SessionVar {
			s := strings.SplitN(session_var, " ", 2) // s = ['XTOKEN', 'detail="(.+)"']
			sessionVars = append(sessionVars, s[0])
		}
		if len(sessionVars) > 0 {
			fmt.Fprintf(os.Stderr, "ERROR: SessionVars \"%s\" from command \"%s\" were not matched in bad/empty/undelivered response (client %d, input \"%s\")\n",
				strings.Join(sessionVars, ","), command, clientId, mdi)
			foundSessionVars = false
		}
		var mustCapture = getFieldString(config, "MustCapture", command)
		if len(mustCapture) > 0 {
			fmt.Fprintf(os.Stderr, "ERROR: MustCapture \"%s\" from command \"%s\" was not matched in bad/empty/undelivered response (client %d, input \"%s\")\n",
				mustCapture, command, clientId, mdi)
			continueSession = false
		}
	}
	//startTimeStamp := float64(startTime.UnixNano()) / 1e9
	duration := (time.Since(startTime)).Seconds()
	if requestType == "" {
		requestType = "REQ"
	}

	const layout = "2006-01-02 15:04:05.000"
	if sessionKey == "" {
		sessionKey = "0"
	}
	d := Delimeter[0]
	stdoutMutex.Lock()
	fmt.Printf("%v%c%s%c%s%c%d%c%s%c%s%c%s%c%s%c%s%c%s%c%s%c%d%c%v%c%d%c%d%c%v%c%.3f%c%.3f%s\n", startTime.Format(layout), d, command, d, nextCommand, d, stepCounter, d, requestType, d, sessionKey, d, session, d, string(grep1), d, string(grep2), d, mdi, d, shortUrl, d, statusCode, d, foundSessionVars, d, clientId, d, byteSize, d, server, d, duration, d, serverTime, inputVals)
	stdoutMutex.Unlock()
	//	if debug {
	//		fmt.Fprintf(os.Stderr, "BODY [%.3f\t[%v]\t%d]\n\n", startTimeStamp, body, len(body))
	//	}
	return
}

func encrypt(key, iv, text []byte) (ciphertextOut []byte, err error) {
	if len(text) < aes.BlockSize {
		err = errors.New(fmt.Sprintf("input text is %d bytes, too short to encrypt", len(text)))
		return
	}
	if len(text)%aes.BlockSize != 0 {
		err = errors.New(fmt.Sprintf("input text is %d bytes, must be a multiple of %d", len(text), aes.BlockSize))
		return
	}
	ciphertextOut = make([]byte, len(string(text)))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cfb := cipher.NewCBCEncrypter(block, iv)
	cfb.CryptBlocks(ciphertextOut, text)

	return
}

func decrypt(key, iv, ciphertext []byte) (plaintextOut []byte, err error) {
	plaintextOut = make([]byte, len(ciphertext))
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	if len(ciphertext) < aes.BlockSize {
		err = errors.New("ciphertext too short")
		return
	}
	cfb := cipher.NewCBCDecrypter(block, iv)
	cfb.CryptBlocks(plaintextOut, ciphertext)
	return
}

//func noRedirect(req *http.Request, via []*http.Request) error {
//	return errors.New("Don't redirect!")
//}
