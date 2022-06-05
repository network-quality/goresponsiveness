#!/bin/bash

# In order to run this script to generate data, you will need the ziplines
# utility: https://github.com/hawkinsw/ziplines

NQRAWDATA=nq.log.FriMay6121542EDT2022
GORAWDATA=go.log.FriMay6121542EDT2022

# Clean up all the CSV files.
rm -rf *.csv

# Create the Go RPM csv file.
cat ${GORAWDATA} | grep RPM | sed 's/RPM:[[:space:]]\+//' > go.csv

# Create the NQ RPM csv file.
cat ${NQRAWDATA} | grep RPM | awk '{print $3;}' | tr -d \( > nq.csv

# Create the Go upload csv file
cat ${GORAWDATA} | grep Upload | sed 's/Upload: //' | awk '{print $1 " " $2;}' | sed 's/Mbps//' > go.upload.csv

# Create the NQ upload csv file
cat ${NQRAWDATA} | grep 'Upload capacity' | awk --field-separator=: '{ print $2; }' | sed 's/ //'| sed 's/Mbps//'  > nq.upload.csv

# Create the Go download csv file
cat ${GORAWDATA} | grep Download | sed 's/Download: //' | awk '{print $1 " " $2;}'| sed 's/Mbps//'  > go.download.csv

# Create the NQ download csv file
cat ${NQRAWDATA} | grep 'Download capacity' | awk --field-separator=: '{ print $2; }' | sed 's/ //'| sed 's/Mbps//'  > nq.download.csv

# Create the Go upload flows csv file
cat ${GORAWDATA} | grep 'Upload.*using' | sed 's/Upload.*using \([[:digit:]]\+\) parallel connections./\1/' > go.upload.flows.csv

# Create the NQ upload flows csv file
cat ${NQRAWDATA} | grep -i 'upload flows'  | awk --field-separator=: '{print $2}' | tr -d ' ' > nq.upload.flows.csv

# Create the Go download flows csv file
cat ${GORAWDATA} | grep 'Download.*using' | sed 's/Download.*using \([[:digit:]]\+\) parallel connections./\1/' > go.download.flows.csv

# Create the NQ download flows csv file
cat ${NQRAWDATA} | grep -i 'download flows'  | awk --field-separator=: '{print $2}' | tr -d ' ' > nq.download.flows.csv

# Create the overall rpm.csv file
echo "Go, Network Quality" > rpm.csv
ziplines , go.csv nq.csv >> rpm.csv

# Create the overall download.csv file
echo "Go, Network Quality" > download.csv
ziplines , go.download.csv nq.download.csv >> download.csv

# Create the overall upload.csv file
echo "Go, Network Quality" > upload.csv
ziplines , go.upload.csv nq.upload.csv >> upload.csv

# Create the overall download.flows.csv file
echo "Go, Network Quality" > download.flows.csv
ziplines , go.download.flows.csv nq.download.flows.csv >> download.flows.csv

# Create the overall upload.flows.csv file
echo "Go, Network Quality" > upload.flows.csv
ziplines , go.upload.flows.csv nq.upload.flows.csv >> upload.flows.csv
