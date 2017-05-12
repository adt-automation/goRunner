package main

import (
        "GoRunnerLib"
//	"strings"
        "WebServerLib"
        "net/http"
//	"bufio"
	"fmt"
	"os"
	"bytes"
	"io"
	"syscall"
	"os/signal"
        )

func main () {
        
	done := make(chan bool)
    	sigs := make(chan os.Signal, 1)

    	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL) //, syscall.SIGABRT, os.Interrupt, syscall.SIGQUIT, syscall.SIGHUP)

    	go func() {
        <-sigs
        done <- true
	    }()

	go http.ListenAndServe(":9009",nil)
        http.HandleFunc("/main",WebServerLib.Landing)
        http.HandleFunc("/GetSession",WebServerLib.ReturnSession)
        http.HandleFunc("/api/oauth/token", WebServerLib.HandleToken)
        http.HandleFunc("/api/v1/account/ticket", WebServerLib.HandleTicket)
        http.HandleFunc("/api/v1/account/ticket/", WebServerLib.HandleTicket2)
        
	
//	readOutput :=bufio.NewScanner(os.Stdout)
//	go func() {
//		for readOutput.Scan(){
//		println("HI",readOutput.Text())
//		fmt.Println(readOutput.Text())
//		}	
				
//             }()

	old := os.Stdout // keep backup of the real stdout
	r, w, _ := os.Pipe()
    	// check error
	os.Stdout = w

//    	print()

    	outC := make(chan string)
    	// copy the output in a separate goroutine so printing can't block indefinitely
    	go func() {
        var buf bytes.Buffer
        //io.Copy(&buf,r)
        	//written, err := io.Copy(&buf, r)
		_, err := io.Copy(&buf, r)
		if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
		}
		//fmt.Fprintf(os.Stderr, "\n/!\\ ioCopy copied %d bytes  /!\\\n", written)
		fmt.Print(os.Stderr)
	outC <- buf.String()
    	}()

        GoRunnerLib.Test_Runner(); // call goRunner
	
	w.Close()
    	os.Stdout = old // restoring the real stdout
    	out := <-outC


//	var compare_string string
//	var bool int
//	compare_string:=strings.Contains(out,"Overall Success Rate:         100.00%")
//	println(compare_string)
//	strings.Compare(compare_string,"true")
//	compare_string == "true"
//	fmt.Println("String Search",strings.Contains(out,"Overall Success Rate:		100.00%")) 


    	// reading our temp stdout
//    	fmt.Println("previous output:")
    	fmt.Print(out)
	
	<-done	

}
