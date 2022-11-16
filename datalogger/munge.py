#!/bin/env python3

from dataclasses import dataclass
import csv

@dataclass
class ProbeDataPoint:
  time: str
  rtt_count: int
  duration: float

@dataclass
class ThroughputDataPoint:
  time: str
  throughput: str

def convertDuration(duration: str) -> str:
  if duration.endswith("ms"):
    return duration[0:-2]
  elif duration.endswith("s"):
    return str(float(duration[0:-1]) * 1000)

if __name__ == '__main__':
  # Let's open all the files!
  foreign_dps = {}
  self_dps = {}
  dl_throughput_dps = {}
  ul_throughput_dps = {}
  max_happens_after = 0
  with open("log-foreign-07-29-2022-14-36-39.csv") as foreign_dp_fh:
    seen_header = False
    for record in csv.reader(foreign_dp_fh):
      if not seen_header:
        seen_header = True
        continue
      foreign_dps[record[0]] = ProbeDataPoint(record[1], record[2], convertDuration(record[3]))
      if int(record[0]) > max_happens_after:
        max_happens_after = int(record[0])
  with open("log-self-07-29-2022-14-36-39.csv") as self_dp_fh:
    seen_header = False
    for record in csv.reader(self_dp_fh):
      if not seen_header:
        seen_header = True
        continue
      self_dps[record[0]] = ProbeDataPoint(record[1], record[2], convertDuration(record[3]))
      if int(record[0]) > max_happens_after:
        max_happens_after = int(record[0])
  with open("log-throughput-download07-29-2022-14-36-39.csv") as download_dp_fh:
    seen_header = False
    for record in csv.reader(download_dp_fh):
      if not seen_header:
        seen_header = True
        continue
      dl_throughput_dps[record[0]] = ThroughputDataPoint(record[1], record[2])
      if int(record[0]) > max_happens_after:
        max_happens_after = int(record[0])
  with open("log-throughput-upload07-29-2022-14-36-39.csv") as upload_dp_fh:
    seen_header = False
    for record in csv.reader(upload_dp_fh):
      if not seen_header:
        seen_header = True
        continue
      ul_throughput_dps[record[0]] = ThroughputDataPoint(record[1], record[2])
      if int(record[0]) > max_happens_after:
        max_happens_after = int(record[0])

  # Happens After, RT Count (self), RTT (self), RT Count (foreign), RTT (foreign), DL Throughput, UL Throughput
  current = ["" for _ in range(0, 8)]
  for ha in range(0, max_happens_after):
    ha = str(ha)
    found = True
    if ha in self_dps:
      print(f"{ha} is in self_dps")
      current[0] = str(ha)
      current[1] = self_dps[str(ha)].rtt_count
      current[2] = self_dps[str(ha)].duration
    elif ha in foreign_dps:
      print(f"{ha} is in foreign_dps")
      current[0] = str(ha)
      current[3] = foreign_dps[str(ha)].rtt_count
      current[4] = foreign_dps[str(ha)].duration
    elif ha in dl_throughput_dps:
      print(f"{ha} is in throughput download")
      current[0] = str(ha)
      current[5] = dl_throughput_dps[str(ha)].throughput
    elif ha in ul_throughput_dps:
      print(f"{ha} is in throughput upload")
      current[0] = str(ha)
      current[6] = ul_throughput_dps[str(ha)].throughput
    else:
      print(f"Cannot find {ha}.")
      found = False
    if found:
      print(",".join(current))