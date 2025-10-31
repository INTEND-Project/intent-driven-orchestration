//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"flag"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	genClient "github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned"
)

const TestNamespace = "test-ns"

var (
	registry    string = "127.0.0.1:5000/"
	imageTag    string = "e2e"
	kubeConfig  string
	sleepTime   time.Duration = 30 * time.Second
	timeoutTime time.Duration = 6 * time.Minute
	kubeClient  *kubernetes.Clientset
	crdClient   *genClient.Clientset
)

func init() {
	flag.StringVar(&kubeConfig, "kubeconfig", kubeConfig, "absolute path to the kubeconfig file")
	flag.StringVar(&imageTag, "imagetag", imageTag, "tag used by planner and plugin images")
	flag.StringVar(&registry, "registry", registry, "registry used by planner and plugin images")
	flag.DurationVar(&sleepTime, "sleep", sleepTime, "time for sleep between pod scaling actions")
	flag.DurationVar(&timeoutTime, "timeout", timeoutTime, "time for timeout waiting for actuator action")
}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IDO E2E Tests")
}

// This section verifies if all the preliminary apis and instances need to run the
// planner/actuators behavior are installed and configured properly.
var _ = BeforeSuite(func() {
	By("Create k8s configuration")
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	Expect(err).To(BeNil())
	kubeClient, err = kubernetes.NewForConfig(config)
	Expect(err).To(BeNil())
	crdClient, err = genClient.NewForConfig(config)
	Expect(err).To(BeNil())

	By("Check if k8s adheres to IDO environment requirements")
	crds, err := listCrds()
	Expect(err).To(BeNil())

	By("Check if IDOs CRDs are installed")
	Expect(crds).To(
		ContainElements("intents.ido.intel.com", "kpiprofiles.ido.intel.com"),
		"IDO requires custom resource definitions to be installed. "+
			"Do so by invoking kubectl apply -f artefacts/intents_crds_v1alpha1.yaml",
	)

	By("Check if linkerd is installed")
	Expect(crds).To(
		ContainElement(ContainSubstring(".linkerd.io")),
		"IDO e2e requires linkerd to be installed. "+
			"Check instruction on https://linkerd.io/",
	)

	By("Check if linkerd-viz is installed")
	podList, err := kubeClient.CoreV1().Pods("linkerd-viz").List(context.TODO(), v1.ListOptions{})
	Expect(err).To(BeNil())
	Expect(podList.Items).To(
		ContainElement(HaveField("Name", ContainSubstring("prometheus"))),
		"IDO e2e requires linkerd-viz with prometheus to be installed",
	)

	By("Checking if test namespace is present")
	_, err = kubeClient.CoreV1().Namespaces().Get(context.TODO(), TestNamespace, v1.GetOptions{})
	Expect(err).To(BeNil(), "Test namespace should be created")

	By("Checking if the IDO Service Account is present")
	_, err = kubeClient.CoreV1().ServiceAccounts(TestNamespace).Get(context.TODO(), "planner-service-account", metav1.GetOptions{})
	Expect(err).To(BeNil(), "Service account `planner-service-account` is not present. See e2e-deployment.yaml")

	By("Checking if the IDO services are present")
	for _, service := range []string{"plugin-manager-service", "planner-mongodb-service"} {
		_, err = kubeClient.CoreV1().Services(TestNamespace).Get(context.TODO(), service, metav1.GetOptions{})
		Expect(err).To(BeNil(), "Service %s needs to be deployed. See e2e-deployment.yaml", service)
	}
})
