package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"tailscale.com/envknob"
)

var overrideListenAddr = envknob.String("TASMOTA_EXPORTER_LISTEN_ADDR")

// plugMetrics maps each exported gauge to the reading it reports.
var plugMetrics = []struct {
	name, help string
	value      func(TasmotaPlug) float64
}{
	{"tasmota_on", "Indicates if the tasmota plug is on/off", func(p TasmotaPlug) float64 {
		if p.On {
			return 1
		}
		return 0
	}},
	{"tasmota_voltage_volts", "voltage of tasmota plug in volt (V)", func(p TasmotaPlug) float64 { return p.Voltage }},
	{"tasmota_current_amperes", "current of tasmota plug in ampere (A)", func(p TasmotaPlug) float64 { return p.Current }},
	{"tasmota_power_watts", "current power of tasmota plug in watts (W)", func(p TasmotaPlug) float64 { return p.Power }},
	{"tasmota_apparent_power_voltamperes", "apparent power of tasmota plug in volt-amperes (VA)", func(p TasmotaPlug) float64 { return p.ApparentPower }},
	{"tasmota_reactive_power_voltamperesreactive", "reactive power of tasmota plug in volt-amperes reactive (VAr)", func(p TasmotaPlug) float64 { return p.ReactivePower }},
	{"tasmota_power_factor", "current power factor of tasmota plug", func(p TasmotaPlug) float64 { return p.Factor }},
	{"tasmota_today_kwh_total", "todays energy usage total in kilowatts hours (kWh)", func(p TasmotaPlug) float64 { return p.Today }},
	{"tasmota_yesterday_kwh_total", "yesterdays energy usage total in kilowatts hours (kWh)", func(p TasmotaPlug) float64 { return p.Yesterday }},
	{"tasmota_kwh_total", "total energy usage in kilowatts hours (kWh)", func(p TasmotaPlug) float64 { return p.Total }},
}

func main() {
	http.HandleFunc("/probe", tasmotaHandler)

	listenAddr := ":9090"
	if overrideListenAddr != "" {
		listenAddr = overrideListenAddr
	}

	log.Printf("starting tasmota exporter on %s", listenAddr)
	err := http.ListenAndServe(listenAddr, nil)
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed")
	} else if err != nil {
		log.Fatalf("error starting server: %s", err)
	}
}

func tasmotaHandler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()
	plug, err := probeTasmota(ctx, target)
	duration := time.Since(start).Seconds()

	// A registry per request: probes of different targets run concurrently
	// and must neither share readings nor inherit them from a previous probe.
	registry := prometheus.NewRegistry()
	addGauge(registry, "probe_duration_seconds", "Returns how long the probe took to complete in seconds", duration)

	if err != nil {
		log.Printf("%s: probe failed, duration: %fs: %s", target, duration, err)
		addGauge(registry, "probe_success", "Displays whether or not the probe was a success", 0)
	} else {
		log.Printf("%s: probe succeeded, duration: %fs", target, duration)
		addGauge(registry, "probe_success", "Displays whether or not the probe was a success", 1)
		for _, m := range plugMetrics {
			addGauge(registry, m.name, m.help, m.value(plug))
		}
	}

	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

func addGauge(registry *prometheus.Registry, name, help string, value float64) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
	g.Set(value)
	registry.MustRegister(g)
}

func probeTasmota(ctx context.Context, target string) (TasmotaPlug, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s?m", target), nil)
	if err != nil {
		return TasmotaPlug{}, fmt.Errorf("building request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TasmotaPlug{}, fmt.Errorf("querying target: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Anything but 200 (e.g. 401 behind a web password) has no readings,
	// and parse would report them as zeros.
	if resp.StatusCode != http.StatusOK {
		return TasmotaPlug{}, fmt.Errorf("target returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TasmotaPlug{}, fmt.Errorf("reading response: %w", err)
	}

	return parse(string(body)), nil
}

type TasmotaPlug struct {
	// On indicates if the plug is on or off.
	On bool `json:"On"`

	// Voltage describes the voltage used of the appliance
	// denoted in V.
	Voltage float64 `json:"Voltage"`

	// Current describes the amount of amperes used, denoted
	// in A.
	Current float64 `json:"Current"`

	// Power describes the current power used, denoted in W (watt)
	Power float64 `json:"Power"`

	// ApparentPower describes the volt-ampere (VA)
	ApparentPower float64 `json:"ApparentPower"`

	// ReactivePower describes Volt-Amps Reactive (VAr)
	ReactivePower float64 `json:"ReactivePower"`

	// Factor describes the power factor
	Factor float64 `json:"Factor"`

	// Today is the total usage of energy in kilowatts hours (kWh)
	// meassured by the internal clock of the plug for today.
	Today float64 `json:"Today"`

	// Yesterday is the total usage of energy in kilowatts hours (kWh)
	// meassured by the internal clock of the plug for yesterday.
	Yesterday float64 `json:"Yesterday"`

	// Total is the total usage of energy in kilowatts hours (kWh)
	// since the plug was last factory reset.
	Total float64 `json:"Total"`
}

func parse(input string) TasmotaPlug {
	ret := TasmotaPlug{
		On: strings.Contains(input, "ON"),
	}

	rows := strings.SplitSeq(input, "{s}")
	for row := range rows {
		rowRaw := strings.Split(row, "{m}")

		if len(rowRaw) < 2 {
			continue
		}

		label := rowRaw[0]
		valueRaw := rowRaw[1]

		valueSplit := strings.Split(valueRaw, "{e}")

		if len(valueSplit) == 0 {
			continue
		}

		valueStrWithUnit := valueSplit[0]
		if strings.Contains(valueStrWithUnit, "<td") {
			valueStrWithUnit = strings.ReplaceAll(valueStrWithUnit, "</td><td style='text-align:left'>", "")
			valueStrWithUnit = strings.ReplaceAll(valueStrWithUnit, "</td><td>&nbsp;</td><td>", "")
		}

		valueSplitWithUnit := strings.Split(valueStrWithUnit, " ")
		if len(valueSplitWithUnit) == 0 {
			continue
		}

		value, err := strconv.ParseFloat(valueSplitWithUnit[0], 64)
		if err != nil {
			continue
		}

		switch label {
		case "Voltage":
			ret.Voltage = value
		case "Current":
			ret.Current = value
		case "Active Power":
			ret.Power = value
		case "Apparent Power":
			ret.ApparentPower = value
		case "Reactive Power":
			ret.ReactivePower = value
		case "Power Factor":
			ret.Factor = value
		case "Energy Today":
			ret.Today = value
		case "Energy Yesterday":
			ret.Yesterday = value
		case "Energy Total":
			ret.Total = value
		default:
			log.Printf("unable to match label, got: %s, value: %f", label, value)

		}
	}

	return ret
}
