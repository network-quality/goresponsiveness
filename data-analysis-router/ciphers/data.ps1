$FILENAME = "./CHACHA/originalSamples.log"
$OUTPUTFILE = "./CHACHAOut.csv"

$RAWDATA = Get-Content $FILENAME -Raw

# Expected Output
# Download:  66.228 Mbps (  8.279 MBps), using 16 parallel connections.
# Upload:    36.257 Mbps (  4.532 MBps), using 16 parallel connections.
# Total measurements: 15
# RPM:   213

$PATTERN = ("Download:\s+(?<Download>\d*\.\d*) Mbps.*using (?<DownloadConnections>\d*) parallel connections.\n" +
            "Upload:\s+(?<Upload>\d*\.\d*) Mbps.*using (?<UploadConnections>\d*) parallel connections.\n" +
            ".*\n" +
            "RPM:\s+(?<RPM>\d*)")

"Download (Mbps), Upload(Mbps), Download Connections, Upload Connections, RPM" > $OUTPUTFILE

$Matches = $RAWDATA | Select-String -Pattern $PATTERN -AllMatches
Foreach ($Match in $Matches.Matches) {
    $Match.groups["Download"].Value.ToString() + "," + $Match.groups["Upload"].Value.ToString() + "," + $Match.groups["DownloadConnections"].Value.ToString() + "," + $Match.groups["UploadConnections"].Value.ToString() + "," + $Match.groups["RPM"].Value.ToString() >> $OUTPUTFILE
}