//go:build e2e
// +build e2e

package e2e

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
)

func create(i int64) *int64 {
	return &i
}

func answer(i bool) *bool {
	return &i
}

// GetPlannerSpec returns planner pod spec, with config read from planner-configmap and
// planner-queries-configmap connected to mongodb on planner-mongodb-service:27017
func GetPlannerSpec(registry, imageTag string) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "planner",
			Labels: map[string]string{
				"name": "planner",
			},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: "planner-service-account",
			Volumes: []v1.Volume{
				{Name: "planner-config",
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{
							LocalObjectReference: v1.LocalObjectReference{Name: "planner-configmap"},
							Items:                []v1.KeyToPath{{Key: "defaults.json", Path: "defaults.json"}},
						},
					},
				},
				{Name: "planner-queries",
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{
							LocalObjectReference: v1.LocalObjectReference{Name: "planner-queries-configmap"},
							Items:                []v1.KeyToPath{{Key: "default_queries.json", Path: "default_queries.json"}},
						},
					},
				},
			},
			Containers: []v1.Container{
				{Name: "planner",
					Image: fmt.Sprintf("%splanner:%s", registry, imageTag),
					Ports: []v1.ContainerPort{{ContainerPort: 33333}},
					Args:  []string{"-config", "/config/defaults.json", "-v", "2"},
					VolumeMounts: []v1.VolumeMount{
						{Name: "planner-config", MountPath: "/config/"},
						{Name: "planner-queries", MountPath: "/queries/"},
					},
					Env: []v1.EnvVar{
						{Name: "MONGO_URL", Value: "mongodb://planner-mongodb-service:27017/"},
					},
					SecurityContext: &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
						RunAsUser:                create(10001),
						RunAsGroup:               create(10001),
						RunAsNonRoot:             answer(true),
						ReadOnlyRootFilesystem:   answer(true),
						AllowPrivilegeEscalation: answer(false),
						SeccompProfile:           &v1.SeccompProfile{Type: "RuntimeDefault"},
					},
				},
			},
			Affinity: &v1.Affinity{NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{MatchExpressions: []v1.NodeSelectorRequirement{
							{Key: "node-role.kubernetes.io/control-plane", Operator: "Exists"}},
						},
					},
				},
			},
			},
			Tolerations: []v1.Toleration{
				{Key: "node-role.kubernetes.io/master", Operator: "Exists"},
				{Key: "node-role.kubernetes.io/control-plane", Operator: "Exists"},
			},
		},
	}
}

// GetMongoDBSpec returns mongodb pod spec
func GetMongoDBSpec() v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "planner-mongodb",
			Labels: map[string]string{
				"name": "planner-mongodb",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: "mongodb",
					Image: "mongo",
					Ports: []v1.ContainerPort{{ContainerPort: 27017}},
				},
			},
		},
	}
}

// GetWorkerDeploymentSpec returns the deployment spec of the function computation
func GetWorkerDeploymentSpec(numReplicas int32, millicpu int64) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "function-deployment"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "function"},
			},
			Replicas: &numReplicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "function"},
					Annotations: map[string]string{"linkerd.io/inject": "enabled"},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "function",
							Image: "testfunction/rust_function:0.1",
							Ports: []v1.ContainerPort{{ContainerPort: 8080}},
							Env:   []v1.EnvVar{{Name: "WORKERS", Value: "2"}},
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU: *resource.NewMilliQuantity(millicpu, resource.DecimalSI),
								},
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU: *resource.NewMilliQuantity(millicpu, resource.DecimalSI),
								},
							},
						},
					},
					RestartPolicy: v1.RestartPolicyAlways,
				},
			},
		},
	}
}

// GetWorkerServiceSpec returns service spec connected to deployment created by GetWorkerDeployment()
func GetWorkerServiceSpec() v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "function-service"},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "function"},
			Ports: []v1.ServicePort{
				{Protocol: v1.ProtocolTCP, Port: 8080, TargetPort: intstr.FromInt(8080)},
			},
		},
	}
}

// GetLoadGeneratorSpec returns the deployment spec of requests generator (client for worker deployment)
func GetLoadGeneratorSpec(numReplicas int32) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "function-loadgen"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "loadgen"},
			},
			Replicas: &numReplicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "loadgen"}},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "loadgen", Image: "devth/alpine-bench", Args: []string{"-r", "-n99999", "-c", "3", "http://function-service:8080/matrix/150"}},
					},
				},
			},
		},
	}
}

// GetIntentSpec returns intent spec with given p95latency value
func GetIntentSpec(p95latency float64) v1alpha1.Intent {
	return v1alpha1.Intent{
		ObjectMeta: metav1.ObjectMeta{Name: "my-intent"},
		Spec: v1alpha1.IntentSpec{
			TargetRef: v1alpha1.TargetRef{
				Kind: "Deployment",
				Name: TestNamespace + "/function-deployment",
			},
			Objectives: []v1alpha1.TargetObjective{
				{Name: "p95compliance", Value: p95latency, MeasuredBy: TestNamespace + "/p95latency"},
				{Name: "tput", Value: 0, MeasuredBy: TestNamespace + "/throughput"},
			},
			Priority: 1,
		},
	}
}

// GetPluginSpecs returns configmap, service and pod specs for given plugin.
func GetPluginSpecs(name string, image string, configuration string) (v1.ConfigMap, v1.Service, v1.Pod) {
	config := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-configmap"},
		Data: map[string]string{
			"defaults.json": configuration,
		},
	}
	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-service"},
		Spec: v1.ServiceSpec{
			ClusterIP: "",
			Selector:  map[string]string{"app": name},
			Ports:     []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 33334, TargetPort: intstr.FromInt(33334)}},
		},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"app": name}},
		Spec: v1.PodSpec{
			ServiceAccountName: "planner-service-account",
			Containers: []v1.Container{{
				Name:  name,
				Image: image,
				Args:  []string{"-config", "/config/defaults.json", "-v", "2"},
				Ports: []v1.ContainerPort{{ContainerPort: 33334}},
				VolumeMounts: []v1.VolumeMount{
					{Name: name + "-volume", MountPath: "/config/"},
					{Name: "matplotlib-tmp", MountPath: "/var/tmp"},
				},
				Env: []v1.EnvVar{
					{Name: "MONGO_URL", Value: "mongodb://planner-mongodb-service:27017/"},
					{Name: "MPLCONFIGDIR", Value: "/var/tmp"}},
				SecurityContext: &v1.SecurityContext{
					Capabilities: &v1.Capabilities{
						Drop: []v1.Capability{"ALL"},
					},
					RunAsUser:                create(10001),
					RunAsGroup:               create(10001),
					RunAsNonRoot:             answer(true),
					ReadOnlyRootFilesystem:   answer(true),
					AllowPrivilegeEscalation: answer(false),
					SeccompProfile:           &v1.SeccompProfile{Type: "RuntimeDefault"},
				},
			}},
			Volumes: []v1.Volume{
				{Name: name + "-volume",
					VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{Name: name + "-configmap"},
						Items: []v1.KeyToPath{
							{Key: "defaults.json", Path: "defaults.json"},
						}},
					},
				},
				{Name: "matplotlib-tmp",
					VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
				},
			},
			Affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{MatchExpressions: []v1.NodeSelectorRequirement{
								{Key: "node-role.kubernetes.io/control-plane",
									Operator: "Exists",
								}},
							},
						},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{Key: "node-role.kubernetes.io/master", Operator: "Exists"},
				{Key: "node-role.kubernetes.io/control-plane", Operator: "Exists"},
			},
		},
	}

	return config, service, pod
}
