package discovery

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type K8sDiscovery struct {
	client    *kubernetes.Clientset
	namespace string
	service   string
	nodeName  string
	nodeIP    string
}

func NewK8sDiscovery() (*K8sDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("not running in kubernetes: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	service := os.Getenv("SERVICE_NAME")
	if service == "" {
		return nil, fmt.Errorf("SERVICE_NAME environment variable not set")
	}

	nodeName := os.Getenv("POD_NAME")
	if nodeName == "" {
		nodeName = getHostname()
	}

	nodeIP := os.Getenv("POD_IP")
	if nodeIP == "" {
		return nil, fmt.Errorf("POD_IP environment variable not set")
	}

	return &K8sDiscovery{
		client:    clientset,
		namespace: namespace,
		service:   service,
		nodeName:  nodeName,
		nodeIP:    nodeIP,
	}, nil
}

func (k *K8sDiscovery) GetPeers(ctx context.Context) ([]string, error) {
	endpoints, err := k.client.CoreV1().Endpoints(k.namespace).
		Get(ctx, k.service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	var peers []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef != nil {
				peer := fmt.Sprintf("%s=http://%s:2380",
					addr.TargetRef.Name, addr.IP)
				peers = append(peers, peer)
			} else {
				// Fallback if TargetRef is nil
				peer := fmt.Sprintf("node-%s=http://%s:2380",
					addr.IP, addr.IP)
				peers = append(peers, peer)
			}
		}
	}

	return peers, nil
}

func (k *K8sDiscovery) GetNodeName() string {
	return k.nodeName
}

func (k *K8sDiscovery) GetNodeIP() string {
	return k.nodeIP
}
