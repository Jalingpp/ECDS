if [ -z $1 ]; then
    round=1
else
    round=$1
fi

CLIENT_IP="10.0.4.19"

snNum=31
snmNum=1

dsnModes=(ec storj sia filecoin)
clientnums=(50 150 250 350 450)

SCRIPT_NAME_AC=exp_ac.sh
SCRIPT_NAME_Client=exp_client.sh
SCRIPT_NAME_SN=exp_sn.sh

EndKeyword_AC=exp_ac
EndKeyword_Client=client
EndKeyword_SN=exp_sn

# datafiledirs=("/home/ubuntu/ECDS/data/NM/")
datafiledirs=("/root/DSN/ECDS/data/NM/")
datafilenums=(6234)
datasetNum=1

# 遍历每个方法
for dsnMode in ${dsnModes[*]};
do
    # 遍历每种ClientNum
    for clientnum in ${clientnums[*]};
    do
        # 遍历每个数据集
        for ((j=0;j<$datasetNum;j++))
        do
            datafiledir=${datafiledirs[$j]}
            datafilenum=${datafilenums[$j]}

            # 启动SNs
            bash startSNs.sh $snNum $SCRIPT_NAME_SN $dsnMode
            sleep 1
            echo "********SNs already started*********"

            # 启动AC
            bash startAC.sh $SCRIPT_NAME_AC $dsnMode $clientnum
            sleep 10
            echo "********AC already started*********"

            # 启动Client
            bash startClient.sh $CLIENT_IP $SCRIPT_NAME_Client $dsnMode $clientnum $datafiledir $datafilenum
            sleep 1
            echo "********Client already done*********"

            bash endClient.sh $CLIENT_IP $EndKeyword_Client
            sleep 1

            bash endAC.sh $EndKeyword_AC $EndKeyword_SN $EndKeyword_Client
            sleep 1

            bash endSNs.sh $snmNum $EndKeyword_SN
            sleep 1

            endkw="ECDS"
            bash endSNs.sh $snmNum $endkw
            sleep 1

        done
    done
done