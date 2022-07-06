$FILENAME = "./HTTPS.log.TueJul5173219-42022"
$OUTPUTFILE = "./HTTPS.csv"

$RAWDATA = Get-Content $FILENAME -Raw

# Expected Output
# Download:  66.228 Mbps (  8.279 MBps), using 16 parallel connections.
# Upload:    36.257 Mbps (  4.532 MBps), using 16 parallel connections.
# Total measurements: 15
# RPM:   213
# Extended Statistics:
# 	Maximum Segment Size: 1460
# 	Total Bytes Retransmitted: 52820
# 	Retransmission Ratio: 3.64%
# 	Total Bytes Reordered: 35
# 	Average RTT: 50152.55

$PATTERN = ("Download:\s+(?<Download>\d*\.\d*) Mbps.*using (?<DownloadConnections>\d*) parallel connections.\r\n" +
            "Upload:\s+(?<Upload>\d*\.\d*) Mbps.*using (?<UploadConnections>\d*) parallel connections.\r\n" +
            ".*\r\n" +
            "RPM:\s+(?<RPM>\d*)\r\n" +
            ".*\r\n" +
            ".*Size:\s+(?<MSS>\d*)\r\n" +
            ".*Retransmitted:\s+(?<Retransmitted>\d*)\r\n" +
            ".*Ratio:\s+(?<RetransRatio>\d*\.\d*)%\r\n" +
            ".*Reordered:\s+(?<Reordered>\d*)\r\n" +
            ".*RTT:\s+(?<RTT>\d*\.\d*)\r\n"
            )

### IF USING WINDOWS CRLF MUST REPLACE \n WITH \r\n ###

"Download (Mbps), Upload(Mbps), Download Connections, Upload Connections, RPM, Segment Size, Retransmitted(Bytes), Retransmission Ratio(%), Reordered(Bytes), RTT(Us)" > $OUTPUTFILE

$Matches = $RAWDATA | Select-String -Pattern $PATTERN -AllMatches
Foreach ($Match in $Matches.Matches) {
    ($Match.groups["Download"].Value.ToString() + "," +
     $Match.groups["Upload"].Value.ToString() + "," +
     $Match.groups["DownloadConnections"].Value.ToString() + "," +
     $Match.groups["UploadConnections"].Value.ToString() + "," +
     $Match.groups["RPM"].Value.ToString() +  "," +
     $Match.groups["MSS"].Value.ToString() + "," +
     $Match.groups["Retransmitted"].Value.ToString() + "," +
     $Match.groups["RetransRatio"].Value.ToString() + "," +
     $Match.groups["Reordered"].Value.ToString() + "," +
     $Match.groups["RTT"].Value.ToString()
    ) >> $OUTPUTFILE
}