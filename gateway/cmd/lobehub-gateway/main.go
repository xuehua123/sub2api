package main

import (
	"log"
	"net/http"

	"github.com/Wei-Shaw/sub2api/gateway"
)

func main() {
	cfg, err := gateway.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load gateway config: %v", err)
	}

	server, err := gateway.NewServer(cfg)
	if err != nil {
		log.Fatalf("create gateway server: %v", err)
	}

	log.Printf(
		"lobehub gateway listening on %s -> upstream=%s api=%s frontend=%s",
		cfg.ListenAddr,
		cfg.UpstreamURL,
		cfg.Sub2APIAPIBaseURL,
		cfg.Sub2APIFrontendURL,
	)
	if err := http.ListenAndServe(cfg.ListenAddr, server); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}
