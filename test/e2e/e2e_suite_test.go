package e2e_test

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/kafkatest"
)

const (
	matchReason    = "E2EMatch" // matched by the filter in config.yml
	otherNamespace = "kube2kafka-e2e-other"

	deployNamespace  = "kube2kafka"
	deployName       = "kube2kafka"
	podSelector      = "app=kube2kafka"
	syncedLogMessage = "initial watcher cache synced"

	specTimeout    = 30 * time.Second
	rolloutTimeout = 2 * time.Minute
)

type payload map[string]string

var (
	config    *k2kconfig.Config
	clientset kubernetes.Interface
	reader    *kafka.Reader

	// The topic and namespaces are reused between runs; assertions only look at events
	// named with this run's ID.
	runID = strings.ToLower(rand.Text()[:8])
	seq   int
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "e2e Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	// Other runs (make test) have no cluster.
	if !Label("e2e").MatchesLabelFilter(GinkgoLabelFilter()) {
		return
	}

	var err error
	config, err = k2kconfig.Read("config.yml")
	Expect(err).NotTo(HaveOccurred())

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{},
	).ClientConfig()
	Expect(err).NotTo(HaveOccurred(), "load kubeconfig")
	clientset, err = kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	broker, err := kafkatest.Broker(ctx)
	Expect(err).NotTo(HaveOccurred())

	err = kafkatest.CreateTopic(ctx, []string{broker}, config.Kafka.Topic)
	if !errors.Is(err, kafka.TopicAlreadyExists) {
		Expect(err).NotTo(HaveOccurred())
	}
	ensureNamespace(ctx, config.TargetNamespace)
	ensureNamespace(ctx, otherNamespace)

	// The startup list is exported in no particular order; scenarios rely on ordering.
	waitForSyncedPod(ctx)

	reader = kafkatest.NewReader([]string{broker}, config.Kafka.Topic)
	Expect(reader.SetOffset(kafka.LastOffset)).To(Succeed())
	DeferCleanup(reader.Close)
}, NodeTimeout(rolloutTimeout))

func ensureNamespace(ctx SpecContext, name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func waitForSyncedPod(ctx SpecContext) {
	Eventually(func(g Gomega) {
		pods := listPods(ctx, g)
		// A Terminating pod may still export.
		g.Expect(pods).To(HaveLen(1), "kube2kafka pods")
		pod := pods[0]
		g.Expect(pod.DeletionTimestamp).To(BeNil())
		g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))

		logs, err := clientset.CoreV1().Pods(deployNamespace).
			GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(logs)).To(ContainSubstring(syncedLogMessage))
	}).WithContext(ctx).WithPolling(time.Second).Should(Succeed())
}

func listPods(ctx SpecContext, g Gomega) []corev1.Pod {
	pods, err := clientset.CoreV1().Pods(deployNamespace).List(ctx,
		metav1.ListOptions{LabelSelector: podSelector})
	g.Expect(err).NotTo(HaveOccurred())
	return pods.Items
}

func scaleKube2kafka(ctx SpecContext, replicas int) {
	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := clientset.AppsV1().Deployments(deployNamespace).Patch(ctx, deployName,
		types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	Expect(err).NotTo(HaveOccurred())
	if replicas == 0 {
		Eventually(func(g Gomega) {
			g.Expect(listPods(ctx, g)).To(BeEmpty(), "kube2kafka pods")
		}).WithContext(ctx).WithPolling(time.Second).Should(Succeed())
	}
}

func newID() string {
	seq++
	return fmt.Sprintf("e2e-%s-%d", runID, seq)
}

func createEvent(
	ctx SpecContext,
	namespace, reason, id string,
	lastTimestamp time.Time,
) *corev1.Event {
	ts := metav1.NewTime(lastTimestamp)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: id},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: namespace,
			Name:      id,
		},
		Reason:         reason,
		Message:        id,
		Type:           corev1.EventTypeNormal,
		Source:         corev1.EventSource{Component: "kube2kafka-e2e"},
		FirstTimestamp: ts,
		LastTimestamp:  ts,
		Count:          1,
	}
	created, err := clientset.CoreV1().Events(namespace).
		Create(ctx, event, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	return created
}

// readUntil returns the first payload with the given message and the payloads before
// it, in export order since the topic has one partition.
func readUntil(ctx SpecContext, message string) (payload, []payload) {
	var before []payload
	for {
		msg, err := reader.ReadMessage(ctx)
		Expect(err).NotTo(HaveOccurred(), "waiting for message %q on topic %q",
			message, config.Kafka.Topic)

		var p payload
		Expect(json.Unmarshal(msg.Value, &p)).To(Succeed(), "decode %s", msg.Value)
		if p["message"] == message {
			return p, before
		}
		before = append(before, p)
	}
}

func expectSkipped(ctx SpecContext, sentinel string, skipped ...string) {
	_, before := readUntil(ctx, sentinel)
	for _, p := range before {
		Expect(skipped).NotTo(ContainElement(p["message"]),
			"event %q should have been skipped but was exported", p["message"])
	}
}
