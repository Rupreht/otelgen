package traces

import (
	"flag"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Config struct {
	WorkerCount      int
	NumTraces        int
	PropagateContext bool
	Rate             int64
	TotalDuration    time.Duration
	ServiceName      string

	// OTLP config
	Endpoint string
	Insecure bool
	UseHTTP  bool
	Headers  HeaderValue
}

type HeaderValue map[string]string

var _ flag.Value = (*HeaderValue)(nil)

func (v *HeaderValue) String() string {
	return ""
}

func (v *HeaderValue) Set(s string) error {
	kv := strings.SplitN(s, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("value should be of the format key=value")
	}
	(*v)[kv[0]] = kv[1]
	return nil
}

// Flags registers config flags.
// func (c *Config) Flags(fs *flag.FlagSet) {
// 	fs.IntVar(&c.WorkerCount, "workers", 1, "Number of workers (goroutines) to run")
// 	fs.IntVar(&c.NumTraces, "traces", 1, "Number of traces to generate in each worker (ignored if duration is provided")
// 	fs.BoolVar(&c.PropagateContext, "marshal", false, "Whether to marshal trace context via HTTP headers")
// 	fs.Int64Var(&c.Rate, "rate", 0, "Approximately how many traces per second each worker should generate. Zero means no throttling.")
// 	fs.DurationVar(&c.TotalDuration, "duration", 0, "For how long to run the test")
// 	fs.StringVar(&c.ServiceName, "service", "tracegen", "Service name to use")

// 	// unfortunately, at this moment, the otel-go client doesn't support configuring OTLP via env vars
// 	fs.StringVar(&c.Endpoint, "otlp-endpoint", "localhost:4317", "Target to which the exporter is going to send spans or metrics. This MAY be configured to include a path (e.g. example.com/v1/traces)")
// 	fs.BoolVar(&c.Insecure, "otlp-insecure", false, "Whether to enable client transport security for the exporter's grpc or http connection")
// 	fs.BoolVar(&c.UseHTTP, "otlp-http", false, "Whether to use HTTP exporter rather than a gRPC one")

// 	// custom headers
// 	c.Headers = make(map[string]string)
// 	fs.Var(&c.Headers, "otlp-header", "Custom header to be passed along with each OTLP request. The value is expected in the format key=value."+
// 		"Flag may be repeated to set multiple headers (e.g -otlp-header key1=value1 -otlp-header key2=value2)")
// }

// Run executes the test scenario.
func Run(c *Config, logger *zap.Logger) error {
	if c.TotalDuration > 0 {
		c.NumTraces = 0
	} else if c.NumTraces <= 0 {
		return fmt.Errorf("either `traces` or `duration` must be greater than 0")
	}

	limit := rate.Limit(c.Rate)
	if c.Rate == 0 {
		limit = rate.Inf
		logger.Info("generation of traces isn't being throttled")
	} else {
		logger.Info("generation of traces is limited", zap.Float64("per-second", float64(limit)))
	}

	wg := sync.WaitGroup{}
	running := atomic.NewBool(true)

	for i := 0; i < c.WorkerCount; i++ {
		wg.Add(1)
		w := worker{
			numTraces:        c.NumTraces,
			propagateContext: c.PropagateContext,
			limitPerSecond:   limit,
			totalDuration:    c.TotalDuration,
			running:          running,
			wg:               &wg,
			logger:           logger.With(zap.Int("worker", i)),
		}

		go w.simulateTraces(c.ServiceName)
	}
	if c.TotalDuration > 0 {
		time.Sleep(c.TotalDuration)
		running.Store(false)
	}
	wg.Wait()
	return nil
}
