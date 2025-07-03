package cmd

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
)

// SetupPprof sets up pprof HTTP server for profiling
func SetupPprof() {
	if os.Getenv("ENABLE_PPROF") == "true" {
		fmt.Println("🔬 Starting pprof server on :6060")
		fmt.Println("📊 Available endpoints:")
		fmt.Println("   • http://localhost:6060/debug/pprof/")
		fmt.Println("   • http://localhost:6060/debug/pprof/heap")
		fmt.Println("   • http://localhost:6060/debug/pprof/goroutine")
		fmt.Println("   • http://localhost:6060/debug/pprof/profile")
		fmt.Println("   • http://localhost:6060/debug/pprof/trace")

		go func() {
			http.ListenAndServe(":6060", nil)
		}()
	}
}
