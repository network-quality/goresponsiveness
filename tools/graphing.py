import re

import pandas as pd
import matplotlib.pyplot as plt
import os

__FILENAMES__ = {
    "foreign": "-foreign-",
    "self": "-self-",
    "download": "-throughput-download-",
    "upload": "-throughput-upload-",
    "granular": "-throughput-granular-",
}


def seconds_since_start(dfs, start, column_name="SecondsSinceStart"):
    """
    Adds "Seconds Since Start" column to all DataFrames in List of DataFrames,
    based on "CreationTime" column within them and start time passed.

    :param dfs: List of DataFrames. Each DataFrame MUST contain DateTime column named "CreationTime"
    :param start: DateTime start time
    :param column_name: String of column name to add, default "SecondsSinceStart"
    :return: Inplace addition of column using passed column name
    """
    for df in dfs:
        df[column_name] = (df["CreationTime"]-start).apply(pd.Timedelta.total_seconds)


def find_earliest(dfs):
    """
    Returns earliest DateTime in List of DataFrames based on "CreationTime" column within them.
    ASSUMES DATAFRAMES ARE SORTED

    :param dfs: List of DataFrames. Each DataFrame MUST contain DateTime column named "CreationTime" and MUST BE SORTED by it.
    :return: DateTime of earliest time within all dfs.
    """
    earliest = dfs[0]["CreationTime"].iloc[0]
    for df in dfs:
        if df["CreationTime"].iloc[0] < earliest:
            earliest = df["CreationTime"].iloc[0]
    return earliest


def timeSinceStart(dfs, start):
    """
    Adds "TimeSinceStart" column to all dataframes
    :param dfs:
    :param start:
    :return:
    """
    for df in dfs:
        df["TimeSinceStart"] = df["CreationTime"]-start

def probeClean(df):
    # ConnRTT and ConnCongestionWindow refer to Underlying Connection
    df.columns = ["CreationTime", "NumRTT", "Duration", "ConnRTT", "ConnCongestionWindow", "Type", "Empty"]
    df = df.drop(columns=["Empty"])
    df["CreationTime"] = pd.to_datetime(df["CreationTime"], format="%m-%d-%Y-%H-%M-%S.%f")
    df["Type"] = df["Type"].apply(str.strip)
    df["ADJ_Duration"] = df["Duration"] / df["NumRTT"]
    df = df.sort_values(by=["CreationTime"])
    return df


def throughputClean(df):
    df.columns = ["CreationTime", "Throughput", "NumberConnections", "Empty"]
    df = df.drop(columns=["Empty"])
    df["CreationTime"] = pd.to_datetime(df["CreationTime"], format="%m-%d-%Y-%H-%M-%S.%f")
    df["ADJ_Throughput"] = df["Throughput"] / 1000000
    df = df.sort_values(by=["CreationTime"])
    return df


def granularClean(df):
    df.columns = ["CreationTime", "Throughput", "ID", "Type", "Empty"]
    df = df.drop(columns=["Empty"])
    df["CreationTime"] = pd.to_datetime(df["CreationTime"], format="%m-%d-%Y-%H-%M-%S.%f")
    df["Type"] = df["Type"].apply(str.strip)
    df["ADJ_Throughput"] = df["Throughput"] / 1000000
    df = df.sort_values(by=["CreationTime"])
    return df


def make90Percentile(df):
    df = df.sort_values(by=["ADJ_Duration"])
    df = df.reset_index()
    df = df.iloc[:int(len(df)*.9)]
    df = df.sort_values(by=["CreationTime"])
    return df


def main(title, paths):
    # Data Ingestion
    foreign = pd.read_csv(paths["foreign"])
    self = pd.read_csv(paths["self"])
    download = pd.read_csv(paths["download"])
    upload = pd.read_csv(paths["upload"])
    granular = pd.read_csv(paths["granular"])

    # Data Cleaning
    foreign = probeClean(foreign)
    self = probeClean(self)
    download = throughputClean(download)
    upload = throughputClean(upload)
    granular = granularClean(granular)

    # Data Separation
    selfUp = self[self["Type"] == "SelfUp"]
    selfUp = selfUp.reset_index()
    selfDown = self[self["Type"] == "SelfDown"]
    selfDown = selfDown.reset_index()
    granularUp = granular[granular["Type"] == "Upload"]
    granularUp = granularUp.reset_index()
    granularDown = granular[granular["Type"] == "Download"]
    granularDown = granularDown.reset_index()



    # Moving Average
    foreign["DurationMA5"] = foreign["ADJ_Duration"].rolling(window=5).mean()
    selfUp["DurationMA5"] = selfUp["ADJ_Duration"].rolling(window=5).mean()
    selfDown["DurationMA5"] = selfDown["ADJ_Duration"].rolling(window=5).mean()

    # Normalize
    dfs = [foreign, selfUp, selfDown, download, upload, granularUp, granularDown]
    timeSinceStart(dfs, find_earliest(dfs))
    seconds_since_start(dfs, find_earliest(dfs))

    yCol = "SecondsSinceStart"

    def GraphNormal():
        ########## Graphing Complete
        fig, ax = plt.subplots()
        ax.set_title(title)
        ax.plot(foreign[yCol], foreign["ADJ_Duration"], "b.", label="foreign")
        ax.plot(selfUp[yCol], selfUp["ADJ_Duration"], "r.", label="selfUP")
        ax.plot(selfDown[yCol], selfDown["ADJ_Duration"], "c.", label="selfDOWN")
        ax.plot(foreign[yCol], foreign["DurationMA5"], "b--", label="foreignMA")
        ax.plot(selfUp[yCol], selfUp["DurationMA5"], "r--", label="selfUPMA")
        ax.plot(selfDown[yCol], selfDown["DurationMA5"], "c--", label="selfDOWNMA")
        ax.set_ylim([0, max(foreign["ADJ_Duration"].max(), self["ADJ_Duration"].max())])
        ax.legend(loc="upper left")

        secax = ax.twinx()
        secax.plot(download[yCol], download["ADJ_Throughput"], "g-", label="download (MB/s)")
        secax.plot(granularDown[granularDown["ID"] == 0][yCol], granularDown[granularDown["ID"] == 0]["ADJ_Throughput"], "g--", label="Download Connection 0 (MB/S)")
        secax.plot(upload[yCol], upload["ADJ_Throughput"], "y-", label="upload (MB/s)")
        secax.plot(granularUp[granularUp["ID"] == 0][yCol], granularUp[granularUp["ID"] == 0]["ADJ_Throughput"], "y--", label="Upload Connection 0 (MB/S)")
        secax.legend(loc="upper right")
    #GraphNormal()

    def StackedThroughput():
        ########## Graphing Stacked
        fig, ax = plt.subplots()
        ax.set_title(title + " Granular Throughput")
        # ax.plot(foreign[yCol], foreign["ADJ_Duration"], "b.", label="foreign")
        # ax.plot(selfUp[yCol], selfUp["ADJ_Duration"], "r.", label="selfUP")
        # ax.plot(selfDown[yCol], selfDown["ADJ_Duration"], "c.", label="selfDOWN")
        # ax.plot(foreign[yCol], foreign["DurationMA5"], "b--", label="foreignMA")
        # ax.plot(selfUp[yCol], selfUp["DurationMA5"], "r--", label="selfUPMA")
        # ax.plot(selfDown[yCol], selfDown["DurationMA5"], "c--", label="selfDOWNMA")
        # ax.set_ylim([0, max(foreign["ADJ_Duration"].max(), self["ADJ_Duration"].max())])
        # ax.legend(loc="upper left")

        secax = ax.twinx()
        secax.plot(download[yCol], download["ADJ_Throughput"], "g-", label="download (MB/s)")
        secax.plot(upload[yCol], upload["ADJ_Throughput"], "y-", label="upload (MB/s)")

        granularDown["bucket"] = granularDown["SecondsSinceStart"].round(0)
        buckets = pd.DataFrame(granularDown["bucket"].unique())
        buckets.columns = ["bucket"]
        buckets = buckets.set_index("bucket")
        buckets["SecondsSinceStart"] = granularDown.drop_duplicates(subset=["bucket"]).reset_index()["SecondsSinceStart"]
        buckets["bottom"] = 0
        for id in sorted(granularDown["ID"].unique()):
            secax.bar(granularDown[yCol][granularDown["ID"] == id] + .05,
                      granularDown["ADJ_Throughput"][granularDown["ID"] == id],
                      width=.09, bottom=buckets.iloc[len(buckets) - len(granularDown[granularDown["ID"] == id]):]["bottom"]
                      )
            # ,label=f"Download Connection {id}")
            buckets["toadd_bottom"] = (granularDown[granularDown["ID"] == id]).set_index("bucket")["ADJ_Throughput"]
            buckets["toadd_bottom"] = buckets["toadd_bottom"].fillna(0)
            buckets["bottom"] += buckets["toadd_bottom"]


        granularUp["bucket"] = granularUp["SecondsSinceStart"].round(0)
        buckets = pd.DataFrame(granularUp["bucket"].unique())
        buckets.columns = ["bucket"]
        buckets = buckets.set_index("bucket")
        buckets["SecondsSinceStart"] = granularUp.drop_duplicates(subset=["bucket"]).reset_index()["SecondsSinceStart"]
        buckets["bottom"] = 0
        for id in sorted(granularUp["ID"].unique()):
            secax.bar(granularUp[yCol][granularUp["ID"] == id] - .05, granularUp["ADJ_Throughput"][granularUp["ID"] == id],
                      width=.09, bottom=buckets.iloc[len(buckets) - len(granularUp[granularUp["ID"] == id]):]["bottom"]
                      )
                      #,label=f"Upload Connection {id}")
            buckets["toadd_bottom"] = (granularUp[granularUp["ID"] == id]).set_index("bucket")["ADJ_Throughput"]
            buckets["toadd_bottom"] = buckets["toadd_bottom"].fillna(0)
            buckets["bottom"] += buckets["toadd_bottom"]
        secax.legend(loc="upper right")


        secax.legend(loc="upper left")

    #StackedThroughput()
    stacked_bar_throughput(upload, granularUp, "SecondsSinceStart", "ADJ_Throughput", title + " Upload Stacked",
                           "Upload Throughput MB/s")
    stacked_bar_throughput(download, granularDown, "SecondsSinceStart", "ADJ_Throughput", title + " Download Stacked",
                           "Download Throughput MB/s")

    def Percent90():
        ######### Graphing Removing 90th Percentile
        nonlocal selfUp
        nonlocal selfDown
        nonlocal foreign
        selfUp = make90Percentile(selfUp)
        selfDown = make90Percentile(selfDown)
        foreign = make90Percentile(foreign)

        # Recalculate MA
        foreign["DurationMA5"] = foreign["ADJ_Duration"].rolling(window=5).mean()
        selfUp["DurationMA5"] = selfUp["ADJ_Duration"].rolling(window=5).mean()
        selfDown["DurationMA5"] = selfDown["ADJ_Duration"].rolling(window=5).mean()

        # Graphing Complete
        fig, ax = plt.subplots()
        ax.set_title(title + " 90th Percentile (ordered lowest to highest duration)")
        # ax.plot(foreign[yCol], foreign["ADJ_Duration"], "b.", label="foreign")
        # ax.plot(selfUp[yCol], selfUp["ADJ_Duration"], "r.", label="selfUP")
        # ax.plot(selfDown[yCol], selfDown["ADJ_Duration"], "c.", label="selfDOWN")
        ax.plot(foreign[yCol], foreign["DurationMA5"], "b--", label="foreignMA")
        ax.plot(selfUp[yCol], selfUp["DurationMA5"], "r--", label="selfUPMA")
        ax.plot(selfDown[yCol], selfDown["DurationMA5"], "c--", label="selfDOWNMA")
        ax.set_ylim([0, max(foreign["ADJ_Duration"].max(), selfUp["ADJ_Duration"].max(), selfDown["ADJ_Duration"].max())])
        ax.legend(loc="upper left")

        secax = ax.twinx()
        secax.plot(download[yCol], download["ADJ_Throughput"], "g-", label="download (MB/s)")
        secax.plot(granularDown[granularDown["ID"] == 0][yCol], granularDown[granularDown["ID"] == 0]["ADJ_Throughput"],
                   "g--", label="Download Connection 0 (MB/S)")
        secax.plot(upload[yCol], upload["ADJ_Throughput"], "y-", label="upload (MB/s)")
        secax.plot(granularUp[granularUp["ID"] == 0][yCol], granularUp[granularUp["ID"] == 0]["ADJ_Throughput"], "y--",
                   label="Upload Connection 0 (MB/S)")
        secax.legend(loc="upper right")

    Percent90()

def stacked_bar_throughput(df, granular, xcolumn, ycolumn, title, label):
    fig, ax = plt.subplots()
    ax.set_title(title)

    secax = ax.twinx()
    ax.get_yaxis().set_visible(False)
    ax.set_xlabel("Seconds Since Start (s)")
    secax.set_ylabel("Throughput (MB/s)")
    # secax.set_xticks(range(0, round(granular[xcolumn].max()) + 1)) # Ticks every 1 second

    # Plot Main Throughput
    secax.plot(df[xcolumn], df[ycolumn], "k--", label=label)

    df_gran = granular.copy()
    # df_gran["bucket"] = df_gran[xcolumn].round(0) # With rounding
    df_gran["bucket"] = df_gran[xcolumn] # Without rounding (csv creation time points need to be aligned)
    buckets = pd.DataFrame(df_gran["bucket"].unique())
    buckets.columns = ["bucket"]
    buckets = buckets.set_index("bucket")
    buckets[xcolumn] = df_gran.drop_duplicates(subset=["bucket"]).reset_index()[xcolumn]
    buckets["bottom"] = 0
    for id in sorted(df_gran["ID"].unique()):
        secax.bar(df_gran[xcolumn][df_gran["ID"] == id],
                  df_gran[ycolumn][df_gran["ID"] == id],
                  width=.25, bottom=buckets.iloc[len(buckets) - len(df_gran[df_gran["ID"] == id]):]["bottom"]
                  )
        # ,label=f"Download Connection {id}")
        buckets["toadd_bottom"] = (df_gran[df_gran["ID"] == id]).set_index("bucket")[ycolumn]
        buckets["toadd_bottom"] = buckets["toadd_bottom"].fillna(0)
        buckets["bottom"] += buckets["toadd_bottom"]

    secax.legend(loc="upper right")


def findFiles(dir):
    matches = {}

    files = os.listdir(dir)
    for file in files:
        if os.path.isfile(dir+file):
            for name in __FILENAMES__:
                regex = "(?P<start>.*)(?P<type>" + __FILENAMES__[name] + ")(?P<end>.*)"
                match = re.match(regex, file)
                if match is not None:
                    start = match.group("start")
                    end = match.group("end")
                    if start not in matches:
                        matches[start] = {}
                    if end not in matches[start]:
                        matches[start][end] = {}
                    if name in matches[start][end]:
                        print("ERROR ALREADY FOUND A FILE THAT HAS THE SAME MATCHING")
                    matches[start][end][name] = dir+file
    return matches

def generatePaths():
    return {
        "foreign": "",
        "self": "",
        "download": "",
        "upload": "",
        "granular": "",
    }

def makeGraphs(files):
    for start in files:
        x = 0
        for end in files[start]:
            # Check if it contains all file fields
            containsALL = True
            for key in __FILENAMES__:
                if key not in files[start][end]:
                    containsALL = False
            # If we don't have all files then loop to next one
            if not containsALL:
                continue

            main(start + " - " + str(x), files[start][end])
            x += 1

# Press the green button in the gutter to run the script.
if __name__ == '__main__':
    paths = generatePaths()

    files = findFiles("./Data/WillTest/")
    print(files)
    makeGraphs(files)

    plt.show()