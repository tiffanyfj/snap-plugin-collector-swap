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
	"os"
	"testing"

	"github.com/intelsdi-x/snap/control/plugin"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	mockMts = []plugin.PluginMetricType{
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "io", "in_bytes_per_sec"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "io", "in_pages_per_sec"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "io", "out_bytes_per_sec"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "io", "out_pages_per_sec"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda5", "used_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda6", "used_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda5", "used_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda6", "used_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda5", "free_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda6", "free_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda5", "free_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "device", "dev_sda6", "free_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "used_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "used_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "free_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "free_percent"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "cached_bytes"},
		},
		plugin.PluginMetricType{
			Namespace_: []string{"intel", "procfs", "swap", "all", "cached_percent"},
		},
	}
	ioNewMockFile  = "/tmp/vmstat_test"
	ioOldMockFile  = "/tmp/stat_test"
	perDevMockFile = "/tmp/swaps_test"
	compMockFile   = "/tmp/meminfo_test"
)

func TestGetConfigPolicy(t *testing.T) {
	SourceIOnew = ioNewMockFile
	SourceIOold = ioOldMockFile
	SourcePerDev = perDevMockFile
	SourceCombined = compMockFile
	createMockFiles()

	swap := New()
	Convey("normal case", t, func() {
		So(func() { swap.GetConfigPolicy() }, ShouldNotPanic)
		_, err := swap.GetConfigPolicy()
		So(err, ShouldBeNil)
	})
	deleteMockFiles()
}

func TestGetMetricTypes(t *testing.T) {
	SourceIOnew = ioNewMockFile
	SourceIOold = ioOldMockFile
	SourcePerDev = perDevMockFile
	SourceCombined = compMockFile
	createMockFiles()

	swap := New()
	var pl plugin.PluginConfigType
	Convey("source files available", t, func() {
		So(func() { swap.GetMetricTypes(pl) }, ShouldNotPanic)
		m, err := swap.GetMetricTypes(pl)
		So(err, ShouldBeNil)
		// 4 - IO metrics, 8 - dev metrics (2 devices), 6 - combined metrics
		So(len(m), ShouldEqual, 18)
	})

	Convey("dev source file not available", t, func() {
		os.Remove(perDevMockFile)
		m, err := swap.GetMetricTypes(pl)
		So(err, ShouldNotBeNil)
		So(m, ShouldBeNil)
	})
	deleteMockFiles()
}

func TestCollectMetrics(t *testing.T) {
	SourceIOnew = ioNewMockFile
	SourceIOold = ioOldMockFile
	SourcePerDev = perDevMockFile
	SourceCombined = compMockFile
	createMockFiles()

	swap := New()
	Convey("source files available", t, func() {
		So(func() { swap.CollectMetrics(mockMts) }, ShouldNotPanic)
		m, err := swap.CollectMetrics(mockMts)
		So(err, ShouldBeNil)
		// 4 - IO metrics, 8 - dev metrics (2 devices), 6 - combined metrics
		So(len(m), ShouldEqual, 18)
	})

	Convey("dev source file not available", t, func() {
		os.Remove(perDevMockFile)
		m, err := swap.CollectMetrics(mockMts)
		So(err, ShouldNotBeNil)
		So(m, ShouldBeNil)
	})
	deleteMockFiles()
}

func createMockFiles() {
	deleteMockFiles()
	ioNewMockFileCont := []byte("pswpin 11111\npswpout 22222\n")
	ioOldMockFileCont := []byte("page 33333 44444\n")
	perDevMockFileCont := []byte("Filename Type Size Used Priority\n/dev/sda5 partition 55555 6666 -1\n/dev/sda6 partition  77777 8888   -1\n")
	compMockFileCont := []byte("SwapTotal: 99999 kB\nSwapFree: 1010 kB\nSwapCached: 2020 kB")

	f, _ := os.Create(ioNewMockFile)
	f.Write(ioNewMockFileCont)
	f, _ = os.Create(ioOldMockFile)
	f.Write(ioOldMockFileCont)
	f, _ = os.Create(perDevMockFile)
	f.Write(perDevMockFileCont)
	f, _ = os.Create(compMockFile)
	f.Write(compMockFileCont)
}

func deleteMockFiles() {
	os.Remove(ioNewMockFile)
	os.Remove(ioOldMockFile)
	os.Remove(perDevMockFile)
	os.Remove(compMockFile)
}
