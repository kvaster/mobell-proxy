package main

import (
	"flag"
	"mobell-proxy/log"
	"mobell-proxy/mobell"
	"net"
	"os"
	"os/signal"
	"syscall"
)

var listenAddr = flag.String("listen.addr", ":8080", "listen address and port")
var mobotixAddr = flag.String("mobotix.addr", "", "mobotix camera address (ip:port)")
var mobotixUser = flag.String("mobotix.user", "", "mobotix camera user")
var mobotixPass = flag.String("mobotix.pass", "", "mobotix camera password")
var iface = flag.String("iface", "", "interface name for mac address detection")

func main() {
	flag.Parse()

	log.Info("starting mobell proxy")

	var hwAddr net.HardwareAddr
	ifs, _ := net.Interfaces()
	if *iface == "" {
		for _, iv := range ifs {
			if iv.HardwareAddr != nil {
				hwAddr = iv.HardwareAddr
				break
			}
		}
	} else {
		for _, iv := range ifs {
			if iv.Name == *iface {
				hwAddr = iv.HardwareAddr
				break
			}
		}
	}

	if hwAddr == nil {
		log.Error("can't detect mac address")
		os.Exit(1)
	}

	mac := hwAddr.String()

	if *mobotixAddr == "" {
		log.Error("-mobotix.addr is not povided")
		os.Exit(1)
	}

	s := mobell.New(*listenAddr, *mobotixAddr, *mobotixUser, *mobotixPass, mac)

	if err := s.Start(); err != nil {
		log.WithError(err).Error("error starting mobell proxy")
	}

	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-stopChan

	log.Info("stopping mobell proxy")
	s.Stop()

	log.Info("stopped mobell proxy")
}
