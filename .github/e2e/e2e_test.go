//go:build e2e
// +build e2e

package e2e

import (
	_ "embed"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

//go:embed data/default_planner_config.json
var defaultPlannerConfiguration string

//go:embed data/default_planner_queries.json
var defaultPlannerQueries string

//go:embed data/default_scaleout_config.json
var defaultScaleoutConfig string

//go:embed data/default_rmpod_config.json
var defaultRmpodConfig string

//go:embed data/default_cpuscale_config.json
var defaultCPUscaleoutConfig string

type latencyData struct {
	numWorkers    int
	numGenerators int
}

const (
	initialNumWorkers        = 1
	maxNumWorkers            = 5
	initialNumLoadGenerators = 1
	maxNumLoadGenerators     = 2
	minimumLatency           = 300.0
	maximumLatency           = 900.0
	millicpuDefault          = 1400
	millicpuMax              = 4000
)

var (
	runHorizontalScale    = true
	runCPUwithTraining    = true
	runCPUwithoutTraining = false
)

var maximumEnvLatency float64 // up limit for the latency value in the test suite

var _ = Describe("Can deploy planner component", Serial, func() {
	var (
		configMaps  []v1.ConfigMap
		pods        []v1.Pod
		deployments map[string]appsv1.Deployment
		services    []v1.Service
	)

	JustBeforeEach(func() {
		deleters := []func(){}

		for _, configMap := range configMaps {
			configMap := configMap
			deleters = append(deleters, createConfigMap(&configMap))
		}

		for _, service := range services {
			service := service
			deleters = append(deleters, createService(&service))
		}

		for _, pod := range pods {
			pod := pod
			deleters = append(deleters, createPod(&pod))
		}

		for _, deployment := range deployments {
			deployment := deployment
			deleters = append(deleters, createDeployment(&deployment))
		}

		DeferCleanup(func() {
			for _, deleter := range deleters {
				deleter()
			}
		})
	})

	// setting the context for the instances to be use in the tests
	Context("with default planner configuration", func() {
		BeforeEach(func() {
			configMaps = []v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "planner-configmap"},
					Data:       map[string]string{"defaults.json": defaultPlannerConfiguration},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "planner-queries-configmap"},
					Data:       map[string]string{"default_queries.json": defaultPlannerQueries},
				},
			}

			pods = []v1.Pod{GetMongoDBSpec(), GetPlannerSpec(registry, imageTag)}
			deployments = map[string]appsv1.Deployment{
				"worker":  GetWorkerDeploymentSpec(int32(initialNumWorkers), millicpuDefault),
				"loadgen": GetLoadGeneratorSpec(int32(initialNumLoadGenerators)),
			}
			services = []v1.Service{GetWorkerServiceSpec()}
		})

		// start horizontal scaling tests
		// creates all the configmaps, services, and initiate PODs and
		// deployments for the Planner, planner-db, example (demo) deployments.
		// Do the checks in each steps for scaleout and rmpod grpc plugins deployment.

		Context("with scaleout and rmpod grpc plugins", func() {
			BeforeEach(func() {
				// allow control from an external flag
				if !runHorizontalScale {
					Skip("Skip to run horizontal scale.")
				}

				scConfigMap, scSvc, scPod := GetPluginSpecs("scaleout-actuator", registry+"scaleout:"+imageTag, defaultScaleoutConfig)
				rmConfigMap, rmSvc, rmPod := GetPluginSpecs("rmpod-actuator", registry+"rmpod:"+imageTag, defaultRmpodConfig)

				configMaps = append(configMaps, scConfigMap, rmConfigMap)
				services = append(services, scSvc, rmSvc)
				pods = append(pods, scPod, rmPod)
			})
			// start the verification of the steps during a model training
			// perform and assert all the steps on the KPI queries and return values.
			It("Works after training", func() {
				queryText, err := getQuery(defaultPlannerQueries, TestNamespace+"/p95latency")
				workerName := deployments["worker"].Name
				generatorName := deployments["loadgen"].Name
				Expect(err).To(BeNil())
				getLatency := func() float64 {
					query := fmt.Sprintf(
						queryText,
						TestNamespace, "deployment", "function-deployment", "deployment",
					)
					resp, err := serviceGet("linkerd-viz", "prometheus", 9090, "api/v1/query", "query", query)
					Expect(err).To(BeNil())
					p95latency, err := parsePrometheusResp(resp)
					Expect(err).To(BeNil())
					By(fmt.Sprintf("... latency is %.2f", p95latency))
					return p95latency
				}
				By(fmt.Sprintf("Wait %v to get initial latency data (num workers = %d)", sleepTime, initialNumWorkers))
				time.Sleep(sleepTime)
				initialSetupLatency := getLatency()
				// start to setup the test cases by including the initial latency value as one of the intents
				By("Set intent to initial latency")
				intent := GetIntentSpec(initialSetupLatency)
				deleteIntent := createIntent(&intent)

				// Start the learning setup
				By("Starting learning process for the actuators")
				// TODO: may need to set the lock file to pause planner  "/tmp/trainings_lock"
				podToLatency := map[latencyData]float64{}
				// the utilitity functions have an assertion and/or a timeout
				for i := initialNumLoadGenerators; i <= maxNumLoadGenerators; i++ {
					By(fmt.Sprintf("Scaling load generator to %d pods.", i))
					scaleDeployment(generatorName, int32(i))
					for j := initialNumWorkers; j <= maxNumWorkers; j++ {
						By(fmt.Sprintf("Scaling workers to %d pods.", j))
						scaleDeployment(workerName, int32(j))
						By(fmt.Sprintf("Sleeping for %v", sleepTime))
						time.Sleep(sleepTime)
						podToLatency[latencyData{j, i}] = getLatency()
						if maximumEnvLatency < podToLatency[latencyData{j, i}] {
							maximumEnvLatency = podToLatency[latencyData{j, i}]
						}
						klog.V(2).Infof("LatencyData(%d,%d): %.2f", j, i, getLatency())
					}
				}
				// Start the Scale up behavior assertion
				By(
					fmt.Sprintf(
						"We test scale up action. Worker has %d pods and %d loadgen pods (latency = %f), while requiring latency %f",
						initialNumWorkers,
						maxNumLoadGenerators,
						podToLatency[latencyData{initialNumWorkers, maxNumLoadGenerators}],
						podToLatency[latencyData{maxNumWorkers, maxNumLoadGenerators}],
					),
				)
				By(fmt.Sprintf("-- testing scale up action..."))
				scaleDeployment(workerName, int32(initialNumWorkers))
				By(fmt.Sprintf("Worker has %d pods and %d loadgen pods", initialNumWorkers, maxNumLoadGenerators))
				intent = GetIntentSpec(podToLatency[latencyData{maxNumWorkers, maxNumLoadGenerators}])
				deleteIntent()
				createIntent(&intent)
				// giving an large number of possible scalled pods: initialNumWorkers*20
				// waiting for plan to increase number of pods to a max of initialNumWorkers*20
				waitUntilScaleBetween(workerName, initialNumWorkers+1, initialNumWorkers*20, timeoutTime)
				// Start the Scale down behavior assertion
				By(
					fmt.Sprintf(
						"We test scale down action. Worker has %d pods and %d loadgen pods (latency = %f), while requiring latency %f",
						maxNumWorkers,
						initialNumLoadGenerators,
						podToLatency[latencyData{maxNumWorkers, initialNumLoadGenerators}],
						podToLatency[latencyData{initialNumWorkers, initialNumLoadGenerators}],
					),
				)
				By(fmt.Sprintf("-- testing scale down action..."))
				scaleDeployment(generatorName, int32(initialNumLoadGenerators))
				By(fmt.Sprintf("Sleeping for %v", sleepTime))
				time.Sleep(sleepTime)
				scaleDeployment(workerName, int32(maxNumWorkers))
				By(fmt.Sprintf("Worker has %d pods and %d loadgen pods", maxNumWorkers, initialNumLoadGenerators))
				// this guarantee that the reset intent latency has the highest value
				if maximumEnvLatency < maximumLatency {
					maximumEnvLatency = maximumLatency
				}
				intent = GetIntentSpec(maximumEnvLatency)
				deleteIntent()
				deleteIntent = createIntent(&intent)
				defer deleteIntent()
				waitUntilScaleBetween(workerName, 1, maxNumWorkers-1, timeoutTime)

				By("Everything done, thank you for your patience. Cheers.")
			})
		})

		// end horizontal & start vertical
		Context("with cpuscale grpc plugin", func() {
			// creates all the cpuscale configmaps, services, and initiate cpuscale POD.
			BeforeEach(func() {
				By(fmt.Sprintf("Checking cpuscale grpc plugins deployment"))
				cpuscConfigMap, cpuscSvc, cpuscPod := GetPluginSpecs("cpu-scale-actuator", registry+"cpuscale:"+imageTag, defaultCPUscaleoutConfig)
				configMaps = append(configMaps, cpuscConfigMap)
				services = append(services, cpuscSvc)
				pods = append(pods, cpuscPod)
			})
			// start the verification of the steps during a model training
			It("Works with training", func() {
				// allow control from an external flag
				if !runCPUwithTraining {
					Skip("Skip to run cpu scale with training model.")
				}
				// perform and assert all the steps on the KPI queries and return values.
				queryText, err := getQuery(defaultPlannerQueries, TestNamespace+"/p95latency")
				workerName := deployments["worker"].Name
				Expect(err).To(BeNil())
				getLatency := func() float64 {
					query := fmt.Sprintf(
						queryText,
						TestNamespace, "deployment", "function-deployment", "deployment",
					)
					resp, err := serviceGet("linkerd-viz", "prometheus", 9090, "api/v1/query", "query", query)
					Expect(err).To(BeNil())
					p95latency, err := parsePrometheusResp(resp)
					Expect(err).To(BeNil())
					By(fmt.Sprintf("... latency is %.2f", p95latency))
					return p95latency
				}
				// start to setup the test cases by including the initial latency value as one of the intents
				By(fmt.Sprintf("Wait %v to get initial latency data (num workers = %d)", sleepTime, initialNumWorkers))
				time.Sleep(sleepTime)
				initialSetupLatency := getLatency()
				intent := GetIntentSpec(initialSetupLatency * 2)
				deleteIntent := createIntent(&intent)
				tmp := getLatency()
				tmpCpu := int64(0)
				minCpuInModel := getResource(workerName, "cpu")
				maxCpuInModel := int64(0)
				// TODO: need a test to ensure that a model was created - perhaps a check in the db
				By(fmt.Sprintf("-- try to learn something..."))
				// the loop set different values for the intents based on the initial latency values
				// Also, alternated between a low and a high latency values by a factor of 4 and 2, respectively.
				// That way, the model is built pro-actively at the same time the actuator behavior is evaluated.
				// For example: if the latency intent is 2000ms for 1.4 cpu and the kpi 95% is 400ms, the actuator
				// proactively will set a high value for cpu, in the first iteration in the loop. The new latency and
				// new cpu value is recorded for the model (note that the kpi is not reached). Then, the current intent
				// is replaced by a new one where the kpi 95% is changed again to a high value 2000ms,
				// but the measured latency is lower than that value. Actuator will decrease  cpu value proactively,
				// and the new pair (latency,cpu) is recorded for the model in this second iteration.
				// On the third iteration, the kpi 95% is changed back to 400ms.
				// the expected behavior of the Actuator is to increase proactively cpu value,
				//  and the new pair (latency,cpu) is recorded for the model, and this cycle is repeat. The iteration
				// number is optimized to ensure that the model is built at the end.
				// in each iteration of this loop the actuator behavior is being assessed.
				for i := 1; i < 10; i++ {
					tmp = getLatency()
					if i%2 == 0 {
						intent = GetIntentSpec(initialSetupLatency / 4) // cpu scale down
					} else {
						intent = GetIntentSpec(initialSetupLatency * 2) // cpu scale up
					}
					tmpCpu = getResource(workerName, "cpu")
					Expect(tmpCpu).NotTo(BeZero())
					if tmpCpu > maxCpuInModel {
						maxCpuInModel = tmpCpu
					}
					if tmpCpu < minCpuInModel {
						minCpuInModel = tmpCpu
					}
					deleteIntent()
					deleteIntent = createIntent(&intent) // scaling in action
					klog.V(2).Infof("latency(%d): %.2f \tcpu: %d", i, tmp, tmpCpu)
					time.Sleep(60 * time.Second) // it is optimized. if the time is decreased will get a redundant data
				}

				// ensure the we are in the reasonable range of cpus for the model
				currentCPU := getResource(workerName, "cpu")
				minDesireLatency := initialSetupLatency / 3
				maxDesireLatency := initialSetupLatency * 2

				if getLatency() < minDesireLatency {
					intent = GetIntentSpec(maxDesireLatency)
					deleteIntent()
					deleteIntent = createIntent(&intent)
					waitUntilValuesBetween(minDesireLatency, getLatency(), "latency", timeoutTime)
				}

				// using the model
				intent = GetIntentSpec(minDesireLatency)

				defer deleteIntent()
				deleteIntent()
				deleteIntent = createIntent(&intent)

				// verify if the actuator will increase cpus values
				By(fmt.Sprintf("-- testing cpuscale UP action..."))
				currentCPU = getResource(workerName, "cpu")
				if currentCPU > maxCpuInModel {
					klog.Warningf("current cpu values out of the range used in the model [%d,%d]", minCpuInModel, maxCpuInModel)
				}
				// assertion of the cpuscale up test
				// currentCPU is the min value in the assertion. So, it will check if the actuator will increase cpu above this value.
				waitUntilVerticalScaleBetween(workerName, "cpu", currentCPU, millicpuMax, 10, timeoutTime)

				// update after scale up test
				currentCPU = getResource(workerName, "cpu")

				// let's set for scaling down test
				intent = GetIntentSpec(maxDesireLatency)
				deleteIntent()
				deleteIntent = createIntent(&intent)

				// verify if the actuator will decrease cpus values
				By(fmt.Sprintf("-- testing cpuscale DOWN action..."))
				currentCPU = getResource(workerName, "cpu")
				if getLatency() > maxDesireLatency {
					time.Sleep(sleepTime)
				}
				// in case of the latency has problem and make the actuator to set a cpu
				// value lower than the lowest value from the model
				if currentCPU < minCpuInModel {
					klog.Warningf("current cpu values out of the range used in the model [%d,%d]", minCpuInModel, maxCpuInModel)
				}
				// assertion of the cpuscale up test
				waitUntilVerticalScaleBetween(workerName, "cpu", 1, currentCPU, 10, timeoutTime)

				By("Everything done, thank you for your patience. Cheers.")
			})

			It("Works without training", func() {
				// allow control from an external flag - This is the fastest test
				if !runCPUwithoutTraining {
					Skip("Skip to run cpu scale without training.")
				}
				queryText, err := getQuery(defaultPlannerQueries, TestNamespace+"/p95latency")
				workerName := deployments["worker"].Name
				Expect(err).To(BeNil())
				getLatency := func() float64 {
					query := fmt.Sprintf(
						queryText,
						TestNamespace, "deployment", "function-deployment", "deployment",
					)
					resp, err := serviceGet("linkerd-viz", "prometheus", 9090, "api/v1/query", "query", query)
					Expect(err).To(BeNil())
					p95latency, err := parsePrometheusResp(resp)
					Expect(err).To(BeNil())
					By(fmt.Sprintf("... latency is %f", p95latency))
					return p95latency
				}
				By(fmt.Sprintf("Wait %v to get initial latency data (num workers = %d)", sleepTime, initialNumWorkers))
				time.Sleep(sleepTime)

				for j := 1; j <= 2; j++ {
					initialSetupLatency := getLatency()
					By("Set intent to initial latency")
					intent := GetIntentSpec(initialSetupLatency / 2)

					By(fmt.Sprintf("Scaling workers to %d pods.", j))
					scaleDeployment(workerName, int32(j))

					By(fmt.Sprintf("Sleeping for %v", sleepTime))
					time.Sleep(sleepTime)

					currentCPU := getResource(workerName, "cpu")
					deleteIntent := createIntent(&intent)
					klog.Infof("initial latency: %.2f with %d workers pods", getLatency(), j)
					klog.Info("testing cpuscale UP action...")
					klog.Infof("current latency: %.2f --- desire latency: %.2f", getLatency(), initialSetupLatency/2)
					waitUntilVerticalScaleBetween(workerName, "cpu", currentCPU, millicpuMax, 10, timeoutTime)
					By(fmt.Sprintf("Sleeping for %v", sleepTime))
					time.Sleep(sleepTime + 10)

					// objective lower than initiall with 1000 mcpu for 1 worker pod
					intent = GetIntentSpec(initialSetupLatency * 2)
					deleteIntent()

					currentCPU = getResource(workerName, "cpu")
					deleteIntent = createIntent(&intent)

					// it may have issues in set millicpuDefault intead of current value.
					klog.Info("testing cpuscale DOWN action...")
					klog.Infof("current latency: %.2f --- desire latency: %.2f", getLatency(), initialSetupLatency*2)
					waitUntilVerticalScaleBetween(workerName, "cpu", 1, currentCPU, 10, timeoutTime)
					deleteIntent()
				}
				By("Everything done, thank you for your patience. Cheers.")
			})
		})
	})
})
