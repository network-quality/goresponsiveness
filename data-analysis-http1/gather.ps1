$UUID = Get-Date -Format "dddMMMdHHmmsszyyyy"
$HTTP = "HTTP.log.$UUID"
$HTTPS = "HTTPS.log.$UUID"

for (($i = 0); $i -lt 100; $i++)
{
    "i: $i"
    "HTTP Run Start"
    "`nRun: ${i}:" + (Get-Date | Out-String -Stream).Trim("`n") >> $HTTP
    & ".\networkQPatch2.exe" --config rpm.obs.cr --path config --port 4043 --extended-stats --http 2>&1 | Out-File -FilePath $HTTP -Append
    "HTTPS Run Start"
    "`nRun: ${i}:" + (Get-Date | Out-String -Stream).Trim("`n") >> $HTTPS
    & ".\networkQPatch2.exe" --config rpm.obs.cr --path config --port 4043 --extended-stats 2>&1 | Out-File -FilePath $HTTPS -Append
}