package metrics

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
)

func InitPprof() {
	go func() {
		log.Println("\nRunning pprof!")
		log.Println("\nOpen http://localhost:6060/debug/pprof/ in your browser")
		fmt.Println(http.ListenAndServe(":6060", nil)) //nolint:gosec
	}()
}
