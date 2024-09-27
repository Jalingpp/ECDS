if [ -z $1 ]; then
    dsnMode="ec"
else
    dsnMode=$1
fi

if [ -z $2 ]; then
    clientnum=1
else
    clientnum=$2
fi

putfile_ac="/home/ubuntu/ECDS/expgo/exp_putfile_ac.go"
datadir="/home/ubuntu/ECDS/data/"

go run $putfile_ac $dsnMode $clientnum $datadir