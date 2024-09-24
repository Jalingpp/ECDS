if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnModes=(ec storj sia filecoin)
dsnMode=${dsnModes[2]}
putfile_sn="/root/DSN/ECDS/expgo/exp_putfile_sn.go"
datadir="/root/DSN/ECDS/data/"

go run $putfile_sn $dsnMode $datadir