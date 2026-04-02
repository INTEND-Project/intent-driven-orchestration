package profiling

import (
	"context"
	"fmt"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	appsV1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

// dummyTracerCpu allows us to control what information we give to the actuator.
type dummyTracerCPU struct{}

func (d dummyTracerCPU) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Fatalf("implement me")
}

// for model with lookup table (not current)
func (d dummyTracerCPU) GetEffect(_ string, _ string, profileName string,
	_ int, constructor func() interface{}) (interface{}, error) {
	if profileName == "default/blurb" || profileName == "default/p99" {
		return nil, fmt.Errorf("no model was found - retuning error")
	}

	tmp := constructor().(*CPUProfileEffect)
	// the values will affect the latency and the tests results
	//lookupTable synthetically populated
	tmp.LookupTable = map[int64][]IndividualCPUEffect{
		500: {
			{ID: 1, Latency: 120, Mean: 120, Sdt: 0},
			{ID: 2, Latency: 103, Mean: 103, Sdt: 0},
			{ID: 3, Latency: 100, Mean: 100, Sdt: 0},
			{ID: 4, Latency: 90, Mean: 90, Sdt: 0},
		},
		1000: {
			{ID: 1, Latency: 110, Mean: 110, Sdt: 0},
			{ID: 2, Latency: 101, Mean: 101, Sdt: 0},
			{ID: 3, Latency: 96, Mean: 96, Sdt: 0},
			{ID: 4, Latency: 85, Mean: 85, Sdt: 0},
		},
		1500: {
			{ID: 1, Latency: 90, Mean: 90, Sdt: 0},
			{ID: 2, Latency: 73, Mean: 73, Sdt: 0},
			{ID: 3, Latency: 70, Mean: 70, Sdt: 0},
			{ID: 4, Latency: 60, Mean: 60, Sdt: 0},
		},
	}
	return tmp, nil
}

// CPUProfileActuatorFixture represents a fixture for testing.
type CPUProfileActuatorFixture struct {
	test    *testing.T
	client  *fake.Clientset
	objects []runtime.Object
}

// newCPUProfileActuatorFixture initializes a new fixture for testing.
func newCPUProfileActuatorFixture(t *testing.T) *CPUProfileActuatorFixture {
	f := &CPUProfileActuatorFixture{}
	f.test = t
	return f
}

// newCPUProfileTestActuator initializes an actuator for testing.
func (f *CPUProfileActuatorFixture) newCPUProfileTestActuator(proactive bool) *CPUProfileActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	//cpuProfiles populated with Vanilla defined profiles
	cpuProfiles := []CPUProfile{
		{
			ID:         1,
			Name:       "standard",
			CPUManager: 0,
			Affinity:   1,
			Settings:   map[string]string{},
		},
		{
			ID:         2,
			Name:       "exclusive",
			CPUManager: 0,
			Affinity:   1,
			Settings: map[string]string{
				"cpu_manager_policy": "static",
			},
		},
		{
			ID:         3,
			Name:       "numa_exclusive",
			CPUManager: 0,
			Affinity:   1,
			Settings: map[string]string{
				"topology_policy": "single-numa-node",
			},
		},
		{
			ID:         4,
			Name:       "pexclusive",
			CPUManager: 0,
			Affinity:   1,
			Settings: map[string]string{
				"full_pcpu_only": "true",
			},
		},
	}

	cfg := CPUProfileConfig{
		Interpreter: "python3",
		Analytics:   "test_analyze.py",
		Prediction:  "test_predict.py",
	
		CPUMax:      10000,
		CPUProfiles: cpuProfiles,
	}
	if proactive {
		klog.Fatalf("proactive actions are not yet implemented")
	}
	actuator := NewCPUProfileActuator(f.client, dummyTracerCPU{}, cfg)

	return actuator
}

// getInt32Pointer return a ref to an int32.
func getInt32Pointer(value int32) *int32 {
	val := value
	return &val
}

// createDeployment instantiates deployment workload for testing.
func createDeployment() runtime.Object {
	return &appsV1.Deployment{
		TypeMeta: metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			Annotations: map[string]string{ // annotations at the deployment level
				CPUProfileKey: "1",
			},
		},
		Spec: appsV1.DeploymentSpec{
			Replicas: getInt32Pointer(1),
			Selector: &metaV1.LabelSelector{MatchLabels: map[string]string{
				"app": "my-function"}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						CPUProfileKey: "1", // annotation at the pod level
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "my-function",
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("2000m"),
									v1.ResourceMemory: {}},
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("1000m"),
									v1.ResourceMemory: {}}},
						},
						{Name: "my-function-2",
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("123m"),
									v1.ResourceMemory: {}}},
						},
					},
				},
			},
		},
	}
}

func createReplicaSet() runtime.Object {
	return &appsV1.ReplicaSet{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-replicaset",
			Namespace: "default",
			Annotations: map[string]string{ // annotation at RS level
				CPUProfileKey: "1",
			},
		},
		Spec: appsV1.ReplicaSetSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						CPUProfileKey: "1", // annotation at pod level
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "pod0123",
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU: resource.MustParse("200"),
								},
							},
						},
					},
				},
			},
		},
	}
}

// Tests for success.

// TestCPUProfileNextStateForSuccess tests for success.
func TestCPUProfileNextStateForSuccess(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		ActivelyManaged bool
		Objectives map[string]float64
		Tolerations map[string]float64
	}{
		Key:        "default/my-objective",
		Priority:   1.0,
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: map[string]float64{"p99": 100.0},
	},
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
				QoSClass:     "Guaranteed",
			},
		},
		Resources: map[string]int64{
			"1_cpu_requests": 1000,
			"1_cpu_limits":   1000,
		},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{"p99": 90.0}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator.NextState(&state, &goal, profiles)
}

// TestCPUProfilePerformForSuccess tests for success.
func TestCPUProfilePerformForSuccess(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	f.objects = append(f.objects, createDeployment())
	actuator := f.newCPUProfileTestActuator(false)
	s0 := common.State{
		Intent: common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		Resources: map[string]int64{
			"1_cpu_limits": 100,
		},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	s0.Intent.TargetKind = "Deployment"
	plan := []planner.Action{{Name: actionName, Properties: map[string]string{"id": "2"}}}
	actuator.Perform(&s0, plan)
}

// TestCPUProfileEffectForSuccess tests for success.
func TestCPUProfileEffectForSuccess(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)
	s0 := common.State{}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator.Effect(&s0, profiles)
}

// TestCPUProfileGetResourcesForSuccess tests for success.
func TestCPUProfileGetResourcesForSuccess(_ *testing.T) {
	s0 := common.State{Resources: map[string]int64{}}
	getResourceValues(&s0, Vanilla)
}

// TestCPUProfileGetCPUProfileForSuccess tests for success.
func TestCPUProfileGetCPUProfileForSuccess(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)
	state := common.State{
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
			},
		},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	CProfile, _ := actuator.getCPUProfile(&state)
	if CProfile == nil {
		t.Error()
	}
}

// Tests for failure.

// TestCPUProfileNextStateForFailure tests for failure.
func TestCPUProfileNextStateForFailure(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)

	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			ActivelyManaged bool
			Objectives map[string]float64
			Tolerations map[string]float64
		}{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"p99": 10.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
				QoSClass:     "Guaranteed",
			},
		},
		Resources: map[string]int64{
			"1_cpu_requests": 100,
			"1_cpu_limits":   100,
		},
		Annotations: map[string]string{
			CPUProfileKey: "11",
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{
		"default/p99":          6.0,
		"default/rps":          0.0,
		"default/availability": 0.999,
	}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency")},
	}

	// CPUProfile ID out of range
	states, _, _ := actuator.NextState(&state, &goal, profiles)
	if states != nil {
		t.Errorf("Expected empty results set as CPUProfile doesnt match any ID in the config. - got: %v", states)
	}
	podState := state.CurrentPods["pod0"]
	state.Annotations[CPUProfileKey] = "1"
	state.CurrentPods["pod0"] = podState

	// no data in knowledge base.
	profiles["default/throughput"] = common.Profile{ProfileType: common.ProfileTypeFromText("throughput")}
	profiles["default/blurb"] = common.Profile{ProfileType: common.ProfileTypeFromText("latency")}
	state.Intent.Objectives["default/blurb"] = 42.0
	state.Intent.Objectives["default/throughput"] = 200.0
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results set as knowledge base is corrupt/empty. - got: %v", states)
	}
	delete(profiles, "default/throughput")
	delete(profiles, "default/blurb")
	delete(state.Intent.Objectives, "default/throughput")
	delete(state.Intent.Objectives, "default/blurb")

	profiles["p99"] = common.Profile{ProfileType: common.ProfileTypeFromText("latency")}
	goal.Intent.Objectives = map[string]float64{
		"p99": 90.0,
	}
	// negative resource limit.
	state.Resources = map[string]int64{
		"1_cpu_limits": -100,
	}
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results - got: %v", states)
	}

	// too high resource limit
	state.Resources = map[string]int64{
		"1_cpu_limits": 100000,
	}
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results  - got: %v", states)
	}

	//TODO: more cpus than max for isolated CPU profile
}

// TestCPUProfilePerformForFailure tests for failure.
func TestCPUProfilePerformForFailure(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	f.objects = []runtime.Object{}
	actuator := f.newCPUProfileTestActuator(false)

	// deployment does not exist!
	s0 := common.State{
		Intent: common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	plan := []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "2"}},
	}
	actuator.Perform(&s0, plan)

	expectedActions := []string{"get"}
	fmt.Println(f.client.Actions())
	if len(f.client.Actions()) != len(expectedActions) {
		t.Errorf("this should not happen - should be equal length.")
		return
	}
	for i, item := range expectedActions {
		if f.client.Actions()[i].GetVerb() != item {
			t.Errorf("Expected: %s - got: %s.", item, f.client.Actions()[i].GetVerb())
		}

	}

	// replicaset does not exist!
	f.client.ClearActions()
	s1 := common.State{
		Intent: common.Intent{TargetKey: "default/my-rs", TargetKind: "ReplicaSet"},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "2"}},
	}
	actuator.Perform(&s1, plan)
	expectedActions = []string{"get"}

	if len(f.client.Actions()) != len(expectedActions) {
		t.Errorf("this should not happen - should be equal length.")
		return
	}
	for i, item := range expectedActions {
		if f.client.Actions()[i].GetVerb() != item {
			t.Errorf("Expected: %s - got: %s.", item, f.client.Actions()[i].GetVerb())
		}
	}

	// plan property is invalid.
	f.client.ClearActions()
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]string{"profile": "5"}},
	}
	actuator.Perform(&s0, plan)
	if len(f.client.Actions()) != 0 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}

}

// TestCPUProfileGetResourcesForFailure tests for failure
func TestCPUProfileGetResourcesForFailure(t *testing.T) {
	s0 := common.State{Resources: map[string]int64{"a_cpu_limits": 100}}
	res := getResourceValues(&s0, Vanilla)
	if res != 0 {
		t.Errorf("Should have been 0 - was: %d.", res)
	}
}

// TestCPUProfileCPUProfileForFailure tests for failure
func TestCPUProfileGetCPUProfileForFailure(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)
	state := common.State{
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
			},
		},
		Annotations: map[string]string{
			CPUProfileKey: "99",
		},
	}
	CProfile, _ := actuator.getCPUProfile(&state)
	if CProfile != nil {
		t.Errorf("Should have been nil - was: %v.", CProfile)
	}
}

// Tests for sanity.

// TestCPUProfileNextStateForSanity tests for sanity.
func TestCPUProfileNextStateForSanity(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	actuator := f.newCPUProfileTestActuator(false)

	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			ActivelyManaged bool
			Objectives map[string]float64
			Tolerations map[string]float64
		}{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"p99": 60.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
				QoSClass:     "Guaranteed",
			},
		},
		Resources: map[string]int64{
			"1_cpu_limits":   1500,
			"1_cpu_requests": 1500,
		},
		CurrentData: make(map[string]map[string]float64),
		Annotations: map[string]string{
			CPUProfileKey: "4",
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"p99": 65.0}
	profiles := map[string]common.Profile{
		"p99": {ProfileType: common.ProfileTypeFromText("latency")},
		"p95": {ProfileType: common.ProfileTypeFromText("latency")},
	}

	// if pod is not in Guarantted QoSClass
	_, _, actions := actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should be empty, was: %v.", actions)
	}

	podState := state.CurrentPods["pod0"]
	podState.QoSClass = "Guaranteed"
	state.CurrentPods["pod0"] = podState

	// if me meet the goal & using the most isolated cpu profile -> do nothing -- checked in perform function
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should be empty, was: %v.", actions)
	}

	// if we've already looked at cpu profiling in this planning cycle, skip...
	goal.Intent.Objectives["p99"] = 30
	state.CurrentData[actionName] = map[string]float64{"actionName": 1}
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should be empty, was: %v.", actions)
	}

	// now this should work...
	state.Intent.Objectives["p99"] = 100
	state.Annotations[CPUProfileKey] = "1"
	goal.Intent.Objectives["p99"] = 65
	delete(state.CurrentData, actionName)
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	fmt.Printf("Objectives in state: %+v\n", state.Intent.Objectives)
	if len(actions) < 1 || actions[0].Properties.(map[string]string)["id"] != "4" {
		t.Errorf("Expected one action to set cpu profile id to 4 - got: %v", actions)
	}

	// if we are better than goal -> switch to a lower profile Id.
	state.Intent.Objectives["p99"] = 60.0
	goal.Intent.Objectives["p99"] = 75.0
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	found := false
	for _, action := range actions {
		if id, ok := action.Properties.(map[string]int)["id"]; ok && id == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected at least one action to set cpu profile id to 2 - got: %v", actions)
	}
	// multiple objectives (conflicts - requiring different cpu profiles)
	state.Intent.Objectives["p95"] = 120
	goal.Intent.Objectives["p95"] = 60
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) < 1 || actions[0].Properties.(map[string]int)["id"] != 4 {
		t.Errorf("Expected one action to set cpu profile id to 4 - got: %v", actions)
	}

	// too strict of a goal.
	goal.Intent.Objectives["p99"] = 1.0
	states, utilities, actions := actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Result sets should be empty: %v, %v, %v.", states, utilities, actions)
	}


	// ensure an "empty" state does not crash the actuator.
	actuator = f.newCPUProfileTestActuator(false)
	delete(goal.Intent.Objectives, "p95")
	goal.Intent.Objectives["p99"] = 120
	emptyState := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			ActivelyManaged bool
			Objectives map[string]float64
			Tolerations map[string]float64
		}{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"p99": 250.0},
		},
	}
	_, _, actions = actuator.NextState(&emptyState, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should contain no action; was: %v", actions)
	}
}

// TestCPUProfilePerformForSanity tests for sanity.
func TestCPUProfilePerformForSanity(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	f.objects = append(f.objects, createDeployment())
	actuator := f.newCPUProfileTestActuator(false)

	// test for deployment.
	s0 := common.State{
		Intent: common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		Annotations: map[string]string{
			CPUProfileKey: "1",
		},
	}
	plan := []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "3"}},
	}
	actuator.Perform(&s0, plan)
	expectedActions := []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
	if len(expectedActions) != len(f.client.Actions()) {
		t.Errorf("Expecting action list to be equal length: %v, %v", expectedActions, f.client.Actions())
	}
	updatedObject, _ := f.client.AppsV1().Deployments("default").Get(context.TODO(), "my-deployment", metaV1.GetOptions{})
	res := updatedObject.Spec.Template.ObjectMeta.Annotations
	val, ok := res[CPUProfileKey]
	if val != "3" || !ok {
		t.Errorf("Request should have been 3; was: %v", val)
	}
	f.client.ClearActions()

	// planned cpu profile same as current one
	s0.Annotations[CPUProfileKey] = "4"
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "4"}},
	}
	actuator.Perform(&s0, plan)
	for _, action := range f.client.Actions() {
		if len(action.GetVerb()) != 0 {
			t.Errorf("Expected no action - got %s.", action.GetVerb())
		}
	}
	f.client.ClearActions()

	// test for replicaset.
	f.client.ClearActions()
	f.objects = []runtime.Object{createReplicaSet()}
	actuator = f.newCPUProfileTestActuator(false)
	s0.Intent.TargetKey = "default/my-replicaset"
	s0.Intent.TargetKind = "ReplicaSet"
	s0.Annotations[CPUProfileKey] = "1"
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "3"}},
	}
	actuator.Perform(&s0, plan)
	expectedActions = []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
	if len(expectedActions) != len(f.client.Actions()) {
		t.Errorf("Expecting action list to be equal length: %v, %v", expectedActions, f.client.Actions())
	}
	updatedRS, _ := f.client.AppsV1().ReplicaSets("default").Get(context.TODO(), "my-replicaset", metaV1.GetOptions{})
	//res = updatedRS.Spec.Template.Annotations
	res = updatedRS.Spec.Template.ObjectMeta.Annotations
	val, ok = res[CPUProfileKey]
	if val != "3" || !ok {
		t.Errorf("Request should have been 3; was: %v", val)
	}
	f.client.ClearActions()

	// planned cpu profile same as current one
	s0.Annotations[CPUProfileKey] = "4"
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]string{"id": "4"}},
	}
	actuator.Perform(&s0, plan)
	for _, action := range f.client.Actions() {
		if len(action.GetVerb()) != 0 {
			t.Errorf("Expected no action - got %s.", action.GetVerb())
		}
	}
}

// TestCPUProfileEffectForSanity tests for sanity.
func TestCPUProfileEffectForSanity(t *testing.T) {
	f := newCPUProfileActuatorFixture(t)
	// this will "just" trigger a python script.
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		ActivelyManaged bool
		Objectives map[string]float64
		Tolerations map[string]float64
	}{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment",
		TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0}}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator := f.newCPUProfileTestActuator(false)
	actuator.Effect(&state, profiles)

	// check with None.
	actuator.cfg.Analytics = "None"
	actuator.Effect(&state, profiles)
}

// TestCPUProfileGetResourcesForSuccess tests for sanity.
func TestCPUProfileGetResourcesForSanity(t *testing.T) {
	s0 := common.State{Resources: map[string]int64{}}
	res := getResourceValues(&s0, Vanilla)
	if res != 0 {
		t.Errorf("Should have been 0 - was: %v", res)
	}

	// request defined.
	s0.Resources["0_cpu_requests"] = 200
	res = getResourceValues(&s0, Vanilla)
	if res != 200 {
		t.Errorf("Should have been 200 - was: %v", res)
	}
	// limits defined.
	s0.Resources["0_cpu_limits"] = 400
	res = getResourceValues(&s0, Vanilla)
	if res != 400 {
		t.Errorf("Should have been 400 - was: %v", res)
	}
	// the last container matters.
	s0.Resources["1_cpu_limits"] = 100
	res = getResourceValues(&s0, Vanilla)
	if res != 100 {
		t.Errorf("Should have been 100 - was: %v", res)
	}
}
