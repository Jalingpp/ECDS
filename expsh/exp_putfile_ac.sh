if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnModes=(ec storj sia filecoin)
dsnMode=${dsnModes[2]}
clientnums=(1 50 100 150 200 250)
clientnum=${clientnums[2]}
putfile_ac="/root/DSN/ECDS/expgo/exp_putfile_ac.go"
datadir="/root/DSN/ECDS/data/"

go run $putfile_ac $dsnMode $clientnum $datadir