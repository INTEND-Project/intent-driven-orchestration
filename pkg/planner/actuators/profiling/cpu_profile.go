package profiling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"os/exec"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// actionName represents the name of the action.
const actionName = "profileCPU"

// groupName represents the name for the scaling related set of actions.
const groupName = "profiling"

const CPUProfileKey = "cpu_profile"

// General settings
type CPUManager int

const (
	Vanilla CPUManager = iota
	CPUControlPlane
)

type Affinity int

const (
	Preferred Affinity = iota
	Required
)

type CPUProfile struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	CPUManager CPUManager        `json:"cpu_manager"`
	Affinity   Affinity          `json:"affinity"`
	Settings   map[string]string `json:"settings"`
}

// CPUProfileConfig describes the configuration for this actuator.
type CPUProfileConfig struct {
	Interpreter           string       `json:"interpreter"`
	Analytics             string       `json:"analytics_script"`
	Prediction            string       `json:"prediction_script"`
	CPUMax                int64        `json:"cpu_max"`
	LookBack              int          `json:"look_back"`
	Endpoint              string       `json:"endpoint"`
	Port                  int          `json:"port"`
	PluginManagerEndpoint string       `json:"plugin_manager_endpoint"`
	PluginManagerPort     int          `json:"plugin_manager_port"`
	MongoEndpoint         string       `json:"mongo_endpoint"`
	TelemetryEndpoint     string       `json:"telemetry_endpoint"`
	CPUProfiles           []CPUProfile `json:"cpu_profiles"`
}

type IndividualCPUEffect struct {
	ID      int
	Latency float64
	Mean    float64
	Sdt     float32
}

type CPUProfileEffect struct {
	// Never ever think about making these non-public! Needed for marshalling this struct.
	LookupTable      map[int64][]IndividualCPUEffect
	TrainingFeatures [1]string
	TargetFeature    string
	Image            string
}

// CPUProfileActuator is an actuator supporting the cpu profiling.
type CPUProfileActuator struct {
	cfg    CPUProfileConfig
	tracer controller.Tracer
	apps   kubernetes.Interface
}

func (cp CPUProfileActuator) Name() string {
	return actionName
}

func (cp CPUProfileActuator) Group() string {
	return groupName
}

// requestBody represents the json send to prediction function.
type requestBody struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	CPUs       int64  `json:"cpus"`
	CPUProfile int    `json:"cpu_profile"`
	Replicas   int    `json:"replicas"`
	Telemetry  string `json:"telemetry"`
}

// responseBody represents the json returned by the prediction function.
type responseBody struct {
	Val float64 `json:"val"`
}

// doQuery calls the prediction function.
func doQuery(body requestBody) float64 {
	tmp, _ := json.Marshal(body)
	client := http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Post("http://localhost:8000", "application/json", bytes.NewBuffer(tmp))
	if err != nil {
		klog.Errorf("Could not reach prediction endpoint: %s.", err)
		return -1.0
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("Error to read the body: %s", err)
		return -1.0
	}
	var res responseBody
	err = json.Unmarshal(respBody, &res)
	if err != nil {
		klog.Errorf("Could not unmarshall response: %s - request: %+v - response: %s", err, body, respBody)
		return -1.0
	}
	return res.Val
}

// getCPUProfile return the cpu profile associated with the POD.
func (cp CPUProfileActuator) getCPUProfile(state *common.State) (*CPUProfile, error) {

	CPUProfileIDStr, exists := state.Annotations[CPUProfileKey]
	if !exists {
		return nil, fmt.Errorf("CPU profile key %s not found in annotations", CPUProfileKey)
	}
	CPUProfileID, err := strconv.Atoi(CPUProfileIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CPU profile ID: %v", err)
	}

	// match the pod's CPUProfile with details of the CPU profile in the config
	for _, cpuProfile := range cp.cfg.CPUProfiles {
		if cpuProfile.ID == CPUProfileID {
			return &cpuProfile, nil
		}
	}
	return nil, fmt.Errorf("current CPU profile id not found in the actuator's config file")
}

// setCPUProfile() updates the workload with the new CPUprofile's config.
func (cp CPUProfileActuator) setCPUProfile(state *common.State, newCPUProfileID int) {

	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]

	// get the cpu profile of the current state
	currentCPUProfile, err := cp.getCPUProfile(state)
	if err != nil {
		klog.Errorf("%v", err)
		return
	}

	// if the current cpu profile is the same as the new one, return and do nothing
	if currentCPUProfile.ID == newCPUProfileID {
		klog.Warning("new and current CPU profiles are the same -- nothing to be done")
		return
	}

	// get the new cpu profile DS object (including settings and properties)
	// assumption: cpu profiles are ordered in the config file. Their IDs are aligned with the slice index oredring +1 (slice indexing starts at 0, profile IDs start at 1)
	if newCPUProfileID <= 0 || newCPUProfileID > len(cp.cfg.CPUProfiles) {
		klog.Errorf("new CPU profile ID %d is out of range", newCPUProfileID)
		return
	}
	newCPUProfile := cp.cfg.CPUProfiles[newCPUProfileID-1]
	// Setup the new cpu profile
	if state.Intent.TargetKind == "Deployment" {
		client := cp.apps.AppsV1().Deployments(namespace)
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			deployment, err := client.Get(context.TODO(), tmp[1], metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("failed to get latest version of Deployment: %v", err)
				return nil
			}
			updatedDeployment := deployment.DeepCopy()
			pod := updatedDeployment.Spec.Template

			// Modify the current pod with the target profile
			if newCPUProfile.CPUManager == Vanilla {
				klog.Infof("set new CPU profile: %v", newCPUProfile)
				err = toVanillaProfile(&pod, newCPUProfile, *currentCPUProfile)
				if err != nil {
					klog.Errorf("failed to set the new CPU profile: %v", err)
					return nil
				}
				
			} else if newCPUProfile.CPUManager == CPUControlPlane {
				klog.Errorf("CPU Control Plane profiles not yet supported")
				return nil
			}

			// set annotations for pods (check if working propoerly in real k8s environment)
			annotations := pod.Annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[CPUProfileKey] = strconv.Itoa(newCPUProfileID)
			pod.Annotations = annotations


			updatedDeployment.Spec.Template = pod
			// Update the deployment
			_, updateErr := client.Update(context.TODO(), updatedDeployment, metaV1.UpdateOptions{})
			 if updateErr != nil {
                klog.Errorf("Deployment update failed: %v", updateErr)
            } else {
                klog.Infof("Deployment update successful")
            }
			return updateErr
		})
		if retryErr != nil {
			klog.Errorf("update of deployment %s failed: %v", state.Intent.TargetKey, retryErr)
		}

	} else if state.Intent.TargetKind == "ReplicaSet" {
		client := cp.apps.AppsV1().ReplicaSets(namespace)
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			replicaSet, err := client.Get(context.TODO(), tmp[1], metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("failed to get latest version of ReplicaSet: %v", err)
				return nil
			}
			updatedReplicaSet := replicaSet.DeepCopy()
			Pod := updatedReplicaSet.Spec.Template

			if newCPUProfile.CPUManager == Vanilla {
				err = toVanillaProfile(&Pod, newCPUProfile, *currentCPUProfile)
				if err != nil {
					klog.Errorf("failed to set the new CPU profile: %v", err)
					return nil
				}
			} else if newCPUProfile.CPUManager == CPUControlPlane {
				klog.Errorf("CPU Control Plane profiles not yet supported")
				return nil

			}

			// set annotations
			annotations := Pod.Annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[CPUProfileKey] = strconv.Itoa(newCPUProfileID)
			Pod.Annotations = annotations

			updatedReplicaSet.Spec.Template = Pod
			// Update the ReplicaSet
			_, updateErr := client.Update(context.TODO(), updatedReplicaSet, metaV1.UpdateOptions{})
			return updateErr
		})
		if retryErr != nil {
			klog.Errorf("update of ReplicaSet %s failed: %v", state.Intent.TargetKey, retryErr)
		}
	}
}

func getResourceValues(state *common.State, cpuManager CPUManager) int64 {
	if cpuManager == Vanilla {
		return getVanillaResourceValues(state)
	}

	klog.Error("Cannot get ResourceValues for CPU Control Plane profiles -- feature not yet supported.")
	return 0

}

// findStates determines all CPU profiles that meet target objectives.
func (cp CPUProfileActuator) findStates(
	state *common.State, goal *common.State,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {

	var states []common.State
	var utilities []float64
	var actions []planner.Action

	// get current cpu profile
	currentCPUProfile, err := cp.getCPUProfile(state)
	if err != nil {
		//assume the default profile in vanilla cpu manager
		klog.Warning(err, "\nAssumption: current cpu profile is: the default shared - with vanilla cpu manager (the first one in the config file)")
		currentCPUProfile = &cp.cfg.CPUProfiles[0] // assumption first cpu profile is the default one
		if state.Annotations == nil {
			state.Annotations = make(map[string]string)
		}
		state.Annotations[CPUProfileKey] = strconv.Itoa(currentCPUProfile.ID)
	}
	// initially consider all profiles as valid
	validProfiles := make(map[int]map[string]float64)
	for _, cpuProfile := range cp.cfg.CPUProfiles {
		//if cpuProfile.ID == currentCPUProfile.ID {
		//	continue
		//}
		validProfiles[cpuProfile.ID] = make(map[string]float64)
	}
	// get cpu resources
	cpus := getResourceValues(state, currentCPUProfile.CPUManager)
	if cpus <= 0 {
		klog.Error("Failed to get the cpus value")
		return nil, nil, nil
	}
	if cpus > cp.cfg.CPUMax {
		klog.Errorf("cpus value %d is higher than the max supported", cpus)
		return nil, nil, nil
	}

	replicas := len(state.CurrentPods)

	// loop over objectives
	klog.Infof("Evaluating objectives: %v", goal.Intent.Objectives)
	for k, v := range goal.Intent.Objectives {
		if profiles[k].ProfileType != common.ProfileTypeFromText("latency") {
			continue
		}

		//predict value for every profile
		currentValidProfiles := make(map[int]map[string]float64)
		for _, cpuProfile := range cp.cfg.CPUProfiles {

			// actually predict.
			body := requestBody{
				Name:   state.Intent.Key,
				Target: k,
				//	IPCValue:           ipc,
				//	CPUUsageValue:      CPU_usage,
				//	CacheValue:         cache,
				//	ContextSwitchValue: context_switch,
				CPUs:       cpus,
				CPUProfile: cpuProfile.ID,
				Replicas:   replicas,
				Telemetry:  cp.cfg.TelemetryEndpoint,
			}
			//klog.Infof("predict value for request: %v", body)
			predictedValue := doQuery(body)
			if predictedValue == -1.0 {
				// Predict script couldn't figure sth out -> need to skip this option.
				klog.Warningf("Prediction failed for profile %d, objective %s - excluding from valid profiles", cpuProfile.ID, k)
				continue
			}
			klog.Infof("Predicted value for profile %d, objective %s, target %f: %f\n", cpuProfile.ID, k, v, predictedValue)

			if predictedValue <= v {
				if _, ok := currentValidProfiles[cpuProfile.ID]; !ok {
					currentValidProfiles[cpuProfile.ID] = make(map[string]float64)
				}
				currentValidProfiles[cpuProfile.ID][k] = predictedValue
			}
		}

		// Intersect with the global validProfiles map
		newValidProfiles := make(map[int]map[string]float64)
		for cpuProfileID := range validProfiles {
			if _, ok := currentValidProfiles[cpuProfileID]; ok {
				 newValidProfiles[cpuProfileID] = validProfiles[cpuProfileID]
				// populate current objective's value in the valid CPU profile
				validProfiles[cpuProfileID][k] = currentValidProfiles[cpuProfileID][k]
			}
		}
		validProfiles = newValidProfiles
		klog.Infof("Valid profiles after objective %s: %+v", k, validProfiles)

		// If no valid profiles remain, exit early
		if len(validProfiles) == 0 {
			klog.Warningf("No CPU profiles satisfy all objectives for %s", state.Intent.TargetKey)
			return nil, nil, nil
		}
	}

	// create new states for valid profiles
	for cpuProfileID := range validProfiles {
		newState := state.DeepCopy()
		// Assign objectives values from validProfiles[cpuProfileID] to newState.Intent.Objectives
		for objKey, objValue := range validProfiles[cpuProfileID] {
			newState.Intent.Objectives[objKey] = objValue
		}

		if newState.IsBetter(goal, profiles) {
			newState.Annotations[CPUProfileKey] = strconv.Itoa(cpuProfileID)
			newState.CurrentData[cp.Name()] = map[string]float64{cp.Name(): 1}

			// TODO: write a better utility function
			utility := (1.0 / (10 - float64(cpuProfileID))) * (1.0 / goal.Intent.Priority)
			states = append(states, newState)
			utilities = append(utilities, utility)
			actions = append(actions, planner.Action{
				Name:       cp.Name(),
				Properties: map[string]string{"id": strconv.Itoa(cpuProfileID)},
			})

		}

	}
	klog.Infof("returned actions: %+v", actions)
	return states, utilities, actions
}

func (cp CPUProfileActuator) NextState(state *common.State, goal *common.State,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	// we don't need to try this multiple times in a single planning cycle.
	if _, ok := state.CurrentData[cp.Name()]; ok {
		return nil, nil, nil
	}
	// we don't need to do anything if there are no PODs.
	if len(state.CurrentPods) == 0 {
		return nil, nil, nil
	}
	// check pods QoS class
	for _, podState := range state.CurrentPods {
		if podState.QoSClass != "Guaranteed" {
			klog.Warningf("Targeted workload is not in guaranteed QoS class.")
			return nil, nil, nil
		}
		break
	}
	states, utilities, plan := cp.findStates(state, goal, profiles)

	return states, utilities, plan
}

func (cp CPUProfileActuator) Perform(state *common.State, plan []planner.Action) {

	klog.Infof("Perform action: %v", plan)
	for _, item := range plan {
		if item.Name == actionName {
			a := item.Properties.(map[string]string)
			 if cpuProfileStr, ok := a["id"]; ok {
                cpuProfile, err := strconv.Atoi(cpuProfileStr)
                if err != nil {
                    klog.Errorf("Invalid CPU profile ID: %v", err)
                    continue
                }
				cp.setCPUProfile(state, cpuProfile)
			}
			break
		}
	}
}

func (cp CPUProfileActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	if cp.cfg.Analytics == "None" {
		klog.V(2).Infof("Effect calculation is disabled - will not run analytics.")
		return
	}

	var latencyObjectives []string
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
			latencyObjectives = append(latencyObjectives, k)
		}
	}

	// for all latency related objectives we (re-)analyse what the effect of cpu profiling is.
	for _, objective := range latencyObjectives {

		cmd := exec.Command(cp.cfg.Interpreter, "-u",
			cp.cfg.Analytics, state.Intent.Key, objective, "--cpu_profiles", strconv.Itoa(len(cp.cfg.CPUProfiles)))
		out, err := cmd.CombinedOutput()
		if err != nil {
			klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
		}
		klog.V(2).Infof("Script output was: %v", string(out))
	}
}

// NewCPUProfileActuator initializes a new actuator.
func NewCPUProfileActuator(apps kubernetes.Interface, tracer controller.Tracer, cfg CPUProfileConfig) *CPUProfileActuator {
	cmd := exec.Command(cfg.Interpreter, "-u", cfg.Prediction) 
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		klog.Errorf("Could not get stdout pipe for prediction script: %v", err)
	}
	stderrPipe, err2 := cmd.StderrPipe()
	if err2 != nil {
		klog.Errorf("Could not get stderr pipe for prediction script: %v", err2)
	}
	err = cmd.Start()
	if err == nil {
		// Stream stdout
		go func() {
			buf := make([]byte, 1024)
			for {
				n, rerr := stdoutPipe.Read(buf)
				if n > 0 {
					klog.Infof("Prediction stdout: %s", strings.TrimSpace(string(buf[:n])))
				}
				if rerr != nil {
					if rerr != io.EOF {
						klog.Errorf("Error reading prediction stdout: %v", rerr)
					}
					break
				}
			}
		}()

		// Stream stderr
		go func() {
			buf := make([]byte, 1024)
			for {
				n, rerr := stderrPipe.Read(buf)
				if n > 0 {
					klog.Errorf("Prediction stderr: %s", strings.TrimSpace(string(buf[:n])))
				}
				if rerr != nil {
					if rerr != io.EOF {
						klog.Errorf("Error reading prediction stderr: %v", rerr)
					}
					break
				}
			}
		}()
	}
	if err != nil {
		klog.Errorf("Could not start the prediction script: %s.", err)
	}
	time.Sleep(500 * time.Millisecond)
	return &CPUProfileActuator{
		cfg:    cfg,
		tracer: tracer,
		apps:   apps,
	}
}
