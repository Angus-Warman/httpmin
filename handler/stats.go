package handler

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/Angus-Warman/httpmin/response"
)

// Gets server start time, uptime, memory usage and other metrics
func Stats() http.Handler {
	startTime := time.Now()
	host, err := os.Hostname()

	if err != nil {
		host = "(unknown)"
	}

	fn := func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		uptimeNS := now.Sub(startTime)
		uptimeMS := uptimeNS / time.Millisecond
		uptime := uptimeNS.Round(time.Second).String()

		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		data := map[string]any{
			"startTime":  startTime,
			"uptimeMS":   uptimeMS,
			"uptime":     uptime,
			"memAllocMB": mem.Alloc / 1024 / 1024, // currently allocated
			"memSysMB":   mem.Sys / 1024 / 1024,   // total from OS
			"routinues":  runtime.NumGoroutine(),
			"host":       host,
		}

		response.JSON(w, data)
	}

	return http.HandlerFunc(fn)
}
