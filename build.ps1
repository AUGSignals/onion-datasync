$ENV:GOOS="linux"
$ENV:GOARCH="mipsle"
$date=Get-Date -Format "yyyyMMddHHmm"
$builddate=Get-Date -Format "yyyy-MM-ddTHH:mm:ss"
$filename="datasync_" + $date
$Version="v0.1.0"
$tags="json1"
echo $builddate
echo $filename
echo $Version
go build -o $filename -ldflags="-s -w -X main.BuildDate="$builddate" -X main.Version="$Version --tags $tags