#!/bin/sh
LOG="HTTP2Samples.log"
i=0
while [ "$i" -lt 100 ]
do
	date >> ${LOG}
	./networkQARM5Original -config rpm.obs.cr -path config >> ${LOG}
	echo "Done with patch (HTTP2) run (#${i})"
    i=`expr $i + 1`
done
