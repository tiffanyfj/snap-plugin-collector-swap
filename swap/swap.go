// +build linux

/*
http://www.apache.org/licenses/LICENSE-2.0.txt


Copyright 2015-2016 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package swap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/intelsdi-x/snap/control/plugin"
	"github.com/intelsdi-x/snap/control/plugin/cpolicy"
)

const (
	// Name of the plugin
	name = "swap"
	// Version of the plugin
	version = 1
	// Type of the plugin
	pluginType = plugin.CollectorPluginType

	// Namespace definition
	vendorPrefix = "intel"
	srcPrefix    = "procfs"
	typePrefix   = "swap"
	ioPrefix     = "io"
	devPrefix    = "device"
	combPrefix   = "all"
)

var (
	// Swap IO data source for kernel 2.6+
	SourceIOnew = "/proc/vmstat"
	// Swap IO data source for kernel <2.6
	SourceIOold = "/proc/stat"
	// Per device swap data source
	SourcePerDev = "/proc/swaps"
	// Combined swap data source
	SourceCombined = "/proc/meminfo"

	// Swap IO metrics
	ioMetrics = []string{"in_bytes_per_sec", "in_pages_per_sec", "out_bytes_per_sec", "out_pages_per_sec"}
	// Swap per device metrics
	devMetrics = []string{"used_bytes", "used_percent", "free_bytes", "free_percent"}
	// Swap combined metrics
	combMetrics = []string{"used_bytes", "used_percent", "free_bytes", "free_percent", "cached_bytes", "cached_percent"}
)

// Swap holds Linux swap related metrics
type Swap struct {
	ioStats   map[string]float64
	devStats  map[string]float64
	combStats map[string]float64
	ioHistory ioData
	newIOfile bool
}

// ioData holds historic data for trend calculation
type ioData struct {
	swapIn    float64
	swapOut   float64
	timestamp time.Time
}

// Meta returns plugin meta data
func Meta() *plugin.PluginMeta {
	return plugin.NewPluginMeta(name, version, pluginType, []string{}, []string{plugin.SnapGOBContentType})
}

// New returns new swap plugin instance
func New() *Swap {
	newIOfile := true
	// Check if we should use new or old source for IO data
	files := []string{SourcePerDev, SourceCombined}
	fh, err := os.Open(SourceIOnew)
	if err != nil {
		files = append(files, SourceIOold)
		newIOfile = false
	}
	defer fh.Close()

	// Bail out if not all data sources are accessible
	for _, f := range files {
		if !fileOK(f) {
			return nil
		}
	}

	ih := ioData{
		swapIn:    0,
		swapOut:   0,
		timestamp: time.Now(),
	}
	s := &Swap{
		ioStats:   map[string]float64{},
		devStats:  map[string]float64{},
		combStats: map[string]float64{},
		ioHistory: ih,
		newIOfile: newIOfile,
	}
	return s
}

// CollectMetrics returns metrics relevant to Linux swap
func (swap *Swap) CollectMetrics(mts []plugin.PluginMetricType) ([]plugin.PluginMetricType, error) {
	// Gather metrics
	getDevDone := false
	getCombDone := false
	getIODone := false
	for _, mt := range mts {
		ns := mt.Namespace()
		switch ns[3] {
		case devPrefix:
			if !getDevDone {
				getDevDone = true
				err := getDevMetrics(swap.devStats)
				if err != nil {
					return nil, err
				}
			}
		case combPrefix:
			if !getCombDone {
				getCombDone = true
				err := getCombinedMetrics(swap.combStats)
				if err != nil {
					return nil, err
				}
			}
		case ioPrefix:
			if !getIODone {
				getIODone = true
				err := getIOmetrics(swap)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	//Populate metrics
	host, _ := os.Hostname()
	metrics := []plugin.PluginMetricType{}
	var m plugin.PluginMetricType
	for _, mt := range mts {
		ns := mt.Namespace()
		switch ns[3] {
		case devPrefix:
			stat := ns[4] + "/" + ns[5]
			val, ok := swap.devStats[stat]
			if !ok {
				return metrics, fmt.Errorf("Requested per device swap stat %s is not available!", stat)
			}
			m.Data_ = val
		case combPrefix:
			stat := ns[4]
			val, ok := swap.combStats[stat]
			if !ok {
				return metrics, fmt.Errorf("Requested combined swap stat %s is not available!", stat)
			}
			m.Data_ = val
		case ioPrefix:
			stat := ns[4]
			val, ok := swap.ioStats[stat]
			if !ok {
				return metrics, fmt.Errorf("Requested IO swap stat %s is not available!", stat)
			}
			m.Data_ = val
		}
		m.Namespace_ = ns
		m.Source_ = host
		m.Timestamp_ = time.Now()
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// GetMetricTypes returns the metric types relevant to Linux swap
func (swap *Swap) GetMetricTypes(_ plugin.PluginConfigType) ([]plugin.PluginMetricType, error) {
	metricTypes := []plugin.PluginMetricType{}
	fd, err := os.Open(SourcePerDev)
	if err != nil {
		return nil, fmt.Errorf("Failed to open file for reading: %s", SourcePerDev)
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	devices := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		fields := len(strings.Fields(line))
		if fields != 5 {
			continue
		}

		dev := strings.Fields(line)[0]
		if dev == "Filename" {
			continue
		}

		devices = append(devices, noSlashes(dev))
	}
	for _, metric := range ioMetrics {
		metricType := plugin.PluginMetricType{Namespace_: []string{vendorPrefix, srcPrefix, typePrefix, ioPrefix, metric}}
		metricTypes = append(metricTypes, metricType)
	}
	for _, device := range devices {
		for _, metric := range devMetrics {
			metricType := plugin.PluginMetricType{Namespace_: []string{vendorPrefix, srcPrefix, typePrefix, devPrefix, device, metric}}
			metricTypes = append(metricTypes, metricType)
		}
	}
	for _, metric := range combMetrics {
		metricType := plugin.PluginMetricType{Namespace_: []string{vendorPrefix, srcPrefix, typePrefix, combPrefix, metric}}
		metricTypes = append(metricTypes, metricType)
	}
	return metricTypes, nil
}

// GetConfigPolicy returns a ConfigPolicy
func (swap *Swap) GetConfigPolicy() (*cpolicy.ConfigPolicy, error) {
	c := cpolicy.New()
	return c, nil
}

// calcPercantage returns outcome of fraction defined by nominator and denominator in percents
func calcPercentage(nom, denom float64) float64 {
	if denom == 0 {
		// avoid dividing by zero
		return 0
	}
	return 100 * nom / denom
}

func getDevMetrics(dest map[string]float64) error {
	fd, err := os.Open(SourcePerDev)
	if err != nil {
		return fmt.Errorf("Failed to open file for reading: %s", SourcePerDev)
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		fields := len(strings.Fields(line))
		if fields != 5 {
			continue
		}
		dev := strings.Fields(line)[0]
		if dev == "Filename" {
			continue
		}
		dev = noSlashes(dev)
		totalS := strings.Fields(line)[2]
		total, err := strconv.ParseFloat(totalS, 64)
		if err != nil {
			return fmt.Errorf("Swap size for %s is not a number: %s", dev, totalS)
		}

		usedS := strings.Fields(line)[3]
		used, err := strconv.ParseFloat(usedS, 64)
		if err != nil {
			return fmt.Errorf("Used swap size for %s is not a number: %s", dev, usedS)
		}

		usedBytes := used * 1024.0
		freeBytes := (total - used) * 1024.0

		keyUsedBytes := dev + "/" + devMetrics[0]
		dest[keyUsedBytes] = usedBytes

		keyUsedPerc := dev + "/" + devMetrics[1]
		dest[keyUsedPerc] = calcPercentage(used, total)

		keyFreeBytes := dev + "/" + devMetrics[2]
		dest[keyFreeBytes] = freeBytes

		keyFreePerc := dev + "/" + devMetrics[3]
		dest[keyFreePerc] = calcPercentage(total-used, total)
	}
	return nil
}

func getCombinedMetrics(dest map[string]float64) error {
	fd, err := os.Open(SourceCombined)
	if err != nil {
		return fmt.Errorf("Failed to open following file for reading: %s", SourceCombined)
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	total := 0.0
	free := 0.0
	cached := 0.0
	for scanner.Scan() {
		line := scanner.Text()
		fields := len(strings.Fields(line))
		if fields < 2 {
			continue
		}
		if strings.Fields(line)[0] == "SwapTotal:" {
			totalS := strings.Fields(line)[1]
			total, err = strconv.ParseFloat(totalS, 64)

			if err != nil {
				return fmt.Errorf("SwapTotal is not a number: %s", totalS)
			}
		}
		if strings.Fields(line)[0] == "SwapFree:" {
			freeS := strings.Fields(line)[1]
			free, err = strconv.ParseFloat(freeS, 64)
			if err != nil {
				return fmt.Errorf("SwapFree is not a number: %s", freeS)
			}
		}
		if strings.Fields(line)[0] == "SwapCached:" {
			cachedS := strings.Fields(line)[1]
			cached, err = strconv.ParseFloat(cachedS, 64)
			if err != nil {
				return fmt.Errorf("SwapCached is not a number: %s", cachedS)
			}
		}
	}

	if total == 0 {
		fmt.Fprintln(os.Stderr, "Total size of swap is zero, swap might be turned off")
	}

	used := total - free
	totalSwap := total + cached

	dest[combMetrics[0]] = used * 1024.0
	dest[combMetrics[1]] = calcPercentage(used, totalSwap)
	dest[combMetrics[2]] = free * 1024.0
	dest[combMetrics[3]] = calcPercentage(free, totalSwap)
	dest[combMetrics[4]] = cached * 1024.0
	dest[combMetrics[5]] = calcPercentage(cached, totalSwap)
	return nil
}

func getIOmetrics(swap *Swap) error {
	fileToOpen := ""
	if swap.newIOfile {
		fileToOpen = SourceIOnew
	} else {
		fileToOpen = SourceIOold
	}
	fd, err := os.Open(fileToOpen)
	if err != nil {
		return fmt.Errorf("Failed to open following file for reading: %s", fileToOpen)
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	var swapIn float64
	var swapOut float64
	for scanner.Scan() {
		line := scanner.Text()
		fields := len(strings.Fields(line))
		if swap.newIOfile {
			if fields < 2 {
				continue
			}
			if strings.Fields(line)[0] == "pswpin" {
				swapInS := strings.Fields(line)[1]
				swapIn, err = strconv.ParseFloat(swapInS, 64)
				if err != nil {
					return fmt.Errorf("pswpin is not a number: %s", swapInS)
				}
			}
			if strings.Fields(line)[0] == "pswpout" {
				swapOutS := strings.Fields(line)[1]
				swapOut, err = strconv.ParseFloat(swapOutS, 64)
				if err != nil {
					return fmt.Errorf("pswpout is not a number: %s", swapOutS)
				}

			}
		} else {
			if strings.Fields(line)[0] == "page" {
				swapInS := strings.Fields(line)[1]
				swapIn, err = strconv.ParseFloat(swapInS, 64)
				if err != nil {
					return fmt.Errorf("Swap in metric is not a number: %s", swapInS)
				}
				swapOutS := strings.Fields(line)[2]
				swapOut, err = strconv.ParseFloat(swapOutS, 64)
				if err != nil {
					return fmt.Errorf("Swap out metric is not a number: %s", swapOutS)
				}
			}
		}
	}
	pageSize := float64(os.Getpagesize())
	oldSwapIn := swap.ioHistory.swapIn
	oldSwapOut := swap.ioHistory.swapOut
	oldTimestamp := swap.ioHistory.timestamp
	duration := time.Since(oldTimestamp).Seconds()

	if duration == 0 {
		return errors.New("Invalid duration time")
	}

	swap.ioStats[ioMetrics[0]] = (swapIn - oldSwapIn) * pageSize / duration
	swap.ioStats[ioMetrics[1]] = (swapIn - oldSwapIn) / duration
	swap.ioStats[ioMetrics[2]] = (swapOut - oldSwapOut) * pageSize / duration
	swap.ioStats[ioMetrics[3]] = (swapOut - oldSwapOut) / duration
	swap.ioHistory.swapIn = swapIn
	swap.ioHistory.swapOut = swapOut
	swap.ioHistory.timestamp = time.Now()
	return nil
}

func fileOK(f string) bool {
	fh, err := os.Open(f)
	if err != nil {
		return false
	}
	defer fh.Close()
	return true
}

func noSlashes(s string) string {
	return strings.Replace(strings.TrimPrefix(s, "/"), "/", "_", -1)
}
