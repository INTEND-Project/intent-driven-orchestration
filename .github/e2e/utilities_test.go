//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// listCrds lists all CRD from kubernetes server
func listCrds() (items []string, err error) {
	// defer so we don't need to check every interface{} conversion
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()

	res, err := kubeClient.RESTClient().Get().Prefix("apis").Resource("apiextensions.k8s.io").Name("v1").SubResource("customresourcedefinitions").DoRaw(context.TODO())
	if err != nil {
		return
	}
	var result map[string]interface{}
	err = json.Unmarshal(res, &result)
	if err != nil {
		return
	}
	if result["kind"].(string) != "CustomResourceDefinitionList" {
		err = errors.New("Invalid response")
		return
	}

	for _, item := range result["items"].([]interface{}) {
		metadata := item.(map[string]interface{})["metadata"]
		items = append(items, metadata.(map[string]interface{})["name"].(string))
	}
	return
}

func parsePrometheusResp(response []byte) (resp float64, err error) {
	// defer so we don't need to check every interface{} conversion
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return
	}
	val := result["data"].(map[string]interface{})["result"].([]interface{})[0].(map[string]interface{})["value"].([]interface{})[1].(string)
	if len(val) == 0 {
		return
	}
	resp, err = strconv.ParseFloat(val, 64)
	return
}

// serviceGet queries given service http endpoint by building proper kubernetes proxy url
func serviceGet(namespace, service string, port int, url, getParamName, getParamValue string) ([]byte, error) {
	request := kubeClient.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Resource("services").
		SubResource(fmt.Sprintf("%s:%d", service, port)).
		Suffix("proxy", url).Param(getParamName, getParamValue)
	res, err := request.DoRaw(context.TODO())
	return res, err
}

// getQuery returns query string configured for a given query in a planner queries json file.
func getQuery(plannerQueriesContent, queryName string) (string, error) {
	var data map[string]map[string]string
	err := json.Unmarshal([]byte(plannerQueriesContent), &data)
	if err != nil {
		return "", err
	}
	query, ok := data[queryName]["query"]
	if !ok {
		return "", fmt.Errorf("cannot find query %s", queryName)
	}
	return query, nil
}

func createPod(podSpec *v1.Pod) func() {
	By(fmt.Sprintf("Creating pod %s", podSpec.Name))
	podClient := kubeClient.CoreV1().Pods(TestNamespace)
	_, err := podClient.Create(context.TODO(), podSpec, metav1.CreateOptions{})
	Expect(err).To(BeNil())
	Eventually(func() bool {
		p, err := podClient.Get(context.TODO(), podSpec.Name, metav1.GetOptions{})
		if err == nil {
			return p.Status.Phase == v1.PodRunning && isAllContainersReady(p)
		}
		return false
	}).
		WithTimeout(time.Minute).
		WithPolling(time.Second).
		Should(BeTrue(), "Pod %s not ready", podSpec.Name)

	return func() {
		By(fmt.Sprintf("Deleting pod %s", podSpec.Name))
		err := podClient.Delete(context.TODO(), podSpec.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			_, err := podClient.Get(context.TODO(), podSpec.Name, metav1.GetOptions{})
			return err != nil
		}).
			WithTimeout(time.Minute).
			WithPolling(time.Second).
			Should(BeTrue(), "Pod %s should be deleted", podSpec.Name)
	}
}

func createDeployment(deploymentSpec *appsv1.Deployment) func() {
	By(fmt.Sprintf("Creating deployment %s", deploymentSpec.Name))
	deploymentClient := kubeClient.AppsV1().Deployments(TestNamespace)
	_, err := deploymentClient.Create(context.TODO(), deploymentSpec, metav1.CreateOptions{})
	Expect(err).To(BeNil())
	Eventually(func() bool {
		p, err := deploymentClient.Get(context.TODO(), deploymentSpec.Name, metav1.GetOptions{})
		if err == nil {
			return p.Status.UnavailableReplicas == 0 && p.Status.Replicas > 0
		}
		return false
	}).
		WithTimeout(time.Minute).
		WithPolling(time.Second).
		Should(BeTrue(), "Deployment %s not ready", deploymentSpec.Name)

	return func() {
		By(fmt.Sprintf("Deleting deployment %s", deploymentSpec.Name))
		err := deploymentClient.Delete(context.TODO(), deploymentSpec.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			_, err := deploymentClient.Get(context.TODO(), deploymentSpec.Name, metav1.GetOptions{})
			return err != nil
		}).
			WithTimeout(time.Minute).
			WithPolling(time.Second).
			Should(BeTrue(), "Pod %s should be deleted", deploymentSpec.Name)
	}
}

func createService(serviceSpec *v1.Service) func() {
	By(fmt.Sprintf("Creating service %s", serviceSpec.Name))
	serviceClient := kubeClient.CoreV1().Services(TestNamespace)
	_, err := serviceClient.Create(context.TODO(), serviceSpec, metav1.CreateOptions{})
	Expect(err).To(BeNil())
	Eventually(func() bool {
		_, err := serviceClient.Get(context.TODO(), serviceSpec.Name, metav1.GetOptions{})
		return err == nil
	}).
		WithTimeout(time.Minute).
		WithPolling(time.Second).
		Should(BeTrue(), "Service %s not ready", serviceSpec.Name)

	return func() {
		By(fmt.Sprintf("Deleting service %s", serviceSpec.Name))
		err := serviceClient.Delete(context.TODO(), serviceSpec.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			_, err := serviceClient.Get(context.TODO(), serviceSpec.Name, metav1.GetOptions{})
			return err != nil
		}).
			WithTimeout(time.Minute).
			WithPolling(time.Second).
			Should(BeTrue(), "Pod %s should be deleted", serviceSpec.Name)
	}
}

func createIntent(intentSpec *v1alpha1.Intent) func() {
	By(fmt.Sprintf("Creating intent %s", intentSpec.Name))
	client := crdClient.IdoV1alpha1().Intents(TestNamespace)

	_, err := client.Create(context.TODO(), intentSpec, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	return func() {
		By(fmt.Sprintf("Deleting intent %s", intentSpec.Name))
		err := client.Delete(context.TODO(), intentSpec.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
	}
}

func createConfigMap(configMapSpec *v1.ConfigMap) func() {
	By(fmt.Sprintf("Creating config map %s", configMapSpec.Name))
	configMapClient := kubeClient.CoreV1().ConfigMaps(TestNamespace)
	_, err := configMapClient.Create(context.TODO(), configMapSpec, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	return func() {
		By(fmt.Sprintf("Deleting config map %s", configMapSpec.Name))
		err := configMapClient.Delete(context.TODO(), configMapSpec.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
	}
}

func scaleDeployment(name string, numReplicas int32) {
	deploymentClient := kubeClient.AppsV1().Deployments(TestNamespace)
	Eventually(func() bool {
		scale, err := deploymentClient.GetScale(context.TODO(), name, metav1.GetOptions{})
		Expect(err).To(BeNil())
		if scale.Spec.Replicas == numReplicas {
			return true
		}
		nscale := *scale
		nscale.Spec.Replicas = numReplicas
		_, _ = deploymentClient.UpdateScale(context.TODO(), name, &nscale, metav1.UpdateOptions{})
		return false
	}).
		WithPolling(5 * time.Second).
		WithTimeout(time.Minute).
		Should(BeTrue())
}

func waitUntilScaleBetween(name string, min int, max int, timeout time.Duration) {
	By(fmt.Sprintf("Checking if deployment %s will change num of pods in next %v", name, timeout))
	client := kubeClient.AppsV1().Deployments(TestNamespace)
	lastScale := -1
	Eventually(func() int {
		scale, err := client.GetScale(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return -1
		}
		scaleVal := int(scale.Status.Replicas)
		if scaleVal != lastScale {
			GinkgoWriter.Printf("-- Current number of replicas of %s: %d\n", name, scaleVal)
			lastScale = scaleVal
		}
		return scaleVal
	}).
		WithPolling(10 * time.Second).
		WithTimeout(timeout).
		Should(And(BeNumerically(">=", min), BeNumerically("<=", max)))
}

func getResource(deploymentName, resourceType string) int64 {
	deploymentClient := kubeClient.AppsV1().Deployments(TestNamespace)
	deployList, err := deploymentClient.List(context.TODO(), metav1.ListOptions{})
	Expect(err).To(BeNil())
	for _, deployItem := range deployList.Items {
		if deployItem.Spec.Template.Labels["app"] == "function" {
			for _, containerItem := range deployItem.Spec.Template.Spec.Containers {
				if containerItem.Name == "function" {
					cpuMilli := containerItem.Resources.Limits.Cpu().MilliValue()
					By(fmt.Sprintf("... %s resource is %d", resourceType, cpuMilli))
					return cpuMilli
				}
			}
		}
	}
	return 0
}

func scaleResource(deploymentName, resourceType string, milliCPU int64) {
	deploymentClient := kubeClient.AppsV1().Deployments(TestNamespace)
	Eventually(func() bool {
		deployList, err := deploymentClient.List(context.TODO(), metav1.ListOptions{})
		Expect(err).To(BeNil())
		for _, deployItem := range deployList.Items {
			if deployItem.Spec.Template.Labels["app"] == "function" {
				for _, containerItem := range deployItem.Spec.Template.Spec.Containers {
					if containerItem.Resources.Limits.Cpu().MilliValue() == milliCPU {
						return true
					}
					// change the cpu limits in deployment file
					data := fmt.Sprintf(`[{ "op": "replace", "path": "/spec/template/spec/containers/0/resources/requests/%s", "value": "%sm" }]`, resourceType, strconv.FormatInt(milliCPU, 10))
					_, err := deploymentClient.Patch(context.TODO(), deployItem.Name, types.JSONPatchType, []byte(data), metav1.PatchOptions{})
					Expect(err).To(BeNil())

					data = fmt.Sprintf(`[{ "op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/%s", "value": "%sm" }]`, resourceType, strconv.FormatInt(milliCPU, 10))
					_, err = deploymentClient.Patch(context.TODO(), deployItem.Name, types.JSONPatchType, []byte(data), metav1.PatchOptions{})
					Expect(err).To(BeNil())
					return true
				}
			}
		}
		return false
	}).
		WithPolling(5 * time.Second).
		WithTimeout(time.Minute).
		Should(BeTrue())
}

// this function will assert if the resource value is within the expect range [min,max]
// delta is extra adjustment that can be used to short or enlarge the range.
// note that if the scaling up is asserted the min arg will take the current cpu value and
// max the maximum value set for the entire test. OTOH, for the scaling up test,
// min arg will be 1, the max will be the current value.
func waitUntilVerticalScaleBetween(deploymentName, resourceType string, min, max, delta int64, timeout time.Duration) {
	By(fmt.Sprintf("Checking if %s in %s will be updated in next %v", resourceType, deploymentName, timeout))
	// TODO: refactor for memory
	lastCpuScale := int64(-1)
	var cpuScaleVal, lastCurrent int64
	if min != 1 { // this setup for scaling up test
		lastCurrent = min
		min += delta // this set the lower limit
	} else {
		lastCurrent = max
		max -= delta // this set the upper limit
	}
	Eventually(func() int64 {
		cpuScaleVal = getResource(deploymentName, resourceType)
		lastCpuScale = lastCurrent
		if cpuScaleVal != lastCpuScale {
			lastCpuScale = cpuScaleVal
		}
		return cpuScaleVal
	}).
		// Test pass if the return cpuScaleVal >= min and <= max
		// note that min and max may be redefined by the delta value accordingly to the test type:
		// in scaling up, it checks if cpuscaleVal >= min (base cpu value + delta) and <= than max (not redefined)
		// in scaling down, it checks if cpuscaleVal <= max (base cpu value - delta) and >= min (1, not redefined)
		WithPolling(10 * time.Second).
		WithTimeout(timeout).
		Should(And(BeNumerically(">=", min), BeNumerically("<=", max)))
}

func waitUntilValuesBetween(min, max float64, variable string, timeout time.Duration) {
	By(fmt.Sprintf("Checking if %s to be set %.0f is lower than %.0f the current %s during %v", variable, min, max, variable, timeout))

	Eventually(func() bool {
		if min < max {
			return true
		}
		return false
	}).
		WithPolling(10 * time.Second).
		WithTimeout(timeout).
		Should(BeTrue())
}

func isAllContainersReady(pod *v1.Pod) bool {
	for _, cont := range pod.Status.ContainerStatuses {
		if !cont.Ready {
			return false
		}
	}
	return true
}
