# Go Responsiveness

In late 2021 and early 2022, researchers at Apple took to heart the internet-wide call for giving users more actionable information about the state of their network connections and proposed a new metric, RPM:

> This document specifies the "RPM Test" for measuring responsiveness. It uses common protocols and mechanisms to measure user experience especially when the network is under working conditions. The measurement is expressed as "Round-trips Per Minute" (RPM) and should be included with throughput (up and down) and idle latency as critical indicators of network quality.

Apple wrote and released an implementation of the test in its iOS and macOS operating systems in [versions 15 and Monterey](https://support.apple.com/en-gb/HT212313), respectively.

The researchers at Apple, in collaboration with others throughout the internet-measurement community, proposed RPM as an [IETF RFC](https://github.com/network-quality/draft-ietf-ippm-responsiveness/blob/master/draft-ietf-ippm-responsiveness.txt).

## Independent Implementation

To become a Draft Standard, "at least two independent and interoperable implementation[s]" must exist [RFC2026]. The goal of this implementation of the standard is to satisfy that requirement.

## Operation

### Requirements

1. Go (tested with version 1.16.6)
2. The source code

### Satisfy Requirements

To install Go, follow the excellent documentation [online](https://go.dev/doc/install).

To get the source code, 

```
$ git clone https://github.com/hawkinsw/goresponsiveness.git
```

For the remainder of the instructions, we will assume that `${RSPVNSS_SOURCE_DIR}` is the location of the source code.

### Build

From `${RSPVNSS_SOURCE_DIR}`, 
```
$ go build networkQuality.go
```

That will create an executable in `${RSPVNSS_SOURCE_DIR}` named `networkQuality`.


### Run

From `${RSPVNSS_SOURCE_DIR}`, running the client is straightforward. Simply 

```
$ ./networkQuality
```

Without any options, the tool will attempt to contact `networkquality.example.com` on port 4043 to conduct a measurement. That's likely *not* what you intended. To find out all the options for configuring the execution of the tool, specify the `--help` option:

```
$./networkQuality --help
```

`networkQuality` with the `--help` option will generate the following output:

```
Usage of ./networkQuality:
  -config string
    	name/IP of responsiveness configuration server. (default "networkquality.example.com")
  -debug
    	Enable debugging.
  -path string
    	path on the server to the configuration endpoint. (default "config")
  -port int
    	port number on which to access responsiveness configuration server. (default 4043)
  -profile string
    	Enable client runtime profiling and specify storage location. Disabled by default.
  -store-ssl-keys
    	Store SSL keys from connections for debugging. (currently unused)
  -timeout int
    	Maximum time to spend measuring. (default 20)
```

To facilitate testing, you may want to use the open-source RPM server available from [Apple on GitHub](https://github.com/network-quality/server/tree/main/go).

## References

[RFC2026] https://datatracker.ietf.org/doc/html/rfc2026