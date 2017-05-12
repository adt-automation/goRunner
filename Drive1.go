package main

import (
        "GoRunnerLib"
        "net/http"
	"fmt"
	"os"
	"io"
	"syscall"
	"os/signal"
	"strings"
        )
var port string

func main () {
        
	done := make(chan bool)
    	sigs := make(chan os.Signal, 1)

    	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

    	go func() {
        <-sigs
        done <- true
	    }()

	go http.ListenAndServe(":9009",nil)
        http.HandleFunc("/main",Landing)
        http.HandleFunc("/GetSession",ReturnSession)
        http.HandleFunc("/reqapi/check/token", HandleToken)
        http.HandleFunc("/reqapi/test/account/pass", HandleTicket)
        http.HandleFunc("/reqapi/test/account/pass/", HandleTicket2)


        GoRunnerLib.Test_Runner(); // call goRunner
	
	<-done

}



func ReturnSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("SESSIONVAR","C1NC0d3M6Y0")
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