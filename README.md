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


How to analyse the results in a tool like Tableau (This becomes critical with million row automation or test runs):

![Example3](https://github.com/adt-automation/goRunner/blob/master/img/goRunnerTestAnalysis.gif?raw=true)


Example Tableau charts of large load test with the API calls/minute broken out by HTTP return code: 
![Example3](https://github.com/adt-automation/goRunner/blob/master/img/imageLoadTestMix.png?raw=true)



Example TPS chart for the same load test:

![Example4](https://github.com/adt-automation/goRunner/blob/master/img/imageLoadtestTPS.png?raw=true)
