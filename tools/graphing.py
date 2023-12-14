import re

import pandas as pd
import matplotlib.pyplot as plt
import matplotlib.backends.backend_pdf # For pdf output
import os
import argparse
import pprint

parser = argparse.ArgumentParser(description='Make some graphs using CSV files!')
# parser.add_argument("filename", type=argparse.FileType('r'))
parser.add_argument("filename", type=str, help="Put a single one of the log files from a set here, " +
                                               "and it will parse the rest")


# Regex should be described below in match_filename(filename, component)
__COMPONENTS__ = {
    "foreign": "-foreign-",
    "self": "-self-",
    "download": "-throughput-download-",
    "upload": "-throughput-upload-",
    "granular": "-throughput-granular-",
}

__LINECOLOR__ = {
    "download": "#0095ed",
    "upload": "#44BB66",
    "foreign": "#ac7ae7", # "#7522d7",
    "selfUp": "#7ccf93",
    "selfDown": "#4cb4f2" # "#7fcaf6",
}


def match_filename(filename, component):
    """
    Input a filename and a component regex component to match the filename to its <start><component><end> regex.
    Returns a match object with groups: start, component, end.

    :param filename: String of filename
    :param component: String to add into the regex
    :return: Match object or None
    """
    regex = f"(?P<start>.*)(?P<component>{component})(?P<end>.*)"
    return re.match(regex, filename)


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
        df[column_name] = (df["CreationTime"] - start).apply(pd.Timedelta.total_seconds)


def find_earliest(dfs):
    """
    Returns earliest DateTime in List of DataFrames based on "CreationTime" column within them.
    ASSUMES DATAFRAMES ARE SORTED

    :param dfs: List of DataFrames. Each DataFrame MUST contain DateTime column named "CreationTime" and MUST BE SORTED by it.
    :return: DateTime of earliest time within all dfs.
    """
    earliest = dfs[0]["CreationTime"].iloc[0]
    for df in dfs:
        print(f"A data frame: {df['CreationTime']}")
        if df["CreationTime"].iloc[0] < earliest:
            earliest = df["CreationTime"].iloc[0]
    return earliest


def time_since_start(dfs, start, column_name="TimeSinceStart"):
    """
    Adds "Seconds Since Start" column to all DataFrames in List of DataFrames,
    based on "CreationTime" column within them and start time passed.

    :param dfs: List of DataFrames. Each DataFrame MUST contain DateTime column named "CreationTime"
    :param start: DateTime start time
    :param column_name: String of column name to add, default "SecondsSinceStart"
    :return: Inplace addition of column using passed column name
    """
    for df in dfs:
        df[column_name] = df["CreationTime"] - start


def probeClean(df):
    # ConnRTT and ConnCongestionWindow refer to Underlying Connection
    df.columns = ["CreationTime", "NumRTT", "Duration", "ConnRTT", "ConnCongestionWindow", "Type", "Algorithm", "Empty"]
    df = df.drop(columns=["Empty"])
    df["CreationTime"] = pd.to_datetime(df["CreationTime"], format="%m-%d-%Y-%H-%M-%S.%f")
    df["Type"] = df["Type"].apply(str.strip)
    df["ADJ_Duration"] = df["Duration"] / df["NumRTT"]
    df = df.sort_values(by=["CreationTime"])
    return df


def throughputClean(df):
    df.columns = ["CreationTime", "Throughput", "NumberActiveConnections", "NumberConnections", "Empty"]
    df = df.drop(columns=["Empty"])
    df["CreationTime"] = pd.to_datetime(df["CreationTime"], format="%m-%d-%Y-%H-%M-%S.%f")
    df["ADJ_Throughput"] = df["Throughput"] / 1000000
    df = df.sort_values(by=["CreationTime"])
    return df


def granularClean(df):
    df.columns = ["CreationTime", "Throughput", "ID", "RTT", "Cwnd", "Type", "Empty"]
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
    foreign["DurationMA10"] = foreign["ADJ_Duration"].rolling(window=10).mean()
    selfUp["DurationMA10"] = selfUp["ADJ_Duration"].rolling(window=10).mean()
    selfDown["DurationMA10"] = selfDown["ADJ_Duration"].rolling(window=10).mean()

    # Normalize
    dfs = [foreign, selfUp, selfDown, download, upload, granularUp, granularDown]
    time_since_start(dfs, find_earliest(dfs))
    seconds_since_start(dfs, find_earliest(dfs))

    yCol = "SecondsSinceStart"

    # stacked_bar_throughput(upload, granularUp, "SecondsSinceStart", "ADJ_Throughput", title + " Upload Stacked",
    #                       "Upload Throughput MB/s")
    # stacked_bar_throughput(download, granularDown, "SecondsSinceStart", "ADJ_Throughput", title + " Download Stacked",
    #                       "Download Throughput MB/s")
    dfs_dict = {
        "foreign": foreign,
        "self": self,
        "download": download,
        "upload": upload,
        "granular": granular,
        "selfUp": selfUp,
        "selfDown": selfDown,
        "granularUp": granularUp,
        "granularDown": granularDown
    }
    fig, ax = plt.subplots()
    fig.canvas.manager.set_window_title(title + " Standard")
    graph_normal(dfs_dict, "SecondsSinceStart", ax, title + " Standard")

    fig, ax = plt.subplots()
    fig.canvas.manager.set_window_title(title + " Standard ms")
    graph_normal_ms(dfs_dict, "SecondsSinceStart", ax, title + " Standard ms")
    
    # Both Upload/Download Granular on one figure
    fig, axs = plt.subplots(2, 1)
    fig.canvas.manager.set_window_title(title + " Combined Throughput")
    stacked_area_throughput(download, granularDown, "SecondsSinceStart", "ADJ_Throughput", axs[0],
                            title + " Download Stacked",
                            "Download Throughput MB/s", __LINECOLOR__["download"])
    stacked_area_throughput(upload, granularUp, "SecondsSinceStart", "ADJ_Throughput", axs[1],
                            title + " Upload Stacked",
                            "Upload Throughput MB/s",  __LINECOLOR__["upload"])
    # Individual figure
    fig, ax = plt.subplots()
    fig.canvas.manager.set_window_title(title + " Download Throughput")
    stacked_area_throughput(download, granularDown, "SecondsSinceStart", "ADJ_Throughput", ax,
                            title + " Download Stacked",
                            "Download Throughput MB/s",  __LINECOLOR__["download"])
    fig, ax = plt.subplots()
    fig.canvas.manager.set_window_title(title + " Upload Throughput")
    stacked_area_throughput(upload, granularUp, "SecondsSinceStart", "ADJ_Throughput", ax,
                            title + " Upload Stacked",
                            "Upload Throughput MB/s",  __LINECOLOR__["upload"])

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

    # Percent90()


def graph_normal_ms(dfs, xcolumn, ax, title):
    ax.set_title(title)
    ax.set_xlabel("Seconds Since Start (s)")

    # To plot points
    # ax.plot(dfs["foreign"][xcolumn], dfs["foreign"]["ADJ_Duration"], "b.", label="foreign")
    # ax.plot(dfs["selfUp"][xcolumn], dfs["selfUp"]["ADJ_Duration"], "r.", label="selfUP")
    # ax.plot(dfs["selfDown"][xcolumn], dfs["selfDown"]["ADJ_Duration"], "c.", label="selfDOWN")
    dfs["foreign"]["DurationMA10ms"] = dfs["foreign"]["ADJ_Duration"].rolling(window=10, step=10).mean() * 1000
    dfs["selfUp"]["DurationMA10ms"] = dfs["selfUp"]["ADJ_Duration"].rolling(window=10, step=10).mean() * 1000
    dfs["selfDown"]["DurationMA10ms"] = dfs["selfDown"]["ADJ_Duration"].rolling(window=10, step=10).mean() * 1000
    # Plot lines
    ax.plot(dfs["foreign"][xcolumn][dfs["foreign"]["DurationMA10ms"].notnull()], dfs["foreign"]["DurationMA10ms"][dfs["foreign"]["DurationMA10ms"].notnull()], "--", linewidth=2, color=__LINECOLOR__["foreign"], label="foreignMA10 (ms)")
    ax.plot(dfs["selfUp"][xcolumn][dfs["selfUp"]["DurationMA10ms"].notnull()], dfs["selfUp"]["DurationMA10ms"][dfs["selfUp"]["DurationMA10ms"].notnull()], "--", linewidth=2, color=__LINECOLOR__["selfUp"], label="selfUpMA10 (ms)")
    ax.plot(dfs["selfDown"][xcolumn][dfs["selfDown"]["DurationMA10ms"].notnull()], dfs["selfDown"]["DurationMA10ms"][dfs["selfDown"]["DurationMA10ms"].notnull()], "--", linewidth=2, color=__LINECOLOR__["selfDown"], label="selfDownMA10 (ms)")
    ax.set_ylim([0, max(dfs["foreign"]["DurationMA10ms"].max(), dfs["selfUp"]["DurationMA10ms"].max(), dfs["selfDown"]["DurationMA10ms"].max()) * 1.01])
    ax.set_ylabel("RTT (ms)")
    ax.legend(loc="upper left", title="Probes")


    secax = ax.twinx()
    secax.plot(dfs["download"][xcolumn], dfs["download"]["ADJ_Throughput"], "-", linewidth=2, color=__LINECOLOR__["download"], label="download (MB/s)")
    # secax.plot(dfs.granularDown[dfs.granularDown["ID"] == 0][xcolumn], dfs.granularDown[dfs.granularDown["ID"] == 0]["ADJ_Throughput"],
    #            "g--", label="Download Connection 0 (MB/S)")
    secax.plot(dfs["upload"][xcolumn], dfs["upload"]["ADJ_Throughput"], "-", linewidth=2, color=__LINECOLOR__["upload"], label="upload (MB/s)")
    # secax.plot(dfs.granularUp[dfs.granularUp["ID"] == 0][xcolumn], dfs.granularUp[dfs.granularUp["ID"] == 0]["ADJ_Throughput"], "y--",
    #            label="Upload Connection 0 (MB/S)")
    secax.set_ylabel("Throughput (MB/s)")
    secax.legend(loc="upper right")


def graph_normal(dfs, xcolumn, ax, title):
    ax.set_title(title)
    ax.set_xlabel("Seconds Since Start (s)")

    # To plot points
    # ax.plot(dfs["foreign"][xcolumn], dfs["foreign"]["ADJ_Duration"], "b.", label="foreign")
    # ax.plot(dfs["selfUp"][xcolumn], dfs["selfUp"]["ADJ_Duration"], "r.", label="selfUP")
    # ax.plot(dfs["selfDown"][xcolumn], dfs["selfDown"]["ADJ_Duration"], "c.", label="selfDOWN")
    # Plot lines
    ax.plot(dfs["foreign"][xcolumn], dfs["foreign"]["DurationMA10"], "--", linewidth=2, color=__LINECOLOR__["foreign"], label="foreignMA10 (s)")
    ax.plot(dfs["selfUp"][xcolumn], dfs["selfUp"]["DurationMA10"], "--", linewidth=2, color=__LINECOLOR__["selfUp"], label="selfUpMA10 (s)")
    ax.plot(dfs["selfDown"][xcolumn], dfs["selfDown"]["DurationMA10"], "--", linewidth=2, color=__LINECOLOR__["selfDown"], label="selfDownMA10 (s)")
    ax.set_ylim([0, max(dfs["foreign"]["DurationMA10"].max(), dfs["selfUp"]["DurationMA10"].max(), dfs["selfDown"]["DurationMA10"].max()) * 1.01])
    ax.set_ylabel("RTT (s)")
    ax.legend(loc="upper left", title="Probes")


    secax = ax.twinx()
    secax.plot(dfs["download"][xcolumn], dfs["download"]["ADJ_Throughput"], "-", linewidth=2, color=__LINECOLOR__["download"], label="download (MB/s)")
    # secax.plot(dfs.granularDown[dfs.granularDown["ID"] == 0][xcolumn], dfs.granularDown[dfs.granularDown["ID"] == 0]["ADJ_Throughput"],
    #            "g--", label="Download Connection 0 (MB/S)")
    secax.plot(dfs["upload"][xcolumn], dfs["upload"]["ADJ_Throughput"], "-", linewidth=2, color=__LINECOLOR__["upload"], label="upload (MB/s)")
    # secax.plot(dfs.granularUp[dfs.granularUp["ID"] == 0][xcolumn], dfs.granularUp[dfs.granularUp["ID"] == 0]["ADJ_Throughput"], "y--",
    #            label="Upload Connection 0 (MB/S)")
    secax.set_ylabel("Throughput (MB/s)")
    secax.legend(loc="upper right")
    

def stacked_area_throughput(throughput_df, granular, xcolumn, ycolumn, ax, title, label, linecolor="black"):

    print(f"Stacked area throughput!")
    ax.set_title(title)

    ax.yaxis.tick_right()
    ax.yaxis.set_label_position("right")
    ax.set_xlabel("Seconds Since Start (s)")
    ax.set_ylabel("Throughput (MB/s)")
    # ax.set_xticks(range(0, round(granular[xcolumn].max()) + 1)) # Ticks every 1 second

    # Plot Main Throughput
    ax.plot(throughput_df[xcolumn], throughput_df[ycolumn], "-", color="white", linewidth=3)
    ax.plot(throughput_df[xcolumn], throughput_df[ycolumn], "-", color=linecolor, linewidth=2, label=label)

    df_gran = granular.copy()

    # df_gran["bucket"] = df_gran[xcolumn].round(0) # With rounding
    df_gran["bucket"] = df_gran[xcolumn]  # Without rounding (csv creation time points need to be aligned)
    df_gran = df_gran.set_index(xcolumn)

    buckets = pd.DataFrame(df_gran["bucket"].unique())
    buckets.columns = ["bucket"]
    buckets = buckets.set_index("bucket")
    for id in sorted(df_gran["ID"].unique()):
        buckets[id] = df_gran[ycolumn][df_gran["ID"] == id]
    buckets = buckets.fillna(0)

    # Plot Stacked Area Throughput
    ax.stackplot(buckets.index, buckets.transpose())
    ax.legend(loc="upper right")


def stacked_bar_throughput(throughput_df, granular, xcolumn, ycolumn, ax, title, label, linecolor="black"):
    ax.set_title(title)

    ax.yaxis.tick_right()
    ax.yaxis.set_label_position("right")
    ax.set_xlabel("Seconds Since Start (s)")
    ax.set_ylabel("Throughput (MB/s)")
    # ax.set_xticks(range(0, round(granular[xcolumn].max()) + 1)) # Ticks every 1 second

    # Plot Main Throughput
    ax.plot(throughput_df[xcolumn], throughput_df[ycolumn], "-", color=linecolor, label=label)

    df_gran = granular.copy()

    # df_gran["bucket"] = df_gran[xcolumn].round(0) # With rounding
    df_gran["bucket"] = df_gran[xcolumn] # Without rounding (csv creation time points need to be aligned)

    buckets = pd.DataFrame(df_gran["bucket"].unique())
    buckets.columns = ["bucket"]
    buckets = buckets.set_index("bucket")
    buckets[xcolumn] = df_gran.drop_duplicates(subset=["bucket"]).reset_index()[xcolumn]
    buckets["bottom"] = 0
    for id in sorted(df_gran["ID"].unique()):
        ax.bar(df_gran[xcolumn][df_gran["ID"] == id],
               df_gran[ycolumn][df_gran["ID"] == id],
               width=.1, bottom=buckets.iloc[len(buckets) - len(df_gran[df_gran["ID"] == id]):]["bottom"]
               )
        # ,label=f"Download Connection {id}")
        buckets["toadd_bottom"] = (df_gran[df_gran["ID"] == id]).set_index("bucket")[ycolumn]
        buckets["toadd_bottom"] = buckets["toadd_bottom"].fillna(0)
        buckets["bottom"] += buckets["toadd_bottom"]

    ax.legend(loc="upper right")


def find_files(directory):
    matches = {}

    files = os.listdir(directory)
    for file in files:
        if os.path.isfile(directory + file):
            for name in __COMPONENTS__:
                match = match_filename(file, __COMPONENTS__[name])
                if match is not None:
                    start = match.group("start")
                    end = match.group("end")
                    if start not in matches:
                        matches[start] = {}
                    if end not in matches[start]:
                        matches[start][end] = {}
                    if name in matches[start][end]:
                        print("ERROR ALREADY FOUND A FILE THAT HAS THE SAME MATCHING")
                    matches[start][end][name] = directory + file
    return matches


def find_matching_files(directory, filename):
    matches = {}

    # First determine the file's structure
    match = match_filename(os.path.basename(filename), "|".join(__COMPONENTS__.values()))
    if match is not None:
        file_start = match.group("start")
        file_end = match.group("end")
    else:
        print(f"ERROR COULD NOT MATCH FILE TO KNOWN SCHEMA: {filename}")
        return matches

    # Find its other matching files
    files = os.listdir(directory)
    for file in files:
        if os.path.isfile(directory + file):
            for name in __COMPONENTS__:
                match = match_filename(file, __COMPONENTS__[name])
                if match is not None:
                    start = match.group("start")
                    end = match.group("end")
                    if file_start == start and file_end == end:
                        if start not in matches:
                            matches[start] = {}
                        if end not in matches[start]:
                            matches[start][end] = {}
                        if name in matches[start][end]:
                            print("ERROR ALREADY FOUND A FILE THAT HAS THE SAME MATCHING")
                        matches[start][end][name] = directory + file
    return matches


def generate_paths():
    return {
        "foreign": "",
        "self": "",
        "download": "",
        "upload": "",
        "granular": "",
    }


def make_graphs(files, save):
    num_fig = 1
    for start in files:
        x = 0
        for end in files[start]:
            # Check if it contains all file fields
            containsALL = True
            for key in __COMPONENTS__:
                if key not in files[start][end]:
                    containsALL = False
            # If we don't have all files then loop to next one
            if not containsALL:
                continue

            print(f"About to call main()")
            main(start + " - " + str(x), files[start][end])
            if save:
                pdf = matplotlib.backends.backend_pdf.PdfPages(f"{start} - {x}.pdf")
                for fig in range(num_fig, plt.gcf().number + 1):
                    plt.figure(fig).set(size_inches=(11, 6.1875))  # 16:9 ratio for screens (11 x 6.1875) # 11 x 8.5 for page size
                    plt.figure(fig).tight_layout()
                    pdf.savefig(fig)
                    plt.figure(fig).set(size_inches=(10, 6.6))
                    plt.figure(fig).tight_layout()
                pdf.close()
                num_fig = plt.gcf().number + 1
            x += 1


# Press the green button in the gutter to run the script.
if __name__ == '__main__':
    ARGS = parser.parse_args()
    paths = generate_paths()

    print(f"Looking for files in directory: {os.path.dirname(ARGS.filename)}")
    # files = find_files(os.path.dirname(ARGS.filename) + "/")
    if os.path.isfile(ARGS.filename):
        files = find_matching_files(os.path.dirname(ARGS.filename) + "/", ARGS.filename)
    elif os.path.isdir(ARGS.filename):
        files = find_files(ARGS.filename)
    else:
        print("Error: filename passed is not recognized as a file or directory.")
        exit()

    print("Found files:")
    pprint.pprint(files, indent=1)
    make_graphs(files, True)
    plt.show()