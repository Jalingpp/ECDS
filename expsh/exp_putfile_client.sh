if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnModes=(ec storj sia filecoin)
dsnMode=${dsnModes[2]}
filenum=1000
filedir="/root/DSN/ECDS/data/NM/"
clientnums=(1 50 100 150 200 250)
clientnum=${clientnums[2]}
putfile_client="/root/DSN/ECDS/expgo/exp_putfile_client.go"
datadir="/root/DSN/ECDS/data/"

go run $putfile_client $dsnMode $filedir $clientnum $filenum $datadir