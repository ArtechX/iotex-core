// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided ‘as is’ and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/iotexproject/iotex-core/blockchain"
	"github.com/iotexproject/iotex-core/blocksync"
	"github.com/iotexproject/iotex-core/config"
	"github.com/iotexproject/iotex-core/consensus"
	"github.com/iotexproject/iotex-core/delegate"
	"github.com/iotexproject/iotex-core/iotxaddress"
	"github.com/iotexproject/iotex-core/network"
	pb "github.com/iotexproject/iotex-core/simulator/proto/simulator"
	"github.com/iotexproject/iotex-core/txpool"
)

const (
	port        = ":50051"
	rdposConfig = "./config_local_rdpos_sim.yaml"
)

// server is used to implement message.SimulatorServer.
type server struct {
	nodes []consensus.ConsensusSim
}

// Ping implements simulator.SimulatorServer
func (s *server) Init(ctx context.Context, in *pb.InitRequest) (*pb.Empty, error) {
	for i := 0; i < int(in.NPlayers); i++ {
		cfg, err := config.LoadConfigWithPathWithoutValidation(rdposConfig)
		if err != nil {
			glog.Error("Error loading config file")
		}

		// create public/private key pair and address
		chainID := make([]byte, 4)
		binary.LittleEndian.PutUint32(chainID, uint32(i))

		addr, err := iotxaddress.NewAddress(true, chainID)

		cfg.Chain.RawMinerAddr.PublicKey = hex.EncodeToString(addr.PublicKey)
		cfg.Chain.RawMinerAddr.PrivateKey = hex.EncodeToString(addr.PrivateKey)
		cfg.Chain.RawMinerAddr.RawAddress = addr.RawAddress

		// set chain database path
		cfg.Chain.ChainDBPath = "./chain" + strconv.Itoa(i) + ".db"

		bc := blockchain.CreateBlockchain(cfg, blockchain.Gen)
		tp := txpool.New(bc)

		overlay := network.NewOverlay(&cfg.Network)
		dlg := delegate.NewConfigBasedPool(&cfg.Delegate)
		bs := blocksync.NewBlockSyncer(cfg, bc, tp, overlay, dlg)

		node := consensus.NewConsensusSim(cfg, bc, tp, bs, dlg)

		node.Start()

		fmt.Printf("Node %d initialized and consensus engine started\n", i)

		s.nodes = append(s.nodes, node)
	}

	fmt.Printf("Simulator initialized with %d players\n", in.NPlayers)
	return &pb.Empty{}, nil
}

// Ping implements simulator.SimulatorServer
func (s *server) Ping(in *pb.Request, stream pb.Simulator_PingServer) error {
	msgValue, err := hex.DecodeString(in.Value)
	if err != nil {
		glog.Error("Could not decode message value into byte array")
	}

	msg := consensus.CombineMsg(in.InternalMsgType, msgValue)
	node := s.nodes[in.PlayerID]

	done := make(chan bool)
	node.SetStream(stream)

	node.HandleViewChange(msg, done)

	<-done
	return nil
}

func main() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSimulatorServer(s, &server{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
