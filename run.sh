if [ -z $1 ]; then
    round=1
else
    round=$1
fi

CLIENT_IP="10.24.0.62"

snNum=31
snmNum=3

SCRIPT_NAME_AC=(exp_putfile_ac.sh)
SCRIPT_NAME_Client=(exp_putfile_client.sh)
SCRIPT_NAME_SN=(exp_putfile_sn.sh)
SCRIPT_NUME=1
EndKeyword_AC=(exp_putfile_ac)
EndKeyword_Client=(exp_putfile_client)
EndKeyword_SN=(exp_putfile_sn)

dsnModes=(ec storj sia filecoin)
clientnums=(50 150 250 350 450)

datafiledirs=("/home/ubuntu/ECDS/data/NM/")
datafilenums=(6234)
datasetNum=1

# 遍历每组脚本
for (( i=0; i<$SCRIPT_NUME; i++ ))
do
    SCRIPT_AC=${SCRIPT_NAME_AC[$i]}
    SCRIPT_Client=${SCRIPT_NAME_Client[$i]}
    SCRIPT_SN=${SCRIPT_NAME_SN[$i]}
    EndKW_AC=${EndKeyword_AC[$i]}
    EndKW_Client=${EndKeyword_Client[$i]}
    EndKW_SN=${EndKeyword_SN[$i]}
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
                bash startSNs.sh $snNum $SCRIPT_SN $dsnMode
                sleep 1
                echo "********SNs already started*********"

                # 启动AC
                bash startAC.sh $SCRIPT_AC $dsnMode $clientnum
                sleep 10
                echo "********AC already started*********"

                # 启动Client
                bash startClient.sh $CLIENT_IP $SCRIPT_Client $dsnMode $clientnum $datafiledir $datafilenum
                sleep 1

                bash endClient.sh $CLIENT_IP $EndKeyword_Client
                sleep 1

                bash endAC.sh $EndKW_AC $EndKW_SN $EndKW_Client
                sleep 1

                bash endSNs.sh $snmNum $EndKW_SN
                sleep 1

            done
        done
    done
done