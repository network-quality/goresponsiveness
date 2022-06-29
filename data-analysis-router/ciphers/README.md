# Purpose
Collect performance metrics with regards to TLS Ciphers on a router.  

## Ciphers
TLS_CHACHA20_POLY1305_SHA256 - used in the original (non-patched) version of the tool. It is used by default when hardware does not have AES support (and should be lighter on CPU).  
TLS_AES_128_GCM_SHA256 - used in the patched version of the tool to measure metrics.   

## Router Info
Router: RT-AC68U

`$ cat /proc/cpuinfo`
```
Processor       : ARMv7 Processor rev 0 (v7l)
processor       : 0
BogoMIPS        : 1998.84

processor       : 1
BogoMIPS        : 1998.84

Features        : swp half thumb fastmult edsp
CPU implementer : 0x41
CPU architecture: 7
CPU variant     : 0x3
CPU part        : 0xc09
CPU revision    : 0

Hardware        : Northstar Prototype
Revision        : 0000
Serial          : 0000000000000000
```

## Server Info
Running go implementation from [server](https://github.com/network-quality/server) on a Digital Ocean VM.  
Data center: NYC1  
1000Gbps / 1GB Memory / 1 AMD vCPU / 25 GB Disk + 100 / Ubuntu 20.05 LTS x64  


# Patch Applied
Patch is where I manually set the `transport.TLSClientConfig.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}` in config.go and lgc.go (both lgd & lgu) to "force" AES cipher choice (because of Go's assumption [CipherSuite preference](https://cs.opensource.google/go/go/+/9d0819b27ca248f9949e7cf6bf7cb9fe7cf574e8:src/crypto/tls/cipher_suites.go;l=390;drc=2a78e8afc0994f5b292bc9a5a7258c749e43032f;bpv=1;bpt=1) used by our server's [handshake](https://cs.opensource.google/go/go/+/9d0819b27ca248f9949e7cf6bf7cb9fe7cf574e8:src/crypto/tls/handshake_server.go;l=328;drc=2a78e8afc0994f5b292bc9a5a7258c749e43032f))

# Build Command
We built with ARM version 5 because the linux kernel on the router is a older one, and other versions (6, 7) did not run.  
```Powershell
$env:GOOS = "linux"; $env:GOARCH = "arm"; $env:GOARM= "5";  go build -o build/networkQARM5 ./networkQuality.go
```

# Commands to collect data:
```
./networkQARM5 -config rpm.obs.cr -path config -port 4043 -profile original.out >> original.log
```
```
./networkQARM5Patch -config rpm.obs.cr -path config -port 4043 -profile patch.out >> patch.log
```

# Cipher Choice Confirmation
Using Wireshark and ciphersuite field from server hello packet to confirm that the cipher was used:    
Confirmed that original ran with CHACHA  
Confirmed that patch ran with AES  

# Binaries
For reference these are the binaries we used.  
[AES](https://mailuc-my.sharepoint.com/:u:/g/personal/wang2ba_ucmail_uc_edu/EVyoihXkjPFNnpeSXLF4qY4B9T4X1NvF52veItO3E9t1sQ?e=Zelf8L)  
[CHACHA](https://mailuc-my.sharepoint.com/:u:/g/personal/wang2ba_ucmail_uc_edu/ETMyWxK-rtlOrgqbCTquG58Bp_xNoOEdR8KUirW0PTwdVQ?e=rnXrFI)

# View Data
Using the above binaries  
AES:
```
go tool pprof -http localhost:8000 .\AES\networkQARM5Patch .\AES\patch.out
```
CHACHA:
```
go tool pprof -http localhost:8001 .\CHACHA\networkQARM5 .\CHACHA\original.out
```

# Sampling Data
We collected 100 samples with AES and CHACHA in order to run a comparison.  
These samples are stored under patchSamples.log (AES) and originalSamples.log (CHACHA).  

Using local data-analysis-router/ciphers/gather.sh to collect the information:
```
#!/bin/sh

CHACHALOG="originalSamples.log"
AESLOG="patchSamples.log"
i=0

while [ "$i" -lt 100 ]
do
	date >> ${CHACHALOG}
	./networkQARM5 -config rpm.obs.cr -path config -port 4043 >> ${CHACHALOG}
	echo "Done with original (CHACHA) run (#${i})"
	date >> ${AESLOG}
	./networkQARM5Patch -config rpm.obs.cr -path config -port 4043 >> ${AESLOG}
	echo "Done with patch (AES) run (#${i})"
    i=`expr $i + 1`
done
```

## Notes on data
While running there was a repeating error for the CHACHA runs: `Error: Saturation could not be completed in time and no provisional rates could be assessed. Test failed.`  
These failed on runs: 18, 20, 22, 38, 55, 81, 97, 99  