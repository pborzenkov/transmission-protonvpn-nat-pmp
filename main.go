// Package main implements transmission-protonvpn-nat-pmp
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/pborzenkov/go-transmission/transmission"
)

var (
	verbose         = flag.Bool("verbose", false, "Enable verbose logging")
	transmissionURL = flag.String("transmission.url", "", "Transmission RPC server URL")
	gatewayIP       = flag.String("gateway.ip", "", "IP address of NAT-PMP gateway")
	period          = flag.Duration("period", 60*time.Second, "Port refresh period")
)

func debug(format string, args ...any) {
	if !*verbose {
		return
	}

	log.Printf("DEBUG: %s", fmt.Sprintf(format, args...))
}

func main() {
	flag.Parse()

	if *transmissionURL == "" {
		log.Fatal("-transmission.url flag is required")
	}
	if *gatewayIP == "" {
		log.Fatal("-gateway.ip flag is required")
	}

	gateway := net.ParseIP(*gatewayIP)
	if gateway == nil {
		log.Fatalf("-gateway.ip is not an IP address")
	}

	var options []transmission.Option
	if strings.HasPrefix(*transmissionURL, "unix://") {
		sock := strings.TrimPrefix(*transmissionURL, "unix://")
		*transmissionURL = "http://localhost"
		options = append(options, transmission.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sock)
				},
			},
		}))
	}
	trans, err := transmission.New(*transmissionURL, options...)
	if err != nil {
		log.Fatalf("failed to create transmission client: %v", err)
	}

	nat := natpmp.NewClient(gateway)

	run(trans, nat, *period)
}

func run(trans *transmission.Client, nat *natpmp.Client, period time.Duration) {
	ticker := time.NewTicker(period / 3)

	for {
		if err := runOnce(trans, nat, period); err != nil {
			log.Printf("failed to map ports: %v", err)
		}

		<-ticker.C
	}
}

func runOnce(trans *transmission.Client, nat *natpmp.Client, period time.Duration) error {
	tcp, err := nat.AddPortMapping("tcp", 0, 0, int(period.Seconds()))
	if err != nil {
		return fmt.Errorf("failed to request TCP mapping: %v", err)
	}
	debug("Got TCP port %v -> %v", tcp.MappedExternalPort, tcp.InternalPort)

	udp, err := nat.AddPortMapping("udp", 0, 0, int(period.Seconds()))
	if err != nil {
		return fmt.Errorf("failed to request UDP mapping: %v", err)
	}
	debug("Got UDP port %v -> %v", udp.MappedExternalPort, udp.InternalPort)

	if tcp.InternalPort != tcp.MappedExternalPort {
		return fmt.Errorf("TCP internal (%v) and external (%v) ports do not match", tcp.InternalPort, tcp.MappedExternalPort)
	}
	if udp.InternalPort != udp.MappedExternalPort {
		return fmt.Errorf("UDP internal (%v) and external (%v) port do not match", udp.InternalPort, udp.MappedExternalPort)
	}

	if tcp.InternalPort != udp.InternalPort {
		log.Printf("WARN: TCP (%v) and UDP (%v) ports do not match, using TCP", tcp.InternalPort, udp.InternalPort)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sess, err := trans.GetSession(ctx, transmission.SessionFieldPeerPort)
	if err != nil {
		return fmt.Errorf("failed to get peer port from Transmission: %v", err)
	}
	debug("Transmission peer port: %v", sess.PeerPort)

	if sess.PeerPort != int(tcp.InternalPort) {
		log.Printf("Transmission peer port (%v) does not match TCP port (%v), reconfiguring", sess.PeerPort, tcp.InternalPort)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := trans.SetSession(ctx, &transmission.SetSessionReq{
			PeerPort: transmission.OptInt(int(tcp.InternalPort)),
		}); err != nil {
			return fmt.Errorf("failed to set peer port in Transmission: %v", err)
		}
	}

	return nil
}
