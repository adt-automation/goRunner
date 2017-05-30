jmx2ini will be an executable that transforms JMeter JMX config files into goRunner INI files.


Build:
```
go get launchpad.net/xmlpath
go build jmx2ini.go
```

Execute:
```
jmx2ini memory.jmx >memory.ini
```

goRunner is built to execute one test sequence at high scale, but JMX files can contain multiple test sequences. jmx2ini will print INI's for each Logic Controller in the JMX. This initial version only recognizes the LoopController, but the structure to support other Logic Controllers is in place.


goRunner also splits test configuration between INI files and command line options, whereas JMeter stores everything to JMX. jmx2ini will print a goRunner command as a comment at the top of each INI file, and that command will include arguments based on entries in the JMX document.


The memory.jmx example is also included in JMeter downloads as an initial JMeter example. It is meant to run against a local tomcat installation, but requires a memory.jsp file that unfortunately is no longer included in tomcat downloads.


Copy the memory.jsp file from this directory to <tomcat root>/webapps/examples/jsp/memory.jsp, and the test can be run from either JMeter or goRunner.
