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