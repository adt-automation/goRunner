package GoRunnerLib

import (
    "crypto/tls"
    "net/http"
    "regexp"
    "time"
    "os"
    runner "github.com/adt-automation/goRunner/golib"
)

func Test_Runner() {

    runner.Delimeter = ","
    baseUrl := "http://localhost:9009"
    baseUrlFilter := regexp.MustCompile(baseUrl)
    configuration := runner.NewConfiguration2("test_public.ini")
    results := make(map[int]*runner.Result)
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
    overallStartTime:=time.Now()
    runner.DoReq(0, id, configuration, result, clientId, baseUrl, baseUrlFilter, msDelay, tr, cookieMap, sessionVars, grep1, grep2, stopTime, 0.0) //val,resp, err
    message,exitCode:=runner.GetResults(results,time.Now())

    runner.PrintResults(message,overallStartTime)

    os.Exit(int(exitCode))
}
