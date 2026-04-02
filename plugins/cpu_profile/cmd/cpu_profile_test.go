package main

import (
	"fmt"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/profiling"
)

// pathToAnalyticsScript defines the path to an existing script for this actuator.
const pathToAnalyticsScript = "../../../pkg/planner/actuators/profiling/test_analyze.py"
const pathToPredictScript = "../../../pkg/planner/actuators/profiling/test_predict.py"

func TestIsValidConf(t *testing.T) {
	type args struct {
		interpreter      string
		analyticsScript  string
		predictionScript string
		cpuMax           int64
		cpuProfiles      []profiling.CPUProfile
		lookBack         int
	}
	validCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
		{
			ID:         4,
			Name:       "topology-aware",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy":      "static",
				"topology_manager_policy": "besteffort",
				"topology-manager-scope":  "pod",
			},
		},
	}

	duplicateIdCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1, //duplicated
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         1, //duplicate
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	negativeIdCPUProfiles := []profiling.CPUProfile{
		{
			ID:         -1, //incorrect
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	zeroIdCPUProfiles := []profiling.CPUProfile{
		{
			ID:         0, //incorrect
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	emptyCPUProfiles := []profiling.CPUProfile{}

	invalidNameCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "", //incorrect
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	invalidManagerCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: 3, //incorrect
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	invalidAffinityCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   3, // incorrect
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	invalidSettingsKeyCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"_cpu_manager_policy": "static", //incorrect
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	invalidSettingsValueCPUProfiles := []profiling.CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static-", //incorrect
			},
		},
		{
			ID:         3,
			Name:       "pexclusive",
			CPUManager: profiling.Vanilla,
			Affinity:   profiling.Preferred,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
				"full_pcpu_only":     "true",
			},
		},
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, validCPUProfiles, 10000},
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    args{"", pathToAnalyticsScript, pathToPredictScript, 4000, validCPUProfiles, 10000},
			wantErr: true,
		},
		{
			name:    "tc-2",
			args:    args{"python3", "", pathToPredictScript, 4000, validCPUProfiles, 10000},
			wantErr: true,
		},
		{
			name:    "tc-3",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, duplicateIdCPUProfiles, 10000},
			wantErr: true, // duplicate ID
		},
		{
			name:    "tc-4",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, negativeIdCPUProfiles, 10000},
			wantErr: true, //negative ID
		},
		{
			name:    "tc-5",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, zeroIdCPUProfiles, 10000},
			wantErr: true, //zero ID
		},
		{
			name:    "tc-6",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, emptyCPUProfiles, 10000},
			wantErr: true, //empty CPUProfiles slice
		},
		{
			name:    "tc-7",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, invalidNameCPUProfiles, 10000},
			wantErr: true, //invalid Name
		},
		{
			name:    "tc-8",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, invalidManagerCPUProfiles, 10000},
			wantErr: true, //invalid CPUManager
		},
		{
			name:    "tc-9",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, invalidAffinityCPUProfiles, 10000},
			wantErr: true, //invalid affinity
		},
		{
			name:    "tc-10",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, invalidSettingsKeyCPUProfiles, 10000},
			wantErr: true, //invalid settings key
		},
		{
			name:    "tc-11",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, invalidSettingsValueCPUProfiles, 10000},
			wantErr: true, //invalid settings value
		},
		{
			name:    "tc-12",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, validCPUProfiles, -1},
			wantErr: true, // negative lookback.
		},
		{
			name:    "tc-13",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, 4000, validCPUProfiles, 999999},
			wantErr: true, // over limit lookback.
		},
	}
	for _, tt := range tests {
		fmt.Printf("test: %v\n", tt.name)
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.interpreter, tt.args.analyticsScript, tt.args.predictionScript, tt.args.cpuMax,
				tt.args.cpuProfiles, tt.args.lookBack); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
