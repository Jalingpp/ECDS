package nodes

import (
	pb "ECDS/proto"
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"

	"github.com/Nik-U/pbc"
	"google.golang.org/grpc"
)

type StorageNode struct {
	SNId    string       //存储节点id
	SNAddr  string       //存储节点ip地址
	Pairing *pbc.Pairing //双线性映射
	// ClientPublicInfor               map[string]*util.PublicInfo           //客户端的公钥信息，key:clientID,value:公钥信息指针
	ClientFileMap                   map[string][]string                   //为客户端存储的文件列表，key:clientID,value:filename
	FileShardsMap                   map[string]map[string]*util.DataShard //文件的数据分片列表，key:clientID-filename,value:(key:分片序号)
	pb.UnimplementedSNServiceServer                                       // 嵌入匿名字段
}

// 新建存储分片
func NewStorageNode(snid string, snaddr string) *StorageNode {
	sn := &StorageNode{}
	sn.SNId = snid
	sn.SNAddr = snaddr
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	sn.Pairing = pairing
	sn.ClientFileMap = make(map[string][]string)
	sn.FileShardsMap = make(map[string]map[string]*util.DataShard)
	//设置监听地址
	lis, err := net.Listen("tcp", snaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSNServiceServer(s, sn)
	log.Println("Server listening on port 50051")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return sn
}

// 存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *StorageNode) PutDataShard(ctx context.Context, preq *pb.PutDSRequest) (*pb.PutDSResponse, error) {
	log.Printf("Received message from client: %s\n", preq.ClientId)
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	message := ""
	//1-验证分片序号是否与审计方通知的一致
	//2-验签
	//3-存放数据分片
	//3-1-提取数据分片对象
	ds, err := util.DeserializeDS(preq.DatashardSerialized)
	if err != nil {
		log.Println("Deserialize DataShard Error!")
		message = "Deserialize DataShard Error!"
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, err
	}
	//3-2-放置文件分片列表
	cid_fn := clientId + "-" + filename
	if sn.FileShardsMap[cid_fn] != nil {
		message = "Filename Already Exist!"
		e := errors.New("Filename Already Exist!")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	sn.FileShardsMap[cid_fn] = make(map[string]*util.DataShard)
	sn.FileShardsMap[cid_fn][dsno] = ds
	//3-3-放置客户端文件名列表
	if sn.ClientFileMap[clientId] == nil {
		sn.ClientFileMap[clientId] = make([]string, 0)
	}
	sn.ClientFileMap[clientId] = append(sn.ClientFileMap[clientId], filename)
	message = "Put File Success!"
	//4-告知审计方分片放置结果
	return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, nil
}

// 打印storage node
func (sn *StorageNode) PrintSN() {
	str := "StorageNode:{SNId:" + sn.SNId + ",SNAddr:" + sn.SNAddr + ",ClientFileMap:{"
	for key, value := range sn.ClientFileMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "},FileShardsMap:{"
	for key, value := range sn.FileShardsMap {
		str = str + key + ":{"
		for skey, _ := range value {
			str = str + skey + ","
		}
		str = str + "},"
	}
	str = str + "}}"
	log.Println(str)
}
