package nodes

import (
	"ECDS/encode"
	"ECDS/pdp"
	"log"

	pb "ECDS/proto" // 根据实际路径修改

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	ClientID   string              //客户端ID
	Filecoder  *encode.FileCoder   //文件编码器，内含每个文件的<文件名,元信息>表
	Sigger     *pdp.Signature      //客户端的签名器
	AuditorRPC *pb.ACServiceClient //审计方RPC对象，用于调用审计方的方法
}

// 新建客户端，dn和pn分别是数据块和校验块的数量
func NewClient(id string, dn int, pn int, addr string) *Client {
	filecode := encode.NewFileCoder(dn, pn)
	sigger := pdp.NewSig()
	// 设置连接服务器的地址
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewACServiceClient(conn)
	return &Client{id, filecode, sigger, &c}
}
