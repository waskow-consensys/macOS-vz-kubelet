package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agoda-com/macOS-vz-kubelet/pkg/client"
	"github.com/agoda-com/macOS-vz-kubelet/pkg/event"
	"github.com/agoda-com/macOS-vz-kubelet/pkg/provider"

	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"

	"github.com/mitchellh/go-homedir"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	taintKey    = "virtual-kubelet.io/provider"
	taintValue  = "macos-vz-e2e"
	taintEffect = corev1.TaintEffectNoSchedule
)

var (
	namespace               = flag.String("namespace", os.Getenv("POD_NAMESPACE"), "Namespace scope for requests")
	image                   = flag.String("image", "localhost:5000/macos:latest", "Image to use for the pod")
	podCreationTimeout      = flag.Duration("pod-creation-timeout", 5*time.Minute, "Timeout for waiting for pod readiness")
	podCreationPollInterval = flag.Duration("pod-creation-poll-interval", 5*time.Second, "Polling interval for pod readiness")

	clientCACert = flag.String("client-verify-ca", os.Getenv("APISERVER_CA_CERT_LOCATION"), "CA cert to use to verify client requests")
	certPath     = flag.String("cert-path", os.Getenv("APISERVER_CERT_LOCATION"), "Path to the certificate file")
	keyPath      = flag.String("key-path", os.Getenv("APISERVER_KEY_LOCATION"), "Path to the key file")
)

func TestCreatePod(t *testing.T) {
	if *namespace == "" {
		t.SkipNow()
	}

	nodeName := fmt.Sprintf("macos-vz-kubelet-e2e-test-node-%d", time.Now().Unix())

	home, err := homedir.Dir()
	require.NoError(t, err)

	kubeconfigPath := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	require.NoError(t, err)
	kcl, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	logger := logrus.StandardLogger()
	lvl, err := logrus.ParseLevel("info")
	if err != nil {
		logrus.WithError(err).Fatal("Error parsing log level")
	}
	logger.SetLevel(lvl)

	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))

	// Set the default logger
	ctx := log.WithLogger(t.Context(), log.L)
	ctx, cancel, node, kcl := setupNodeProvider(t, ctx, kcl, nodeName, 10253)
	t.Cleanup(func() {
		cancel()
		<-node.Done()
		assert.NoError(t, node.Err(), "node should shutdown without error")

		// Delete the node
		// @note: testing context is already cancelled
		err = kcl.CoreV1().Nodes().Delete(context.Background(), nodeName, metav1.DeleteOptions{GracePeriodSeconds: func() *int64 { v := int64(0); return &v }()})
		require.NoError(t, err)
	})

	// Create the pod
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("macos-vz-kubelet-e2e-test-pod-%d", time.Now().Unix()),
			Namespace: *namespace,
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &[]int64{0}[0],
			Containers: []corev1.Container{
				{
					Name:  "macos",
					Image: *image,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("12Gi"),
						},
					},
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/os": "darwin",
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      taintKey,
					Operator: corev1.TolerationOpEqual,
					Value:    taintValue,
					Effect:   taintEffect,
				},
			},
		},
	}

	// Set pod owner reference to the node
	knode, err := kcl.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	require.NoError(t, err, "failed to get node")
	pod.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       nodeName,
			UID:        knode.UID,
		},
	}

	// Create the namespace if it doesn't exist
	_, err = kcl.CoreV1().Namespaces().Get(ctx, *namespace, metav1.GetOptions{})
	if err != nil {
		_, err = kcl.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: *namespace},
		}, metav1.CreateOptions{})
		require.NoError(t, err, "failed to create namespace")
	}

	// Create the pod
	createdPod, err := kcl.CoreV1().Pods(*namespace).Create(ctx, &pod, metav1.CreateOptions{})
	require.NoError(t, err, "failed to create pod")

	// Poll pod status until it's ready or timeout
	pollCtx, pollCancel := context.WithTimeout(ctx, *podCreationTimeout)
	defer pollCancel()
	err = wait.PollUntilContextCancel(pollCtx, *podCreationPollInterval, true, func(ctx context.Context) (bool, error) {
		// Get latest pod status
		pod, err := kcl.CoreV1().Pods(*namespace).Get(ctx, createdPod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		t.Logf("Pod phase: %s", pod.Status.Phase)

		if pod.Status.Phase == corev1.PodRunning {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					t.Log("Pod is ready")
					return true, nil
				}
			}
		}

		return false, nil
	})
	require.NoError(t, err, "failed to get pod status")

	// Run the exec test
	t.Run("exec-uname", func(t *testing.T) {
		if *certPath == "" || *keyPath == "" {
			t.SkipNow()
		}

		// Prepare the exec request
		req := kcl.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(createdPod.Name).
			Namespace(*namespace).
			SubResource("exec").
			Param("container", "macos").
			Param("command", "uname").
			Param("stdout", "true").
			Param("stderr", "true")

		exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
		require.NoError(t, err, "failed to create executor")

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		require.NoError(t, err, "failed to exec uname: %s", stderr.String())

		t.Logf("uname output: %s", stdout.String())
		assert.Contains(t, stdout.String(), "Darwin", "uname output should contain Darwin")
	})
}

func setupNodeProvider(t *testing.T, ctx context.Context, kcl *kubernetes.Clientset, nodeName string, daemonEndpointPort int32) (context.Context, context.CancelFunc, *nodeutil.Node, *kubernetes.Clientset) {
	t.Helper()

	ctx, cancel := context.WithCancel(ctx)

	platform, _, _, err := host.PlatformInformationWithContext(ctx)
	require.NoError(t, err)

	node, err := nodeutil.NewNode(
		nodeName,
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			eventBroadcaster := record.NewBroadcaster()
			eventBroadcaster.StartLogging(log.G(ctx).Infof)
			eventBroadcaster.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: kcl.CoreV1().Events(corev1.NamespaceAll)})
			eventRecorder := event.NewKubeEventRecorder(
				eventBroadcaster.NewRecorder(
					scheme.Scheme,
					corev1.EventSource{
						Component: provider.ComponentName,
						Host:      nodeName,
					},
				),
			)
			cachePath := t.TempDir()
			t.Logf("cachePath: %s", cachePath)
			vzClient := client.NewVzClientAPIs(ctx, eventRecorder, "", cachePath, nil)

			providerConfig := provider.MacOSVZProviderConfig{
				NodeName:           nodeName,
				Platform:           platform,
				DaemonEndpointPort: daemonEndpointPort,

				K8sClient:     kcl,
				EventRecorder: eventRecorder,
				PodsLister:    cfg.Pods,
			}

			p, err := provider.NewMacOSVZProvider(ctx, vzClient, providerConfig)
			require.NoError(t, err)

			assert.NoError(t, p.ConfigureNode(ctx, cfg.Node))

			return p, nil, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			return nodeutil.WithClient(kcl)(cfg)
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.Handler = api.InstrumentHandler(nodeutil.WithAuth(nodeutil.NoAuth(), cfg.Handler))
			return nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			if *certPath == "" || *keyPath == "" {
				return nil
			}
			return nodeutil.WithTLSConfig(nodeutil.WithKeyPairFromPath(*certPath, *keyPath), withCA)(cfg)
		},
		func(c *nodeutil.NodeConfig) error {
			mux := http.NewServeMux()
			c.Handler = mux
			return nodeutil.AttachProviderRoutes(mux)(c)
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.HTTPListenAddr = fmt.Sprintf(":%d", daemonEndpointPort)
			return nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			taint := corev1.Taint{
				Key:    taintKey,
				Value:  taintValue,
				Effect: taintEffect,
			}
			cfg.NodeSpec.Spec.Taints = append(cfg.NodeSpec.Spec.Taints, taint)
			return nil
		},
	)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- node.Run(ctx)
		select {
		case err := <-errCh:
			assert.NoError(t, err)
		case <-ctx.Done():
		}
	}()

	startupTimeout := 5 * time.Second
	assert.NoErrorf(t, node.WaitReady(ctx, startupTimeout), "error waiting for node to be ready: %v", err)

	return ctx, cancel, node, kcl
}

func withCA(cfg *tls.Config) error {
	if *clientCACert == "" {
		return nil
	}
	if err := nodeutil.WithCAFromPath(*clientCACert)(cfg); err != nil {
		return fmt.Errorf("error getting CA from path: %w", err)
	}
	cfg.ClientAuth = tls.NoClientCert
	return nil
}
