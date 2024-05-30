package pdp

import (
	"ECDS/util"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nik-U/pbc"
)

type POS struct {
	DSno      string
	DataProof []byte
	SigProof  []byte
}

type AggPOS struct {
	ClientFNDSno map[string]map[string][]string //clientId,filename,dsnolist
	DataProof    []byte
	SigProof     []byte
}

// 【存储节点执行】生成存储证明
func ProvePos(params string, gb []byte, pkb []byte, ds *util.DataShard, randoms int32) *POS {
	pairing, _ := pbc.NewPairingFromString(params)
	//生成data proof
	g := pairing.NewG1().SetBytes(gb)
	pk := pairing.NewG2().SetBytes(pkb)
	egpk := pairing.NewGT().Pair(g, pk)
	m := pairing.NewZr().SetInt32(MessageToInt32(ds.Data))
	ms := pairing.NewZr().MulInt32(m, randoms)
	dp := pairing.NewGT().PowZn(egpk, ms)
	//生成sig proof
	sig := pairing.NewG1().SetBytes(ds.Sig)
	randomsz := pairing.NewZr().SetInt32(randoms)
	sp := pairing.NewG1().PowZn(sig, randomsz)
	pos := POS{ds.DSno, dp.Bytes(), sp.Bytes()}
	return &pos
}

// 【存储节点执行】生成聚合存储证明，dssmap:clientId,filename,dsnolist
func ProveAggPos(params string, gb []byte, clientPKs map[string][]byte, dssmap map[string]map[string][]*util.DataShard, randoms int32) *AggPOS {
	pairing, _ := pbc.NewPairingFromString(params)
	clientFNDSno := make(map[string]map[string][]string)
	var dp *pbc.Element
	var sp *pbc.Element
	g := pairing.NewG1().SetBytes(gb)
	for clientid, fndssmap := range dssmap {
		pk := pairing.NewG2().SetBytes(clientPKs[clientid])
		egpk := pairing.NewGT().Pair(g, pk)
		for fn, dss := range fndssmap {
			if clientFNDSno[clientid] == nil {
				clientFNDSno[clientid] = make(map[string][]string)
			}
			if clientFNDSno[clientid][fn] == nil {
				clientFNDSno[clientid][fn] = make([]string, 0)
			}
			clientFNDSno[clientid][fn] = append(clientFNDSno[clientid][fn], dss[0].DSno)
			m := pairing.NewZr().SetInt32(MessageToInt32(dss[0].Data))
			ms := pairing.NewZr().MulInt32(m, randoms)
			if sp == nil {
				sig := pairing.NewG1().SetBytes(dss[0].Sig)
				randomsz := pairing.NewZr().SetInt32(randoms)
				sp = pairing.NewG1().PowZn(sig, randomsz)
			} else {
				sig := pairing.NewG1().SetBytes(dss[0].Sig)
				randomsz := pairing.NewZr().SetInt32(randoms)
				sp = pairing.NewG1().Mul(sp, pairing.NewG1().PowZn(sig, randomsz))
			}
			//遍历dss中每个ds，生成其dp和sp
			for i := 1; i < len(dss); i++ {
				m1 := pairing.NewZr().SetInt32(MessageToInt32(dss[i].Data))
				ms1 := pairing.NewZr().MulInt32(m1, randoms)
				ms = pairing.NewZr().Add(ms, ms1)
				clientFNDSno[clientid][fn] = append(clientFNDSno[clientid][fn], dss[i].DSno)
				sig := pairing.NewG1().SetBytes(dss[0].Sig)
				randomsz := pairing.NewZr().SetInt32(randoms)
				sp = pairing.NewG1().Mul(sp, pairing.NewG1().PowZn(sig, randomsz))
			}
			if dp == nil {
				dp = pairing.NewGT().PowZn(egpk, ms)
			} else {
				dp = pairing.NewGT().Mul(dp, pairing.NewGT().PowZn(egpk, ms))
			}
		}
	}
	return &AggPOS{clientFNDSno, dp.Bytes(), sp.Bytes()}
}

// 【客户端执行】验证存储证明
func VerifyPos(pos *POS, params string, gb []byte, pubkey []byte, v int32, t string, randoms int32) bool {
	pairing, _ := pbc.NewPairingFromString(params)
	vs := v * randoms
	h := pairing.NewG1().SetFromStringHash(t, sha256.New())
	vsz := pairing.NewZr().SetInt32(vs)
	vts := pairing.NewG1().PowZn(h, vsz)
	pk := pairing.NewG2().SetBytes(pubkey)
	evtspk := pairing.NewGT().Pair(vts, pk)
	g := pairing.NewG2().SetBytes(gb)
	sp := pairing.NewG1().SetBytes(pos.SigProof)
	espg := pairing.NewGT().Pair(sp, g)
	dp := pairing.NewGT().SetBytes(pos.DataProof)
	temp := pairing.NewGT().Mul(dp, evtspk)
	if !temp.Equals(espg) {
		// log.Println(pos.DSno, ":*BUG* pos check failed *BUG*")
		return false
	} else {
		// log.Println(pos.DSno, ":pos verified correctly")
		return true
	}
}

// 【审计方执行】验证聚合存储证明
func VerifyAggPos(aggpos *AggPOS, params string, gb []byte, clientPKs map[string][]byte, metamap map[string]*util.Meta4File, randoms int32) bool {
	pairing, _ := pbc.NewPairingFromString(params)
	g := pairing.NewG2().SetBytes(gb)
	//计算每个文件的htvs乘积
	var alehtvspk *pbc.Element
	for clientId, fndsnomap := range aggpos.ClientFNDSno {
		pk := pairing.NewG2().SetBytes(clientPKs[clientId])
		for fn, dsnolist := range fndsnomap {
			//计算一个文件对应分片的htvs
			var allhtvs *pbc.Element
			for i := 0; i < len(dsnolist); i++ {
				cid_fn := clientId + "-" + fn
				vs := pairing.NewZr().SetInt32(metamap[cid_fn].LatestVersionSlice[dsnolist[i]] * randoms)
				ht := pairing.NewG1().SetFromStringHash(metamap[cid_fn].LatestTimestampSlice[dsnolist[i]], sha256.New())
				htvs := pairing.NewG1().PowZn(ht, vs)
				if allhtvs == nil {
					allhtvs = htvs
				} else {
					allhtvs = pairing.NewG1().Mul(allhtvs, htvs)
				}
			}
			//累乘所有文件对应的ehtvspk
			if alehtvspk == nil {
				alehtvspk = pairing.NewGT().Pair(allhtvs, pk)
			} else {
				alehtvspk = pairing.NewGT().Mul(alehtvspk, pairing.NewGT().Pair(allhtvs, pk))
			}
		}
	}
	//计算等式左边
	dp := pairing.NewGT().SetBytes(aggpos.DataProof)
	dpalehtvspk := pairing.NewGT().Mul(dp, alehtvspk)
	//计算等式右边
	sp := pairing.NewG1().SetBytes(aggpos.SigProof)
	espg := pairing.NewGT().Pair(sp, g)
	if !dpalehtvspk.Equals(espg) {
		// log.Println(aggpos.ClientFNDSno, ":*BUG* pos check failed *BUG*")
		return false
	} else {
		// log.Println(aggpos.ClientFNDSno, ":pos verified correctly")
		return true
	}
}

func (pos *POS) Print() {
	fmt.Println("data proof:", pos.DataProof)
	fmt.Println("sig proof:", pos.SigProof)
}

// 序列化POS
func SerializePOS(pos *POS) []byte {
	jsonPOS, err := json.Marshal(pos)
	if err != nil {
		fmt.Printf("SerializePOS error: %v\n", err)
		return nil
	}
	return jsonPOS
}

// 反序列化POS
func DeserializePOS(sepos []byte) (*POS, error) {
	var pos POS
	if err := json.Unmarshal(sepos, &pos); err != nil {
		fmt.Printf("DeserializePOS error: %v\n", err)
		return nil, err
	}
	return &pos, nil
}

type SeAggPOS struct {
	ClientFNDSnoStr string //ClientFNDSnoStr转换后的string:clientId1-filename1-dsno1,dsno2;clientId2-filename2-dsno1,dsno2
	DataProof       []byte
	SigProof        []byte
}

// 序列化AggPOS
func SerializeAggPOS(aggpos *AggPOS) []byte {
	//根据aggpos构建seAggPOS
	cfndsnostr := ""
	for clientid, fnmap := range aggpos.ClientFNDSno {
		for fn, dsnolist := range fnmap {
			if cfndsnostr != "" {
				cfndsnostr = cfndsnostr + ";"
			}
			cfndsnostr = cfndsnostr + clientid + "," + fn + ","
			for i := 0; i < len(dsnolist); i++ {
				cfndsnostr = cfndsnostr + dsnolist[i]
				if i < len(dsnolist)-1 {
					cfndsnostr = cfndsnostr + "/"
				}
			}
		}
	}
	seaggpos := &SeAggPOS{cfndsnostr, aggpos.DataProof, aggpos.SigProof}
	jsonPOS, err := json.Marshal(seaggpos)
	if err != nil {
		fmt.Printf("SerializeAggPOS error: %v\n", err)
		return nil
	}
	return jsonPOS
}

// 反序列化AggPOS
func DeserializeAggPOS(seaggposb []byte) (*AggPOS, error) {
	var seaggpos SeAggPOS
	if err := json.Unmarshal(seaggposb, &seaggpos); err != nil {
		fmt.Printf("DeserializeAggPOS error: %v\n", err)
		return nil, err
	}
	//解析出ClientFNDSno map[string]map[string][]string：clientId,filename,dsnolist
	cfndsnomap := make(map[string]map[string][]string)
	cid_fn_List := strings.Split(seaggpos.ClientFNDSnoStr, ";")
	for i := 0; i < len(cid_fn_List); i++ {
		cid_fn_dsno := strings.Split(cid_fn_List[i], ",")
		if cfndsnomap[cid_fn_dsno[0]] == nil {
			cfndsnomap[cid_fn_dsno[0]] = make(map[string][]string)
		}
		dsno_list := strings.Split(cid_fn_dsno[2], "/")
		cfndsnomap[cid_fn_dsno[0]][cid_fn_dsno[1]] = dsno_list
	}
	aggpos := &AggPOS{cfndsnomap, seaggpos.DataProof, seaggpos.SigProof}
	return aggpos, nil
}
