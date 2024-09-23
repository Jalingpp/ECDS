if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnMode=ec
filenum=6234
filedir="data/NM/"
clientnum=50
putfile_client="expgo/exp_putfile_client.go"

go run $putfile_client $dsnMode $filedir $clientnum $filenum
sleep