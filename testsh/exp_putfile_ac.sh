if [ -z $1 ]; then
    round=1
else
    round=$1
fi
dsnMode=ec
clientnum=50
putfile_ac="expgo/exp_putfile_ac.go"

go run $putfile_ac $dsnMode $clientnum
sleep