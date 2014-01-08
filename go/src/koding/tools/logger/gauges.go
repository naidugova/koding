package logger

import (
	stdlog "log"
	"strings"
	"time"
)

//----------------------------------------------------------
// Copied over from koding/tool/log.
//----------------------------------------------------------

type Unit string
type Gauge struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Time   int64   `json:"measure_time"`
	Source string  `json:"source"`
	unit   Unit
	input  func() float64
}

const ISO8601 = "2006-01-02T15:04:05.000"
const (
	NoUnit     = Unit("")
	Seconds    = Unit("s")
	Percentage = Unit("%")
	Bytes      = Unit("B")
)

var interval = 60000
var gauges = make([]*Gauge, 0)
var GaugeChanges = make(chan func())
var tags string
var hostname string

func CreateGauge(name string, unit Unit, input func() float64) {
	gauges = append(gauges, &Gauge{name, 0, 0, "", unit, input})
}

func CreateCounterGauge(name string, unit Unit, resetOnReport bool) func(int) {
	value := new(int)
	CreateGauge(name, unit, func() float64 {
		v := *value
		if resetOnReport {
			*value = 0
		}
		return float64(v)
	})
	return func(diff int) {
		GaugeChanges <- func() {
			*value += diff
		}
	}
}

func RunGaugesLoop() {
	reportTrigger := make(chan int64)
	go func() {
		reportInterval := int64(interval) / 1000
		nextReportTime := time.Now().Unix() / reportInterval * reportInterval
		for {
			nextReportTime += reportInterval
			time.Sleep(time.Duration(nextReportTime-time.Now().Unix()) * time.Second)
			reportTrigger <- nextReportTime
		}
	}()
	go func() {
		for {
			select {
			case change := <-GaugeChanges:
				change()
			}
		}
	}()
}

func LogGauges(reportTime int64) {
	indent := strings.Repeat(" ", len(ISO8601)+1)
	stdlog.Printf("%s [gauges %s]\n", time.Now().Format(ISO8601), tags)

	for _, gauge := range gauges {
		stdlog.Printf("%s%s: %v\n", indent, gauge.Name, gauge.input())
	}

	for _, gauge := range gauges {
		gauge.Value = gauge.input()
		gauge.Time = reportTime
	}
}
