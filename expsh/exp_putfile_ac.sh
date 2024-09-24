if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnModes=(ec storj sia filecoin)
dsnMode=${dsnModes[3]}
clientnums=(1 50 100 150 200 250)
clientnum=${clientnums[0]}
putfile_ac="/root/DSN/ECDS/expgo/exp_putfile_ac.go"

go run $putfile_ac $dsnMode $clientnum