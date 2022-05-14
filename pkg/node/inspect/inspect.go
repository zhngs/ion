package inspect

import (
	"github.com/cloudwebrtc/nats-discovery/pkg/discovery"
	log "github.com/pion/ion-log"
	"github.com/pion/ion/pkg/ion"
	"github.com/pion/ion/pkg/proto"
	"github.com/pion/ion/pkg/util"
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

// Config for biz node
type Config struct {
	Global global   `mapstructure:"global"`
	Log    logConf  `mapstructure:"log"`
	Nats   natsConf `mapstructure:"nats"`
}

type Inspect struct {
	ion.Node
	conf Config
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

func (i *Inspect) Close() {
	i.Node.Close()
}
