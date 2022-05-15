package inspect

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cloudwebrtc/nats-discovery/pkg/discovery"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	log "github.com/pion/ion-log"
	"github.com/pion/ion/pkg/ion"
	"github.com/pion/ion/pkg/proto"
	"github.com/pion/ion/pkg/util"
	pb "github.com/pion/ion/proto/inspect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type global struct {
	Dc string `mapstructure:"dc"`
}

type logConf struct {
	Level string `mapstructure:"level"`
}

type natsConf struct {
	URL string `mapstructure:"url"`
}

type svcConf struct {
	Port int `mapstructure:"port"`
}

// Config for biz node
type Config struct {
	Global global   `mapstructure:"global"`
	Log    logConf  `mapstructure:"log"`
	Nats   natsConf `mapstructure:"nats"`
	Svc    svcConf  `mpastructure:"svc"`
}

type Inspect struct {
	ion.Node
	conf Config
	pb.UnimplementedGreeterServer
}

func NewInspect(conf Config) (*Inspect, error) {
	return &Inspect{
		conf: conf,
		Node: ion.NewNode("inspect-" + util.RandomString(6)),
	}, nil
}

func (i *Inspect) Start() error {
	log.Infof("i.Node.Start node=%+v", i.conf.Nats.URL)
	err := i.Node.Start(i.conf.Nats.URL)
	if err != nil {
		log.Errorf("i.Node.Start error err=%+v", err)
		i.Close()
		return err
	}

	node := discovery.Node{
		DC:      i.conf.Global.Dc,
		Service: proto.ServiceINSPECT,
		NID:     i.Node.NID,
		RPC: discovery.RPC{
			Protocol: discovery.NGRPC,
			Addr:     i.conf.Nats.URL,
			//Params:   map[string]string{"username": "foo", "password": "bar"},
		},
	}

	go func() {
		log.Infof("KeepAlive node=%+v", node)
		err := i.Node.KeepAlive(node)
		if err != nil {
			log.Errorf("sig.Node.KeepAlive(%v) error %v", i.Node.NID, err)
		}
	}()

	go func() {
		err := i.Node.Watch(proto.ServiceALL)
		if err != nil {
			log.Errorf("Node.Watch(proto.ServiceALL) error %v", err)
		}
	}()

	return nil
}

func (i *Inspect) Serve() error {
	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, i)

	wrappedServer := grpcweb.WrapServer(grpcServer)
	handler := func(resp http.ResponseWriter, req *http.Request) {
		log.Infof("handle http requst")
		wrappedServer.ServeHTTP(resp, req)
	}

	httpServer := http.Server{
		Addr:    fmt.Sprintf(":%d", i.conf.Svc.Port),
		Handler: http.HandlerFunc(handler),
	}

	log.Infof("start Inspect grpc-web Server, listen on %d", i.conf.Svc.Port)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Infof("http listen and serve failed, %v", err)
		return err
	}
	return nil
}

func (i *Inspect) Close() {
	i.Node.Close()
}

func (i *Inspect) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Infof("Received: %v", in.GetName())
	grpc.SendHeader(ctx, metadata.Pairs("Pre-Response-Metadata", "Is-sent-as-headers-unary"))
	grpc.SetTrailer(ctx, metadata.Pairs("Post-Response-Metadata", "Is-sent-as-trailers-unary"))
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}
