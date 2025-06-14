module ECDS

go 1.23.0

toolchain go1.23.9

require (
	github.com/Nik-U/pbc v0.0.0-20181205041846-3e516ca0c5d6
	github.com/filecoin-project/filecoin-ffi v0.0.0-00010101000000-000000000000
	github.com/filecoin-project/go-padreader v0.0.1
	github.com/filecoin-project/go-state-types v0.16.0
	github.com/hashicorp/golang-lru v1.0.2
	github.com/ipfs/go-cid v0.5.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	gonum.org/v1/gonum v0.15.0
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
)

require (
	github.com/filecoin-project/go-address v1.2.0 // indirect
	github.com/filecoin-project/go-fil-commcid v0.2.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/ipfs/bbloom v0.0.4 // indirect
	github.com/ipfs/boxo v0.20.0 // indirect
	github.com/ipfs/go-block-format v0.2.0 // indirect
	github.com/ipfs/go-datastore v0.6.0 // indirect
	github.com/ipfs/go-ipfs-util v0.0.3 // indirect
	github.com/ipfs/go-ipld-cbor v0.2.0 // indirect
	github.com/ipfs/go-ipld-format v0.6.0 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/ipfs/go-metrics-interface v0.0.1 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/polydawn/refmt v0.89.0 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/whyrusleeping/cbor-gen v0.3.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250519155744-55703ea1f237 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace google.golang.org/grpc => google.golang.org/grpc v1.42.0

replace google.golang.org/protobuf => google.golang.org/protobuf v1.36.6
