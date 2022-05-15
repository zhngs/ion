package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/pion/ion-log"
	"github.com/pion/ion/pkg/node/inspect"
	"github.com/spf13/viper"
)

var (
	file string
	conf inspect.Config
)

func showHelp() {
	fmt.Printf("Usage:%s {params}\n", os.Args[0])
	fmt.Println("      -c {config file}")
	fmt.Println("      -h (show help info)")
}

func loadfile() bool {
	_, err := os.Stat(file)
	if err != nil {
		return false
	}

	viper.SetConfigFile(file)
	viper.SetConfigType("toml")

	err = viper.ReadInConfig()
	if err != nil {
		fmt.Printf("config file %s read failed. %v\n", file, err)
		return false
	}

	if err := viper.Unmarshal(&conf); err != nil {
		fmt.Printf("config file %s loaded failed. %v\n", file, err)
		return false
	}

	fmt.Printf("config %s load ok!\n", file)
	return true
}

func parse() bool {
	flag.StringVar(&file, "c", "configs/inspect.toml", "config file")
	help := flag.Bool("h", false, "help info")
	flag.Parse()
	if !loadfile() {
		return false
	}

	if *help {
		showHelp()
		return false
	}
	return true
}

func main() {
	if !parse() {
		showHelp()
		os.Exit(-1)
	}

	log.Init(conf.Log.Level)
	log.Infof("--- Starting Inspect Server ---")

	insp, err := inspect.NewInspect(conf)
	if err != nil {
		log.Infof("NewInspect failed, %v", err)
		os.Exit(-1)
	}

	err = insp.Start()
	if err != nil {
		log.Infof("inpect start failed, %v", err)
		os.Exit(-1)
	}
	defer insp.Close()

	resp, err := insp.GetAllNode()
	if err != nil {
		log.Infof("getallnode faild")
		os.Exit(-1)
	}
	log.Infof("getallnode %v", resp.Nodes)

	insp.Serve()
}
