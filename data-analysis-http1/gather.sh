#!/bin/sh
UUID=`date | sed 's/[ :]//g'`
HTTP="HTTP.log.${UUID}"
HTTPS="HTTPS.log.${UUID}"

i=0
while [ "$i" -lt 100 ]
do
	echo "Run: ${i}: $(date)" >> ${HTTP}
	./networkQARM5Patch -config rpm.obs.cr -path config -http >> ${HTTP} 2>&1
	echo "Done with patch (HTTP) run (#${i})"
	echo "Run: ${i}: $(date)" >> ${HTTPS}
	./networkQARM5Patch -config rpm.obs.cr -path config >> ${HTTPS} 2>&1
	echo "Done with patch (HTTPS) run (#${i})"
    i=`expr $i + 1`
done
