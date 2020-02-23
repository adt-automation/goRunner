The goRunner tool can read in csv data to drive a sequence of API calls. These calls are defined in a custom API config file. Then as each API call completes, a row of timestamped csv data is written (similar to the apache log format).

goRunner was developed in the UNIX tradition to operate as a filter. It can be used in any pipeline connected to text stream inputs and outputs.
```
Write programs to handle text streams, because that is a universal interface.
```

The API load testing tool is written in the GO programming language to take full advantage of today's modern computing architectures. It can efficiently use all CPU cores in the computer to simulate 100's of thousands of concurrent client connections to the server being tested. The test cases that drive the tool behavior are written using the well known ini file format and these test definition files can be created in any text editor. The test cases that drive the tool are represented mathematically as a finite state machine.

 
The API definition files that define each test, include the following real-time macro scriptable features:

``` 
API sequence definition (ex: first login, then perform an action, then view history,… )
HTTP, HTTPS and raw TCP protocol interactions
Custom AES block encryption support at the message byte level
Full regular expression parsing to save reply tokens from the responses and re-use on future requests
Prior API call memory tokens
Session level memory tokens
Configurable Delays
MD5 custom checksum calculations
Timestamp generation
Key/Value input pairs can be read from an external list and used as macros
Number base conversions from hex to decimal
```
 
All load testing rules and the actual runtime behavior of the tool comes from this ini configuration file and it can all be changed on the fly without program recompilation. This flexibility lends itself to more than just load testing such as the nightly database sync API jobs, unit test script runs.
 
Finally, the results from each run will generate a CSV file that is formatted to display correctly in Tableau. After each run, just click a button in Tableau or another analytics tool to upload. You can then drag the dimensions and measures over to quickly see a time series analysis of the just completed test run, along with detailed drill-down capability for full analysis. The result is a clear picture of the performance capabilities for the system that was just tested, and when used in advance a rollout, it can find problems well before they impact the customer.

How to run the test (To make your own custom tests, just create your own ini config file):

![Example3](https://github.com/adt-automation/goRunner/blob/master/img/goRunnerCommandLine.gif?raw=true)


```

```

How to analyse the results in a tool like Tableau (This becomes critical with million row automation or test runs):

![Example3](https://github.com/adt-automation/goRunner/blob/master/img/goRunnerTestAnalysis.gif?raw=true)



```

```
Example Tableau charts of large load test with the API calls/minute broken out by HTTP return code: 

![Example3](https://github.com/adt-automation/goRunner/blob/master/img/imageLoadTestMix.png?raw=true)




Example TPS chart for the same load test:

![Example4](https://github.com/adt-automation/goRunner/blob/master/img/imageLoadtestTPS.png?raw=true)
```

```






How it works:
 
The following 2 step test example will login a list of users to an API server and then run a command (named refresh). It will use 10 concurrent threads and quit after 300 seconds.
The program is normally run from the operating system (linux, osx) command line like this:

goRunner -f userList.csv -configFile loginTest.ini -baseUrl https://aserver.com -c 10

userList - The input file of test users to cycle through (can also read from STDIN)

configFile - This defines the sequence of API calls to run for each user.

baseUrl - The API or webserver to hit 

c - # of concurrent threads to launch

```
Usage of ./goRunner:
  -baseUrl string
    	The host to test. Example https://test2.someserver.org
  -c int
    	Number of concurrent clients to launch. (default 100)
  -configFile string
    	Config file location (default "config.ini")
  -cpuprofile string
    	write cpu profile to file
  -d string
    	Output file delimeter (default ",")
  -debug
    	Show the body returned for the api call
  -dump
    	Show the HTTP dump for the api call
  -f string
    	Read input from file rather than stdin
  -fh string
    	Read input from csv file which has a header row
  -hx
    	Print output header row and exit
  -keepalive
    	enable/disable keepalive (default true)
  -nh
    	Suppress output header row. Ignored if hx is set
  -nr
    	Do not ramp up to total concurrent number of clients
  -readtimeout int
    	Timeout in seconds for the target API to send the first response byte. Default 30 seconds (default 30)
  -t int
    	Time in seconds for a timed load test
  -targetTPS float
    	The default max TPS is set to 1 million. Good luck reaching this :p (default 1e+06)
  -verbose
    	verbose debugging output flag
```     

goRunner can be run from a cron job scheduler, receive input from a file,  standard input, a pipe or even from a network queue (like HTTP).
 
 
It is a generic, multi-purpose load generation program. It's most common use is as a load testing tool but can also be used for nightly cleanup jobs and various scheduled bulk API processing. 
 
Combined with a custom config file (known as an ini), the goRunner program is a generic, high performance load generation tool.


Example:

userList.csv  - A list of items to pass into the sequence as defined in the config file.
 
john,pass123
claire,suns41n3
...
 
 
The configuration file containing our logic to run the test.

```
loginTest.ini
[commandSequence]
Sequence = login, refresh
 
[command "login"]
ReqUrl = /g/rest/test/login
ReqHeader1 = login: login{%KEY}
ReqHeader2 = password: pass{%VAL}
ReqHeader3 = X-expires: 18000000
SessionVar = BTOKEN Token: (.+)
MustCapture = BTOKEN
 
[command "refresh"]
ReqUrl = /g/rest/test/Refresh
ReqHeader1 = token: {%BTOKEN}
ReqHeader2 = login: account{%KEY}
SessionVar = ATOKEN Token: (.+)
MustCapture = ATOKEN
```
 
The simple example shown above, does a login and then makes a refresh call to an API server for each user passed into the tool. The actual capabilities of the goRunner program supports a much wider range of API interactions (from TCP to UDP protocol).
 
To setup goRunner as a web service to read HTTP post command from a port:
```
./goRunner -configFile someTest.ini -c 1 -baseUrl https://localhost -p 8081
```
Then from another window:
```
curl -v http://localhost:8081  --data 1234
```

Or for multiple intput rows:
```
%cat file.txt
doug,pass1
doug,pass2
john,pass3

%curl -v http://localhost:8081  --data-binary @file.txt

* Rebuilt URL to: http://localhost:8081/
*   Trying ::1...
* Connected to localhost (::1) port 8081 (#0)
> POST / HTTP/1.1
> Host: localhost:8081
> User-Agent: curl/7.49.1
> Accept: */*
> Content-Length: 33
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 33 out of 33 bytes
< HTTP/1.1 200 OK
< Date: Fri, 02 Jun 2017 15:31:44 GMT
< Content-Length: 0
< Content-Type: text/plain; charset=utf-8
<
* Connection #0 to host localhost left intact
```
