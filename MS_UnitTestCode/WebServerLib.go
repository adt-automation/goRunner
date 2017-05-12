package WebServerLib

import (
	"fmt"
	"strings"
	"io"
	"net/http"
	)

func ReturnSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("SESSIONVAR","C1NC0d3M6Y0")
	w.WriteHeader(200)
	w.Write([]byte("TokenValue:=abcdefgh.localserver.com"))
        io.WriteString(w, "Return SessionVariable, TokenValue=TOKEN, UserID in header")
        }

func Landing(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "This is the Landing Page Body")
	}

var port string

func HandleToken(w http.ResponseWriter, r *http.Request) {
        r.ParseForm()
        grant_type := r.FormValue("grant_type")

        response := ""
        switch grant_type {
        case "password":
        response = "{\"access_token\":\"babd1aed-2e5b-40bb-a53f-d31d69d8571a\",\"refresh_token\":\"a77a8bfe-e9e6-4009-90f8-3e817685fed9\",\"token_type\":\"bearer\",\"expires_in\":3599,\"scope\":\"DEFAULT_SCOPE\"}"
        case "refresh_token":
         response = "{\"access_token\":\"babd1aed-2e5b-40bb-a53f-d31d69d8571a\",\"refresh_token\":\"a77a8bfe-e9e6-4009-90f8-3e817685fed9\",\"token_type\":\"bearer\",\"expires_in\":3599,\"scope\":\"DEFAULT_SCOPE\"}"
        }
        fmt.Fprintf(w, "%v\n", response)
}
func HandleTicket(w http.ResponseWriter, r *http.Request) {
        asset_id := r.URL.Query().Get("asset_id")
        response := ""
        if asset_id == "" {
                response = "{\"asset_id\":\"sapient_owner@adt.com\"}"
        } else {
                response = "{\"ticket\":\"451dbd05-67bd-4397-9557-ccfef789d8f0\"}"
        }
        fmt.Fprintf(w, "%v\n", response)
}

func HandleTicket2(w http.ResponseWriter, r *http.Request) {
        pathArr := strings.SplitN(r.URL.RequestURI()[1:] /*trim the first slash*/, "/", -1)
        fmt.Fprintf(w, "PATH=%v\n", pathArr[4]) //pull out the ticket /api/v1/account/ticket/
}

