package profiling

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/intel/intent-driven-orchestration/pkg/common"
)

const defaultPreferredWeight = 1

func toVanillaProfile(pod *v1.PodTemplateSpec, newCPUProfile CPUProfile, currentCPUProfile CPUProfile) error {
	switch currentCPUProfile.CPUManager {
	case CPUControlPlane:
		return fmt.Errorf("switching from CPU Control Plane profiles is not yet supported")
		
	case Vanilla:
		err := updateNodeAffinity(pod, newCPUProfile, currentCPUProfile)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateNodeAffinity(pod *v1.PodTemplateSpec, newCPUProfile CPUProfile, currentCPUProfile CPUProfile) error {
	//The profile will be applied at the level of the pod (all containers)

	// 1. clean up current affinities
	nodeAffinity, err := getNodeAffinity(&pod.Spec)
	if err != nil {
		return err
	}
	currentSettings := currentCPUProfile.Settings

	// delete node affinities matching current profile settings
	// warning: a conflict can be caused if these affinities are also used for other purposes
	if len(currentSettings) > 0 {
		deleteNodeAffinities(nodeAffinity, currentSettings)
	}

	// 2. set up new affinities
	nodeSelectorTerms, err := mapProfileToNodeSelectors(newCPUProfile)
	for i, term := range nodeSelectorTerms {
		klog.Infof("NodeSelectorTerm %d: %+v", i, term)
	}
	if err != nil {
		return err
	}

	if len(nodeSelectorTerms) > 0 {
		err = setNodeAffinity(&pod.Spec, nodeSelectorTerms, newCPUProfile.Affinity)
		if err != nil {
			return err
		}
		klog.Infof("Pod NodeSelector: %+v", pod.Spec.NodeSelector)
	}
	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil {
		klog.Infof("Pod NodeAffinity: %+v", pod.Spec.Affinity.NodeAffinity)
		if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			klog.Infof("Required NodeAffinity Terms: %+v", 
					pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)
		}
		if pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
			klog.Infof("Preferred NodeAffinity Terms: %+v", 
					pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
		}
	}
	return nil
}

// mapProfileToNodeSelectors maps profile settings to corresonding nodeSelectorTerms
func mapProfileToNodeSelectors(cpuProfile CPUProfile) ([]v1.NodeSelectorTerm, error) {

	if len(cpuProfile.Settings) == 0 {
		// no specific nodeSelectorTerms (no need to return an error for now)
		return nil, nil
	}

	var nodeSelectorTerms []v1.NodeSelectorTerm
	var matchExpressions []v1.NodeSelectorRequirement

	// assumption: all the settings in the profile are used for node selection
	for key, value := range cpuProfile.Settings {
		strValue := fmt.Sprintf("%v", value)
		matchExpressions = append(matchExpressions, v1.NodeSelectorRequirement{
			Key:      key,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{strValue},
		})
	}

	if len(matchExpressions) == 0 {
		// no valid match expressions found (no need to return an error for now)
		return nil, nil
	}
	nodeSelectorTerms = append(nodeSelectorTerms, v1.NodeSelectorTerm{
		MatchExpressions: matchExpressions,
	})

	return nodeSelectorTerms, nil
}

// deleteNodeAffinities deletes node affinities that match elements in current settings.
func deleteNodeAffinities(nodeAffinity *v1.NodeAffinity, currentSettings map[string]string) {

	if nodeAffinity == nil {
		return
	}

	// Helper function to check if any of the nodeSelectorTerms match the elements in settings of CPU profile.
	matchesAny := func(term v1.NodeSelectorTerm) bool {
		for _, expr := range term.MatchExpressions {
			if value, exists := currentSettings[expr.Key]; exists {
				strValue := fmt.Sprintf("%v", value)
				for _, val := range expr.Values {
					if val == strValue {
						return true
					}
				}
			}
		}
		return false
	}

	// Remove matching required affinities
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		var filteredTerms []v1.NodeSelectorTerm
		for _, term := range nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			if !matchesAny(term) {
				filteredTerms = append(filteredTerms, term)
			}
		}
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = filteredTerms
	}

	// Remove matching preferred affinities
	if nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
		var filteredPreferredTerms []v1.PreferredSchedulingTerm
		for _, preferredTerm := range nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			if !matchesAny(preferredTerm.Preference) {
				filteredPreferredTerms = append(filteredPreferredTerms, preferredTerm)
			}
		}
		nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = filteredPreferredTerms
	}
}

// getNodeAffinity returns the node affinity of the pod if it exists, or create a new one if it doesn't. (podAffinity & podAntiAffinity are ignored for now)
func getNodeAffinity(podSpec *v1.PodSpec) (*v1.NodeAffinity, error) {
	if podSpec == nil {
		return nil, fmt.Errorf("pod.Spec is nil")
	}
	if podSpec.Affinity == nil {
		podSpec.Affinity = &v1.Affinity{}
	}
	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &v1.NodeAffinity{}
	}
	return podSpec.Affinity.NodeAffinity, nil
}

// setNodeAffinity sets the node affinity of the podSpec based on the provided nodeSelectorTerms and affinity type.
func setNodeAffinity(podSpec *v1.PodSpec, nodeSelectorTerms []v1.NodeSelectorTerm, affinity Affinity) error {

	if podSpec == nil {
		return fmt.Errorf("podSpec is nil")
	}
	if nodeSelectorTerms == nil || len(nodeSelectorTerms) == 0 {
		klog.Warning("nodeSelectorTerms is nil or empty")
	} else {
		for i, term := range nodeSelectorTerms {
			klog.Infof("nodeSelectorTerms[%d]: %+v", i, term)
		}
	}

	switch affinity {
	case Required:
		// Preserve existing required affinity terms and append new ones
		var requiredTerms []v1.NodeSelectorTerm

		if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity != nil &&
			podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			requiredTerms = append(requiredTerms,
				podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms...)
		}

		requiredTerms = append(requiredTerms, nodeSelectorTerms...)

		if podSpec.Affinity == nil {
			podSpec.Affinity = &v1.Affinity{}
		}
		if podSpec.Affinity.NodeAffinity == nil {
			podSpec.Affinity.NodeAffinity = &v1.NodeAffinity{}
		}
		if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{}
		}
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = requiredTerms

	case Preferred:
		// Preserve existing preferred affinity terms and append new ones
		var preferredSchedulingTerms []v1.PreferredSchedulingTerm

		if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity != nil &&
			podSpec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
			preferredSchedulingTerms = append(preferredSchedulingTerms,
				podSpec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
		}

		for _, term := range nodeSelectorTerms {
			preferredSchedulingTerms = append(preferredSchedulingTerms, v1.PreferredSchedulingTerm{
				Weight:     defaultPreferredWeight,
				Preference: term,
			})
		}

		if podSpec.Affinity == nil {
			podSpec.Affinity = &v1.Affinity{}
		}
		if podSpec.Affinity.NodeAffinity == nil {
			podSpec.Affinity.NodeAffinity = &v1.NodeAffinity{}
		}
		podSpec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = preferredSchedulingTerms
	}
	return nil
}

// getResourceValues return the cpu resources associated with the last container of a POD.
func getVanillaResourceValues(state *common.State) int64 {
	cpuLimit := int64(0)
	cpuRequest := int64(0)
	lastIndex := -1
	for key, value := range state.Resources {
		items := strings.Split(key, "_")
		index, err := strconv.Atoi(items[0])
		if err != nil {
			klog.Errorf("Failed to convert: %v", err)
			return 0
		}
		if items[1] == "cpu" && index >= lastIndex {
			if items[2] == "requests" {
				cpuRequest = value
			} else if items[2] == "limits" {
				cpuLimit = value
				cpuRequest = value
			}
			lastIndex = index
		}
	}
	if cpuLimit >= cpuRequest {
		return cpuLimit
	}
	return cpuRequest
}
