package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	pluginsHelper "github.com/intel/intent-driven-orchestration/plugins"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/profiling"

	"k8s.io/klog/v2"
)

// maxLookBack defines the maximum age a model in the knowledge base can have (1 week)
const maxLookBack = 10080

var (
	kubeConfig string
	config     string
)

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kube config file.")
	flag.StringVar(&config, "config", "", "Path to configuration file.")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	tmp, err := common.LoadConfig(config, func() interface{} {
		return &profiling.CPUProfileConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*profiling.CPUProfileConfig)

	// validate configuration.
	err = pluginsHelper.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}

	err = isValidConf(cfg.Interpreter, cfg.Analytics, cfg.Prediction, cfg.CPUMax, cfg.CPUProfiles, cfg.LookBack)
	if err != nil {
		klog.Fatalf("Error on configuration for actuator: %s", err)
	}

	// get K8s config.
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		klog.Fatalf("Error getting Kubernetes config: %s", err)
	}
	clusterClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating Kubernetes cluster client: %s", err)
	}

	// once configuration is ready & valid start the plugin mechanism.
	mt := controller.NewMongoTracer(cfg.MongoEndpoint)
	actuator := profiling.NewCPUProfileActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

// TODO: add checking on cpuMax
func isValidConf(interpreter, analyzeScript string, predictionScript string, cpuMax int64, CPUProfiles []profiling.CPUProfile, lookBack int) error {
	if !pluginsHelper.IsStrConfigValid(interpreter) {
		return fmt.Errorf("invalid path to python interpreter: %s", interpreter)
	}

	if analyzeScript != "None" {
		_, err := os.Stat(analyzeScript)
		if err != nil {
			return fmt.Errorf("invalid script %s", err)
		}
	}

	if predictionScript != "None" {
		_, err := os.Stat(predictionScript)
		if err != nil {
			return fmt.Errorf("invalid script %s", err)
		}
	}

	if err := isValidCPUProfiles(CPUProfiles); err != nil {
		return fmt.Errorf("invalid cpu profiles: %s", err)
	}

	if lookBack <= 0 || lookBack > maxLookBack {
		return fmt.Errorf("invalid lookback value: %d", lookBack)
	}

	return nil
}

func isValidCPUProfiles(CPUProfiles []profiling.CPUProfile) error {
	if len(CPUProfiles) == 0 {
		return errors.New("cpuProfiles slice is empty")
	}

	ids := make(map[int]bool)

	for _, profile := range CPUProfiles {
		// Check ID
		if profile.ID <= 0 {
			return fmt.Errorf("invalid ID %d in profile %s", profile.ID, profile.Name)
		}
		if ids[profile.ID] {
			return fmt.Errorf("duplicate ID %d found in profiles", profile.ID)
		}
		ids[profile.ID] = true

		// Check Name
		if !isValidString(profile.Name) {
			return fmt.Errorf("profile with ID %d has invalid name", profile.ID)
		}

		// Check CPUManager
		if profile.CPUManager != profiling.Vanilla && profile.CPUManager != profiling.CPUControlPlane {
			return fmt.Errorf("invalid CPUManager in profile %s", profile.Name)
		}

		// Check Affinity
		if profile.Affinity != profiling.Preferred && profile.Affinity != profiling.Required {
			return fmt.Errorf("invalid Affinity in profile %s", profile.Name)
		}

		// Check Settings
		for key, value := range profile.Settings {
			if !isValidString(key) {
				return fmt.Errorf("empty key in settings for profile %s", profile.Name)
			}
			if !isValidString(value) {
				return fmt.Errorf("empty value for key '%s' in settings for profile %s", key, profile.Name)
			}
		}
	}

	return nil
}

func isValidString(text string) bool {
	if text == "" {
		return false
	}

	if len(text) == 1 {
		return regexp.MustCompile(`^[a-zA-Z0-9]$`).MatchString(text)
	}

	return regexp.MustCompile(`^[a-zA-Z0-9][a-z0-9._-]*[a-zA-Z0-9]$`).MatchString(text)
}
