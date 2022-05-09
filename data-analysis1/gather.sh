#!/bin/bash

UUID=`date | sed 's/[ :]//g'`
GOLOG="go.log.${UUID}"
NQLOG="nq.log.${UUID}"

for i in `seq 1 100`; do
	date >> ${GOLOG}
	./networkQuality -config rpm.obs.cr -path config -port 4043 >> ${GOLOG} 
	echo "Done with Go test (#${i})"
	date >> ${NQLOG}
	/usr/bin/networkQuality -C https://rpm.obs.cr:4043/config >> ${NQLOG}
	echo "Done with networkQuality test (#${i})"
done
