if [ -z $1 ]; then
    dsnMode="ec"
else
    dsnMode=$1
fi

GOPath="/home/ubuntu/ECDS/expgo/"
GOFile="exp_putfile_sn.go"
datadir="/home/ubuntu/ECDS/data/"

cd $GOPath

nohup /usr/local/go/bin/go run $GOFile $dsnMode $datadir > "$datadir/output_sn.log" 2>&1 &
sleep 1