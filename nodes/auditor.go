package nodes

import (
	"ECDS/encode"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
)

type Auditor struct {
	IpAddr                           string                       //审计方的IP地址
	SNAddrMap                        map[string]string            //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientFileDataShardsMap          map[string][]string          //客户端文件的数据分片所在的存储节点表，key:客户端id-文件名，value:存储节点address列表
	ClientFileParityShardsMap        map[string][]string          //客户端文件的校验分片所在的存储节点表，key:客户端id-文件名，value:存储节点address列表
	MetaFileMap                      map[string]*encode.Meta4File // 用于记录每个文件的元数据,key:客户端id-文件名
	DataNum                          int                          //系统中用于文件编码的数据块数量
	ParityNum                        int                          //系统中用于文件编码的校验块数量
	PendingClientFileDataShardsMap   map[string][]string          //已进入Setup阶段的客户端文件数据分片所在的存储节点表
	PendingClientFileParityShardsMap map[string][]string          //已进入Setup阶段的客户端文件校验分片所在的存储节点表
	pb.UnimplementedACServiceServer                               // 嵌入匿名字段
}

// 新建一个审计方，持续监听消息
func NewAuditor(ipaddr string, snaddrfilename string, dn int, pn int) *Auditor {
	snaddrmap := make(map[string]string)
	//读取存储节点地址
	reader := util.BufIOReader(snaddrfilename)
	// 逐行读取文件内容
	for {
		// 读取一行数据
		line, err := reader.ReadString('\n')
		if err != nil {
			break // 文件读取结束或者发生错误时退出循环
		}
		// 处理一行数据
		fmt.Print("ReadSNAddr:", line)
		//写入map
		lineslice := strings.Split(strings.TrimRight(line, "\n"), ",") //去除换行符
		snaddrmap[lineslice[0]] = lineslice[1]
	}
	cfdsmap := make(map[string][]string)
	cfpsmap := make(map[string][]string)
	metaFileMap := make(map[string]*encode.Meta4File)
	pcfdsmap := make(map[string][]string)
	pcfpsmap := make(map[string][]string)

	auditor := &Auditor{ipaddr, snaddrmap, cfdsmap, cfpsmap, metaFileMap, dn, pn, pcfdsmap, pcfpsmap, pb.UnimplementedACServiceServer{}}

	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterACServiceServer(s, auditor)
	log.Println("Server listening on port 50051")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return auditor
}

// 选择存储节点，给客户端回复消息
func (ac *Auditor) SelectSNs(ctx context.Context, sreq *pb.StorageRequest) (*pb.StorageResponse, error) {
	log.Printf("Received message from client: %s\n", sreq.ClientId)
	// 为该客户端文件随机选择存储节点
	dssnaddrs, pssnaddrs := ac.SelectStorNodes(sreq.Filename)
	//将选出来的节点加入等待列表
	cid_fn := sreq.ClientId + "-" + sreq.Filename
	ac.PendingClientFileDataShardsMap[cid_fn] = dssnaddrs
	ac.PendingClientFileParityShardsMap[cid_fn] = pssnaddrs
	return &pb.StorageResponse{Filename: sreq.Filename, SnsForDs: dssnaddrs, SnsForPs: pssnaddrs}, nil
}

// AC选择存储节点
func (ac *Auditor) SelectStorNodes(filename string) ([]string, []string) {
	dssnaddrs := make([]string, 0)
	pssnaddrs := make([]string, 0)
	seed := time.Now().UnixNano()
	randor := rand.New(rand.NewSource(seed))
	selectednum := make(map[string]bool)
	for {
		randomNum := randor.Intn(ac.DataNum+ac.ParityNum) + 1 // 生成 1 到 (ac.DataNum+ac.ParityNum) 之间的随机数
		selectednum[strconv.Itoa(randomNum)] = true
		if len(selectednum) == ac.DataNum {
			break
		}
	}
	for i := 1; i < ac.DataNum+ac.ParityNum+1; i++ {
		if selectednum[strconv.Itoa(i)] {
			dssnaddrs = append(dssnaddrs, "sn"+strconv.Itoa(i))
		} else {
			pssnaddrs = append(pssnaddrs, "sn"+strconv.Itoa(i))
		}
	}
	return dssnaddrs, pssnaddrs
}

// 打印auditor
func (auditor *Auditor) PrintAuditor() {
	str := "Auditor:{IpAddr:" + auditor.IpAddr + ",SNAddrMap:{"
	for key, value := range auditor.SNAddrMap {
		str = str + key + ":" + value + ","
	}
	str = str + "},ClientFileDataShardMap:{"
	for key, value := range auditor.ClientFileDataShardsMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "},ClientFileParityShardsMap:{"
	for key, value := range auditor.ClientFileParityShardsMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "},DataNum:" + strconv.Itoa(auditor.DataNum) + ",ParityNum:" + strconv.Itoa(auditor.ParityNum) + ",PendingClientFileDataShardsMap:{"
	for key, value := range auditor.PendingClientFileDataShardsMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "},PendingClientFileParityShardsMap:{"
	for key, value := range auditor.PendingClientFileParityShardsMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "}}"
	log.Println(str)
}
