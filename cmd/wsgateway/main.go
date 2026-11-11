package main

import (
	"flag"
	"log"
	"net/http"

	"wonderland-gonline/internal/wsgateway"
)

func main() {
	listenFlag := flag.String("listen", "127.0.0.1:8090", "Listen address")
	targetFlag := flag.String("target", "127.0.0.1:6414", "Default target address")
	allowRemoteFlag := flag.Bool("allow-remote", false, "Allow listening on non-loopback addresses")
	webFlag := flag.String("web", "", "Optional directory of static files (page, wasm, asset bundles) served at /")
	flag.Parse()

	if !*allowRemoteFlag && !wsgateway.IsLoopback(*listenFlag) {
		log.Fatalf("refusing to listen on non-loopback address %s without -allow-remote", *listenFlag)
	}

	if err := wsgateway.DefaultValidateTarget(*targetFlag); err != nil {
		log.Fatalf("invalid default target %s: %v", *targetFlag, err)
	}

	cfg := wsgateway.Config{
		DefaultTarget:  *targetFlag,
		ValidateTarget: wsgateway.DefaultValidateTarget,
		WebDir:         *webFlag,
	}

	handler := wsgateway.NewGatewayHandler(cfg)
	server := &http.Server{
		Addr:    *listenFlag,
		Handler: handler,
	}

	log.Printf("Starting wsgateway on %s (default target: %s)", *listenFlag, *targetFlag)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
