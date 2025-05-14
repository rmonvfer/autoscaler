// Copyright (C) 2025 Ramón Vila Ferreres
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

const endpoint = "https://backboard.railway.com/graphql/v2"

// GraphQL payload wrappers
type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type metricPoint struct {
	Cpu float64 `json:"cpuPercent"`
}

type instanceMetrics struct {
	Metrics []metricPoint `json:"metrics"`
}

// minimal pieces of the response payload
// service(id) { instances { metrics { cpuPercent } } replicas }
type serviceData struct {
	Instances []instanceMetrics `json:"instances"`
	Replicas  int               `json:"replicas"`
}

type serviceResp struct {
	Service serviceData `json:"service"`
}

type gqlResponse struct {
	Data   serviceResp                `json:"data"`
	Errors []struct{ Message string } `json:"errors"`
}

// CPU metrics snapshot
type snapshot struct {
	avgCPU   float64
	replicas int
}

// Configuration
type config struct {
	Token     string
	ServiceID string
	High, Low float64
	Min, Max  int
	Cooldown  time.Duration
	Interval  time.Duration
}

func loadConfig() config {
	must := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			log.Fatalf("missing env %s", key)
		}
		return v
	}
	parseF := func(k string, def float64) float64 {
		if v := os.Getenv(k); v != "" {
			f, _ := strconv.ParseFloat(v, 64)
			return f
		}
		return def
	}
	parseI := func(k string, def int) int {
		if v := os.Getenv(k); v != "" {
			i, _ := strconv.Atoi(v)
			return i
		}
		return def
	}
	parseDur := func(k string, def time.Duration) time.Duration {
		if v := os.Getenv(k); v != "" {
			d, _ := time.ParseDuration(v)
			return d
		}
		return def
	}
	return config{
		Token:     must("RAILWAY_TOKEN"),
		ServiceID: must("SERVICE_ID"),
		High:      parseF("CPU_HIGH", 75),
		Low:       parseF("CPU_LOW", 30),
		Min:       parseI("MIN_REPLICAS", 1),
		Max:       parseI("MAX_REPLICAS", 5),
		Cooldown:  parseDur("COOLDOWN", 2*time.Minute),
		Interval:  parseDur("POLL_INTERVAL", 30*time.Second),
	}
}

func main() {
	cfg := loadConfig()
	ctx := context.Background()
	lastScale := time.Now().Add(-cfg.Cooldown)

	for {
		target := fetch(ctx, cfg)
		desired := decide(target.avgCPU, target.replicas, cfg)

		if desired != target.replicas && time.Since(lastScale) > cfg.Cooldown {
			err := scale(ctx, cfg, desired)
			if err == nil {
				lastScale = time.Now()
			} else {
				log.Printf("scale error: %v", err)
			}
		}
		time.Sleep(cfg.Interval)
	}
}

func fetch(ctx context.Context, cfg config) snapshot {
	now := time.Now()
	from := now.Add(-2 * cfg.Interval).Format(time.RFC3339)
	to := now.Format(time.RFC3339)

	query := `query($id:String!,$from:Time!,$to:Time!){service(id:$id){replicas instances{metrics(from:$from,to:$to,interval:"1m"){cpuPercent}}}}`
	variables := map[string]interface{}{"id": cfg.ServiceID, "from": from, "to": to}

	var resp gqlResponse
	if err := doGraphQL(ctx, query, variables, cfg.Token, &resp); err != nil {
		log.Printf("gql err: %v", err)
		return snapshot{}
	}
	if len(resp.Errors) > 0 {
		log.Printf("gql errs: %+v", resp.Errors)
		return snapshot{}
	}

	// aggregate avg across points and instances
	sum := 0.0
	count := 0
	for _, inst := range resp.Data.Service.Instances {
		for _, p := range inst.Metrics {
			sum += p.Cpu
			count++
		}
	}
	avg := 0.0
	if count > 0 {
		avg = sum / float64(count)
	}
	return snapshot{avgCPU: avg, replicas: resp.Data.Service.Replicas}
}

func doGraphQL(ctx context.Context, query string, vars map[string]interface{}, token string, into interface{}) error {
	payload, _ := json.Marshal(gqlRequest{Query: query, Variables: vars})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	// prefer Project‑Access‑Token for the least privilege
	req.Header.Set("Project-Access-Token", token)

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("close err: %v", err)
		}
	}(res.Body)
	return json.NewDecoder(res.Body).Decode(into)
}

// decision logic
func decide(cpu float64, replicas int, cfg config) int {
	switch {
	case cpu > cfg.High && replicas < cfg.Max:
		return replicas + 1
	case cpu < cfg.Low && replicas > cfg.Min:
		return replicas - 1
	default:
		return replicas
	}
}

func scale(ctx context.Context, cfg config, desired int) error {
	mutation := `mutation($id:String!,$count:Int!){serviceReplicaScale(input:{serviceId:$id,replicas:$count}){id}}`
	vars := map[string]interface{}{"id": cfg.ServiceID, "count": desired}
	var out map[string]interface{}
	return doGraphQL(ctx, mutation, vars, cfg.Token, &out)
}

func round(x float64) float64 { return math.Round(x*100) / 100 }
